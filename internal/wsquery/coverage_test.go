package wsquery

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/query"
	"codeindex/internal/wsfresh"
)

// threeMemberManifest is the declared member set every union test below is
// read against: api, shared, web, in that manifest order. Manifest order is
// deliberately NOT alphabetical-by-accident — "web" sorts after "shared", so a
// test that passes under a sorted implementation would also pass here. Where
// order is the claim, the assertion uses an id whose manifest position differs
// from its sorted position.
func threeMemberManifest() *config.Workspace {
	return &config.Workspace{
		Version: 1,
		Members: []config.Member{
			{ID: "web", Root: "services/web"},
			{ID: "api", Root: "services/api"},
			{ID: "shared", Root: "services/shared"},
		},
	}
}

// ---------------------------------------------------------------------------
// Manifest faults are errors, not degrades (§4.2, item 1)
// ---------------------------------------------------------------------------

// TestManifestFaultIsAnErrorNotADegrade pins the discrimination that makes the
// §4.2 exception real. A workspace whose manifest is absent, unparseable, or
// invalid is a CONFIGURATION FAULT: newSession returns an error and no clause
// exists. It must never come back as a degraded answer, because Freshen's ten
// error returns are indistinguishable opaque wraps and a manifest fault
// dressed as staleness is unactionable.
func TestManifestFaultIsAnErrorNotADegrade(t *testing.T) {
	cases := []struct {
		name     string
		manifest string // "" means write no manifest at all
	}{
		{"absent", ""},
		{"unparseable", "{ this is not json"},
		{"invalid", `{"version": 1, "members": [{"id": "", "root": ""}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.manifest != "" {
				if err := os.MkdirAll(filepath.Join(dir, ".codeindex"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, config.WorkspaceFile), []byte(tc.manifest), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			// The freshen must not even be reached: the manifest is loaded
			// first, explicitly, and a fault short-circuits there.
			called := false
			restore := stubFreshen(t, func(string) (wsfresh.Report, error) {
				called = true
				return wsfresh.Report{}, nil
			})
			defer restore()

			s, err := newSession(dir)
			if err == nil {
				t.Fatalf("newSession returned a session for a %s manifest; a manifest fault must be an error, not a degrade (clause: %+v)",
					tc.name, s.clause("callers"))
			}
			if called {
				t.Errorf("Freshen ran despite a %s manifest: the manifest load must come FIRST", tc.name)
			}
		})
	}
}

// TestManifestLoadPrecedesFreshen pins the ORDER item 1 makes mandatory, on
// the happy path where both succeed — the absent-manifest case above cannot
// distinguish "loaded first" from "loaded only".
func TestManifestLoadPrecedesFreshen(t *testing.T) {
	dir := writeManifest(t, `{"version":1,"members":[{"id":"api","root":"services/api"}]}`)
	var gotRoot string
	restore := stubFreshen(t, func(root string) (wsfresh.Report, error) {
		gotRoot = root
		return wsfresh.Report{}, nil
	})
	defer restore()

	s, err := newSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != dir {
		t.Errorf("Freshen ran against %q, want the workspace root %q", gotRoot, dir)
	}
	if got := s.declaredIDs(); !reflect.DeepEqual(got, []string{"api"}) {
		t.Errorf("declared ids = %v, want [api]", got)
	}
}

// TestFreshenSeamDefaultsToWsfreshFreshen guards the injection seam itself: a
// package variable that some future edit repoints, or that a test leaves
// stubbed, would silently disable the whole-workspace freshen of §4.1.
func TestFreshenSeamDefaultsToWsfreshFreshen(t *testing.T) {
	want := reflect.ValueOf(wsfresh.Freshen).Pointer()
	if got := reflect.ValueOf(freshenWorkspace).Pointer(); got != want {
		t.Fatal("freshenWorkspace is not wsfresh.Freshen: the whole-workspace freshen of §4.1 is not what runs on a workspace query")
	}
}

// ---------------------------------------------------------------------------
// The degrade posture (§4.2, item 3)
// ---------------------------------------------------------------------------

// TestFreshenFailureNamesEveryDeclaredMemberStale is the D7 hard-fail guard.
// When Freshen errors, the query proceeds against the overlay as it stands and
// the clause names EVERY DECLARED MEMBER stale — not the members the partial
// Report happens to list. Reading that partial report as authoritative would
// reproduce the "quietly WRONG clean verdict" freshen.go's own comment says
// the mid-flight fold error is deliberately fatal to prevent.
func TestFreshenFailureNamesEveryDeclaredMemberStale(t *testing.T) {
	ws := threeMemberManifest()
	// A partial report that names exactly ONE member. An implementation that
	// trusts it reports [api] and passes nothing else; the bar is all three.
	partial := wsfresh.Report{Dirty: []string{"api"}}
	boom := errors.New("wsfresh: member \"shared\": index vanished mid-flight")

	s := sessionFrom("/ws", ws, partial, boom)
	c := s.clause("callers")

	if got, want := c.MembersStale, []string{"web", "api", "shared"}; !reflect.DeepEqual(got, want) {
		t.Errorf("members_stale = %v, want every declared member in manifest order %v", got, want)
	}
	if !strings.Contains(c.FreshenFailed, "index vanished mid-flight") {
		t.Errorf("freshen_failed = %q, want the Freshen error verbatim", c.FreshenFailed)
	}
	assertDegradeReason(t, c, "freshen_failed")
}

// TestFreshenFailureDoesNotFailTheQuery pins the other half of §4.2: the error
// is a degrade, so newSession still returns a usable session.
func TestFreshenFailureDoesNotFailTheQuery(t *testing.T) {
	dir := writeManifest(t, `{"version":1,"members":[{"id":"api","root":"services/api"},{"id":"web","root":"services/web"}]}`)
	restore := stubFreshen(t, func(string) (wsfresh.Report, error) {
		return wsfresh.Report{}, errors.New("wsfresh: overlay unopenable")
	})
	defer restore()

	s, err := newSession(dir)
	if err != nil {
		t.Fatalf("a Freshen error must degrade the query, not fail it; got error: %v", err)
	}
	c := s.clause("find")
	if got, want := c.MembersStale, []string{"api", "web"}; !reflect.DeepEqual(got, want) {
		t.Errorf("members_stale = %v, want %v", got, want)
	}
	assertDegradeReason(t, c, "freshen_failed")
}

// ---------------------------------------------------------------------------
// The five-way union (§4.3 item 4, plus MembersUnindexedIDs — see staleMembers
// for why §4.3's four-way phrasing is superseded)
// ---------------------------------------------------------------------------

// TestDirtyStaysInTheStaleUnionWhenResolvedIsFalse EXISTS TO STOP A FUTURE
// READER DROPPING DIRTY FROM THE UNION.
//
// The window it pins is REAL, not hypothetical. wsfresh.Freshen appends to
// rep.Dirty at ~freshen.go:255 and sets rep.Resolved = true only at ~:375, and
// its error returns at ~:292, ~:318 and ~:373 sit BETWEEN those two lines —
// which are precisely the paths §4.2 tells the query to proceed through. An
// earlier draft of the design claimed this state "cannot occur"; that claim
// was WITHDRAWN AS FALSE. If you are here because dropping Dirty made this
// test red, the test is right and the change is wrong.
func TestDirtyStaysInTheStaleUnionWhenResolvedIsFalse(t *testing.T) {
	ws := threeMemberManifest()
	rep := wsfresh.Report{
		Dirty:    []string{"api"},
		Resolved: false,
	}
	// No freshenErr: this is the future path §4.3 warns about, where a partial
	// report arrives WITHOUT an error and §4.2's every-declared-member rule
	// therefore does not cover it. The union is the only thing that discloses
	// api here.
	s := sessionFrom("/ws", ws, rep, nil)
	c := s.clause("impact")

	if got, want := c.MembersStale, []string{"api"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("members_stale = %v, want %v — Dirty must stay in the union when Resolved is false", got, want)
	}
	assertDegradeReason(t, c, "members_stale")
}

// TestDirtyIsDroppedFromTheStaleUnionWhenResolvedIsTrue: the pass that just
// ran retired their staleness. Naming them stale there is a false alarm, and a
// permanently-stale-looking workspace teaches the agent to ignore the field.
func TestDirtyIsDroppedFromTheStaleUnionWhenResolvedIsTrue(t *testing.T) {
	ws := threeMemberManifest()
	rep := wsfresh.Report{
		Dirty:    []string{"api", "web"},
		Resolved: true,
	}
	s := sessionFrom("/ws", ws, rep, nil)
	c := s.clause("impact")

	if len(c.MembersStale) != 0 {
		t.Fatalf("members_stale = %v, want empty: a re-resolved pass retires Dirty", c.MembersStale)
	}
	assertDegradeReason(t, c, "clean")
}

// TestMembersUnindexedIDsAreInTheStaleUnion: a declared member that is present
// but whose index will not open has its rows OMITTED from the answer, and
// nothing else discloses that durably — so it is named stale.
//
// §4.3 originally excluded it, on the argument that it is "covered by
// StaleStamped when it previously contributed rows, and by boundary when it
// never did". Both halves are false: StaleStamped is one-shot by its own doc
// comment, and boundary is a fixed constant about symbols OUTSIDE the
// workspace. See staleMembers for the full note.
//
// Resolved is TRUE here on purpose. That is the steady state the old exclusion
// went silent in: the resolution has already run and retired the stamp, and if
// unindexed ids were dropped alongside Dirty the clause would say nothing while
// the member's rows stayed missing.
func TestMembersUnindexedIDsAreInTheStaleUnion(t *testing.T) {
	ws := threeMemberManifest()
	rep := wsfresh.Report{
		MembersUnindexed:    2,
		MembersUnindexedIDs: []string{"web", "shared"},
		Resolved:            true,
	}
	s := sessionFrom("/ws", ws, rep, nil)
	c := s.clause("callers")
	if want := []string{"web", "shared"}; !reflect.DeepEqual(c.MembersStale, want) {
		t.Fatalf("members_stale = %v, want %v — an unindexed declared member's rows are omitted, "+
			"and members_stale is the only durable disclosure of that", c.MembersStale, want)
	}
	assertDegradeReason(t, c, "members_stale")
}

// TestBareMembersUnindexedCountAddsNothingToTheStaleUnion: the union draws from
// the ID SLICE, never from the count. A Report carrying a count with no ids
// names nobody — any implementation reaching for the count would have to invent
// ids, and inventing them is how a name and a denominator come apart.
//
// This pairs with wsfresh's TestUnindexedIDsMatchCount, which is what keeps the
// two in step at the write site; here the read side simply never guesses.
func TestBareMembersUnindexedCountAddsNothingToTheStaleUnion(t *testing.T) {
	ws := threeMemberManifest()
	rep := wsfresh.Report{
		MembersUnindexed: 2,
		Resolved:         true,
	}
	s := sessionFrom("/ws", ws, rep, nil)
	c := s.clause("callers")
	if len(c.MembersStale) != 0 {
		t.Fatalf("members_stale = %v, want empty: the count names no member, so the clause "+
			"must not invent one", c.MembersStale)
	}
}

// TestStaleUnionIsAllFiveSetsInManifestOrder exercises every set at once,
// including the Dirty member, and pins the ORDER as manifest order rather than
// the concatenation order of the five sets (which would be web-last).
func TestStaleUnionIsAllFiveSetsInManifestOrder(t *testing.T) {
	ws := threeMemberManifest() // web, api, shared
	rep := wsfresh.Report{
		Dirty:                   []string{"shared"},
		StaleStamped:            []string{"web"},
		MembersMissing:          []string{"api"},
		MembersFreshenFailedIDs: []string{"shared"}, // overlaps Dirty: a union, not a concatenation
		MembersFreshenFailed:    1,
		MembersUnindexedIDs:     []string{"web"}, // overlaps StaleStamped, for the same reason
		MembersUnindexed:        1,
		Resolved:                false,
	}
	s := sessionFrom("/ws", ws, rep, nil)
	got := s.clause("deps").MembersStale
	want := []string{"web", "api", "shared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("members_stale = %v, want %v (manifest order, de-duplicated)", got, want)
	}
}

// TestStaleUnionNeverDropsAnUndeclaredID: every set the clause draws from is
// declared-scoped today, so this state should not arise — but if it ever does,
// losing a stale id silently is the exact failure the clause exists to
// prevent, so the id is appended rather than filtered away.
func TestStaleUnionNeverDropsAnUndeclaredID(t *testing.T) {
	ws := threeMemberManifest()
	rep := wsfresh.Report{Dirty: []string{"api", "ghost"}}
	s := sessionFrom("/ws", ws, rep, nil)
	got := s.clause("callers").MembersStale
	want := []string{"api", "ghost"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("members_stale = %v, want %v: an undeclared stale id must be disclosed, not dropped", got, want)
	}
}

// ---------------------------------------------------------------------------
// members_consulted (§4.4, item 5)
// ---------------------------------------------------------------------------

// TestMembersConsultedExcludesTheFreshenPass is the load-bearing half of
// §4.4's definition. The whole-workspace freshen opens every present member,
// so a clause that counted it would list every member on every query and carry
// no information at all. Only members recorded by consult() appear.
func TestMembersConsultedExcludesTheFreshenPass(t *testing.T) {
	dir := writeManifest(t, `{"version":1,"members":[{"id":"web","root":"w"},{"id":"api","root":"a"},{"id":"shared","root":"s"}]}`)
	restore := stubFreshen(t, func(string) (wsfresh.Report, error) {
		// A wholly successful pass over all three members.
		return wsfresh.Report{MembersFreshened: 3, Resolved: false}, nil
	})
	defer restore()

	s, err := newSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.clause("callers").MembersConsulted; len(got) != 0 {
		t.Fatalf("members_consulted = %v before any read; the freshen pass must not count", got)
	}

	// The anchor's own member plus one cross-edge source member.
	s.consult("api")
	s.consult("web")
	got := s.clause("callers").MembersConsulted
	if want := []string{"web", "api"}; !reflect.DeepEqual(got, want) {
		t.Errorf("members_consulted = %v, want %v (manifest order, not call order)", got, want)
	}
}

// TestMembersConsultedListsAMemberThatContributedNothing: the clause answers
// "what did you look at", not "what did you find". A caller records at the
// point of the read; whether the read returned rows is not the clause's
// subject.
func TestMembersConsultedListsAMemberThatContributedNothing(t *testing.T) {
	s := sessionFrom("/ws", threeMemberManifest(), wsfresh.Report{}, nil)
	s.consult("shared") // read, zero rows back
	if got, want := s.clause("grep").MembersConsulted, []string{"shared"}; !reflect.DeepEqual(got, want) {
		t.Errorf("members_consulted = %v, want %v", got, want)
	}
}

// TestConsultIsIdempotentAndIgnoresEmptyIDs: fan-out verbs record per member
// per read and may reach the same member twice.
func TestConsultIsIdempotentAndIgnoresEmptyIDs(t *testing.T) {
	s := sessionFrom("/ws", threeMemberManifest(), wsfresh.Report{}, nil)
	s.consult("api", "api", "", "web")
	if got, want := s.clause("find").MembersConsulted, []string{"web", "api"}; !reflect.DeepEqual(got, want) {
		t.Errorf("members_consulted = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// boundary (item 6) and the layer policy (§4.6, item 8)
// ---------------------------------------------------------------------------

func TestBoundaryIsTheFixedSentence(t *testing.T) {
	s := sessionFrom("/ws", threeMemberManifest(), wsfresh.Report{}, nil)
	for _, verb := range []string{"callers", "find", "enclosing"} {
		if got := s.clause(verb).Boundary; got != Boundary {
			t.Errorf("%s boundary = %q, want the fixed sentence %q", verb, got, Boundary)
		}
	}
	if !strings.Contains(Boundary, "outside") || !strings.Contains(Boundary, "unknown") {
		t.Errorf("boundary sentence %q must say that symbols outside the workspace are unknown", Boundary)
	}
}

// TestClauseLayerPolicyPerVerb discharges D6's requirement that the clause
// name which LAYER it describes, verb by verb (§4.6). nav is graph-layer
// despite composing retrieval components, because its headline claim is an
// edge claim.
func TestClauseLayerPolicyPerVerb(t *testing.T) {
	want := map[string]Layer{
		"callers":    LayerGraph,
		"callees":    LayerGraph,
		"impact":     LayerGraph,
		"nav":        LayerGraph,
		"dependents": LayerGraph,
		"deps":       LayerGraph,
		"find":       LayerRetrieval,
		"grep":       LayerRetrieval,
		"enclosing":  LayerGraph,
		"search":     LayerNone,
	}
	if len(clauseLayers) != len(want) {
		t.Errorf("the layer table has %d verbs, the policy names %d — a verb was added or removed without a policy decision", len(clauseLayers), len(want))
	}
	s := sessionFrom("/ws", threeMemberManifest(), wsfresh.Report{}, nil)
	for verb, wantLayer := range want {
		if got := ClauseLayer(verb); got != wantLayer {
			t.Errorf("ClauseLayer(%q) = %q, want %q", verb, got, wantLayer)
		}
		if got := s.clause(verb).Layer; got != wantLayer {
			t.Errorf("clause(%q).Layer = %q, want %q — the policy must be in force at the construction site, not only in the table", verb, got, wantLayer)
		}
	}
	if got := ClauseLayer("not-a-verb"); got != LayerNone {
		t.Errorf("ClauseLayer of an unknown verb = %q, want LayerNone", got)
	}
}

// ---------------------------------------------------------------------------
// Rendering (§4.5, item 7)
// ---------------------------------------------------------------------------

func sampleClause() Clause {
	return Clause{
		MembersConsulted: []string{"api", "shared"},
		MembersStale:     []string{"web"},
		Boundary:         Boundary,
		Layer:            LayerGraph,
	}
}

func TestClauseRendersOnOneLine(t *testing.T) {
	c := sampleClause()
	c.FreshenFailed = "wsfresh: member \"web\":\nindex vanished"
	got := c.String()
	if strings.Contains(got, "\n") {
		t.Fatalf("clause spans multiple lines, which breaks impact's (coverage: %%s) renderer:\n%s", got)
	}
	for _, needle := range []string{"members_consulted: api, shared", "members_stale: web", "freshen_failed:", "boundary: " + Boundary} {
		if !strings.Contains(got, needle) {
			t.Errorf("clause text is missing %q:\n%s", needle, got)
		}
	}
}

func TestEmptyIDSetsRenderAsNoneNotBlank(t *testing.T) {
	c := Clause{Boundary: Boundary}
	got := c.String()
	if !strings.Contains(got, "members_consulted: (none)") || !strings.Contains(got, "members_stale: (none)") {
		t.Errorf("empty sets must render as (none), got:\n%s", got)
	}
	if strings.Contains(got, "freshen_failed") {
		t.Errorf("freshen_failed must be absent when the freshen succeeded, got:\n%s", got)
	}
}

// TestImpactCarriesTheClauseInsideCoverage pins §4.5's impact surface: the
// existing sentence PLUS the clause, still a string, so the renderer needs no
// change — and no trailing line, because that would print the clause twice.
func TestImpactCarriesTheClauseInsideCoverage(t *testing.T) {
	const existing = "call + import/extends/implements edges; type-usage references not included"
	a := &query.ImpactAnswer{Anchor: "Target", Coverage: existing}
	c := sampleClause()

	wrapped := WithImpactClause(a, c)
	if !strings.HasPrefix(a.Coverage, existing) {
		t.Errorf("the existing coverage sentence was not preserved: %q", a.Coverage)
	}
	if !strings.Contains(a.Coverage, c.String()) {
		t.Errorf("Coverage does not carry the clause: %q", a.Coverage)
	}
	text := wrapped.Text()
	if strings.Count(text, "members_consulted") != 1 {
		t.Errorf("the impact clause must appear exactly once (inside Coverage), got:\n%s", text)
	}
}

// TestTrailingClauseLineForTheOtherEight pins the trailing-line surface, added
// at the wsquery layer — internal/query's renderers are not edited for it.
func TestTrailingClauseLineForTheOtherEight(t *testing.T) {
	inner := stubAnswer("callers of Target (2)\n  a.go:5\n")
	wrapped := WithClause(inner, sampleClause())
	text := wrapped.Text()
	if !strings.HasPrefix(text, inner.Text()) {
		t.Errorf("the inner answer's bytes were altered:\n%s", text)
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if last := lines[len(lines)-1]; last != sampleClause().String() {
		t.Errorf("last line = %q, want the clause", last)
	}
}

func TestAppendClauseLineAddsAMissingNewline(t *testing.T) {
	got := AppendClauseLine("no trailing newline", sampleClause())
	if !strings.HasPrefix(got, "no trailing newline\nworkspace: ") {
		t.Errorf("clause was not put on its own line: %q", got)
	}
}

// TestJSONCarriesTheWorkspaceSibling pins §4.5's JSON half: the inner answer's
// own keys, in their own order, plus a `workspace` object.
func TestJSONCarriesTheWorkspaceSibling(t *testing.T) {
	inner := stubAnswer("text")
	b, err := json.Marshal(WithClause(inner, sampleClause()))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Anchor    string `json:"anchor"`
		Workspace *struct {
			MembersConsulted []string `json:"members_consulted"`
			MembersStale     []string `json:"members_stale"`
			Boundary         string   `json:"boundary"`
			FreshenFailed    string   `json:"freshen_failed"`
			Layer            string   `json:"layer"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("marshalled JSON does not parse: %v\n%s", err, b)
	}
	if out.Anchor != "Target" {
		t.Errorf("the inner answer's own fields were lost: %s", b)
	}
	if out.Workspace == nil {
		t.Fatalf("no workspace sibling: %s", b)
	}
	if !reflect.DeepEqual(out.Workspace.MembersConsulted, []string{"api", "shared"}) ||
		!reflect.DeepEqual(out.Workspace.MembersStale, []string{"web"}) ||
		out.Workspace.Boundary != Boundary {
		t.Errorf("workspace sibling is wrong: %s", b)
	}
	if out.Workspace.FreshenFailed != "" {
		t.Errorf("freshen_failed must be omitempty on the clean path: %s", b)
	}
	if out.Workspace.Layer != "" {
		t.Errorf("layer must not be serialized — D6 reserves a three-field shape: %s", b)
	}
	if !strings.HasPrefix(string(b), `{"anchor":"Target"`) {
		t.Errorf("the inner answer's key order was not preserved: %s", b)
	}
	// json.MarshalIndent must also work — this is how the CLI emits answers.
	if _, err := json.MarshalIndent(WithClause(inner, sampleClause()), "", "  "); err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
}

func TestJSONSiblingOnAnEmptyInnerObject(t *testing.T) {
	b, err := json.Marshal(WithClause(emptyStub{}, sampleClause()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), `{"workspace":`) {
		t.Errorf("empty inner object mis-spliced: %s", b)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// assertDegradeReason keys the assertion on the REASON the clause discloses,
// per the capability-skips-assert-the-reason learning: an unrecognized reason
// FAILS. It never skips, and it never falls through — a degradation nobody
// classified is exactly the silent staleness the D7 gate hard-fails.
func assertDegradeReason(t *testing.T, c Clause, want string) {
	t.Helper()
	var got string
	switch {
	case c.FreshenFailed != "":
		got = "freshen_failed"
	case len(c.MembersStale) > 0:
		got = "members_stale"
	case len(c.MembersStale) == 0 && c.FreshenFailed == "":
		got = "clean"
	default:
		t.Fatalf("unrecognized degradation reason in clause %+v — a reason this test cannot name must fail, never pass", c)
	}
	if got != want {
		t.Errorf("degradation reason = %q, want %q (clause: %+v)", got, want, c)
	}
}

func stubFreshen(t *testing.T, fn func(string) (wsfresh.Report, error)) func() {
	t.Helper()
	prev := freshenWorkspace
	freshenWorkspace = fn
	return func() { freshenWorkspace = prev }
}

func writeManifest(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.WorkspaceFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// stubAnswer stands in for one of the eight clause-less answer types: it has
// its own JSON keys in its own order and its own Text().
type stubAnswerType struct {
	Anchor string `json:"anchor"`
	Total  int    `json:"total"`

	text string
}

func stubAnswer(text string) *stubAnswerType {
	return &stubAnswerType{Anchor: "Target", Total: 2, text: text}
}

func (s *stubAnswerType) Text() string { return s.text }

type emptyStub struct{}

func (emptyStub) Text() string { return "" }
