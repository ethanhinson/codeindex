package wsquery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/overlay"
	"codeindex/internal/query"
	"codeindex/internal/wsfresh"
)

// The union fixture (§3.1, §3.5).
//
// # Why the manifest order is web, api, lib
//
// It is deliberately NOT alphabetical. overlay.InEdges orders its rows by
// src_member, which IS alphabetical, so a builder who forgot to project the
// cross rows back onto MANIFEST order would produce api-before-web and the
// ordering assertions below would still look sorted. The two orders have to
// disagree for the claim to be falsifiable.
//
// # The deliberate tie
//
// web contributes THREE cross rows. All three tie on the sort key (the source
// member's manifest index), so the expected sequence pins what breaks the tie:
// InEdges' own total order (src_file, src_qname, kind, line). A fixture where
// every row had a distinct member could not fail a sort that ignored the tie.
const (
	wsMemberWeb = "web"
	wsMemberAPI = "api"
	wsMemberLib = "lib"
)

// unionFixture writes a three-member workspace, builds each member's index for
// real, and writes the cross-edges by hand.
//
// The cross-edges are hand-written rather than resolved by wsresolve because
// this test is about the QUERY-side union, not the ladder: hand-writing is what
// makes the expected sequence independently computable (per the repo learning
// determinism-tests-need-a-total-sort-key, comparing two reads of the same
// store proves nothing). freshenWorkspace is stubbed to a clean report for the
// same reason — a real freshen would re-resolve and discard them.
func unionFixture(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "members": [
    {"id": "web", "root": "services/web", "namespaces": ["App\\Web"]},
    {"id": "api", "root": "services/api", "namespaces": ["App\\Api"]},
    {"id": "lib", "root": "services/lib", "namespaces": ["App\\Lib"]}
  ]
}
`
	if err := os.WriteFile(filepath.Join(ws, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		// web ---------------------------------------------------------------
		"services/web/a.go": `package web

func WebOne() {
	webHelper()
}

func WebZero() {
	WebOne()
}

func webHelper() {}
`,
		"services/web/b.go": `package web

func WebTwo() {
	webHelper()
}
`,
		"services/web/dep.go": `package web

type WebDep struct{}

func (w WebDep) Use() {}
`,
		"services/web/base.go": `package web

type Base struct{}
`,
		"services/web/dup.go": `package web

func Dup() {}
`,
		// api ---------------------------------------------------------------
		"services/api/a.go": `package api

func ApiOne() {
	apiHelper()
}

func apiHelper() {}
`,
		"services/api/dep.go": `package api

type ApiDep struct{}

func (a ApiDep) Use() {}
`,
		"services/api/helper.go": `package api

func Helper() {}
`,
		"services/api/dup.go": `package api

func Dup() {}
`,
		"services/api/type.go": `package api

type Type struct{}

func (t Type) Method() {}

func CallsMethod() {
	var t Type
	t.Method()
}
`,
		// lib ---------------------------------------------------------------
		"services/lib/target.go": `package lib

func Target() {
	libHelper()
}

func libHelper() {}
`,
		"services/lib/local.go": `package lib

func LocalCaller() {
	Target()
}
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
	for _, m := range []string{wsMemberWeb, wsMemberAPI, wsMemberLib} {
		if _, err := query.Fresh(filepath.Join(ws, "services", m)); err != nil {
			t.Fatalf("building member %s: %v", m, err)
		}
	}

	target := overlay.SymKey{Member: wsMemberLib, File: "target.go", QName: "Target"}
	edges := []overlay.CrossEdge{
		// Callers of lib:Target, arriving from two members.
		{Src: overlay.SymKey{Member: wsMemberWeb, File: "a.go", QName: "WebOne"}, Dst: target,
			Kind: "calls", Provenance: "cross_repo_import", Confidence: ConfidenceExact, Line: 4},
		{Src: overlay.SymKey{Member: wsMemberWeb, File: "b.go", QName: "WebTwo"}, Dst: target,
			Kind: "calls", Provenance: "cross_repo_name", Confidence: ConfidenceInferred, Line: 4},
		{Src: overlay.SymKey{Member: wsMemberAPI, File: "a.go", QName: "ApiOne"}, Dst: target,
			Kind: "calls", Provenance: "cross_repo_import", Confidence: ConfidenceExact, Line: 4},
		// Dependents of lib:Target (import/extends/implements kinds).
		{Src: overlay.SymKey{Member: wsMemberWeb, File: "dep.go", QName: "WebDep"}, Dst: target,
			Kind: "extends", Provenance: "cross_repo_import", Confidence: ConfidenceExact, Line: 3},
		{Src: overlay.SymKey{Member: wsMemberAPI, File: "dep.go", QName: "ApiDep"}, Dst: target,
			Kind: "extends", Provenance: "cross_repo_import", Confidence: ConfidenceExact, Line: 3},
		// What lib:Target reaches into: one callee and one dependency.
		{Src: target, Dst: overlay.SymKey{Member: wsMemberAPI, File: "helper.go", QName: "Helper"},
			Kind: "calls", Provenance: "cross_repo_import", Confidence: ConfidenceExact, Line: 4},
		{Src: target, Dst: overlay.SymKey{Member: wsMemberWeb, File: "base.go", QName: "Base"},
			Kind: "extends", Provenance: "cross_repo_import", Confidence: ConfidenceExact, Line: 3},
	}
	ov, err := overlay.Open(overlay.Path(ws))
	if err != nil {
		t.Fatal(err)
	}
	if err := ov.PutCrossEdges(edges); err != nil {
		t.Fatal(err)
	}
	if err := ov.Close(); err != nil {
		t.Fatal(err)
	}
	return ws
}

// cleanFreshen stubs the freshen pass to a clean, resolved report. The fixture's
// cross-edges are hand-written, and a real wsfresh.Freshen would re-resolve the
// workspace and discard them.
func cleanFreshen(t *testing.T) func() {
	t.Helper()
	return stubFreshen(t, func(string) (wsfresh.Report, error) {
		return wsfresh.Report{Resolved: true}, nil
	})
}

// callerLine renders one CallerRef the way the assertions below compare them:
// member id, workspace-relative path, line, qualified name, and the two
// confidence flags.
func callerLine(c query.CallerRef) string {
	return fmt.Sprintf("%s|%s:%d|%s|ambiguous=%t|inferred=%t",
		c.Repo, c.File, c.Line, c.QName, c.Ambiguous, c.Inferred)
}

func callerLines(cs []query.CallerRef) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, callerLine(c))
	}
	return out
}

// wantUnionCallers is the expected callers union for lib:Target, computed BY
// HAND from the fixture rather than by re-reading the store:
//
//   - lib's own caller first, in the per-repo answer's own order;
//   - then the cross rows in MANIFEST order of the source member — web (index
//     0) before api (index 1), even though InEdges hands them back api-first;
//   - within web, the three rows tie on the sort key and fall back to InEdges'
//     total order (src_file, then src_qname, then kind, then line):
//     a.go/WebOne, b.go/WebTwo, dep.go/WebDep.
var wantUnionCallers = []string{
	"lib|services/lib/local.go:4|LocalCaller|ambiguous=false|inferred=false",
	"web|services/web/a.go:4|WebOne|ambiguous=false|inferred=false",
	"web|services/web/b.go:4|WebTwo|ambiguous=false|inferred=true",
	"web|services/web/dep.go:3|WebDep|ambiguous=false|inferred=false",
	"api|services/api/a.go:4|ApiOne|ambiguous=false|inferred=false",
	"api|services/api/dep.go:3|ApiDep|ambiguous=false|inferred=false",
}

func TestCallersUnionOrderIsOwnThenCrossInManifestOrder(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Callers(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	got := callerLines(a.Callers)
	if len(got) != len(wantUnionCallers) {
		t.Fatalf("callers union has %d rows, want %d\n got: %v\nwant: %v",
			len(got), len(wantUnionCallers), got, wantUnionCallers)
	}
	for i := range got {
		if got[i] != wantUnionCallers[i] {
			t.Errorf("callers[%d] = %s, want %s", i, got[i], wantUnionCallers[i])
		}
	}
	if a.CallersTotal != len(wantUnionCallers) {
		t.Errorf("CallersTotal = %d, want %d", a.CallersTotal, len(wantUnionCallers))
	}
	if a.Anchor != "Target" {
		t.Errorf("Anchor = %q, want %q", a.Anchor, "Target")
	}
	// The definition row carries its member and a workspace-relative path too.
	if len(a.Definitions) != 1 {
		t.Fatalf("definitions = %+v, want exactly lib's Target", a.Definitions)
	}
	if a.Definitions[0].Repo != wsMemberLib || a.Definitions[0].File != "services/lib/target.go" {
		t.Errorf("definition = %+v, want repo=lib file=services/lib/target.go", a.Definitions[0])
	}
}

// TestLimitBoundsTheUnionAndTotalCountsIt is §3.5's arithmetic, over a union
// where BOTH halves contribute: applying the limit per member and then
// concatenating would make CallersTotal - len(Callers) wrong in both
// directions, and the renderer prints that difference literally.
func TestLimitBoundsTheUnionAndTotalCountsIt(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	const limit = 2
	a, err := Callers(ws, "Target", limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Callers) != limit {
		t.Fatalf("limit %d returned %d callers: %v", limit, len(a.Callers), callerLines(a.Callers))
	}
	if a.CallersTotal != len(wantUnionCallers) {
		t.Errorf("CallersTotal = %d, want the UNIONED count %d", a.CallersTotal, len(wantUnionCallers))
	}
	got := callerLines(a.Callers)
	for i := range got {
		if got[i] != wantUnionCallers[i] {
			t.Errorf("truncated callers[%d] = %s, want %s", i, got[i], wantUnionCallers[i])
		}
	}
	// The "+N more" the renderer prints is Total - len(list), and it must count
	// the rows the union really has.
	wantMore := len(wantUnionCallers) - limit
	if got := a.CallersTotal - len(a.Callers); got != wantMore {
		t.Errorf("+%d more, want +%d", got, wantMore)
	}
	text, err := CallersText(ws, "Target", limit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, fmt.Sprintf("... (+%d more; raise limit)", wantMore)) {
		t.Errorf("rendered text does not carry the +%d line:\n%s", wantMore, text)
	}
	if !strings.Contains(text, "web: services/web/a.go:4") {
		t.Errorf("rendered text lost the member prefix or the workspace-relative path:\n%s", text)
	}
}

// TestDependentsAndImpactBlockAgree is the two-site coherence check the repo
// learning one-invariant-many-sites-drifts demands: the standalone dependents
// answer and impact's dependents block are read AGAINST EACH OTHER, from the
// same anchor and the same root — not each against its own expectation.
func TestDependentsAndImpactBlockAgree(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	standalone, err := Dependents(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	imp, err := Impact(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	if standalone.Total != imp.DependentsTotal {
		t.Errorf("dependents total disagrees between sites: dependents=%d, impact=%d",
			standalone.Total, imp.DependentsTotal)
	}
	if len(standalone.Dependents) != len(imp.Dependents) {
		t.Fatalf("dependents list length disagrees: dependents=%d, impact=%d",
			len(standalone.Dependents), len(imp.Dependents))
	}
	for i := range standalone.Dependents {
		if standalone.Dependents[i] != imp.Dependents[i] {
			t.Errorf("dependents[%d] disagrees:\n dependents: %+v\n impact:     %+v",
				i, standalone.Dependents[i], imp.Dependents[i])
		}
	}
	// A coherence check between two empty lists proves nothing.
	if standalone.Total == 0 {
		t.Fatal("fixture produced no dependents; the coherence check is vacuous")
	}
	// And the same for the callers block, which is the other shared invariant.
	callers, err := Callers(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	if callers.CallersTotal != imp.CallersTotal {
		t.Errorf("callers total disagrees between sites: callers=%d, impact=%d",
			callers.CallersTotal, imp.CallersTotal)
	}
}

func TestDependentsUnionIsCrossMember(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Dependents(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"web|extends|services/web/dep.go:3|WebDep",
		"api|extends|services/api/dep.go:3|ApiDep",
	}
	got := make([]string, 0, len(a.Dependents))
	for _, d := range a.Dependents {
		got = append(got, fmt.Sprintf("%s|%s|%s:%d|%s", d.Repo, d.Kind, d.File, d.Line, d.QName))
	}
	if len(got) != len(want) {
		t.Fatalf("dependents = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("dependents[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestCalleesUnionAddsTheCrossEdgeTarget(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Callees(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	var own, cross int
	for _, c := range a.Callees {
		switch c.Repo {
		case wsMemberLib:
			own++
		case wsMemberAPI:
			cross++
			if c.QName != "Helper" || c.DefFile != "services/api/helper.go" {
				t.Errorf("cross callee = %+v, want api's Helper at services/api/helper.go", c)
			}
			if c.CallLine != 4 {
				t.Errorf("cross callee call line = %d, want the cross-edge's line 4", c.CallLine)
			}
		default:
			t.Errorf("callee from unexpected member: %+v", c)
		}
	}
	if own == 0 || cross != 1 {
		t.Errorf("callees union: own=%d cross=%d, want own>0 and cross=1 (%+v)", own, cross, a.Callees)
	}
	if a.Total != len(a.Callees) {
		t.Errorf("Total = %d, want the unioned count %d", a.Total, len(a.Callees))
	}
}

func TestDepsUnionAddsTheCrossEdgeTarget(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Deps(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Sections) == 0 {
		t.Fatal("deps returned no sections")
	}
	found := false
	for _, s := range a.Sections {
		for _, d := range s.Deps {
			if d.Repo == wsMemberWeb {
				found = true
				if d.DefFile != "services/web/base.go" {
					t.Errorf("cross dep def file = %q, want services/web/base.go", d.DefFile)
				}
				if d.Kind != "extends" {
					t.Errorf("cross dep kind = %q, want extends", d.Kind)
				}
			}
		}
		if s.Total != len(s.Deps) {
			t.Errorf("section %q total %d != list %d (limit was not binding)", s.Label, s.Total, len(s.Deps))
		}
	}
	if !found {
		t.Errorf("deps union lost the cross-edge dependency: %+v", a.Sections)
	}
}

// TestImpactStaysDepthOne: the depth-1 neighbourhood includes cross-edges with
// no flag, but it is still depth 1. WebZero calls WebOne, which calls Target
// across the member boundary — WebZero is a depth-2 neighbour and must not
// appear anywhere in the answer.
func TestImpactStaysDepthOne(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	text, err := ImpactText(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "WebOne") {
		t.Fatalf("impact lost the depth-1 cross caller WebOne:\n%s", text)
	}
	if strings.Contains(text, "WebZero") {
		t.Errorf("impact reached a depth-2 neighbour (WebZero) — it must stay depth-1:\n%s", text)
	}
}

func TestNavUnionsItsCallersComponent(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Nav(ws, "Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	got := callerLines(a.Callers)
	if len(got) != len(wantUnionCallers) {
		t.Fatalf("nav callers = %v, want the same union callers gives: %v", got, wantUnionCallers)
	}
	for i := range got {
		if got[i] != wantUnionCallers[i] {
			t.Errorf("nav callers[%d] = %s, want %s", i, got[i], wantUnionCallers[i])
		}
	}
	if a.CallersTotal != len(wantUnionCallers) {
		t.Errorf("nav CallersTotal = %d, want %d", a.CallersTotal, len(wantUnionCallers))
	}
	if a.CallersFromGrep {
		t.Error("nav fell back to grep attribution despite having call edges")
	}
}

// --- anchor prefixes (§3.4) -------------------------------------------------

func TestBareAmbiguousAnchorReturnsEveryCandidateDefinition(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Callers(ws, "Dup", 100)
	if err != nil {
		t.Fatal(err)
	}
	// The single-repo path answers a duplicate name with several definition
	// rows; the workspace answer is the SAME shape, in manifest order.
	want := []string{"web|services/web/dup.go", "api|services/api/dup.go"}
	got := make([]string, 0, len(a.Definitions))
	for _, d := range a.Definitions {
		got = append(got, d.Repo+"|"+d.File)
	}
	if len(got) != len(want) {
		t.Fatalf("bare ambiguous anchor gave definitions %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("definitions[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestMemberPrefixScopesTheDefinitionLookup(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Callers(ws, "api:Dup", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Definitions) != 1 {
		t.Fatalf("api:Dup gave %d definitions, want only api's: %+v", len(a.Definitions), a.Definitions)
	}
	if a.Definitions[0].Repo != wsMemberAPI {
		t.Errorf("api:Dup resolved into member %q", a.Definitions[0].Repo)
	}
	if a.Anchor != "api:Dup" {
		t.Errorf("Anchor = %q, want the anchor as typed", a.Anchor)
	}
}

// TestPrefixStripBeforeSplitAnchor covers the dotted form: the prefix is
// removed BEFORE query.SplitAnchor sees the remainder, so "api:Type.Method"
// parses as (Method, Type) inside member api.
func TestPrefixStripBeforeSplitAnchor(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Callers(ws, "api:Type.Method", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Definitions) != 1 || a.Definitions[0].QName != "Type.Method" {
		t.Fatalf("api:Type.Method gave definitions %+v, want api's Type.Method", a.Definitions)
	}
	if a.Definitions[0].Repo != wsMemberAPI {
		t.Errorf("resolved into member %q, want api", a.Definitions[0].Repo)
	}
	if a.CallersTotal == 0 {
		t.Error("api:Type.Method found no callers; CallsMethod calls it")
	}
}

func TestUnknownMemberPrefixIsAnErrorListingTheKnownIDs(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	_, err := Callers(ws, "nope:Target", 100)
	if err == nil {
		t.Fatal("unknown member prefix returned no error")
	}
	msg := err.Error()
	for _, id := range []string{wsMemberWeb, wsMemberAPI, wsMemberLib} {
		if !strings.Contains(msg, id) {
			t.Errorf("error does not list known member %q: %s", id, msg)
		}
	}
	if !strings.Contains(msg, "nope") {
		t.Errorf("error does not name the unknown id: %s", msg)
	}
}

// TestDoubleColonAnchorIsNotTreatedAsAMemberPrefix is the load-bearing second
// clause of §3.4. Without it, "api::HandleLogin" in a workspace with a member
// named api becomes ":HandleLogin", which NEITHER query.SplitAnchor branch
// parses — Index("::") misses and LastIndex(".") misses — so the lookup is
// silently wrong in exactly the PHP/TS anchors the bench corpus is full of.
func TestDoubleColonAnchorIsNotTreatedAsAMemberPrefix(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	a, err := Callers(ws, "api::Target", 100)
	if err != nil {
		t.Fatal(err)
	}
	if a.Anchor != "api::Target" {
		t.Errorf("Anchor = %q, want the anchor as typed", a.Anchor)
	}
	// The remainder must have reached SplitAnchor intact, i.e. parsed as
	// (Target, api) — a parent no member defines, so no definition matches.
	// What must NOT happen is a ":Target" lookup, or the api-scoped Target
	// lookup a naive first-colon split would produce.
	for _, d := range a.Definitions {
		if d.QName == "Target" {
			t.Errorf("api::Target was silently rewritten to a bare Target lookup: %+v", d)
		}
	}
}
