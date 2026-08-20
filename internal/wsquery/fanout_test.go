package wsquery

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/query"
	"codeindex/internal/wsfresh"
)

// fanoutSession opens a session over ws the way an entry point would, with the
// freshen stubbed clean.
func fanoutSession(t *testing.T, ws string) *session {
	t.Helper()
	w, err := config.LoadWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	return sessionFrom(ws, w, wsfresh.Report{Resolved: true}, nil)
}

// --- find/grep: concatenation, manifest order, and the summed aggregates -----

// TestFindFansOutInManifestOrderAndSumsTotal pins BOTH halves of §3.2's find
// contract against the fixture: the concatenation is the per-member answers
// end to end in MANIFEST order (web, api, lib — deliberately not alphabetical),
// and Total is the SUM of the per-member totals rather than the length of the
// concatenated list.
func TestFindFansOutInManifestOrderAndSumsTotal(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	const q = "Helper"
	wantTotal := 0
	var want []string
	for _, id := range []string{wsMemberWeb, wsMemberAPI, wsMemberLib} {
		inner, err := query.Find(filepath.Join(ws, "services", id), q, "", "", 50)
		if err != nil {
			t.Fatal(err)
		}
		wantTotal += inner.Total
		for _, r := range inner.Results {
			want = append(want, fmt.Sprintf("%s|services/%s/%s:%d|%s", id, id, r.File, r.Line, r.QName))
		}
	}
	if wantTotal == 0 {
		t.Fatalf("fixture produced no find results for %q — the assertions below would be vacuous", q)
	}

	a, err := findWorkspace(fanoutSession(t, ws), q, "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range a.Results {
		got = append(got, fmt.Sprintf("%s|%s:%d|%s", r.Repo, r.File, r.Line, r.QName))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("find concatenation:\n got %v\nwant %v", got, want)
	}
	if a.Total != wantTotal {
		t.Errorf("Total = %d, want the sum of the per-member totals %d", a.Total, wantTotal)
	}
	if a.Total != len(want) {
		t.Errorf("sanity: Total %d vs concatenated rows %d", a.Total, len(want))
	}
}

// TestFindLimitBoundsTheConcatenatedList pins that `limit` bounds the
// CONCATENATED list while Total keeps counting the per-member sum — the
// arithmetic FindAnswer.Text() prints.
func TestFindLimitBoundsTheConcatenatedList(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)
	s := fanoutSession(t, ws)

	full, err := findWorkspace(s, "Helper", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Results) < 2 {
		t.Fatalf("fixture has %d find rows, need >= 2 to test truncation", len(full.Results))
	}
	one, err := findWorkspace(fanoutSession(t, ws), "Helper", "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Results) != 1 {
		t.Errorf("limit 1 returned %d rows, want 1", len(one.Results))
	}
	if one.Results[0].Repo != full.Results[0].Repo {
		t.Errorf("truncation kept %q, want the first manifest member %q",
			one.Results[0].Repo, full.Results[0].Repo)
	}

	// Total must still be the SUM of the per-member totals, not the length of
	// the truncated list — that difference is exactly what FindAnswer.Text()
	// prints, and with no truncation the two are indistinguishable.
	wantTotal := 0
	for _, id := range []string{wsMemberWeb, wsMemberAPI, wsMemberLib} {
		inner, err := query.Find(filepath.Join(ws, "services", id), "Helper", "", "", 1)
		if err != nil {
			t.Fatal(err)
		}
		wantTotal += inner.Total
	}
	if wantTotal < 2 {
		t.Fatalf("fixture sum is %d; need >= 2 for the sum to differ from the truncated length", wantTotal)
	}
	if one.Total != wantTotal {
		t.Errorf("truncated Total = %d, want the per-member sum %d", one.Total, wantTotal)
	}
}

// TestGrepFansOutAndSumsRawHits pins grep's concatenation and its summed
// RawHits.
func TestGrepFansOutAndSumsRawHits(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	const pattern = "Helper"
	wantRaw := 0
	var want []string
	for _, id := range []string{wsMemberWeb, wsMemberAPI, wsMemberLib} {
		inner, err := query.Grep(filepath.Join(ws, "services", id), pattern, 50, false)
		if err != nil {
			t.Fatal(err)
		}
		wantRaw += inner.RawHits
		for _, g := range inner.Groups {
			want = append(want, fmt.Sprintf("%s|services/%s/%s:%d|%d", id, id, g.File, g.Line, g.Hits))
		}
	}
	if wantRaw == 0 {
		t.Fatalf("fixture produced no grep hits for %q — the assertions below would be vacuous", pattern)
	}

	a, err := grepWorkspace(fanoutSession(t, ws), pattern, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range a.Groups {
		got = append(got, fmt.Sprintf("%s|%s:%d|%d", g.Repo, g.File, g.Line, g.Hits))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("grep concatenation:\n got %v\nwant %v", got, want)
	}
	if a.RawHits != wantRaw {
		t.Errorf("RawHits = %d, want the sum of the per-member raw hits %d", a.RawHits, wantRaw)
	}
}

// --- Backend: agreement, and the disagreement join ---------------------------

// TestGrepBackendAgreementIsUsedVerbatim runs the REAL per-member grep, where
// every member necessarily reports the same backend (it depends on PATH, not
// on the member), and pins that the agreed value survives verbatim and is
// never blank.
//
// It is deliberately not byte-pinned to "ripgrep" or "internal": which one it
// is depends on whether ripgrep is on PATH, and Task 1's assertion is that the
// field is never blank rather than that it is a particular word.
func TestGrepBackendAgreementIsUsedVerbatim(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	inner, err := query.Grep(filepath.Join(ws, "services", wsMemberWeb), "Helper", 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if inner.Backend == "" {
		t.Fatal("per-repo grep reported a blank backend; the workspace assertion would be vacuous")
	}
	a, err := grepWorkspace(fanoutSession(t, ws), "Helper", 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Backend != inner.Backend {
		t.Errorf("Backend = %q, want the agreed per-member backend %q verbatim", a.Backend, inner.Backend)
	}
	if strings.Contains(a.Backend, "+") {
		t.Errorf("Backend = %q decorated a unanimous backend", a.Backend)
	}
}

// TestGrepBackendDisagreementJoinsDistinctInManifestOrder is the case a
// same-backend fixture CANNOT catch: with every member reporting the same
// string, taking the first member's value looks identical to reconciling them.
//
// Which backend search.Grep picks is a property of the machine's PATH, not of
// the member, so two real backends are not reachable from one fixture. The
// disagreement is therefore driven through the grepMember seam — the same
// production code path, with only the per-member grep replaced — so the
// assertion still exercises grepWorkspace's real collection and join rather
// than joinBackends in isolation.
func TestGrepBackendDisagreementJoinsDistinctInManifestOrder(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	// Manifest order is web, api, lib. web and lib agree; api differs. A
	// first-member fallback yields "ripgrep", an alphabetical join yields
	// "internal+ripgrep", and a set that forgot to dedupe yields
	// "ripgrep+internal+ripgrep".
	backends := map[string]string{
		wsMemberWeb: "ripgrep",
		wsMemberAPI: "internal",
		wsMemberLib: "ripgrep",
	}
	prev := grepMember
	grepMember = func(root, pattern string, limit int, word bool) (*query.GrepAnswer, error) {
		id := filepath.Base(root)
		b, ok := backends[id]
		if !ok {
			t.Errorf("per-member grep called with unexpected root %q", root)
		}
		return &query.GrepAnswer{Pattern: pattern, RawHits: 1, Backend: b, Groups: []query.GrepRef{
			{File: "x.go", Line: 1, Hits: 1},
		}}, nil
	}
	defer func() { grepMember = prev }()

	a, err := grepWorkspace(fanoutSession(t, ws), "Helper", 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Backend != "ripgrep+internal" {
		t.Errorf("Backend = %q, want %q — the distinct backends joined in manifest order",
			a.Backend, "ripgrep+internal")
	}
	if a.RawHits != 3 {
		t.Errorf("RawHits = %d, want 3 (one per member)", a.RawHits)
	}
}

// TestGrepMemberSeamDefaultsToQueryGrep keeps the seam above a TEST seam:
// production must run the real per-repo grep.
func TestGrepMemberSeamDefaultsToQueryGrep(t *testing.T) {
	if reflect.ValueOf(grepMember).Pointer() != reflect.ValueOf(query.Grep).Pointer() {
		t.Fatal("grepMember no longer defaults to query.Grep")
	}
}

func TestJoinBackends(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"ripgrep", "ripgrep", "ripgrep"}, "ripgrep"},
		{[]string{"ripgrep", "internal", "ripgrep"}, "ripgrep+internal"},
		{[]string{"internal", "ripgrep"}, "internal+ripgrep"},
		{[]string{"", "ripgrep"}, "ripgrep"},
	}
	for _, c := range cases {
		if got := joinBackends(c.in); got != c.want {
			t.Errorf("joinBackends(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- consultation ------------------------------------------------------------

// TestFanOutConsultsEveryMemberIncludingTheEmptyOnes pins §4.4 for a fan-out
// verb: the clause answers "what did you look at", so a member that matched
// nothing is still listed.
func TestFanOutConsultsEveryMemberIncludingTheEmptyOnes(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)
	s := fanoutSession(t, ws)

	a, err := findWorkspace(s, "NoSuchSymbolAnywhere", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Results) != 0 {
		t.Fatalf("expected no matches, got %d", len(a.Results))
	}
	want := []string{wsMemberWeb, wsMemberAPI, wsMemberLib}
	if got := s.clause("find").MembersConsulted; !reflect.DeepEqual(got, want) {
		t.Errorf("members_consulted = %v, want every fanned-out member %v", got, want)
	}
}

// --- enclosing routing (§2.3) -------------------------------------------------

// enclosingFixture writes a workspace whose members sit on BOTH sides of the
// workspace root: `in` lives under it, `out` climbs out via "../out". The
// climbing member is the point of the fixture — D1 sanctions that layout, and
// it is the case a prefix match on the DECLARED RELATIVE root cannot handle.
func enclosingFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	ws := filepath.Join(base, "ws")
	if err := os.MkdirAll(filepath.Join(ws, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "members": [
    {"id": "in", "root": "services/in", "namespaces": ["App\\In"]},
    {"id": "out", "root": "../out", "namespaces": ["App\\Out"]}
  ]
}
`
	if err := os.WriteFile(filepath.Join(ws, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(ws, "services", "in", "in.go"): `package in

func InsideOne() {
	InsideTwo()
}

func InsideTwo() {}
`,
		filepath.Join(base, "out", "out.go"): `package out

func OutsideOne() {
	OutsideTwo()
}

func OutsideTwo() {}
`,
	}
	for p, content := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{filepath.Join(ws, "services", "in"), filepath.Join(base, "out")} {
		if _, err := query.Fresh(dir); err != nil {
			t.Fatalf("building %s: %v", dir, err)
		}
	}
	return ws
}

// TestEnclosingRoutesToTheOwningMember pins the in-workspace half.
func TestEnclosingRoutesToTheOwningMember(t *testing.T) {
	defer cleanFreshen(t)()
	ws := enclosingFixture(t)
	s := fanoutSession(t, ws)

	a, err := enclosingWorkspace(s, "services/in/in.go", 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Symbols) != 1 || a.Symbols[0].Name != "InsideOne" {
		t.Fatalf("symbols = %+v, want just InsideOne", a.Symbols)
	}
	e := a.Symbols[0]
	if e.Repo != "in" {
		t.Errorf("Repo = %q, want %q", e.Repo, "in")
	}
	if e.File != "services/in/in.go" {
		t.Errorf("File = %q, want the workspace-relative path", e.File)
	}
	if a.File != "services/in/in.go" {
		t.Errorf("answer File = %q, want the workspace-relative path", a.File)
	}
	if got := s.clause("enclosing").MembersConsulted; !reflect.DeepEqual(got, []string{"in"}) {
		t.Errorf("members_consulted = %v, want just the owning member", got)
	}
}

// TestEnclosingRoutesToAMemberRootThatClimbsOut is the case the ABSOLUTE match
// exists for. The member's declared root is "../out", so the file argument
// "../out/out.go" shares no usable prefix with any in-workspace root and a
// relative-string prefix test either misses it or lands on the wrong member.
// Resolved absolutely, it is unambiguously out's.
func TestEnclosingRoutesToAMemberRootThatClimbsOut(t *testing.T) {
	defer cleanFreshen(t)()
	ws := enclosingFixture(t)
	s := fanoutSession(t, ws)

	a, err := enclosingWorkspace(s, "../out/out.go", 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Symbols) != 1 || a.Symbols[0].Name != "OutsideOne" {
		t.Fatalf("symbols = %+v, want just OutsideOne", a.Symbols)
	}
	e := a.Symbols[0]
	if e.Repo != "out" {
		t.Errorf("Repo = %q, want %q", e.Repo, "out")
	}
	// The workspace-relative form of a file in a member that climbs out of the
	// workspace climbs out too. That is still the workspace-relative path.
	if e.File != "../out/out.go" {
		t.Errorf("File = %q, want %q", e.File, "../out/out.go")
	}
	if got := s.clause("enclosing").MembersConsulted; !reflect.DeepEqual(got, []string{"out"}) {
		t.Errorf("members_consulted = %v, want just the owning member", got)
	}
}

// TestEnclosingFileUnderNoMemberRootIsAnError pins the refusal's content: the
// file, and the member roots it was matched against.
func TestEnclosingFileUnderNoMemberRootIsAnError(t *testing.T) {
	defer cleanFreshen(t)()
	ws := enclosingFixture(t)

	_, err := enclosingWorkspace(fanoutSession(t, ws), "services/nowhere/x.go", 1, 1)
	if err == nil {
		t.Fatal("expected an error for a file under no member root")
	}
	msg := err.Error()
	for _, want := range []string{"services/nowhere/x.go", "in", "services/in", "out", "../out"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %q", msg, want)
		}
	}
}

// TestEnclosingPrefixMatchIsPerComponent stops a raw strings.HasPrefix from
// putting a sibling directory whose name merely EXTENDS a member root inside
// that member.
func TestEnclosingPrefixMatchIsPerComponent(t *testing.T) {
	root := filepath.Join("/ws", "services", "api")
	if underRoot(root, filepath.Join("/ws", "services", "apidocs", "x.go")) {
		t.Error("apidocs matched the api member root")
	}
	if !underRoot(root, filepath.Join("/ws", "services", "api", "x.go")) {
		t.Error("a real child did not match")
	}
}
