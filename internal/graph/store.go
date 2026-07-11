package graph

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Store is the SQLite-backed symbol graph. Name resolution is deterministic
// (ordered by file, then line) so an incremental update yields a graph identical
// to a full rebuild — the property the walking skeleton exists to prove.
type Store struct{ db *sql.DB }

// queryer is satisfied by both *sql.DB and *sql.Tx for shared read helpers.
type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

// FileMeta is the per-file change-detection state persisted alongside the graph.
type FileMeta struct {
	Path  string
	Hash  string
	Size  int64
	Mtime int64
}

// schemaVersion is bumped on any schema change. The index is a derived
// artifact: a version mismatch triggers delete-and-rebuild, not migration.
const schemaVersion = 6 // v6: import-bound resolution persists hints in edges.dst_ns

const schema = `
CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY, path TEXT UNIQUE NOT NULL, hash TEXT NOT NULL,
  size INTEGER NOT NULL, mtime INTEGER NOT NULL, lang TEXT);
CREATE TABLE IF NOT EXISTS symbols (
  id INTEGER PRIMARY KEY, file TEXT NOT NULL, name TEXT NOT NULL,
  parent TEXT NOT NULL DEFAULT '', namespace TEXT NOT NULL DEFAULT '',
  tier INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL,
  signature TEXT, start_line INTEGER NOT NULL, end_line INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_name_parent ON symbols(name, parent);
CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file);
CREATE INDEX IF NOT EXISTS idx_symbols_ns ON symbols(namespace);
CREATE TABLE IF NOT EXISTS depfiles (
  path TEXT PRIMARY KEY, namespace TEXT NOT NULL, version TEXT NOT NULL,
  maphash TEXT NOT NULL, curhash TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0, mtime INTEGER NOT NULL DEFAULT 0,
  modified INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS depmeta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS edges (
  id INTEGER PRIMARY KEY, src_symbol_id INTEGER NOT NULL, dst_symbol_id INTEGER NOT NULL,
  dst_name TEXT NOT NULL, dst_qualifier TEXT NOT NULL DEFAULT '',
  dst_ns TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL, confidence TEXT NOT NULL,
  line INTEGER NOT NULL, src_file TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_symbol_id);
CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst_symbol_id);
CREATE INDEX IF NOT EXISTS idx_edges_dstname ON edges(dst_name);
CREATE INDEX IF NOT EXISTS idx_edges_srcfile ON edges(src_file);
CREATE TABLE IF NOT EXISTS merkle (
  path TEXT PRIMARY KEY, hash TEXT NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL);
`

// Open opens (creating if needed) the graph database at path. An existing
// index with a different schema version is deleted and recreated empty; the
// next fresh-on-query pass repopulates it (empty merkle state = full build).
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version != schemaVersion {
		// Only warn when discarding a populated index (a fresh file is v0 too).
		var tables int
		_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tables)
		db.Close()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if tables > 0 {
			fmt.Fprintf(os.Stderr, "codeindex: index schema v%d -> v%d, rebuilding\n",
				version, schemaVersion)
		}
		if db, err = sql.Open("sqlite3", path); err != nil {
			return nil, err
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// OpenRaw opens the database without schema/version handling — test hook for
// simulating old-version indexes.
func OpenRaw(path string) (*sql.DB, error) { return sql.Open("sqlite3", path) }

// Begin starts a transaction the engine drives across a build or patch.
func (s *Store) Begin() (*sql.Tx, error) { return s.db.Begin() }

// StoredMeta returns the persisted per-file change-detection state.
func (s *Store) StoredMeta() (map[string]FileMeta, error) {
	rows, err := s.db.Query(`SELECT path, hash, size, mtime FROM merkle`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FileMeta{}
	for rows.Next() {
		var m FileMeta
		if err := rows.Scan(&m.Path, &m.Hash, &m.Size, &m.Mtime); err != nil {
			return nil, err
		}
		out[m.Path] = m
	}
	return out, rows.Err()
}

// PutFile records a file's graph in tx: it deletes any prior symbols/edges for
// the file, inserts the new symbols and (resolved) call edges, and writes the
// file/merkle rows. Returns the set of symbol names the file defined before and
// after, so the engine can re-resolve inbound edges to affected names.
func (s *Store) PutFile(tx *sql.Tx, pf *ParsedFile, meta FileMeta) (before, after []string, err error) {
	before, err = namesDefinedBy(tx, pf.Path)
	if err != nil {
		return nil, nil, err
	}
	if err = deleteFileGraph(tx, pf.Path); err != nil {
		return nil, nil, err
	}

	// Project symbols carry a derived namespace: the language's enclosing
	// scope (Go dir/package, Python dotted module, TS file, PHP declared
	// namespace with dir fallback). Scope-preference resolution keys on it.
	ns := DeriveNamespace(pf.Path, pf.Namespace)

	// Insert symbols, remembering the id assigned to each (for this file's edges).
	ids := make([]int64, len(pf.Symbols))
	for i, sym := range pf.Symbols {
		symNS := sym.Namespace
		if sym.Tier == 0 && symNS == "" {
			symNS = ns
		}
		res, err := tx.Exec(
			`INSERT INTO symbols(file,name,parent,namespace,tier,kind,signature,start_line,end_line) VALUES(?,?,?,?,?,?,?,?,?)`,
			sym.File, sym.Name, sym.Parent, symNS, sym.Tier, string(sym.Kind), sym.Signature, sym.StartLine, sym.EndLine)
		if err != nil {
			return nil, nil, err
		}
		ids[i], _ = res.LastInsertId()
	}

	// Import bindings: name -> normalized namespace hint, from this file's
	// import deps. Calls and dep targets bound by an import resolve within
	// the import's mapped namespace (Stage 2); the hint is persisted on the
	// edge (dst_ns) so re-resolution reproduces insert-time results exactly.
	bind := map[string]string{}
	for _, d := range pf.Deps {
		if d.Kind == KindImports && d.Source != "" && d.Target != "" {
			bind[d.Target] = normalizeHint(d.Source, d.Target, pf.Path)
		}
	}

	// Insert this file's outgoing call edges, resolved against the current graph.
	for _, c := range pf.Calls {
		if c.EnclosingIdx < 0 || c.EnclosingIdx >= len(ids) {
			continue // top-level calls have no owning symbol in the skeleton
		}
		hint := c.NsHint // Go alias hint wins; else the file's import binding
		if hint == "" {
			hint = bind[c.Callee]
		}
		dstID, conf, err := resolve(tx, c.Callee, c.Qualifier, ns, hint)
		if err != nil {
			return nil, nil, err
		}
		if _, err := tx.Exec(
			`INSERT INTO edges(src_symbol_id,dst_symbol_id,dst_name,dst_qualifier,dst_ns,kind,confidence,line,src_file)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			ids[c.EnclosingIdx], dstID, c.Callee, c.Qualifier, hint, string(KindCalls), string(conf), c.Line, pf.Path); err != nil {
			return nil, nil, err
		}
	}

	// Dependency edges: imports (file-level, src_symbol_id=0) and
	// extends/implements (from the owning symbol). Go import paths (contain
	// '/') stay unresolved verbatim — packages are not symbols.
	for _, d := range pf.Deps {
		var srcID int64
		if d.EnclosingIdx >= 0 && d.EnclosingIdx < len(ids) {
			srcID = ids[d.EnclosingIdx]
		}
		hint := bind[d.Target] // extends/implements/import targets bind too
		var dstID int64
		conf := ConfUnresolved
		if !strings.Contains(d.Target, "/") {
			var err error
			dstID, conf, err = resolve(tx, d.Target, "", ns, hint)
			if err != nil {
				return nil, nil, err
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO edges(src_symbol_id,dst_symbol_id,dst_name,dst_qualifier,dst_ns,kind,confidence,line,src_file)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			srcID, dstID, d.Target, "", hint, string(d.Kind), string(conf), d.Line, pf.Path); err != nil {
			return nil, nil, err
		}
	}

	if _, err = tx.Exec(
		`INSERT INTO files(path,hash,size,mtime,lang) VALUES(?,?,?,?,?)
		 ON CONFLICT(path) DO UPDATE SET hash=excluded.hash,size=excluded.size,mtime=excluded.mtime`,
		meta.Path, meta.Hash, meta.Size, meta.Mtime, "go"); err != nil {
		return nil, nil, err
	}
	if _, err = tx.Exec(
		`INSERT INTO merkle(path,hash,size,mtime) VALUES(?,?,?,?)
		 ON CONFLICT(path) DO UPDATE SET hash=excluded.hash,size=excluded.size,mtime=excluded.mtime`,
		meta.Path, meta.Hash, meta.Size, meta.Mtime); err != nil {
		return nil, nil, err
	}

	after, err = namesDefinedBy(tx, pf.Path)
	return before, after, err
}

// DeleteFile removes a deleted file's graph and change-detection state, returning
// the names it had defined (so inbound edges to them can be re-resolved).
func (s *Store) DeleteFile(tx *sql.Tx, path string) ([]string, error) {
	names, err := namesDefinedBy(tx, path)
	if err != nil {
		return nil, err
	}
	if err := deleteFileGraph(tx, path); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM files WHERE path=?`, path); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM merkle WHERE path=?`, path); err != nil {
		return nil, err
	}
	return names, nil
}

// ReResolveNames recomputes the resolution of every edge whose target name is in
// names. Cost is proportional to references to those names (the blast radius),
// not to repository size — this is what keeps incremental updates cheap.
func (s *Store) ReResolveNames(tx *sql.Tx, names map[string]struct{}) error {
	if len(names) == 0 {
		return nil
	}
	// File -> namespace map, loaded once: scope-aware re-resolution groups
	// edges by (qualifier, caller namespace) in memory and batch-updates by
	// id — O(edges-of-name), never O(groups × edges).
	fileNS := map[string]string{}
	frows, err := tx.Query(`SELECT file, namespace FROM symbols WHERE tier=0 GROUP BY file`)
	if err != nil {
		return err
	}
	for frows.Next() {
		var f, n string
		if err := frows.Scan(&f, &n); err != nil {
			frows.Close()
			return err
		}
		fileNS[f] = n
	}
	frows.Close()

	for name := range names {
		erows, err := tx.Query(
			`SELECT id, dst_qualifier, dst_ns, src_file FROM edges WHERE dst_name=?`, name)
		if err != nil {
			return err
		}
		type grp struct{ q, ns, hint string }
		groups := map[grp][]int64{}
		for erows.Next() {
			var id int64
			var q, h, f string
			if err := erows.Scan(&id, &q, &h, &f); err != nil {
				erows.Close()
				return err
			}
			g := grp{q: q, ns: fileNS[f], hint: h}
			groups[g] = append(groups[g], id)
		}
		erows.Close()
		for g, ids := range groups {
			dstID, conf, err := resolve(tx, name, g.q, g.ns, g.hint)
			if err != nil {
				return err
			}
			// Batch by id in chunks to keep statements bounded.
			for i := 0; i < len(ids); i += 500 {
				end := i + 500
				if end > len(ids) {
					end = len(ids)
				}
				args := make([]any, 0, end-i+2)
				args = append(args, dstID, string(conf))
				ph := make([]string, 0, end-i)
				for _, id := range ids[i:end] {
					args = append(args, id)
					ph = append(ph, "?")
				}
				if _, err := tx.Exec(
					`UPDATE edges SET dst_symbol_id=?, confidence=? WHERE id IN (`+
						strings.Join(ph, ",")+`)`, args...); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// AllDstNames returns the set of distinct call-target names in the graph, used
// to re-resolve every edge after a full build (where files are inserted in an
// arbitrary order and early edges may have resolved before all defs existed).
func (s *Store) AllDstNames() (map[string]struct{}, error) {
	rows, err := s.db.Query(`SELECT DISTINCT dst_name FROM edges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = struct{}{}
	}
	return out, rows.Err()
}

// Definitions returns the symbols defined with the given name; a non-empty
// parent filters to that owner type (qualified anchor).
func (s *Store) Definitions(name, parent string) ([]Symbol, error) {
	q := `SELECT file, name, parent, kind, signature, start_line, end_line
	      FROM symbols WHERE name=? ORDER BY file, start_line`
	args := []any{name}
	if parent != "" {
		q = `SELECT file, name, parent, kind, signature, start_line, end_line
		     FROM symbols WHERE name=? AND parent=? ORDER BY file, start_line`
		args = append(args, parent)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Symbol
	for rows.Next() {
		var sy Symbol
		var kind string
		if err := rows.Scan(&sy.File, &sy.Name, &sy.Parent, &kind, &sy.Signature, &sy.StartLine, &sy.EndLine); err != nil {
			return nil, err
		}
		sy.Kind = SymbolKind(kind)
		out = append(out, sy)
	}
	return out, rows.Err()
}

// Caller is a symbol that calls a queried name, with the call-site line.
type Caller struct {
	File      string
	Name      string
	Parent    string
	Signature string
	Line      int
	Conf      Confidence
}

// QName is the caller's qualified display name.
func (c Caller) QName() string {
	if c.Parent != "" {
		return c.Parent + "." + c.Name
	}
	return c.Name
}

// Callers returns the symbols that call the given name. A non-empty parent
// restricts to edges that RESOLVED to a definition owned by that parent
// (unresolved name-only edges cannot match a qualified anchor).
func (s *Store) Callers(name, parent string) ([]Caller, error) {
	q := `SELECT sc.file, sc.name, sc.parent, sc.signature, e.line, e.confidence
	      FROM edges e JOIN symbols sc ON sc.id = e.src_symbol_id
	      WHERE e.dst_name = ? ORDER BY sc.file, e.line`
	args := []any{name}
	if parent != "" {
		q = `SELECT sc.file, sc.name, sc.parent, sc.signature, e.line, e.confidence
		     FROM edges e
		     JOIN symbols sc ON sc.id = e.src_symbol_id
		     JOIN symbols d ON d.id = e.dst_symbol_id
		     WHERE e.dst_name = ? AND d.name = ? AND d.parent = ?
		     ORDER BY sc.file, e.line`
		args = []any{name, name, parent}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Caller
	for rows.Next() {
		var c Caller
		var conf string
		if err := rows.Scan(&c.File, &c.Name, &c.Parent, &c.Signature, &c.Line, &conf); err != nil {
			return nil, err
		}
		c.Conf = Confidence(conf)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Callee is a symbol called by a queried symbol, with the call-site line and the
// callee's definition location when resolved.
type Callee struct {
	Name      string
	DefParent string // resolved definition's parent ("" when unresolved/top-level)
	CallLine  int
	Conf      Confidence
	DefFile   string // "" when unresolved
	DefLine   int
}

// QName is the callee's qualified display name (when resolved to a method).
func (c Callee) QName() string {
	if c.DefParent != "" {
		return c.DefParent + "." + c.Name
	}
	return c.Name
}

// Callees returns what the symbol(s) named `name` call (outgoing `calls` edges),
// each with the callee's definition location when the edge resolved. A
// non-empty parent restricts to source symbols owned by that parent.
func (s *Store) Callees(name, parent string) ([]Callee, error) {
	q := `SELECT e.dst_name, COALESCE(d.parent,''), e.line, e.confidence,
	             COALESCE(d.file,''), COALESCE(d.start_line,0)
	      FROM edges e
	      JOIN symbols sc ON sc.id = e.src_symbol_id
	      LEFT JOIN symbols d ON d.id = e.dst_symbol_id AND e.dst_symbol_id != 0
	      WHERE sc.name = ? AND e.kind = ? ORDER BY e.line`
	args := []any{name, string(KindCalls)}
	if parent != "" {
		q = `SELECT e.dst_name, COALESCE(d.parent,''), e.line, e.confidence,
		            COALESCE(d.file,''), COALESCE(d.start_line,0)
		     FROM edges e
		     JOIN symbols sc ON sc.id = e.src_symbol_id
		     LEFT JOIN symbols d ON d.id = e.dst_symbol_id AND e.dst_symbol_id != 0
		     WHERE sc.name = ? AND sc.parent = ? AND e.kind = ? ORDER BY e.line`
		args = []any{name, parent, string(KindCalls)}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Callee
	for rows.Next() {
		var c Callee
		var conf string
		if err := rows.Scan(&c.Name, &c.DefParent, &c.CallLine, &conf, &c.DefFile, &c.DefLine); err != nil {
			return nil, err
		}
		c.Conf = Confidence(conf)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Enclosing is a symbol whose span overlaps a queried line range, with caller
// counts (total, and callers whose source lies outside the symbol's own file).
type Enclosing struct {
	Symbol
	Callers         int
	ExternalCallers int
}

// EnclosingSymbols returns the symbols in file whose [start_line,end_line] span
// overlaps [start,end], each with total and external caller counts (name-based:
// callers are edges targeting the symbol's name).
func (s *Store) EnclosingSymbols(file string, start, end int) ([]Enclosing, error) {
	rows, err := s.db.Query(
		`SELECT name, kind, signature, start_line, end_line FROM symbols
		 WHERE file=? AND start_line<=? AND end_line>=? ORDER BY start_line`,
		file, end, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Enclosing
	for rows.Next() {
		var e Enclosing
		var kind string
		if err := rows.Scan(&e.Name, &kind, &e.Signature, &e.StartLine, &e.EndLine); err != nil {
			return nil, err
		}
		e.Kind = SymbolKind(kind)
		e.File = file
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		row := s.db.QueryRow(
			`SELECT COUNT(*), COALESCE(SUM(CASE WHEN e.src_file != ? THEN 1 ELSE 0 END),0)
			 FROM edges e WHERE e.dst_name = ? AND e.kind = ?`,
			file, out[i].Name, string(KindCalls))
		if err := row.Scan(&out[i].Callers, &out[i].ExternalCallers); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Dependent is a source that imports/extends/implements a queried target:
// either a symbol (Name/Parent set) or a file (Name empty, File set).
type Dependent struct {
	File   string
	Name   string // "" for file-level (import) sources
	Parent string
	Kind   EdgeKind
	Line   int
}

// QName is the dependent's display name: qualified symbol or the file itself.
func (d Dependent) QName() string {
	if d.Name == "" {
		return d.File
	}
	if d.Parent != "" {
		return d.Parent + "." + d.Name
	}
	return d.Name
}

// Dependents returns who imports/extends/implements the anchor. Matching:
// exact dst_name; or, for Go package paths, last-path-segment (so both
// `dependents graph` and `dependents codeindex/internal/graph` work).
func (s *Store) Dependents(name string) ([]Dependent, error) {
	rows, err := s.db.Query(`
		SELECT e.src_file, COALESCE(sc.name,''), COALESCE(sc.parent,''), e.kind, e.line
		FROM edges e
		LEFT JOIN symbols sc ON sc.id = e.src_symbol_id AND e.src_symbol_id != 0
		WHERE e.kind IN ('imports','extends','implements')
		  AND (e.dst_name = ? OR e.dst_name LIKE '%/' || ?)
		ORDER BY e.kind, e.src_file, e.line`, name, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dependent
	for rows.Next() {
		var d Dependent
		var kind string
		if err := rows.Scan(&d.File, &d.Name, &d.Parent, &kind, &d.Line); err != nil {
			return nil, err
		}
		d.Kind = EdgeKind(kind)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Dep is an outgoing dependency of a file or symbol.
type Dep struct {
	Kind    EdgeKind
	Target  string
	DefFile string // resolved definition ("" when unresolved/external)
	DefLine int
	Line    int
}

// FileImports returns a file's import edges.
func (s *Store) FileImports(path string) ([]Dep, error) {
	return s.depQuery(`
		SELECT e.kind, e.dst_name, COALESCE(d.file,''), COALESCE(d.start_line,0), e.line
		FROM edges e
		LEFT JOIN symbols d ON d.id = e.dst_symbol_id AND e.dst_symbol_id != 0
		WHERE e.src_file = ? AND e.kind = 'imports' ORDER BY e.line`, path)
}

// SymbolDeps returns a symbol's outgoing extends/implements edges.
func (s *Store) SymbolDeps(name, parent string) ([]Dep, error) {
	q := `SELECT e.kind, e.dst_name, COALESCE(d.file,''), COALESCE(d.start_line,0), e.line
	      FROM edges e
	      JOIN symbols sc ON sc.id = e.src_symbol_id
	      LEFT JOIN symbols d ON d.id = e.dst_symbol_id AND e.dst_symbol_id != 0
	      WHERE sc.name = ? AND e.kind IN ('extends','implements') ORDER BY e.line`
	args := []any{name}
	if parent != "" {
		q = `SELECT e.kind, e.dst_name, COALESCE(d.file,''), COALESCE(d.start_line,0), e.line
		     FROM edges e
		     JOIN symbols sc ON sc.id = e.src_symbol_id
		     LEFT JOIN symbols d ON d.id = e.dst_symbol_id AND e.dst_symbol_id != 0
		     WHERE sc.name = ? AND sc.parent = ? AND e.kind IN ('extends','implements')
		     ORDER BY e.line`
		args = append(args, parent)
	}
	return s.depQuery(q, args...)
}

func (s *Store) depQuery(q string, args ...any) ([]Dep, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dep
	for rows.Next() {
		var d Dep
		var kind string
		if err := rows.Scan(&kind, &d.Target, &d.DefFile, &d.DefLine, &d.Line); err != nil {
			return nil, err
		}
		d.Kind = EdgeKind(kind)
		out = append(out, d)
	}
	return out, rows.Err()
}

// MedianCalledSymbol returns the symbol name with the median caller count
// among names having at least minCallers call edges — a representative,
// non-pathological query anchor for benchmarking.
func (s *Store) MedianCalledSymbol(minCallers int) (string, error) {
	rows, err := s.db.Query(`
		SELECT dst_name, COUNT(*) AS n FROM edges
		WHERE kind='calls' AND dst_name NOT LIKE '%/%'
		GROUP BY dst_name HAVING n >= ? ORDER BY n`, minCallers)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return "", err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", nil
	}
	return names[len(names)/2], nil
}

// HasFile reports whether path is an indexed file (deps' file-mode detection).
func (s *Store) HasFile(path string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE path=?`, path).Scan(&n)
	return n > 0, err
}

// ReferencingFiles returns the distinct files containing edges (any kind)
// that target the given name — the "which files reference X" answer.
func (s *Store) ReferencingFiles(name string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT src_file FROM edges WHERE dst_name=? ORDER BY src_file`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// RefreshMerkle updates a file's change-detection state without touching its
// graph (content unchanged, only mtime moved).
func (s *Store) RefreshMerkle(tx *sql.Tx, m FileMeta) error {
	if _, err := tx.Exec(
		`UPDATE merkle SET mtime=? WHERE path=?`, m.Mtime, m.Path); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE files SET mtime=? WHERE path=?`, m.Mtime, m.Path)
	return err
}

func deleteFileGraph(tx *sql.Tx, path string) error {
	if _, err := tx.Exec(`DELETE FROM edges WHERE src_file=?`, path); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM symbols WHERE file=?`, path)
	return err
}

func namesDefinedBy(q queryer, path string) ([]string, error) {
	rows, err := q.Query(`SELECT DISTINCT name FROM symbols WHERE file=?`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// resolve deterministically resolves a call target. Qualified-first: when a
// lexical qualifier is present and symbols named `name` with parent=qualifier
// exist, resolution happens within that set (1 -> unambiguous, >1 ->
// deterministic first + ambiguous). Otherwise it falls back to plain
// name-based behavior — a wrong hint can never make things worse.
func resolve(q queryer, name, qualifier, srcNS, nsHint string) (int64, Confidence, error) {
	// Tier ladder: qualified project > qualified dep > import-bound (project
	// then dep) > same-scope > plain project > plain dep. Project symbols
	// always beat attached-map symbols, so maps can never degrade project
	// resolution; hints and bindings fall through totally — they can narrow
	// but never lose a match the plain ladder would have found.
	type step func() ([]int64, error)
	sqlStep := func(query string, args ...any) step {
		return func() ([]int64, error) { return symbolIDs(q, query, args...) }
	}
	var steps []step
	if qualifier != "" {
		steps = append(steps,
			sqlStep(`SELECT id FROM symbols WHERE name=? AND parent=? AND tier=0 ORDER BY file, start_line, id`, name, qualifier),
			sqlStep(`SELECT id FROM symbols WHERE name=? AND parent=? AND tier=1 ORDER BY namespace, file, start_line, id`, name, qualifier))
	}
	if nsHint != "" {
		// Import binding: the calling file imports this name (or, for Go,
		// calls through an import alias) — constrain to the mapped namespace.
		steps = append(steps,
			func() ([]int64, error) { return boundIDs(q, name, 0, nsHint) },
			func() ([]int64, error) { return boundIDs(q, name, 1, nsHint) })
	}
	if srcNS != "" {
		// Same-scope preference: the language's local scope (file/module for
		// Python and TS, package/namespace for Go and PHP) beats global names.
		steps = append(steps,
			sqlStep(`SELECT id FROM symbols WHERE name=? AND namespace=? AND tier=0 ORDER BY file, start_line, id`, name, srcNS))
	}
	steps = append(steps,
		sqlStep(`SELECT id FROM symbols WHERE name=? AND tier=0 ORDER BY file, start_line, id`, name),
		sqlStep(`SELECT id FROM symbols WHERE name=? AND tier=1 ORDER BY namespace, file, start_line, id`, name))
	for _, st := range steps {
		ids, err := st()
		if err != nil {
			return 0, "", err
		}
		if len(ids) == 1 {
			return ids[0], ConfUnambiguous, nil
		}
		if len(ids) > 1 {
			return ids[0], ConfAmbiguous, nil
		}
	}
	return 0, ConfUnresolved, nil
}

// boundIDs returns the ids of symbols named `name` in the given tier whose
// namespace matches the import hint, in deterministic order.
func boundIDs(q queryer, name string, tier int, hint string) ([]int64, error) {
	rows, err := q.Query(
		`SELECT id, namespace FROM symbols WHERE name=? AND tier=? ORDER BY namespace, file, start_line, id`,
		name, tier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		var ns string
		if err := rows.Scan(&id, &ns); err != nil {
			return nil, err
		}
		if nsMatch(ns, hint) {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// nsMatch reports whether a candidate namespace corresponds to an import
// hint. Lexical, language-shaped suffix alignment — no toolchain resolution:
// Go import paths align with directory namespaces by path suffix (either
// direction, covering vendored/staging layouts); Python dotted modules align
// by dot suffix; PHP use-paths (pre-stripped to their namespace part) align
// by backslash suffix; TS extensionless specifier paths match file-path
// namespaces via known extensions and index files.
func nsMatch(candNS, hint string) bool {
	if candNS == "" || hint == "" {
		return false
	}
	if candNS == hint {
		return true
	}
	for _, sep := range []string{"/", ".", `\`} {
		if strings.HasSuffix(hint, sep+candNS) || strings.HasSuffix(candNS, sep+hint) {
			return true
		}
	}
	if rest, ok := strings.CutPrefix(candNS, hint); ok {
		switch rest {
		case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
			"/index.ts", "/index.tsx", "/index.js", "/index.jsx":
			return true
		}
	}
	return false
}

// normalizeHint turns a raw import source into the namespace hint persisted
// on edges (dst_ns) and matched by nsMatch: relative TS specifiers resolve
// against the importing file's directory; PHP use-paths drop the bound final
// segment; everything else (Go paths, Python modules, bare specifiers) stays
// verbatim.
func normalizeHint(source, target, fromFile string) string {
	if source == "" {
		return ""
	}
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		return path.Join(path.Dir(fromFile), source)
	}
	if i := strings.LastIndexByte(source, '\\'); i >= 0 && source[i+1:] == target {
		return source[:i]
	}
	return source
}

func symbolIDs(q queryer, query string, args ...any) ([]int64, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Snapshot is a content-addressed, id-independent view of the graph used to
// prove incremental == full rebuild (symbol/edge identities are content keys,
// never autoincrement ids, which differ between the two build paths).
type Snapshot struct {
	Symbols []string
	Edges   []string
}

// DumpNormalized returns the graph as sorted content strings.
func (s *Store) DumpNormalized() (Snapshot, error) {
	var snap Snapshot

	srows, err := s.db.Query(
		`SELECT file,name,parent,kind,signature,start_line,end_line FROM symbols WHERE tier=0`)
	if err != nil {
		return snap, err
	}
	defer srows.Close()
	for srows.Next() {
		var file, name, parent, kind, sig string
		var sl, el int
		if err := srows.Scan(&file, &name, &parent, &kind, &sig, &sl, &el); err != nil {
			return snap, err
		}
		snap.Symbols = append(snap.Symbols,
			fmt.Sprintf("%s|%s|%s|%s|%d|%d|%s", file, parent, name, kind, sl, el, sig))
	}
	if err := srows.Err(); err != nil {
		return snap, err
	}

	// Edge identity uses content keys for src and dst, not row ids.
	erows, err := s.db.Query(`
		SELECT e.src_file, COALESCE(sc.name,'<file>'), COALESCE(sc.start_line,0),
		       e.dst_name, e.dst_qualifier,
		       e.kind, e.confidence, e.line,
		       COALESCE(d.file,''), COALESCE(d.name,''), COALESCE(d.parent,''), COALESCE(d.start_line,0)
		FROM edges e
		LEFT JOIN symbols sc ON sc.id = e.src_symbol_id AND e.src_symbol_id != 0
		LEFT JOIN symbols d ON d.id = e.dst_symbol_id AND e.dst_symbol_id != 0`)
	if err != nil {
		return snap, err
	}
	defer erows.Close()
	for erows.Next() {
		var sf, sn, dn, dq, kind, conf, df, dnm, dpar string
		var ssl, line, dsl int
		if err := erows.Scan(&sf, &sn, &ssl, &dn, &dq, &kind, &conf, &line, &df, &dnm, &dpar, &dsl); err != nil {
			return snap, err
		}
		snap.Edges = append(snap.Edges, fmt.Sprintf(
			"%s:%s:%d -%s-> %s~%s [%s] dst=%s:%s.%s:%d @%d",
			sf, sn, ssl, kind, dn, dq, conf, df, dpar, dnm, dsl, line))
	}
	if err := erows.Err(); err != nil {
		return snap, err
	}

	sort.Strings(snap.Symbols)
	sort.Strings(snap.Edges)
	return snap, nil
}

// Diff returns a human-readable description of the first differences between two
// snapshots, or "" if they are equal.
func (a Snapshot) Diff(b Snapshot) string {
	var sb strings.Builder
	diffLines("symbols", a.Symbols, b.Symbols, &sb)
	diffLines("edges", a.Edges, b.Edges, &sb)
	return sb.String()
}

func diffLines(label string, a, b []string, sb *strings.Builder) {
	am, bm := toSet(a), toSet(b)
	var onlyA, onlyB []string
	for x := range am {
		if _, ok := bm[x]; !ok {
			onlyA = append(onlyA, x)
		}
	}
	for x := range bm {
		if _, ok := am[x]; !ok {
			onlyB = append(onlyB, x)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	report := func(dir string, xs []string) {
		for i, x := range xs {
			if i >= 5 {
				fmt.Fprintf(sb, "  %s %s: (+%d more)\n", label, dir, len(xs)-5)
				break
			}
			fmt.Fprintf(sb, "  %s %s: %s\n", label, dir, x)
		}
	}
	report("only-in-A", onlyA)
	report("only-in-B", onlyB)
}

func toSet(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}
