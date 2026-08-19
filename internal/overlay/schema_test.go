package overlay

import (
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// rowidRefShape matches the naming shape of a column that points at another
// database's row ids: symbol_id, file_id, src_id, dst_id, name_id, edge_id, …
// The surrogate INTEGER PRIMARY KEY columns of cross_edges and
// cross_ambiguities are named plainly `id` and so do not match — they are the
// overlay's own row ids, not a member's, and need no allowlist entry.
var rowidRefShape = regexp.MustCompile(`_id$`)

// rowidAllowlist is every `_id`-suffixed column the overlay is permitted to
// hold. Each is legal because it is NOT a member graph.db rowid:
//
//	member_id    — a manifest member id (TEXT), authored by the workspace
//	               manifest and stable across member rebuilds.
//	dep_id       — likewise a manifest id (TEXT): the depended-on member.
//	ambiguity_id — an overlay-internal surrogate referencing
//	               cross_ambiguities.id, a row id of THIS database, which the
//	               overlay rewrites wholesale whenever it rebuilds.
var rowidAllowlist = map[string]bool{
	"member_id":    true,
	"dep_id":       true,
	"ambiguity_id": true,
}

// TestNoMemberRowidColumns is the §3.2-local half of "stable-key survival".
// The overlay is derived from the members' own graph.db files, and a member
// rebuild renumbers that member's symbol/file row ids. So every symbol
// reference stored here must be the stable key (member, file, qname) — never a
// per-member-DB row id. This asserts that at the schema level: adding e.g.
// symbol_id, file_id, src_id, dst_id, name_id or edge_id to any overlay table
// turns this test red.
func TestNoMemberRowidColumns(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	rows, err := s.db.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}

	// Guard against a vacuous pass: the walk must actually see the v1 schema.
	want := []string{
		"cross_ambiguities", "cross_ambiguity_candidates", "cross_edges",
		"dep_suppressions", "member_deps", "member_namespaces", "member_stamps",
		"members",
	}
	sort.Strings(want)
	if len(tables) != len(want) {
		t.Fatalf("tables = %v, want %v", tables, want)
	}
	for i, w := range want {
		if tables[i] != w {
			t.Fatalf("tables = %v, want %v", tables, want)
		}
	}

	for _, table := range tables {
		cols, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		for cols.Next() {
			var (
				cid        int
				name, typ  string
				notNull    int
				dflt       any
				primaryKey int
			)
			if err := cols.Scan(&cid, &name, &typ, &notNull, &dflt, &primaryKey); err != nil {
				cols.Close()
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			if rowidRefShape.MatchString(name) && !rowidAllowlist[name] {
				t.Errorf("%s.%s: column names a row id; overlay references must be "+
					"the stable key (member, file, qname) so they survive a member rebuild",
					table, name)
			}
		}
		err = cols.Err()
		cols.Close()
		if err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
	}
}
