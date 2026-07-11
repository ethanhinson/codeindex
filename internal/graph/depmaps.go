package graph

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Depmaps: versioned, symbols-only dependency maps. A map file is a normal
// graph.db holding tier-1 symbols (namespace+version metadata, per-file
// hashes) and no edges. Attach materializes it into a repo index; per-file
// hash verification lets a locally modified ("hacked") dep overlay the map.

// WriteDepMeta stamps a map file's identity.
func (s *Store) WriteDepMeta(namespace, version string) error {
	for k, v := range map[string]string{"namespace": namespace, "version": version} {
		if _, err := s.db.Exec(
			`INSERT INTO depmeta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			k, v); err != nil {
			return err
		}
	}
	return nil
}

// ReadDepMeta returns a map file's namespace and version.
func (s *Store) ReadDepMeta() (namespace, version string, err error) {
	rows, err := s.db.Query(`SELECT key, value FROM depmeta`)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return "", "", err
		}
		switch k {
		case "namespace":
			namespace = v
		case "version":
			version = v
		}
	}
	return namespace, version, rows.Err()
}

// PutDepSymbols writes a dep's parsed definitions into a MAP file (paths
// dep-dir-relative), with the file's content hash for later verification.
func (s *Store) PutDepSymbols(namespace, version, relPath, hash string, size, mtime int64, syms []Symbol) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, sym := range syms {
		if _, err := s.insertSymbol(tx, relPath, sym.Name, sym.Parent, namespace, 1,
			string(sym.Kind), sym.Signature, sym.StartLine, sym.EndLine); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO depfiles(path,namespace,version,maphash,curhash,size,mtime,modified)
		 VALUES(?,?,?,?,?,?,?,0)
		 ON CONFLICT(path) DO UPDATE SET maphash=excluded.maphash,curhash=excluded.curhash,
		   size=excluded.size,mtime=excluded.mtime,modified=0`,
		relPath, namespace, version, hash, hash, size, mtime); err != nil {
		return err
	}
	return tx.Commit()
}

// AttachMap imports a map file's symbols into this repo index under prefix
// (the dep's location in repo coordinates, e.g. vendor/github.com/x/y).
// Re-attaching the same namespace replaces prior rows. Returns the distinct
// symbol names imported so callers can re-resolve affected edges.
func (s *Store) AttachMap(mapPath, prefix string) (namespace, version string, names []string, err error) {
	// Read identity from the map first.
	m, err := Open(mapPath)
	if err != nil {
		return "", "", nil, err
	}
	namespace, version, err = m.ReadDepMeta()
	m.Close()
	if err != nil {
		return "", "", nil, err
	}
	if namespace == "" {
		return "", "", nil, fmt.Errorf("%s: not a depmap (no namespace metadata)", mapPath)
	}

	if _, err := s.db.Exec(`ATTACH DATABASE ? AS depmap`, mapPath); err != nil {
		return "", "", nil, err
	}
	defer s.db.Exec(`DETACH DATABASE depmap`)

	tx, err := s.db.Begin()
	if err != nil {
		return "", "", nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM symbols_t WHERE tier=1 AND namespace_id=(SELECT id FROM strs WHERE s=?)`,
		namespace); err != nil {
		return "", "", nil, err
	}
	if _, err := tx.Exec(`DELETE FROM depfiles WHERE namespace=?`, namespace); err != nil {
		return "", "", nil, err
	}
	join := prefix
	if join != "" && !strings.HasSuffix(join, "/") {
		join += "/"
	}
	// Nested Go modules can cover overlapping trees (a parent module's map
	// walked a child module's dir). First-attached wins per file: skip paths
	// another map already covers.
	for _, ins := range []string{
		fmt.Sprintf(`INSERT OR IGNORE INTO strs(s) SELECT DISTINCT %q || file FROM depmap.symbols WHERE tier=1`, join),
		`INSERT OR IGNORE INTO strs(s) SELECT DISTINCT name FROM depmap.symbols WHERE tier=1`,
		`INSERT OR IGNORE INTO strs(s) SELECT DISTINCT parent FROM depmap.symbols WHERE tier=1`,
		`INSERT OR IGNORE INTO strs(s) SELECT DISTINCT namespace FROM depmap.symbols WHERE tier=1`,
		`INSERT OR IGNORE INTO strs(s) SELECT DISTINCT kind FROM depmap.symbols WHERE tier=1`,
	} {
		if _, err := tx.Exec(ins); err != nil {
			return "", "", nil, err
		}
	}
	if _, err := tx.Exec(fmt.Sprintf(
		`INSERT INTO symbols_t(file_id,name_id,parent_id,namespace_id,tier,kind_id,signature,start_line,end_line)
		 SELECT fs.id, ns.id, ps.id, nss.id, 1, ks.id, m.signature, m.start_line, m.end_line
		 FROM depmap.symbols m
		 JOIN strs fs ON fs.s = %q || m.file
		 JOIN strs ns ON ns.s = m.name
		 JOIN strs ps ON ps.s = m.parent
		 JOIN strs nss ON nss.s = m.namespace
		 JOIN strs ks ON ks.s = m.kind
		 WHERE m.tier=1
		 AND NOT EXISTS (SELECT 1 FROM depfiles df WHERE df.path = %q || m.file)`,
		join, join)); err != nil {
		return "", "", nil, err
	}
	if _, err := tx.Exec(fmt.Sprintf(
		`INSERT INTO depfiles(path,namespace,version,maphash,curhash,size,mtime,modified)
		 SELECT %q || path, namespace, version, maphash, curhash, size, mtime, 0
		 FROM depmap.depfiles m
		 WHERE NOT EXISTS (SELECT 1 FROM depfiles df WHERE df.path = %q || m.path)`,
		join, join)); err != nil {
		return "", "", nil, err
	}
	rows, err := tx.Query(`SELECT DISTINCT name FROM depmap.symbols WHERE tier=1`)
	if err != nil {
		return "", "", nil, err
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return "", "", nil, err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := tx.Commit(); err != nil {
		return "", "", nil, err
	}
	return namespace, version, names, nil
}

// DepFileState is the verification record for one covered file.
type DepFileState struct {
	Path      string
	Namespace string
	Version   string
	MapHash   string
	CurHash   string
	Size      int64
	Mtime     int64
}

// DepFiles returns all covered files (for overlay verification).
func (s *Store) DepFiles() ([]DepFileState, error) {
	rows, err := s.db.Query(
		`SELECT path, namespace, version, maphash, curhash, size, mtime FROM depfiles`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DepFileState
	for rows.Next() {
		var d DepFileState
		if err := rows.Scan(&d.Path, &d.Namespace, &d.Version, &d.MapHash, &d.CurHash, &d.Size, &d.Mtime); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// OverlayDepFile replaces a covered file's tier-1 symbols with freshly parsed
// local content (the hacked-dep case) and records the new hash/state.
// Returns the affected symbol names (old + new) for re-resolution.
func (s *Store) OverlayDepFile(st DepFileState, newHash string, size, mtime int64, syms []Symbol) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	nameSet := map[string]struct{}{}
	rows, err := tx.Query(`SELECT DISTINCT name FROM symbols WHERE tier=1 AND file=?`, st.Path)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		nameSet[n] = struct{}{}
	}
	rows.Close()

	if _, err := tx.Exec(
		`DELETE FROM symbols_t WHERE tier=1 AND file_id=(SELECT id FROM strs WHERE s=?)`,
		st.Path); err != nil {
		return nil, err
	}
	for _, sym := range syms {
		if _, err := s.insertSymbol(tx, st.Path, sym.Name, sym.Parent, st.Namespace, 1,
			string(sym.Kind), sym.Signature, sym.StartLine, sym.EndLine); err != nil {
			return nil, err
		}
		nameSet[sym.Name] = struct{}{}
	}
	modified := 0
	if newHash != st.MapHash {
		modified = 1
	}
	if _, err := tx.Exec(
		`UPDATE depfiles SET curhash=?, size=?, mtime=?, modified=? WHERE path=?`,
		newHash, size, mtime, modified, st.Path); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}
	return names, nil
}

// ReResolve re-resolves edges targeting the given names in a fresh transaction
// (attach/overlay entry point mirroring the engine's affected-names pass).
func (s *Store) ReResolve(names []string) error {
	if len(names) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, n := range names {
		set[n] = struct{}{}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.ReResolveNames(tx, set); err != nil {
		return err
	}
	return tx.Commit()
}

// DepSymbolCount returns the number of attached dep-tier symbols.
func (s *Store) DepSymbolCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM symbols WHERE tier=1`).Scan(&n)
	return n, err
}

// AllSymbolsWithCallers returns every symbol plus a callers-per-symbol map —
// the search scorer's working set (single scan + one GROUP BY).
func (s *Store) AllSymbolsWithCallers() ([]Symbol, map[int64]int, error) {
	rows, err := s.db.Query(
		`SELECT id, file, name, parent, namespace, tier, kind, signature, start_line, end_line FROM symbols`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var syms []Symbol
	for rows.Next() {
		var sy Symbol
		var kind string
		if err := rows.Scan(&sy.ID, &sy.File, &sy.Name, &sy.Parent, &sy.Namespace,
			&sy.Tier, &kind, &sy.Signature, &sy.StartLine, &sy.EndLine); err != nil {
			return nil, nil, err
		}
		sy.Kind = SymbolKind(kind)
		syms = append(syms, sy)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	crows, err := s.db.Query(
		`SELECT dst_symbol_id, COUNT(*) FROM edges WHERE dst_symbol_id != 0 GROUP BY dst_symbol_id`)
	if err != nil {
		return nil, nil, err
	}
	defer crows.Close()
	callers := map[int64]int{}
	for crows.Next() {
		var id int64
		var n int
		if err := crows.Scan(&id, &n); err != nil {
			return nil, nil, err
		}
		callers[id] = n
	}
	return syms, callers, crows.Err()
}

// FileSymbolSpans returns a file's symbols ordered by start line — the
// enriched-grep attribution working set.
func (s *Store) FileSymbolSpans(file string) ([]Symbol, error) {
	rows, err := s.db.Query(
		`SELECT id, file, name, parent, namespace, tier, kind, signature, start_line, end_line
		 FROM symbols WHERE file=? ORDER BY start_line`, file)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Symbol
	for rows.Next() {
		var sy Symbol
		var kind string
		if err := rows.Scan(&sy.ID, &sy.File, &sy.Name, &sy.Parent, &sy.Namespace,
			&sy.Tier, &kind, &sy.Signature, &sy.StartLine, &sy.EndLine); err != nil {
			return nil, err
		}
		sy.Kind = SymbolKind(kind)
		out = append(out, sy)
	}
	return out, rows.Err()
}

// DepProvenance returns (namespace, version, modified) for a dep symbol's file.
func (s *Store) DepProvenance(file string) (string, string, bool, error) {
	var ns, ver string
	var mod int
	err := s.db.QueryRow(
		`SELECT namespace, version, modified FROM depfiles WHERE path=?`, file).
		Scan(&ns, &ver, &mod)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	return ns, ver, mod == 1, err
}

// HashFile is the shared content hash for depmap generation/verification.
func HashFile(path string) (hash string, size, mtime int64, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", 0, 0, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", 0, 0, err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), fi.Size(), fi.ModTime().UnixNano(), nil
}

// CleanRel normalizes a repo-relative path for depfiles keys.
func CleanRel(p string) string { return filepath.ToSlash(filepath.Clean(p)) }
