package wsquery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
	"codeindex/internal/query"
)

// Workspace text goldens (§7.2).
//
// Task 1 pinned the nine renderers in REPO mode. These pin them in WORKSPACE
// mode, over a small fixture with exactly the four features §7.2 names: TWO
// MEMBERS, ONE CROSS-EDGE, ONE AMBIGUITY, ONE SUPPRESSION. Between them the
// goldens below cover
//
//   - the `repo:` prefix on every reference line,
//   - workspace-relative paths (services/<member>/... rather than member-relative),
//   - MANIFEST ordering (see the fixture comment — it is not alphabetical),
//   - `limit` truncation arithmetic over the union (§3.5): the limit bounds the
//     UNIONED list, *Total counts the UNIONED set, and the "+N more" figure the
//     renderer prints is the difference,
//   - the coverage clause on both of its surfaces — the trailing line for the
//     eight clause-less answer types, and inside `(coverage: …)` for impact,
//   - the anchor-prefix path AND the bare-anchor-ambiguity path.
//
// # These are hand-authored literals, not captured output
//
// Per the repo learning determinism-tests-need-a-total-sort-key, a golden that
// was produced by running the code it guards proves nothing. Each expectation
// below is written as an explicit line sequence derived from the fixture, and
// the ordering claim it makes is falsifiable: the manifest declares web BEFORE
// api, which is the reverse of both alphabetical order and the overlay's own
// src_member ordering, so any implementation that sorts by id rather than
// projecting onto manifest order reddens here. The cross-row TIE (several rows
// sharing one member index) is exercised by union_test.go's wantUnionCallers,
// which this fixture deliberately does not duplicate — §7.2 freezes it at ONE
// cross-edge.

const (
	goldenMemberWeb = "web"
	goldenMemberAPI = "api"
	// goldenVendoredNS is the namespace web VENDORS and api OWNS. It is what
	// the fixture's single suppression record suppresses.
	goldenVendoredNS = "acme/lib"
)

// goldenFixture writes the §7.2 workspace.
//
// # The shape, feature by feature
//
// TWO MEMBERS: web (services/web) then api (services/api), declared in that
// order. The order is deliberately not alphabetical — see the package comment
// above for why that is what makes the ordering assertions falsifiable.
//
// ONE AMBIGUITY: both members define a function named Dup, so the BARE anchor
// "Dup" is ambiguous and answers with several definition rows — the same
// multi-candidate shape the single-repo path already returns for a duplicate
// name (§3.4). The member-prefixed anchor "api:Dup" resolves it.
//
// ONE SUPPRESSION + ONE CROSS-EDGE, wired to each other: web VENDORS the
// namespace acme/lib that api owns, so web's own graph.db resolves both of its
// Zeta() calls into a tier-1 snapshot. The overlay carries one suppression
// (web vendors acme/lib, api wins) and one cross-edge, and the cross-edge sits
// at exactly ONE of those two call sites — WebConsumer's, not
// WebAlsoCallsZeta's.
//
// That asymmetry is the point of the fixture rather than an accident of it. It
// puts BOTH halves of §3.6's narrowing on one answer:
//
//   - WebConsumer's intra-repo edge is DROPPED, because a cross-edge speaks for
//     the same call site — counting both would report one call twice;
//   - WebAlsoCallsZeta's intra-repo edge SURVIVES, because no cross-edge stands
//     behind its suppression, and deleting it would remove a still-correct edge
//     with nothing in its place.
//
// suppress_test.go pins that rule on FilterSuppressedEdges directly; this is
// the same rule observed end to end, on the rendered answer, which is where a
// join between the filter's dropped set and the per-repo answer's rows can
// drift without either unit test noticing.
func goldenFixture(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "members": [
    {"id": "web", "root": "services/web", "namespaces": ["example.com/web"]},
    {"id": "api", "root": "services/api", "namespaces": ["acme/lib"]}
  ]
}
`
	if err := os.WriteFile(filepath.Join(ws, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"services/web/consumer.go": `package web

func WebConsumer() {
	Zeta()
}

func WebAlsoCallsZeta() {
	Zeta()
}
`,
		"services/web/dup.go": `package web

func Dup() {}
`,
		"services/api/zeta.go": `package api

func Zeta() {}

type Base struct{}

type ApiChild struct{ Base }
`,
		"services/api/caller.go": `package api

func ApiCallsZeta() {
	Zeta()
}

func ApiAlsoCallsZeta() {
	Zeta()
}
`,
		"services/api/dup.go": `package api

func Dup() {}
`,
	}
	for name, content := range files {
		p := filepath.Join(ws, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{goldenMemberWeb, goldenMemberAPI} {
		if _, err := query.Fresh(filepath.Join(ws, "services", id)); err != nil {
			t.Fatalf("building member %s: %v", id, err)
		}
	}

	attachVendoredZeta(t, filepath.Join(ws, "services", goldenMemberWeb))

	ov, err := overlay.Open(overlay.Path(ws))
	if err != nil {
		t.Fatal(err)
	}
	// The ONE cross-edge. Its src key must name WebConsumer's call site
	// exactly — same file, same qname, same kind, same line — or the §3.6
	// filter has nothing to join against and the suppression half of the
	// fixture goes silent.
	if err := ov.PutCrossEdges([]overlay.CrossEdge{{
		Src:        overlay.SymKey{Member: goldenMemberWeb, File: "consumer.go", QName: "WebConsumer"},
		Dst:        overlay.SymKey{Member: goldenMemberAPI, File: "zeta.go", QName: "Zeta"},
		Kind:       string(graph.KindCalls),
		Provenance: "cross_repo_import",
		Confidence: ConfidenceExact,
		Line:       4,
	}}); err != nil {
		t.Fatal(err)
	}
	// The ONE suppression.
	if err := ov.PutSuppressions([]overlay.Suppression{{
		ConsumerMember:    goldenMemberWeb,
		Namespace:         goldenVendoredNS,
		OwnerMember:       goldenMemberAPI,
		SuppressedVersion: "v1.2.3",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ov.Close(); err != nil {
		t.Fatal(err)
	}
	return ws
}

// attachVendoredZeta gives memberRoot a vendored tier-1 copy of acme/lib's
// Zeta, so its own Zeta() calls resolve to a tier-1 symbol and therefore appear
// in (*graph.Store).TierOneEdges — which is the left-hand side of the §3.6
// join. Without this the suppression record would have nothing to match and the
// fixture's suppression half would be inert.
func attachVendoredZeta(t *testing.T, memberRoot string) {
	t.Helper()
	mapPath := filepath.Join(t.TempDir(), "dep.map.db")
	m, err := graph.Open(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteDepMeta(goldenVendoredNS, "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if err := m.PutDepSymbols(goldenVendoredNS, "v1.2.3", "z.go", "maphash-z", 1, 1,
		[]graph.Symbol{{Name: "Zeta", Kind: graph.KindFunc, StartLine: 1, EndLine: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := graph.Open(filepath.Join(memberRoot, ".codeindex", "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ns, _, names, err := st.AttachMap(mapPath, "vendor/"+goldenVendoredNS)
	if err != nil {
		t.Fatal(err)
	}
	if ns != goldenVendoredNS {
		t.Fatalf("attached namespace = %q, want %q", ns, goldenVendoredNS)
	}
	if err := st.ReResolve(names); err != nil {
		t.Fatal(err)
	}
	// Non-vacuity: both Zeta() call sites must now be tier-1 edges, or the
	// suppression has nothing to act on and every assertion about the filter
	// below would pass for the wrong reason.
	edges, err := st.TierOneEdges()
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 {
		t.Fatalf("fixture: web has %d tier-1 edges, want 2 (WebConsumer's and WebAlsoCallsZeta's): %+v",
			len(edges), edges)
	}
	for _, e := range edges {
		if e.DstNamespace != goldenVendoredNS {
			t.Fatalf("fixture: tier-1 edge %+v is not in the suppressed namespace", e)
		}
	}
}

// golden joins an expected line sequence into the text the renderers produce.
// The lines are listed rather than written as one raw literal so that the
// SIGNIFICANT TRAILING SPACES some rows carry (an empty signature leaves two)
// are visible to a reader and survive an editor that trims them.
func golden(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

// goldenClause is the coverage clause both members' answers carry when the
// workspace is clean. It is spelled out rather than built from Clause{} so a
// change to the clause's own rendering shows up here as a diff rather than
// being tracked silently by a shared helper.
const goldenClauseBoth = "workspace: members_consulted: web, api; members_stale: (none); " +
	"boundary: symbols outside this workspace are unknown to it"

const goldenClauseAPIOnly = "workspace: members_consulted: api; members_stale: (none); " +
	"boundary: symbols outside this workspace are unknown to it"

func assertGolden(t *testing.T, name, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		g, w := "", ""
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("%s: line %d\n got %q\nwant %q", name, i+1, g, w)
		}
	}
	t.Errorf("%s: full got:\n%s", name, got)
}

// TestWorkspaceGoldenCallers is the headline golden: the union of two members'
// own callers plus the one cross-edge, with the suppression filter having
// removed exactly one own row.
//
// Read the caller list against the fixture:
//
//	web: consumer.go:8  WebAlsoCallsZeta   own row, suppressed namespace, NO
//	                                       cross-edge behind it -> SURVIVES
//	api: caller.go:4    ApiCallsZeta       own row
//	api: caller.go:8    ApiAlsoCallsZeta   own row
//	web: consumer.go:4  WebConsumer        the CROSS row; its own intra-repo
//	                                       twin was dropped by §3.6
//
// Own rows come first, in ANCHOR-MEMBER order, which is manifest order: web
// before api. Cross rows follow, in manifest order of the SOURCE member. An
// implementation that sorted by member id would put api first and redden.
func TestWorkspaceGoldenCallers(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	got, err := CallersText(ws, "Zeta", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := golden(
		"def  Zeta  web: services/web/vendor/acme/lib/z.go:1  ",
		"def  Zeta  api: services/api/zeta.go:3  func Zeta()",
		"callers (4):",
		"  web: services/web/consumer.go:8  WebAlsoCallsZeta",
		"  api: services/api/caller.go:4  ApiCallsZeta",
		"  api: services/api/caller.go:8  ApiAlsoCallsZeta",
		"  web: services/web/consumer.go:4  WebConsumer",
		"referenced in 2 file(s): services/web/consumer.go services/api/caller.go",
		goldenClauseBoth,
	)
	assertGolden(t, "callers Zeta", got, want)
}

// TestWorkspaceGoldenSuppressionDedupesExactlyOneCallSite states the §3.6
// claim as a count rather than leaving it implicit in the golden above, so a
// reader who changes the fixture sees which property broke.
//
// WebConsumer must appear EXACTLY ONCE. Twice means the filter stopped
// dropping the intra-repo twin and the union is double-counting one call; zero
// means it over-dropped and deleted the cross row's subject as well.
// WebAlsoCallsZeta must appear exactly once too — it has no cross-edge behind
// its suppression, so nothing may remove it.
func TestWorkspaceGoldenSuppressionDedupesExactlyOneCallSite(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	a, err := Callers(ws, "Zeta", 100)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, c := range a.Callers {
		counts[c.QName]++
	}
	if counts["WebConsumer"] != 1 {
		t.Errorf("WebConsumer appears %d time(s), want exactly 1 — the §3.6 filter must drop the "+
			"intra-repo twin of the cross-edge's call site and keep the cross row: %v",
			counts["WebConsumer"], callerLines(a.Callers))
	}
	if counts["WebAlsoCallsZeta"] != 1 {
		t.Errorf("WebAlsoCallsZeta appears %d time(s), want exactly 1 — a suppression with NO "+
			"cross-edge behind it must not remove a still-correct intra-repo edge: %v",
			counts["WebAlsoCallsZeta"], callerLines(a.Callers))
	}
	// The dropped row and the surviving one are attributed differently, which
	// is the observable difference between "kept" and "replaced by a cross row".
	for _, c := range a.Callers {
		if c.QName == "WebConsumer" && c.Line != 4 {
			t.Errorf("WebConsumer row is at line %d, want the cross-edge's line 4", c.Line)
		}
		if c.QName == "WebAlsoCallsZeta" && c.Line != 8 {
			t.Errorf("WebAlsoCallsZeta row is at line %d, want its own call site 8", c.Line)
		}
	}
}

// TestWorkspaceGoldenCallersLimitTruncatesTheUnion is §3.5's arithmetic pinned
// as bytes AND as numbers: the limit bounds the UNIONED list, CallersTotal
// counts the UNIONED set, and the "+N more" the renderer prints is exactly the
// difference. Applying the limit per member and concatenating would make all
// three disagree.
//
// The clause line here is DELIBERATELY not goldenClauseBoth: a truncated
// workspace answer discloses the cut (rows_withheld / members_truncated), and
// this is the one golden in this file whose answer is truncated. Both members
// are named because both contributed callers that did not survive — the graph
// half reads each member with `unlimited`, so unlike the fan-out verbs a member
// can be cut PARTIALLY here.
func TestWorkspaceGoldenCallersLimitTruncatesTheUnion(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	const limit = 2
	a, err := Callers(ws, "Zeta", limit)
	if err != nil {
		t.Fatal(err)
	}
	if a.CallersTotal != 4 {
		t.Errorf("CallersTotal = %d, want the unioned count 4", a.CallersTotal)
	}
	if len(a.Callers) != limit {
		t.Errorf("returned %d callers, want the limit %d", len(a.Callers), limit)
	}
	if got := a.CallersTotal - len(a.Callers); got != 2 {
		t.Errorf("the renderer's +N figure would be %d, want 2", got)
	}

	got, err := CallersText(ws, "Zeta", limit)
	if err != nil {
		t.Fatal(err)
	}
	want := golden(
		"def  Zeta  web: services/web/vendor/acme/lib/z.go:1  ",
		"def  Zeta  api: services/api/zeta.go:3  func Zeta()",
		"callers (4):",
		"  web: services/web/consumer.go:8  WebAlsoCallsZeta",
		"  api: services/api/caller.go:4  ApiCallsZeta",
		"  ... (+2 more; raise limit)",
		"referenced in 2 file(s): services/web/consumer.go services/api/caller.go",
		"workspace: members_consulted: web, api; members_stale: (none); "+
			"rows_withheld: 2; members_truncated: web, api; "+
			"boundary: symbols outside this workspace are unknown to it",
	)
	assertGolden(t, "callers Zeta limit 2", got, want)
}

// TestWorkspaceGoldenImpact pins impact's TWO differences from the other eight:
// its clause rides INSIDE the existing coverage sentence rather than on a
// trailing line, and it must appear exactly once (§4.5 — a trailing line here
// would put the same sentence on the answer twice).
func TestWorkspaceGoldenImpact(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	got, err := ImpactText(ws, "Zeta", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := golden(
		"impact of Zeta: 2 definition(s), 4 caller(s), 0 callee(s), 0 dependent(s)",
		"(coverage: "+impactCoverage+"; "+goldenClauseBoth+")",
		"",
		"def  Zeta  web: services/web/vendor/acme/lib/z.go:1  ",
		"def  Zeta  api: services/api/zeta.go:3  func Zeta()",
		"",
		"callers — these break if Zeta's behavior/signature changes:",
		"  web: services/web/consumer.go:8  WebAlsoCallsZeta",
		"  api: services/api/caller.go:4  ApiCallsZeta",
		"  api: services/api/caller.go:8  ApiAlsoCallsZeta",
		"  web: services/web/consumer.go:4  WebConsumer",
		"",
		"dependents — who imports/extends/implements Zeta:",
		"",
		"callees — what Zeta depends on:",
	)
	assertGolden(t, "impact Zeta", got, want)
	if n := strings.Count(got, "members_consulted"); n != 1 {
		t.Errorf("impact carries the clause %d times, want exactly 1 (it rides inside Coverage)", n)
	}
}

func TestWorkspaceGoldenCallees(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	got, err := CalleesText(ws, "Zeta", 10)
	if err != nil {
		t.Fatal(err)
	}
	// Zeta's body is empty in api and its vendored copy has none, so the union
	// is empty — and the clause still names BOTH members, because it answers
	// "what did you look at", not "what did you find" (§4.4).
	want := golden(
		"callees of Zeta (0):",
		goldenClauseBoth,
	)
	assertGolden(t, "callees Zeta", got, want)
}

func TestWorkspaceGoldenNav(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	got, err := NavText(ws, "Zeta", 10)
	if err != nil {
		t.Fatal(err)
	}
	// nav's callers component is the same union `callers` returns; its match
	// and file components FAN OUT and concatenate in manifest order (§3.2), so
	// every web row precedes every api row.
	want := golden(
		"nav Zeta: 2 definition(s), 4 caller(s), 4 referencing file(s)",
		"def  Zeta  web: services/web/vendor/acme/lib/z.go:1  ",
		"def  Zeta  api: services/api/zeta.go:3  func Zeta()",
		"matches:",
		"  Zeta  func  web: services/web/vendor/acme/lib/z.go:1  callers=2  [dep acme/lib@v1.2.3]  [exact]",
		"  WebAlsoCallsZeta  func  web: services/web/consumer.go:7  [all-tokens]",
		"  Zeta  func  api: services/api/zeta.go:3  callers=2  [exact]",
		"  ApiAlsoCallsZeta  func  api: services/api/caller.go:7  [all-tokens]",
		"  ApiCallsZeta  func  api: services/api/caller.go:3  [all-tokens]",
		"callers (4):",
		"  web: services/web/consumer.go:8  WebAlsoCallsZeta",
		"  api: services/api/caller.go:4  ApiCallsZeta",
		"  api: services/api/caller.go:8  ApiAlsoCallsZeta",
		"  web: services/web/consumer.go:4  WebConsumer",
		"referenced in 4 file(s): services/web/vendor/acme/lib/z.go services/web/consumer.go "+
			"services/api/zeta.go services/api/caller.go",
		goldenClauseBoth,
	)
	assertGolden(t, "nav Zeta", got, want)
}

// TestWorkspaceGoldenDependentsAndDeps pins the two edge-direction verbs. Their
// anchors live in api only, so the clause names ONE consulted member — which is
// the falsifiable half: members_consulted is what the answer READ, not the
// manifest and not the present set (§4.4).
func TestWorkspaceGoldenDependentsAndDeps(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	gotDependents, err := DependentsText(ws, "Base", 10)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "dependents Base", gotDependents, golden(
		"dependents of Base (1):",
		"  extends    api: services/api/zeta.go:7  ApiChild",
		goldenClauseAPIOnly,
	))

	gotDeps, err := DepsText(ws, "ApiChild", 10)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "deps ApiChild", gotDeps, golden(
		"deps of ApiChild (1):",
		"  extends    Base (api: services/api/zeta.go:5)  @7",
		"file imports (services/api/zeta.go) (0):",
		goldenClauseAPIOnly,
	))
}

// TestWorkspaceGoldenFind pins the fan-out: complete per-member sets
// concatenated in MANIFEST order, within-member order preserved, Total the SUM
// of the per-member totals (§3.2). No rank-merge across members — web's
// weakest match still precedes api's strongest.
func TestWorkspaceGoldenFind(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	got, err := FindText(ws, "Zeta", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := golden(
		`find "Zeta" (5):`,
		"  Zeta  func  web: services/web/vendor/acme/lib/z.go:1  callers=2  [dep acme/lib@v1.2.3]  [exact]",
		"  WebAlsoCallsZeta  func  web: services/web/consumer.go:7  [all-tokens]",
		"  Zeta  func  api: services/api/zeta.go:3  callers=2  [exact]",
		"  ApiAlsoCallsZeta  func  api: services/api/caller.go:7  [all-tokens]",
		"  ApiCallsZeta  func  api: services/api/caller.go:3  [all-tokens]",
		goldenClauseBoth,
	)
	assertGolden(t, "find Zeta", got, want)
}

// TestWorkspaceGoldenGrep pins grep's fan-out and its summed RawHits.
//
// The BACKEND is substituted rather than pinned: which one search.Grep reports
// is a property of whether ripgrep is on the machine's PATH, not of the answer,
// so a literal here would make the golden fail on half the machines that run
// it. It is read off a real per-member answer and asserted non-blank, which is
// the claim §3.2 actually makes about the field.
func TestWorkspaceGoldenGrep(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	perMember, err := query.Grep(filepath.Join(ws, "services", goldenMemberWeb), "Zeta", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if perMember.Backend == "" {
		t.Fatal("per-repo grep reported a blank backend; the golden below would be vacuous")
	}

	got, err := GrepText(ws, "Zeta", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	want := golden(
		fmt.Sprintf(`grep "Zeta": 8 raw hits -> 5 symbols/sites (%s)`, perMember.Backend),
		"  WebAlsoCallsZeta  web: services/web/consumer.go:7  hits=2  [definition]",
		"  WebConsumer  web: services/web/consumer.go:4  hits=1",
		"  ApiCallsZeta  api: services/api/caller.go:3  hits=2  [definition]",
		"  ApiAlsoCallsZeta  api: services/api/caller.go:7  hits=2  [definition]",
		"  Zeta  api: services/api/zeta.go:3  hits=1  [definition]",
		goldenClauseBoth,
	)
	assertGolden(t, "grep Zeta", got, want)
}

// TestWorkspaceGoldenBareAmbiguousAnchor is §7.2's bare-anchor-ambiguity path.
// Dup is defined in both members, so the answer carries BOTH definition rows in
// manifest order — the same multi-candidate shape the single-repo path returns
// for a duplicate name, with no new answer type invented for the workspace.
func TestWorkspaceGoldenBareAmbiguousAnchor(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	got, err := CallersText(ws, "Dup", 10)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "callers Dup", got, golden(
		"def  Dup  web: services/web/dup.go:3  func Dup()",
		"def  Dup  api: services/api/dup.go:3  func Dup()",
		"callers (0):",
		"referenced in 0 file(s):",
		goldenClauseBoth,
	))
}

// TestWorkspaceGoldenMemberPrefixedAnchor is §7.2's anchor-prefix path, read
// AGAINST the bare answer above: the prefix removes web's candidate and web
// from members_consulted. Pinning only the prefixed answer would not show that
// the prefix is what narrowed it.
func TestWorkspaceGoldenMemberPrefixedAnchor(t *testing.T) {
	defer cleanFreshen(t)()
	ws := goldenFixture(t)

	got, err := CallersText(ws, "api:Dup", 10)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "callers api:Dup", got, golden(
		"def  Dup  api: services/api/dup.go:3  func Dup()",
		"callers (0):",
		"referenced in 0 file(s):",
		goldenClauseAPIOnly,
	))
	if strings.Contains(got, "services/web") {
		t.Errorf("the member prefix did not scope the lookup; web still appears:\n%s", got)
	}
}
