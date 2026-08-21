package wsquery

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"codeindex/internal/query"
)

// D7's single-member non-regression bar (§7.4), held VERBATIM:
//
//	a ONE-MEMBER workspace's answers match the SINGLE-REPO answers
//	MODULO THE `repo` FIELD.
//
// An earlier draft of the design added "and workspace-relative paths". That was
// a real relaxation of a frozen bar and is WITHDRAWN (assumption 20). It is not
// reintroduced here, and it does not need to be, because the fixture is built so
// the bar is LITERALLY SATISFIABLE:
//
//	the sole member's declared root is "."
//
// so the workspace root and the member root are the same directory, the
// workspace-relative and repo-relative forms of every path COINCIDE, and the
// only difference left between the two answers is the `repo` field — exactly as
// frozen. A fixture whose sole member sat in a subdirectory would differ in the
// paths too, would therefore not meet the frozen bar, and is not the fixture
// the bar runs on.
//
// This discharges D7's non-regression bar AND §3.5's deferred unit test.
//
// # What is compared, and what is deliberately not
//
// The comparison is over the ANSWER VALUE — the *query.XAnswer the two paths
// return — with every `repo` field cleared. The coverage clause is NOT part of
// that value: §4.5 puts it on a sibling JSON field and a trailing text line at
// the wsquery layer, and impact carries it appended to Coverage. Those are the
// workspace-mode disclosure surface, authorized by §4.5, not a change to the
// answer. Both are pinned separately below (see the impact case, which asserts
// the appended clause EXACTLY before normalizing it away, and the text case,
// which asserts byte-identity after removing only the clause line and the repo
// prefix).

const soloMemberID = "solo"

// soloFixture writes a repo, indexes it, and then declares a one-member
// workspace ON THE SAME DIRECTORY with root ".". The one directory is therefore
// both a valid repo root and a valid workspace root: engine.DetectRootKind sees
// the manifest and classifies it RootWorkspace, so wsquery routes to the union
// layer, while internal/query — which does not look at the manifest at all —
// answers it as an ordinary repo. That is what makes the two answers directly
// comparable rather than merely similar.
func soloFixture(t *testing.T) string {
	t.Helper()
	return soloFixtureFrom(t, map[string]string{
		"a.go": `package p

type Base struct{}

type Child struct{ Base }

func Target() {
	helper()
}

func helper() {}
`,
		"b.go": `package p

func CallerOne() {
	Target()
}

func CallerTwo() {
	Target()
	Target()
}
`,
	})
}

// soloFixtureFrom is soloFixture's body over an arbitrary file set, so a case
// that needs a DIFFERENT shape of repo (see the truncation case below) gets the
// same one-member/root-"." workspace without cloning the manifest.
func soloFixtureFrom(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := query.Fresh(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The sole member's root is "." — the whole point of the fixture.
	manifest := `{
  "version": 1,
  "members": [
    {"id": "solo", "root": ".", "namespaces": ["example.com/solo"]}
  ]
}
`
	if err := os.WriteFile(filepath.Join(dir, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// clearRepoFields walks an answer and blanks every string field named Repo,
// returning how many it cleared.
//
// It is written reflectively rather than as eight hand-written strippers on
// purpose: the bar is "modulo the repo field" over ALL of internal/query's
// reference structs, and a hand-written stripper silently stops covering a
// reference type that gains a Repo later. The returned count is what keeps the
// comparison honest — an answer that carried NO repo field would compare equal
// for the trivial reason that workspace mode never set one, so every case below
// requires the count to be positive.
func clearRepoFields(v any) int {
	return clearRepo(reflect.ValueOf(v))
}

func clearRepo(v reflect.Value) int {
	n := 0
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			n += clearRepo(v.Elem())
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			n += clearRepo(v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			fv := v.Field(i)
			if f.Name == "Repo" && fv.Kind() == reflect.String {
				if fv.String() != "" && fv.CanSet() {
					fv.SetString("")
					n++
				}
				continue
			}
			n += clearRepo(fv)
		}
	}
	return n
}

// soloCase is one verb's two answers plus the clause the workspace path
// produced for it.
type soloCase struct {
	verb string
	repo func(root string) (any, error)
	// ws takes the SUBTEST's *testing.T: it fails fatally on a malformed
	// workspace result, and a Fatal raised on the parent from inside a subtest
	// would abandon the wrong goroutine.
	ws func(t *testing.T, root string) (any, Clause)
	// text, when non-nil, is the pair of *Text renderings for the same query.
	text func(root string) (repoText, wsText string, err error)
}

// soloWS unwraps a *Structured result into the inner answer and its clause. A
// workspace root MUST produce the clause-carrying Answer; getting the bare
// inner value would mean the wrapper did not fire, and the bar would be
// comparing repo mode against itself.
func soloWS(t *testing.T, verb string, call func() (Emittable, error)) (any, Clause) {
	t.Helper()
	e, err := call()
	if err != nil {
		t.Fatalf("%s (workspace): %v", verb, err)
	}
	a, ok := e.(Answer)
	if !ok {
		t.Fatalf("%s on a one-member workspace root returned %T, not the clause-carrying Answer; "+
			"the comparison below would be repo mode against itself", verb, e)
	}
	return a.Inner, a.Clause
}

func soloCases() []soloCase {
	const limit = 10
	return []soloCase{
		{verb: "callers",
			repo: func(r string) (any, error) { return query.Callers(r, "Target", limit) },
			ws: func(t *testing.T, r string) (any, Clause) {
				return soloWS(t, "callers", func() (Emittable, error) { return CallersStructured(r, "Target", limit) })
			},
			text: func(r string) (string, string, error) {
				a, err := query.CallersText(r, "Target", limit)
				if err != nil {
					return "", "", err
				}
				b, err := CallersText(r, "Target", limit)
				return a, b, err
			}},
		{verb: "callees",
			repo: func(r string) (any, error) { return query.Callees(r, "Target", limit) },
			ws: func(t *testing.T, r string) (any, Clause) {
				return soloWS(t, "callees", func() (Emittable, error) { return CalleesStructured(r, "Target", limit) })
			},
			text: func(r string) (string, string, error) {
				a, err := query.CalleesText(r, "Target", limit)
				if err != nil {
					return "", "", err
				}
				b, err := CalleesText(r, "Target", limit)
				return a, b, err
			}},
		{verb: "nav",
			repo: func(r string) (any, error) { return query.Nav(r, "Target", limit) },
			ws: func(t *testing.T, r string) (any, Clause) {
				return soloWS(t, "nav", func() (Emittable, error) { return NavStructured(r, "Target", limit) })
			},
			text: func(r string) (string, string, error) {
				a, err := query.NavText(r, "Target", limit)
				if err != nil {
					return "", "", err
				}
				b, err := NavText(r, "Target", limit)
				return a, b, err
			}},
		{verb: "dependents",
			repo: func(r string) (any, error) { return query.Dependents(r, "Base", limit) },
			ws: func(t *testing.T, r string) (any, Clause) {
				return soloWS(t, "dependents", func() (Emittable, error) { return DependentsStructured(r, "Base", limit) })
			},
			text: func(r string) (string, string, error) {
				a, err := query.DependentsText(r, "Base", limit)
				if err != nil {
					return "", "", err
				}
				b, err := DependentsText(r, "Base", limit)
				return a, b, err
			}},
		{verb: "deps",
			repo: func(r string) (any, error) { return query.Deps(r, "Child", limit) },
			ws: func(t *testing.T, r string) (any, Clause) {
				return soloWS(t, "deps", func() (Emittable, error) { return DepsStructured(r, "Child", limit) })
			},
			text: func(r string) (string, string, error) {
				a, err := query.DepsText(r, "Child", limit)
				if err != nil {
					return "", "", err
				}
				b, err := DepsText(r, "Child", limit)
				return a, b, err
			}},
		{verb: "find",
			repo: func(r string) (any, error) { return query.Find(r, "Target", "", "", limit) },
			ws: func(t *testing.T, r string) (any, Clause) {
				return soloWS(t, "find", func() (Emittable, error) { return FindStructured(r, "Target", "", "", limit) })
			},
			text: func(r string) (string, string, error) {
				a, err := query.FindText(r, "Target", "", "", limit)
				if err != nil {
					return "", "", err
				}
				b, err := FindText(r, "Target", "", "", limit)
				return a, b, err
			}},
		{verb: "grep",
			repo: func(r string) (any, error) { return query.Grep(r, "Target", limit, false) },
			ws: func(t *testing.T, r string) (any, Clause) {
				return soloWS(t, "grep", func() (Emittable, error) { return GrepStructured(r, "Target", limit, false) })
			},
			text: func(r string) (string, string, error) {
				a, err := query.GrepText(r, "Target", limit, false)
				if err != nil {
					return "", "", err
				}
				b, err := GrepText(r, "Target", limit, false)
				return a, b, err
			}},
		{verb: "enclosing",
			repo: func(r string) (any, error) { return query.Enclosing(r, "a.go", 1, 20) },
			ws: func(t *testing.T, r string) (any, Clause) {
				return soloWS(t, "enclosing", func() (Emittable, error) { return EnclosingStructured(r, "a.go", 1, 20) })
			},
			// internal/query has no EnclosingText, so wsquery has none either
			// (§1) and there is no text pair to compare.
			text: nil},
	}
}

// TestSingleMemberWorkspaceMatchesSingleRepoModuloRepo is the frozen bar over
// eight of the nine answer types. impact is held separately below because its
// clause rides INSIDE the answer value (Coverage) rather than beside it, and
// normalizing that away silently inside this loop would hide the one place
// where a workspace-only string really does enter the answer.
func TestSingleMemberWorkspaceMatchesSingleRepoModuloRepo(t *testing.T) {
	defer cleanFreshen(t)()
	root := soloFixture(t)

	for _, c := range soloCases() {
		t.Run(c.verb, func(t *testing.T) {
			repoAnswer, err := c.repo(root)
			if err != nil {
				t.Fatalf("repo mode: %v", err)
			}
			wsAnswer, clause := c.ws(t, root)

			// The workspace answer must actually be a WORKSPACE answer, or the
			// bar is comparing repo mode with itself.
			if !reflect.DeepEqual(clause.MembersConsulted, []string{soloMemberID}) {
				t.Errorf("members_consulted = %v, want the sole member %q",
					clause.MembersConsulted, soloMemberID)
			}
			cleared := clearRepoFields(wsAnswer)
			if cleared == 0 {
				t.Fatalf("the workspace answer carried no repo field at all, so "+
					"\"modulo the repo field\" is vacuous here: %+v", wsAnswer)
			}
			// Repo mode must never have carried one — that half is what keeps
			// repo-mode JSON byte-identical (omitempty on an always-empty field).
			if n := clearRepoFields(repoAnswer); n != 0 {
				t.Errorf("the repo-mode answer carried %d repo field(s); repo mode must set none", n)
			}
			if !reflect.DeepEqual(repoAnswer, wsAnswer) {
				t.Errorf("one-member workspace answer differs from the single-repo answer by more "+
					"than the repo field — D7's frozen bar\n repo: %+v\n  ws:  %+v",
					repoAnswer, wsAnswer)
			}

			if c.text == nil {
				return
			}
			repoText, wsText, err := c.text(root)
			if err != nil {
				t.Fatalf("text: %v", err)
			}
			got := stripClauseLine(t, wsText, clause)
			got = strings.ReplaceAll(got, soloMemberID+": ", "")
			if got != repoText {
				t.Errorf("one-member workspace text differs from single-repo text by more than the "+
					"repo prefix\n got: %q\nwant: %q", got, repoText)
			}
		})
	}
}

// navTruncationFixture is a one-member workspace whose nav answer OVERFLOWS the
// limit on every lane the answer's *Total fields count:
//
//   - "Target" has 8 graph callers in 8 files, and grep attributes it in 9 files
//     (the 8 callers plus its own definition) — so both CallersTotal and
//     FilesTotal exceed a small limit on the ordinary lane;
//   - "Orphan" is DEFINED and never called, but named in a comment in 8 files —
//     zero call edges, so nav falls to grep attribution and CallersTotal is the
//     grep group count, which also exceeds the limit.
//
// The fixture exists because soloFixture cannot enter the truncation branch at
// all: two files and limit 10. A nav bug that only shows up once a *Total
// exceeds the limit — precisely the class where nav's totals are PRE-truncation
// counts while its lists are post-truncation — is invisible to a fixture that
// never truncates, so the frozen bar of §7.4 was being asserted only on the easy
// half of the arithmetic.
func navTruncationFixture(t *testing.T) string {
	t.Helper()
	files := map[string]string{
		"target.go": `package p

func Target() {}

func Orphan() {}
`,
	}
	for i := 0; i < 8; i++ {
		files[fmt.Sprintf("c%d.go", i)] = fmt.Sprintf(`package p

// Orphan is named here but never called.
func Caller%d() {
	Target()
}
`, i)
	}
	return soloFixtureFrom(t, files)
}

// TestSingleMemberWorkspaceNavTruncationMatchesSingleRepo holds §7.4's frozen
// bar for nav ACROSS THE TRUNCATION BOUNDARY, on both of nav's caller lanes.
//
// It is the regression test for a defect the main bar could not see: the
// workspace path recomputed FilesTotal/CallersTotal from the per-member lists it
// had just iterated, but those lists are what query.Nav already TRUNCATED to the
// limit, while the *Total fields it reports are the counts BEFORE truncation. A
// one-member workspace therefore reported FilesTotal == limit where repo mode
// reported the true count, and the renderer's "referenced in %d file(s)" header
// and its "... (+%d more)" tail both went wrong — the tail could never fire.
func TestSingleMemberWorkspaceNavTruncationMatchesSingleRepo(t *testing.T) {
	defer cleanFreshen(t)()
	root := navTruncationFixture(t)

	const limit = 3
	for _, anchor := range []string{"Target", "Orphan"} {
		t.Run(anchor, func(t *testing.T) {
			repoAnswer, err := query.Nav(root, anchor, limit)
			if err != nil {
				t.Fatalf("repo mode: %v", err)
			}
			// The fixture must actually overflow, or this test is the same
			// vacuous assertion the main bar already makes.
			if repoAnswer.FilesTotal <= limit {
				t.Fatalf("fixture does not truncate the file lane: FilesTotal=%d limit=%d",
					repoAnswer.FilesTotal, limit)
			}
			if repoAnswer.CallersTotal <= limit {
				t.Fatalf("fixture does not truncate the caller lane: CallersTotal=%d limit=%d",
					repoAnswer.CallersTotal, limit)
			}
			if anchor == "Orphan" && !repoAnswer.CallersFromGrep {
				t.Fatalf("the Orphan case is meant to exercise grep attribution, but repo mode " +
					"found call edges for it")
			}
			if anchor == "Target" && repoAnswer.CallersFromGrep {
				t.Fatalf("the Target case is meant to exercise the graph caller lane, but repo " +
					"mode fell back to grep attribution")
			}

			wsAnswer, clause := soloWS(t, "nav", func() (Emittable, error) {
				return NavStructured(root, anchor, limit)
			})
			if !reflect.DeepEqual(clause.MembersConsulted, []string{soloMemberID}) {
				t.Errorf("members_consulted = %v, want the sole member %q",
					clause.MembersConsulted, soloMemberID)
			}
			if clearRepoFields(wsAnswer) == 0 {
				t.Fatalf("the workspace answer carried no repo field; the bar is vacuous here")
			}
			if n := clearRepoFields(repoAnswer); n != 0 {
				t.Errorf("the repo-mode answer carried %d repo field(s); repo mode must set none", n)
			}
			if !reflect.DeepEqual(repoAnswer, wsAnswer) {
				t.Errorf("one-member workspace nav differs from single-repo nav across the "+
					"truncation boundary — D7's frozen bar\n repo: %+v\n  ws:  %+v",
					repoAnswer, wsAnswer)
			}

			repoText, err := query.NavText(root, anchor, limit)
			if err != nil {
				t.Fatal(err)
			}
			wsText, err := NavText(root, anchor, limit)
			if err != nil {
				t.Fatal(err)
			}
			got := stripClauseLine(t, wsText, clause)
			got = strings.ReplaceAll(got, soloMemberID+": ", "")
			if got != repoText {
				t.Errorf("one-member workspace nav text differs from single-repo text by more than "+
					"the repo prefix\n got: %q\nwant: %q", got, repoText)
			}
		})
	}
}

// TestSingleMemberWorkspaceImpactMatchesSingleRepoModuloRepo is the impact half
// of the bar. ImpactAnswer.Coverage is an existing string that workspace mode
// APPENDS the clause to (§4.5, assumption 8 — re-typing it to a struct would
// break repo-mode JSON). So the comparison asserts the appended form EXACTLY
// first, then restores the repo-mode sentence and holds the bar on everything
// else.
func TestSingleMemberWorkspaceImpactMatchesSingleRepoModuloRepo(t *testing.T) {
	defer cleanFreshen(t)()
	root := soloFixture(t)

	repoAnswer, err := query.Impact(root, "Target", 10)
	if err != nil {
		t.Fatal(err)
	}
	e, err := ImpactStructured(root, "Target", 10)
	if err != nil {
		t.Fatal(err)
	}
	boxed, ok := e.(Answer)
	if !ok {
		t.Fatalf("impact on a one-member workspace returned %T, not the clause-carrying Answer", e)
	}
	wsAnswer := boxed.Inner.(*query.ImpactAnswer)

	wantCoverage := repoAnswer.Coverage + "; " + boxed.Clause.String()
	if wsAnswer.Coverage != wantCoverage {
		t.Errorf("workspace Coverage = %q\n                want %q", wsAnswer.Coverage, wantCoverage)
	}
	wsAnswer.Coverage = repoAnswer.Coverage

	if n := clearRepoFields(wsAnswer); n == 0 {
		t.Fatalf("the workspace impact answer carried no repo field; the bar is vacuous")
	}
	if n := clearRepoFields(repoAnswer); n != 0 {
		t.Errorf("the repo-mode impact answer carried %d repo field(s); repo mode must set none", n)
	}
	if !reflect.DeepEqual(repoAnswer, wsAnswer) {
		t.Errorf("one-member workspace impact differs from single-repo impact by more than the "+
			"repo field and the §4.5 coverage clause\n repo: %+v\n  ws:  %+v", repoAnswer, wsAnswer)
	}

	// And the text surface: the clause is inside "(coverage: …)" here, so the
	// only other difference is the repo prefix.
	repoText, err := query.ImpactText(root, "Target", 10)
	if err != nil {
		t.Fatal(err)
	}
	wsText, err := ImpactText(root, "Target", 10)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Replace(wsText, "; "+boxed.Clause.String(), "", 1)
	got = strings.ReplaceAll(got, soloMemberID+": ", "")
	if got != repoText {
		t.Errorf("one-member workspace impact text differs by more than the repo prefix and the "+
			"clause\n got: %q\nwant: %q", got, repoText)
	}
}

// stripClauseLine removes the §4.5 trailing clause line from a workspace
// rendering, failing if it is not there: its ABSENCE would be a workspace
// answer that shipped no disclosure at all, which is a different bug from a
// path or prefix regression and must not be silently tolerated by a
// normalization step.
func stripClauseLine(t *testing.T, text string, c Clause) string {
	t.Helper()
	line := c.String() + "\n"
	if !strings.HasSuffix(text, line) {
		t.Fatalf("workspace text does not end with the coverage clause line\n text: %q\nclause: %q",
			text, line)
	}
	return strings.TrimSuffix(text, line)
}

// TestSingleMemberWorkspaceBarIsFalsifiable is the mutation evidence for the
// bar: the comparison is re-run against a DELIBERATELY WRONG expectation, and
// must not hold. Without it, a bar that compared an answer with itself — or one
// whose reflective repo-stripper walked nothing — would look just as green.
//
// The perturbation is the smallest one the bar is supposed to catch: a single
// reference row's file path moved. If the comparison still reports equality
// after that, it is not comparing anything.
func TestSingleMemberWorkspaceBarIsFalsifiable(t *testing.T) {
	defer cleanFreshen(t)()
	root := soloFixture(t)

	repoAnswer, err := query.Callers(root, "Target", 10)
	if err != nil {
		t.Fatal(err)
	}
	e, err := CallersStructured(root, "Target", 10)
	if err != nil {
		t.Fatal(err)
	}
	wsAnswer := e.(Answer).Inner.(*query.CallersAnswer)
	if clearRepoFields(wsAnswer) == 0 {
		t.Fatal("the workspace answer carried no repo field; the stripper walked nothing")
	}
	if !reflect.DeepEqual(repoAnswer, wsAnswer) {
		t.Fatalf("control arm: the bar does not hold before perturbation\n repo: %+v\n  ws: %+v",
			repoAnswer, wsAnswer)
	}
	if len(wsAnswer.Callers) == 0 {
		t.Fatal("fixture produced no caller rows; the perturbation below would be a no-op")
	}
	wsAnswer.Callers[0].File = "moved/" + wsAnswer.Callers[0].File
	if reflect.DeepEqual(repoAnswer, wsAnswer) {
		t.Error("the bar reports equality after a reference path was moved — it is not comparing " +
			"the reference rows at all")
	}
}
