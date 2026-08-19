package overlay

import (
	"path/filepath"
	"reflect"
	"testing"

	"codeindex/internal/config"
)

// newStore opens a fresh overlay in a temp dir and closes it at test end.
func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func threeMemberWorkspace() *config.Workspace {
	return &config.Workspace{
		Version: 1,
		Members: []config.Member{
			{ID: "app", Root: "app", Namespaces: []string{"Acme\\App"}, Deps: []string{"acme/lib"}},
			{ID: "lib", Root: "../lib", Namespaces: []string{"Acme\\Lib", "Acme\\Lib\\Sub"}},
			{ID: "web", Root: "web", Namespaces: []string{"Acme\\Web"}, Deps: []string{"acme/lib", "acme/app"}},
		},
	}
}

func TestReplaceRegistryRoundTrip(t *testing.T) {
	s := newStore(t)
	ws := threeMemberWorkspace()
	if err := s.ReplaceRegistry(ws); err != nil {
		t.Fatalf("ReplaceRegistry: %v", err)
	}
	got, err := s.Registry()
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	if !reflect.DeepEqual(got, ws.Members) {
		t.Fatalf("Registry = %+v, want %+v", got, ws.Members)
	}
}

func TestRegistryEmptyIsNonNil(t *testing.T) {
	s := newStore(t)
	got, err := s.Registry()
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	if got == nil {
		t.Fatal("Registry returned nil slice, want non-nil empty")
	}
	if len(got) != 0 {
		t.Fatalf("Registry = %+v, want empty", got)
	}
}

func TestReplaceRegistryDeduplicates(t *testing.T) {
	s := newStore(t)
	ws := &config.Workspace{
		Version: 1,
		Members: []config.Member{{
			ID:         "app",
			Root:       "app",
			Namespaces: []string{"Acme\\B", "Acme\\A", "Acme\\B", "Acme\\C"},
			Deps:       []string{"acme/z", "acme/y", "acme/z"},
		}},
	}
	if err := s.ReplaceRegistry(ws); err != nil {
		t.Fatalf("ReplaceRegistry: %v", err)
	}
	got, err := s.Registry()
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Registry len = %d, want 1", len(got))
	}
	wantNS := []string{"Acme\\B", "Acme\\A", "Acme\\C"}
	if !reflect.DeepEqual(got[0].Namespaces, wantNS) {
		t.Fatalf("Namespaces = %v, want %v", got[0].Namespaces, wantNS)
	}
	wantDeps := []string{"acme/z", "acme/y"}
	if !reflect.DeepEqual(got[0].Deps, wantDeps) {
		t.Fatalf("Deps = %v, want %v", got[0].Deps, wantDeps)
	}

	// The ord values themselves must be contiguous from 0 — INSERT OR IGNORE
	// would have left a gap that the read API's ORDER BY hides.
	assertOrds(t, s, `SELECT ord FROM member_namespaces WHERE member_id='app' ORDER BY ord`, []int{0, 1, 2})
	assertOrds(t, s, `SELECT ord FROM member_deps WHERE member_id='app' ORDER BY ord`, []int{0, 1})
	assertOrds(t, s, `SELECT ord FROM members ORDER BY ord`, []int{0})
}

func assertOrds(t *testing.T, s *Store, query string, want []int) {
	t.Helper()
	rows, err := s.db.Query(query)
	if err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s -> ord %v, want %v", query, got, want)
	}
}

func TestReplaceRegistryIdempotent(t *testing.T) {
	s := newStore(t)
	ws := threeMemberWorkspace()
	if err := s.ReplaceRegistry(ws); err != nil {
		t.Fatalf("ReplaceRegistry: %v", err)
	}
	first, err := s.Registry()
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	if err := s.ReplaceRegistry(ws); err != nil {
		t.Fatalf("ReplaceRegistry (again): %v", err)
	}
	second, err := s.Registry()
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("not idempotent:\n first = %+v\nsecond = %+v", first, second)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM members`,
		`SELECT COUNT(*) FROM member_namespaces`,
		`SELECT COUNT(*) FROM member_deps`,
	} {
		if got, want := countQ(t, s, q), map[string]int{
			`SELECT COUNT(*) FROM members`:           3,
			`SELECT COUNT(*) FROM member_namespaces`: 4,
			`SELECT COUNT(*) FROM member_deps`:       3,
		}[q]; got != want {
			t.Fatalf("%s = %d, want %d", q, got, want)
		}
	}
}

func TestReplaceRegistryEmptyManifestClears(t *testing.T) {
	s := newStore(t)
	if err := s.ReplaceRegistry(threeMemberWorkspace()); err != nil {
		t.Fatalf("ReplaceRegistry: %v", err)
	}
	if err := s.ReplaceRegistry(&config.Workspace{Version: 1}); err != nil {
		t.Fatalf("ReplaceRegistry(empty): %v", err)
	}
	got, err := s.Registry()
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Registry = %+v, want empty", got)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM members`,
		`SELECT COUNT(*) FROM member_namespaces`,
		`SELECT COUNT(*) FROM member_deps`,
	} {
		if n := countQ(t, s, q); n != 0 {
			t.Fatalf("%s = %d, want 0", q, n)
		}
	}
}

func countQ(t *testing.T, s *Store, q string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

// TestReplaceRegistryPrunesDroppedMember is self-contained: the cross-edge,
// ambiguity, suppression and stamp write APIs are later tasks, so the
// dependent tables are seeded with direct SQL here.
func TestReplaceRegistryPrunesDroppedMember(t *testing.T) {
	s := newStore(t)
	if err := s.ReplaceRegistry(threeMemberWorkspace()); err != nil {
		t.Fatalf("ReplaceRegistry: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			t.Fatalf("seed %s: %v", q, err)
		}
	}
	// Stamps for every member.
	for _, id := range []string{"app", "lib", "web"} {
		exec(`INSERT INTO member_stamps (member_id, merkle_root, resolved_at) VALUES (?,?,1)`, id, "root-"+id)
	}
	// Suppressions: web as consumer, web as owner, and one untouched pair.
	exec(`INSERT INTO dep_suppressions (consumer_member, namespace, owner_member, suppressed_version) VALUES ('web','Acme\Lib','lib','1.0')`)
	exec(`INSERT INTO dep_suppressions (consumer_member, namespace, owner_member, suppressed_version) VALUES ('app','Acme\Web','web','2.0')`)
	exec(`INSERT INTO dep_suppressions (consumer_member, namespace, owner_member, suppressed_version) VALUES ('app','Acme\Lib','lib','3.0')`)
	// Cross-edges: web as src, web as dst, and one untouched app->lib edge.
	edge := `INSERT INTO cross_edges
	  (src_member, src_file, src_qname, dst_member, dst_file, dst_qname, kind, provenance, confidence, line)
	  VALUES (?,?,?,?,?,?, 'call', 'cross_repo_import', 'exact', 1)`
	exec(edge, "web", "a.php", "A::x", "lib", "b.php", "B::y")
	exec(edge, "lib", "b.php", "B::y", "web", "a.php", "A::x")
	exec(edge, "app", "c.php", "C::z", "lib", "b.php", "B::y")
	// Ambiguities: one sourced from web (goes away), one from app (stays but
	// loses its web candidate).
	exec(`INSERT INTO cross_ambiguities (id, src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count)
	  VALUES (1,'web','a.php','A::x','Thing','Acme','call',3,2)`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (1,0,'lib','b.php','B::y')`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (1,1,'app','c.php','C::z')`)
	exec(`INSERT INTO cross_ambiguities (id, src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count)
	  VALUES (2,'app','c.php','C::z','Thing','Acme','call',4,2)`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (2,0,'lib','b.php','B::y')`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (2,1,'web','a.php','A::x')`)

	// Drop "web".
	dropped := &config.Workspace{Version: 1, Members: threeMemberWorkspace().Members[:2]}
	if err := s.ReplaceRegistry(dropped); err != nil {
		t.Fatalf("ReplaceRegistry(dropped): %v", err)
	}

	if got := countQ(t, s, `SELECT COUNT(*) FROM member_stamps WHERE member_id='web'`); got != 0 {
		t.Fatalf("web stamp survived: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM member_stamps`); got != 2 {
		t.Fatalf("member_stamps = %d, want 2", got)
	}
	if got := countQ(t, s,
		`SELECT COUNT(*) FROM dep_suppressions WHERE consumer_member='web' OR owner_member='web'`); got != 0 {
		t.Fatalf("web suppressions survived: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM dep_suppressions`); got != 1 {
		t.Fatalf("dep_suppressions = %d, want 1 (app->lib)", got)
	}
	if got := countQ(t, s,
		`SELECT COUNT(*) FROM cross_edges WHERE src_member='web' OR dst_member='web'`); got != 0 {
		t.Fatalf("web cross-edges survived: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_edges`); got != 1 {
		t.Fatalf("cross_edges = %d, want 1 (app->lib)", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities`); got != 1 {
		t.Fatalf("cross_ambiguities = %d, want 1", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates WHERE ambiguity_id=1`); got != 0 {
		t.Fatalf("candidates of the deleted ambiguity survived: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates WHERE member_id='web'`); got != 0 {
		t.Fatalf("candidate rows naming web survived: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`); got != 1 {
		t.Fatalf("cross_ambiguity_candidates = %d, want 1", got)
	}
}
