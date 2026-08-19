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
	// Seed an ambiguity so the empty-manifest branch — which cannot emit a
	// NOT IN () list and so deletes unconditionally — is exercised on rows.
	for _, q := range []string{
		`INSERT INTO cross_ambiguities (id, src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count)
		   VALUES (1,'app','c.php','C::z','Thing','Acme','call',3,1)`,
		`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (1,0,'lib','b.php','B::y')`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatalf("seed %s: %v", q, err)
		}
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
		`SELECT COUNT(*) FROM cross_ambiguities`,
		`SELECT COUNT(*) FROM cross_ambiguity_candidates`,
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
	// Ambiguities. Every seeded candidate_count equals its own seeded candidate
	// row count, so any post-prune inequality is thinning, not upstream
	// truncation.
	//   1: sourced from web           -> deleted, source side.
	//   2: sourced from app, web is a candidate -> deleted WHOLE, candidate side.
	//   3: sourced from app, no web anywhere    -> untouched.
	exec(`INSERT INTO cross_ambiguities (id, src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count)
	  VALUES (1,'web','a.php','A::x','Thing','Acme','call',3,2)`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (1,0,'lib','b.php','B::y')`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (1,1,'app','c.php','C::z')`)
	exec(`INSERT INTO cross_ambiguities (id, src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count)
	  VALUES (2,'app','c.php','C::z','Thing','Acme','call',4,2)`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (2,0,'lib','b.php','B::y')`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (2,1,'web','a.php','A::x')`)
	exec(`INSERT INTO cross_ambiguities (id, src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count)
	  VALUES (3,'app','c.php','C::z','Other','Acme','call',5,1)`)
	exec(`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (3,0,'lib','b.php','B::y')`)

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
	// Ambiguity 1 (sourced from web) and ambiguity 2 (web on the candidate side
	// only) both go WHOLE; only ambiguity 3 survives.
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities WHERE id IN (1,2)`); got != 0 {
		t.Fatalf("ambiguities incident to web survived: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities`); got != 1 {
		t.Fatalf("cross_ambiguities = %d, want 1 (ambiguity 3)", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates WHERE ambiguity_id IN (1,2)`); got != 0 {
		t.Fatalf("candidates of the deleted ambiguities survived: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates WHERE member_id='web'`); got != 0 {
		t.Fatalf("candidate rows naming web survived: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates`); got != 1 {
		t.Fatalf("cross_ambiguity_candidates = %d, want 1 (ambiguity 3's lib candidate)", got)
	}
	assertAmbiguitiesCoherent(t, s)
}

// TestReplaceRegistryDeletesAmbiguityWhoseCandidateLeaves isolates the
// candidate-side arm: the dropped member is named nowhere but in one candidate
// row of an ambiguity sourced from a member that survives. Thinning that row
// would leave candidate_count = 2 over a single surviving candidate — stored
// data contradicting itself, and indistinguishable from the legitimate
// "upstream truncated the list" case Count >= len(Candidates) permits. The
// whole parent goes instead; app re-derives the reference when app re-resolves.
func TestReplaceRegistryDeletesAmbiguityWhoseCandidateLeaves(t *testing.T) {
	s := newStore(t)
	if err := s.ReplaceRegistry(threeMemberWorkspace()); err != nil {
		t.Fatalf("ReplaceRegistry: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO cross_ambiguities (id, src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count)
		   VALUES (7,'app','c.php','C::z','Thing','Acme','call',9,2)`,
		`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (7,0,'lib','b.php','B::y')`,
		`INSERT INTO cross_ambiguity_candidates (ambiguity_id, rank, member_id, file, qname) VALUES (7,1,'web','a.php','A::x')`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatalf("seed %s: %v", q, err)
		}
	}

	dropped := &config.Workspace{Version: 1, Members: threeMemberWorkspace().Members[:2]}
	if err := s.ReplaceRegistry(dropped); err != nil {
		t.Fatalf("ReplaceRegistry(dropped): %v", err)
	}

	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities WHERE id=7`); got != 0 {
		t.Fatalf("parent ambiguity survived its dropped candidate: %d rows", got)
	}
	if got := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates WHERE ambiguity_id=7`); got != 0 {
		t.Fatalf("candidate rows of the deleted parent survived: %d rows", got)
	}
	// The read API must not see it either, under app — where it was sourced.
	got, err := s.AmbiguitiesFor("app")
	if err != nil {
		t.Fatalf("AmbiguitiesFor: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("AmbiguitiesFor(app) = %+v, want none", got)
	}
	assertAmbiguitiesCoherent(t, s)
}

// assertAmbiguitiesCoherent fails if pruning left stored data contradicting
// itself: a surviving ambiguity whose candidate rows were thinned below its
// recorded candidate_count, or a candidate row with no parent. Every ambiguity
// these tests seed records a candidate_count equal to its own candidate rows,
// so here — unlike production data, where upstream truncation may legally push
// the count higher — inequality in either direction means pruning did it.
func assertAmbiguitiesCoherent(t *testing.T, s *Store) {
	t.Helper()
	if n := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguities a
	  WHERE a.candidate_count <> (SELECT COUNT(*) FROM cross_ambiguity_candidates c
	                              WHERE c.ambiguity_id = a.id)`); n != 0 {
		t.Fatalf("%d surviving ambiguities have candidate_count contradicting their candidate rows", n)
	}
	if n := countQ(t, s, `SELECT COUNT(*) FROM cross_ambiguity_candidates c
	  WHERE NOT EXISTS (SELECT 1 FROM cross_ambiguities a WHERE a.id = c.ambiguity_id)`); n != 0 {
		t.Fatalf("%d orphaned candidate rows have no parent ambiguity", n)
	}
}
