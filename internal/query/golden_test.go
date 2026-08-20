package query

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Repo-mode goldens for all nine Text() renderers plus their `--json` shapes.
//
// The nine renderers in answers.go are shared between repo mode and workspace
// mode. Workspace mode adds a conditional `<member-id>: ` prefix and a coverage
// clause, so repo-mode identity is MEASURED here, not structural: these goldens
// are the non-regression bar. Any renderer byte that moves — text or JSON key —
// reddens one of these. The JSON pins are exact marshaled bytes because
// `omitempty` is the only thing keeping the key set stable once the additive
// `repo` / `inferred` fields land.
//
// Five renderers already had text goldens in query_test.go
// (Callers, Callees, Enclosing, Nav, Grep); this file adds the missing four
// (Find, Dependents, Deps, Impact) and pins JSON for all nine.

func goldenTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// multiPkgRepo carries imports (dependents), an embedded type (extends), and
// cross-package calls — the edges find/dependents/deps need.
func multiPkgRepo(t *testing.T) string {
	t.Helper()
	return goldenTree(t, map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.21\n",
		"core/core.go": `package core

// Base is embedded by consumers.
type Base struct{}

func Target() {}
`,
		"app/one.go": `package app

import (
	"fmt"

	"example.com/m/core"
)

type Square struct {
	core.Base
}

func UseOne() {
	fmt.Println("x")
	core.Target()
}
`,
		"app/two.go": `package app

import "example.com/m/core"

func UseTwo() {
	core.Target()
	core.Target()
}
`,
	})
}

// impactRepo gives Fan three callers and three callees (so a limit of 1
// exercises both `... (+%d more)` truncations) and Hub a dependent.
func impactRepo(t *testing.T) string {
	t.Helper()
	return goldenTree(t, map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.21\n",
		"core/core.go": `package core

type Hub struct{}

func One()   {}
func Two()   {}
func Three() {}

func Fan() {
	One()
	Two()
	Three()
}
`,
		"app/a.go": `package app

import "example.com/m/core"

type Child struct {
	core.Hub
}

func CallA() { core.Fan() }
func CallB() { core.Fan() }
func CallC() { core.Fan() }
`,
	})
}

// ambiguousRepo defines Run on two types, so calls to it resolve ambiguously
// and CallerRef.Ambiguous is set — the ambigTag path.
func ambiguousRepo(t *testing.T) string {
	t.Helper()
	return goldenTree(t, map[string]string{
		"a.go": `package p

type Alpha struct{}

func (a Alpha) Run() {}

type Beta struct{}

func (b Beta) Run() {}

func KickOne(x Alpha) {
	x.Run()
}

func KickTwo(y Beta) {
	y.Run()
	y.Run()
}
`,
	})
}

func checkText(t *testing.T, label, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s text drifted from the published contract\ngot:\n%s\nwant:\n%s", label, got, want)
	}
}

func checkJSON(t *testing.T, label string, v any, want string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != want {
		t.Errorf("%s json shape drifted\ngot:\n%s\nwant:\n%s", label, b, want)
	}
}

// ---------------------------------------------------------------- find

func TestFindTextAndJSONGolden(t *testing.T) {
	root := multiPkgRepo(t)
	a, err := Find(root, "Target", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "find", a.Text(), `find "Target" (1):
  Target  func  core/core.go:6  callers=3  [exact]
`)
	checkJSON(t, "find", a,
		`{"query":"Target","total":1,"results":[{"qname":"Target","kind":"func","file":"core/core.go","line":6,"callers":3,"match":"exact"}]}`)

	// FindText is the twin the CLI/MCP call; it must render identically.
	got, err := FindText(root, "Target", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "findtext", got, a.Text())
}

func TestFindNoMatchTextGolden(t *testing.T) {
	root := multiPkgRepo(t)

	// Single-token miss: route to grep.
	one, err := Find(root, "zzz", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "find single-token miss", one.Text(), `find "zzz" (0):
  (no symbol matches — try plain grep for content search)
`)
	checkJSON(t, "find single-token miss", one, `{"query":"zzz","total":0,"results":[]}`)

	// Multi-token miss: route to semantic search.
	many, err := Find(root, "how does login work", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "find concept miss", many.Text(), `find "how does login work" (0):
  (no symbol matches — this looks like a concept query; use `+"`search`"+` for feature/topic questions)
`)
}

// ---------------------------------------------------------------- dependents

func TestDependentsTextAndJSONGolden(t *testing.T) {
	root := multiPkgRepo(t)

	a, err := Dependents(root, "core", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "dependents", a.Text(), `dependents of core (2):
  imports    app/one.go:6  app/one.go
  imports    app/two.go:3  app/two.go
`)
	checkJSON(t, "dependents", a,
		`{"anchor":"core","dependents_total":2,"dependents":[{"kind":"imports","qname":"app/one.go","file":"app/one.go","line":6},{"kind":"imports","qname":"app/two.go","file":"app/two.go","line":3}]}`)

	got, err := DependentsText(root, "core", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "dependentstext", got, a.Text())

	// Truncation line.
	trunc, err := Dependents(root, "core", 1)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "dependents truncated", trunc.Text(), `dependents of core (2):
  imports    app/one.go:6  app/one.go
  ... (+1 more; raise limit)
`)

	// The extends kind, and the %-10s kind column against a shorter word.
	ext, err := Dependents(root, "Base", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "dependents extends", ext.Text(), `dependents of Base (1):
  extends    app/one.go:10  Square
`)
	checkJSON(t, "dependents extends", ext,
		`{"anchor":"Base","dependents_total":1,"dependents":[{"kind":"extends","qname":"Square","file":"app/one.go","line":10}]}`)

	// Empty answers marshal as [] for machine consumers, never null.
	none, err := Dependents(root, "NoSuchSymbol", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "dependents empty", none.Text(), "dependents of NoSuchSymbol (0):\n")
	checkJSON(t, "dependents empty", none,
		`{"anchor":"NoSuchSymbol","dependents_total":0,"dependents":[]}`)
}

// ---------------------------------------------------------------- deps

func TestDepsTextAndJSONGolden(t *testing.T) {
	root := multiPkgRepo(t)

	// Symbol anchor: two sections, one with a resolved def target.
	a, err := Deps(root, "Square", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "deps", a.Text(), `deps of Square (1):
  extends    Base (core/core.go:4)  @10
file imports (app/one.go) (2):
  imports    fmt  @4
  imports    example.com/m/core  @6
`)
	checkJSON(t, "deps", a,
		`{"anchor":"Square","sections":[{"label":"deps of Square","total":1,"deps":[{"kind":"extends","target":"Base","def_file":"core/core.go","def_line":4,"line":10}]},{"label":"file imports (app/one.go)","total":2,"deps":[{"kind":"imports","target":"fmt","line":4},{"kind":"imports","target":"example.com/m/core","line":6}]}]}`)

	got, err := DepsText(root, "Square", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "depstext", got, a.Text())

	// Per-section truncation.
	trunc, err := Deps(root, "Square", 1)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "deps truncated", trunc.Text(), `deps of Square (1):
  extends    Base (core/core.go:4)  @10
file imports (app/one.go) (2):
  imports    fmt  @4
  ... (+1 more; raise limit)
`)

	// File anchor: a single imports section, unresolved target (no def_file).
	f, err := Deps(root, "app/two.go", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "deps file anchor", f.Text(), `imports of app/two.go (1):
  imports    example.com/m/core  @3
`)
	checkJSON(t, "deps file anchor", f,
		`{"anchor":"app/two.go","sections":[{"label":"imports of app/two.go","total":1,"deps":[{"kind":"imports","target":"example.com/m/core","line":3}]}]}`)
}

// ---------------------------------------------------------------- impact

func TestImpactTextAndJSONGolden(t *testing.T) {
	root := impactRepo(t)

	a, err := Impact(root, "Fan", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "impact", a.Text(), `impact of Fan: 1 definition(s), 3 caller(s), 3 callee(s), 0 dependent(s)
(coverage: call + import/extends/implements edges; type-usage references not included)

def  Fan  core/core.go:9  func Fan()

callers — these break if Fan's behavior/signature changes:
  app/a.go:9  CallA
  app/a.go:10  CallB
  app/a.go:11  CallC

dependents — who imports/extends/implements Fan:

callees — what Fan depends on:
  One  core/core.go:5
  Two  core/core.go:6
  Three  core/core.go:7
`)
	checkJSON(t, "impact", a,
		`{"anchor":"Fan","coverage":"call + import/extends/implements edges; type-usage references not included","definitions":[{"name":"Fan","qname":"Fan","kind":"func","file":"core/core.go","line":9,"signature":"func Fan()"}],"callers_total":3,"callers":[{"name":"CallA","qname":"CallA","file":"app/a.go","line":9},{"name":"CallB","qname":"CallB","file":"app/a.go","line":10},{"name":"CallC","qname":"CallC","file":"app/a.go","line":11}],"dependents_total":0,"dependents":[],"callees_total":3,"callees":[{"name":"One","qname":"One","call_line":10,"def_file":"core/core.go","def_line":5},{"name":"Two","qname":"Two","call_line":11,"def_file":"core/core.go","def_line":6},{"name":"Three","qname":"Three","call_line":12,"def_file":"core/core.go","def_line":7}]}`)

	got, err := ImpactText(root, "Fan", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "impacttext", got, a.Text())
}

// TestImpactTruncationTextGolden pins the `  ... (+%d more)\n` arithmetic in
// both the callers and the callees blocks. §3.5 changes what that arithmetic
// counts in workspace mode; the repo-mode string must not move.
func TestImpactTruncationTextGolden(t *testing.T) {
	root := impactRepo(t)
	a, err := Impact(root, "Fan", 1)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "impact truncated", a.Text(), `impact of Fan: 1 definition(s), 3 caller(s), 3 callee(s), 0 dependent(s)
(coverage: call + import/extends/implements edges; type-usage references not included)

def  Fan  core/core.go:9  func Fan()

callers — these break if Fan's behavior/signature changes:
  app/a.go:9  CallA
  ... (+2 more)

dependents — who imports/extends/implements Fan:

callees — what Fan depends on:
  One  core/core.go:5
  ... (+2 more)
`)
	checkJSON(t, "impact truncated", a,
		`{"anchor":"Fan","coverage":"call + import/extends/implements edges; type-usage references not included","definitions":[{"name":"Fan","qname":"Fan","kind":"func","file":"core/core.go","line":9,"signature":"func Fan()"}],"callers_total":3,"callers":[{"name":"CallA","qname":"CallA","file":"app/a.go","line":9}],"dependents_total":0,"dependents":[],"callees_total":3,"callees":[{"name":"One","qname":"One","call_line":10,"def_file":"core/core.go","def_line":5}]}`)
}

// TestImpactDependentsBlockTextGolden pins the non-empty dependents block,
// which the Fan fixture leaves empty.
func TestImpactDependentsBlockTextGolden(t *testing.T) {
	root := impactRepo(t)
	a, err := Impact(root, "Hub", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "impact dependents", a.Text(), `impact of Hub: 1 definition(s), 1 caller(s), 0 callee(s), 1 dependent(s)
(coverage: call + import/extends/implements edges; type-usage references not included)

def  Hub  core/core.go:3  Hub struct{}

callers — these break if Hub's behavior/signature changes:
  app/a.go:6  Child

dependents — who imports/extends/implements Hub:
  extends    app/a.go:6  Child

callees — what Hub depends on:
`)
	checkJSON(t, "impact dependents", a,
		`{"anchor":"Hub","coverage":"call + import/extends/implements edges; type-usage references not included","definitions":[{"name":"Hub","qname":"Hub","kind":"type","file":"core/core.go","line":3,"signature":"Hub struct{}"}],"callers_total":1,"callers":[{"name":"Child","qname":"Child","file":"app/a.go","line":6}],"dependents_total":1,"dependents":[{"kind":"extends","qname":"Child","file":"app/a.go","line":6}],"callees_total":0,"callees":[]}`)
}

// ---------------------------------------------------------------- callers

// TestCallersAmbiguousTruncatedGolden pins the two Callers paths the plain
// golden misses: the `  ... (+%d more; raise limit)\n` line and ambigTag.
func TestCallersAmbiguousTruncatedGolden(t *testing.T) {
	root := ambiguousRepo(t)
	a, err := Callers(root, "Run", 1)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "callers ambiguous+truncated", a.Text(), `def  Alpha.Run  a.go:5  func (a Alpha) Run()
def  Beta.Run  a.go:9  func (b Beta) Run()
callers (3):
  a.go:12  KickOne  [ambiguous]
  ... (+2 more; raise limit)
referenced in 1 file(s): a.go
`)
	checkJSON(t, "callers ambiguous+truncated", a,
		`{"anchor":"Run","definitions":[{"name":"Run","parent":"Alpha","qname":"Alpha.Run","kind":"method","file":"a.go","line":5,"signature":"func (a Alpha) Run()"},{"name":"Run","parent":"Beta","qname":"Beta.Run","kind":"method","file":"a.go","line":9,"signature":"func (b Beta) Run()"}],"callers_total":3,"callers":[{"name":"KickOne","qname":"KickOne","file":"a.go","line":12,"ambiguous":true}],"referenced_files_total":1,"referenced_files":["a.go"]}`)
}

func TestCallersJSONGolden(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Callers(root, "Target", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkJSON(t, "callers", a,
		`{"anchor":"Target","definitions":[{"name":"Target","qname":"Target","kind":"func","file":"a.go","line":3,"signature":"func Target()"}],"callers_total":3,"callers":[{"name":"CallerOne","qname":"CallerOne","file":"a.go","line":6},{"name":"CallerTwo","qname":"CallerTwo","file":"b.go","line":4},{"name":"CallerTwo","qname":"CallerTwo","file":"b.go","line":5}],"referenced_files_total":2,"referenced_files":["a.go","b.go"]}`)
}

// ---------------------------------------------------------------- callees

func TestCalleesJSONGolden(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Callees(root, "CallerTwo", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkJSON(t, "callees", a,
		`{"anchor":"CallerTwo","callees_total":2,"callees":[{"name":"Target","qname":"Target","call_line":4,"def_file":"a.go","def_line":3},{"name":"Target","qname":"Target","call_line":5,"def_file":"a.go","def_line":3}]}`)
}

func TestCalleesAmbiguousTruncatedGolden(t *testing.T) {
	root := ambiguousRepo(t)
	a, err := Callees(root, "KickTwo", 1)
	if err != nil {
		t.Fatal(err)
	}
	checkText(t, "callees ambiguous+truncated", a.Text(), `callees of KickTwo (2):
  Alpha.Run  -> a.go:5  @call:16  [ambiguous]
  ... (+1 more; raise limit)
`)
	checkJSON(t, "callees ambiguous+truncated", a,
		`{"anchor":"KickTwo","callees_total":2,"callees":[{"name":"Run","parent":"Alpha","qname":"Alpha.Run","call_line":16,"def_file":"a.go","def_line":5,"ambiguous":true}]}`)
}

// ---------------------------------------------------------------- nav

func TestNavJSONGolden(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Nav(root, "Target", 50)
	if err != nil {
		t.Fatal(err)
	}
	checkJSON(t, "nav", a,
		`{"anchor":"Target","definitions":[{"name":"Target","qname":"Target","kind":"func","file":"a.go","line":3,"signature":"func Target()"}],"matches":[{"qname":"Target","kind":"func","file":"a.go","line":3,"callers":3,"match":"exact"}],"callers_total":3,"callers":[{"name":"CallerOne","qname":"CallerOne","file":"a.go","line":6},{"name":"CallerTwo","qname":"CallerTwo","file":"b.go","line":4},{"name":"CallerTwo","qname":"CallerTwo","file":"b.go","line":5}],"files_total":2,"files":["a.go","b.go"]}`)
}

// ---------------------------------------------------------------- grep

// The grep backend is environment-dependent (ripgrep when on PATH, else the
// internal scan), so it is interpolated rather than pinned — every other byte
// of the text and the whole JSON key set is pinned.
func TestGrepTextAndJSONGolden(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Grep(root, "Target", 30, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Backend == "" {
		t.Fatal("grep backend must never be blank — Text() prints it verbatim")
	}
	checkText(t, "grep", a.Text(), fmt.Sprintf(`grep "Target": 4 raw hits -> 3 symbols/sites (%s)
  Target  a.go:3  hits=1  [definition]
  CallerTwo  b.go:4  hits=2
  CallerOne  a.go:6  hits=1
`, a.Backend))
	checkJSON(t, "grep", a, fmt.Sprintf(
		`{"pattern":"Target","raw_hits":4,"backend":%q,"groups":[{"qname":"Target","file":"a.go","line":3,"hits":1,"is_definition":true},{"qname":"CallerTwo","file":"b.go","line":4,"hits":2},{"qname":"CallerOne","file":"a.go","line":6,"hits":1}]}`,
		a.Backend))

	// word=true is the only thing that emits the omitempty "word" key.
	w, err := Grep(root, "Target", 30, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"pattern":"Target","word":true,`; len(b) < len(want) || string(b[:len(want)]) != want {
		t.Errorf("grep word json prefix drifted: %s", b)
	}
}

// ---------------------------------------------------------------- enclosing

func TestEnclosingJSONGolden(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Enclosing(root, "a.go", 5, 6)
	if err != nil {
		t.Fatal(err)
	}
	checkJSON(t, "enclosing", a,
		`{"file":"a.go","start":5,"end":6,"symbols":[{"name":"CallerOne","kind":"func","file":"a.go","start_line":5,"end_line":7,"callers":0,"external_callers":0}]}`)
}
