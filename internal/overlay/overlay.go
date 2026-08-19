// Package overlay is the workspace overlay store: the member registry as
// built, cross-member edges keyed by stable key on both ends, and per-member
// freshness stamps. It holds no primary data — every row is re-derivable from
// the members' own graph.db files, so a version bump rebuilds the overlay
// only, never a member index.
package overlay

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// FileName is the overlay database, relative to the workspace root.
const FileName = ".codeindex/workspace.db"

// Path returns wsRoot's overlay database path.
func Path(wsRoot string) string { return filepath.Join(wsRoot, FileName) }

// Store is the SQLite-backed workspace overlay. It wraps the database and
// nothing else: cross-member rows are keyed by (member, file, qname), so
// there is no string-interning table to cache ids for.
type Store struct {
	db *sql.DB
}

// schemaVersion is bumped on any schema change, independently of
// graph.schemaVersion — a different constant over a different file. The
// overlay is a derived artifact: a mismatch triggers delete-and-rebuild.
const schemaVersion = 1 // v1: member registry, cross-edges, stamps

const schema = `
-- Member registry: the manifest as built, in manifest order.
CREATE TABLE IF NOT EXISTS members (
  id TEXT PRIMARY KEY, root TEXT NOT NULL, ord INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS member_namespaces (
  member_id TEXT NOT NULL, namespace TEXT NOT NULL, ord INTEGER NOT NULL,
  PRIMARY KEY (member_id, namespace));
CREATE INDEX IF NOT EXISTS idx_member_ns ON member_namespaces(namespace);
CREATE TABLE IF NOT EXISTS member_deps (
  member_id TEXT NOT NULL, dep_id TEXT NOT NULL, ord INTEGER NOT NULL,
  PRIMARY KEY (member_id, dep_id));

-- Cross-repo edges, keyed by stable key on BOTH ends.
CREATE TABLE IF NOT EXISTS cross_edges (
  id INTEGER PRIMARY KEY,
  src_member TEXT NOT NULL, src_file TEXT NOT NULL, src_qname TEXT NOT NULL,
  dst_member TEXT NOT NULL, dst_file TEXT NOT NULL, dst_qname TEXT NOT NULL,
  kind TEXT NOT NULL, provenance TEXT NOT NULL, confidence TEXT NOT NULL,
  line INTEGER NOT NULL DEFAULT 0,
  UNIQUE (src_member, src_file, src_qname, dst_member, dst_file, dst_qname,
          kind, line));
CREATE INDEX IF NOT EXISTS idx_cross_src ON cross_edges(src_member, src_file, src_qname);
CREATE INDEX IF NOT EXISTS idx_cross_dst ON cross_edges(dst_member, dst_file, dst_qname);
CREATE INDEX IF NOT EXISTS idx_cross_dst_member ON cross_edges(dst_member);

-- D3 rung 3: ambiguity, keyed by the unresolved reference, with its count.
CREATE TABLE IF NOT EXISTS cross_ambiguities (
  id INTEGER PRIMARY KEY,
  src_member TEXT NOT NULL, src_file TEXT NOT NULL, src_qname TEXT NOT NULL,
  ref_name TEXT NOT NULL, ref_ns TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL, line INTEGER NOT NULL DEFAULT 0,
  candidate_count INTEGER NOT NULL,
  UNIQUE (src_member, src_file, src_qname, ref_name, kind, line));
CREATE INDEX IF NOT EXISTS idx_ambig_src ON cross_ambiguities(src_member, src_file, src_qname);
CREATE TABLE IF NOT EXISTS cross_ambiguity_candidates (
  ambiguity_id INTEGER NOT NULL, rank INTEGER NOT NULL,
  member_id TEXT NOT NULL, file TEXT NOT NULL, qname TEXT NOT NULL,
  PRIMARY KEY (ambiguity_id, rank));

-- D3 member-over-dep precedence: what was suppressed, for skew reporting.
-- consumer_member = the member whose tier-1 depmap attachment was suppressed.
-- owner_member    = the workspace member that claims the namespace and won.
-- suppressed_version = the vendored copy's version as recorded in the
--                      consumer's own depfiles ('' when unknown).
CREATE TABLE IF NOT EXISTS dep_suppressions (
  consumer_member TEXT NOT NULL, namespace TEXT NOT NULL,
  owner_member TEXT NOT NULL, suppressed_version TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (consumer_member, namespace));
CREATE INDEX IF NOT EXISTS idx_supp_owner ON dep_suppressions(owner_member);

-- Per-member freshness stamps: the member's merkle root at last resolution.
CREATE TABLE IF NOT EXISTS member_stamps (
  member_id TEXT PRIMARY KEY, merkle_root TEXT NOT NULL,
  resolved_at INTEGER NOT NULL);
`

// Open opens (creating if needed) the overlay database at path. An existing
// overlay with a different schema version is deleted and recreated empty; the
// next resolution pass repopulates it from the members' own indexes. Open does
// not create parent directories.
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
		// Only warn when discarding a populated overlay (a fresh file is v0 too).
		var tables int
		_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tables)
		db.Close()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if tables > 0 {
			fmt.Fprintf(os.Stderr, "codeindex: workspace overlay schema v%d -> v%d, rebuilding\n",
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
// simulating old-version overlays.
func OpenRaw(path string) (*sql.DB, error) { return sql.Open("sqlite3", path) }

// SchemaVersion is the overlay version this binary writes and requires.
func SchemaVersion() int { return schemaVersion }

// FileSchemaVersion reads the embedded overlay version of a database file
// without opening it through the version-enforcing path.
func FileSchemaVersion(path string) (int, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var v int
	err = db.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v, err
}
