package graph

import (
	"database/sql"
	"fmt"
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

const schema = `
CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY, path TEXT UNIQUE NOT NULL, hash TEXT NOT NULL,
  size INTEGER NOT NULL, mtime INTEGER NOT NULL, lang TEXT);
CREATE TABLE IF NOT EXISTS symbols (
  id INTEGER PRIMARY KEY, file TEXT NOT NULL, name TEXT NOT NULL, kind TEXT NOT NULL,
  signature TEXT, start_line INTEGER NOT NULL, end_line INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file);
CREATE TABLE IF NOT EXISTS edges (
  id INTEGER PRIMARY KEY, src_symbol_id INTEGER NOT NULL, dst_symbol_id INTEGER NOT NULL,
  dst_name TEXT NOT NULL, kind TEXT NOT NULL, confidence TEXT NOT NULL,
  line INTEGER NOT NULL, src_file TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_symbol_id);
CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst_symbol_id);
CREATE INDEX IF NOT EXISTS idx_edges_dstname ON edges(dst_name);
CREATE INDEX IF NOT EXISTS idx_edges_srcfile ON edges(src_file);
CREATE TABLE IF NOT EXISTS merkle (
  path TEXT PRIMARY KEY, hash TEXT NOT NULL, size INTEGER NOT NULL, mtime INTEGER NOT NULL);
`

// Open opens (creating if needed) the graph database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

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

	// Insert symbols, remembering the id assigned to each (for this file's edges).
	ids := make([]int64, len(pf.Symbols))
	for i, sym := range pf.Symbols {
		res, err := tx.Exec(
			`INSERT INTO symbols(file,name,kind,signature,start_line,end_line) VALUES(?,?,?,?,?,?)`,
			sym.File, sym.Name, string(sym.Kind), sym.Signature, sym.StartLine, sym.EndLine)
		if err != nil {
			return nil, nil, err
		}
		ids[i], _ = res.LastInsertId()
	}

	// Insert this file's outgoing call edges, resolved against the current graph.
	for _, c := range pf.Calls {
		if c.EnclosingIdx < 0 || c.EnclosingIdx >= len(ids) {
			continue // top-level calls have no owning symbol in the skeleton
		}
		dstID, conf, err := resolveName(tx, c.Callee)
		if err != nil {
			return nil, nil, err
		}
		if _, err := tx.Exec(
			`INSERT INTO edges(src_symbol_id,dst_symbol_id,dst_name,kind,confidence,line,src_file)
			 VALUES(?,?,?,?,?,?,?)`,
			ids[c.EnclosingIdx], dstID, c.Callee, string(KindCalls), string(conf), c.Line, pf.Path); err != nil {
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
	for name := range names {
		dstID, conf, err := resolveName(tx, name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE edges SET dst_symbol_id=?, confidence=? WHERE dst_name=?`,
			dstID, string(conf), name); err != nil {
			return err
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

// resolveName deterministically resolves a call target by name: 0 matches ->
// unresolved; 1 -> unambiguous; >1 -> ambiguous, canonically the first by
// (file, start_line). Deterministic ordering is what makes incremental == full.
func resolveName(q queryer, name string) (int64, Confidence, error) {
	rows, err := q.Query(
		`SELECT id FROM symbols WHERE name=? ORDER BY file, start_line, id`, name)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	switch len(ids) {
	case 0:
		return 0, ConfUnresolved, nil
	case 1:
		return ids[0], ConfUnambiguous, nil
	default:
		return ids[0], ConfAmbiguous, nil
	}
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
		`SELECT file,name,kind,signature,start_line,end_line FROM symbols`)
	if err != nil {
		return snap, err
	}
	defer srows.Close()
	for srows.Next() {
		var file, name, kind, sig string
		var sl, el int
		if err := srows.Scan(&file, &name, &kind, &sig, &sl, &el); err != nil {
			return snap, err
		}
		snap.Symbols = append(snap.Symbols,
			fmt.Sprintf("%s|%s|%s|%d|%d|%s", file, name, kind, sl, el, sig))
	}
	if err := srows.Err(); err != nil {
		return snap, err
	}

	// Edge identity uses content keys for src and dst, not row ids.
	erows, err := s.db.Query(`
		SELECT sc.file, sc.name, sc.start_line, e.dst_name, e.kind, e.confidence, e.line,
		       COALESCE(d.file,''), COALESCE(d.name,''), COALESCE(d.start_line,0)
		FROM edges e
		JOIN symbols sc ON sc.id = e.src_symbol_id
		LEFT JOIN symbols d ON d.id = e.dst_symbol_id AND e.dst_symbol_id != 0`)
	if err != nil {
		return snap, err
	}
	defer erows.Close()
	for erows.Next() {
		var sf, sn, dn, kind, conf, df, dnm string
		var ssl, line, dsl int
		if err := erows.Scan(&sf, &sn, &ssl, &dn, &kind, &conf, &line, &df, &dnm, &dsl); err != nil {
			return snap, err
		}
		snap.Edges = append(snap.Edges, fmt.Sprintf(
			"%s:%s:%d -%s-> %s [%s] dst=%s:%s:%d @%d",
			sf, sn, ssl, kind, dn, conf, df, dnm, dsl, line))
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
