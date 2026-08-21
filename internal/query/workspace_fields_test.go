package query

import (
	"encoding/json"
	"testing"
)

// Workspace-mode rendering of the additive `Repo` / `Inferred` fields.
//
// These answers are built by hand: internal/query never populates either field
// (no workspace logic lives here — the union layer in internal/wsquery does).
// The renderers only have to honour them, and repo mode — where `Repo` is ""
// and `Inferred` is false — must stay byte-identical, which the goldens in
// golden_test.go / query_test.go measure.

func TestRepoPrefixRendersOnEveryReferenceLine(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			"callers",
			(&CallersAnswer{
				Anchor:               "Run",
				Definitions:          []DefRef{{QName: "Run", File: "core/r.go", Line: 3, Signature: "func Run()", Repo: "core"}},
				CallersTotal:         1,
				Callers:              []CallerRef{{QName: "Use", File: "services/api/u.go", Line: 42, Repo: "api"}},
				ReferencedFilesTotal: 1,
				ReferencedFiles:      []string{"services/api/u.go"},
			}).Text(),
			`def  Run  core: core/r.go:3  func Run()
callers (1):
  api: services/api/u.go:42  Use
referenced in 1 file(s): services/api/u.go
`,
		},
		{
			"callees",
			(&CalleesAnswer{
				Anchor: "Use",
				Total:  2,
				Callees: []CalleeRef{
					{QName: "Run", CallLine: 42, DefFile: "core/r.go", DefLine: 3, Repo: "core"},
					{QName: "Ext", CallLine: 43, Repo: "api"},
				},
			}).Text(),
			// The unresolved row carries the prefix too. This test's own name
			// says EVERY reference line, and an unresolved callee — stdlib or
			// external, the common case — is the row where the attribution
			// matters most: on a bare anchor the union merges several members'
			// callees into ONE list, so without it nothing says whose code made
			// the call. Repo mode is unaffected (Repo == "" there).
			`callees of Use (2):
  Run  -> core: core/r.go:3  @call:42
  Ext  -> api: unresolved  @call:43
`,
		},
		{
			"find",
			(&FindAnswer{
				Query:   "Run",
				Total:   1,
				Results: []FindRef{{QName: "Run", Kind: "func", File: "core/r.go", Line: 3, Match: "exact", Repo: "core"}},
			}).Text(),
			`find "Run" (1):
  Run  func  core: core/r.go:3  [exact]
`,
		},
		{
			"grep",
			(&GrepAnswer{
				Pattern: "Run",
				RawHits: 1,
				Backend: "rg",
				Groups:  []GrepRef{{QName: "Use", File: "services/api/u.go", Line: 42, Hits: 1, Repo: "api"}},
			}).Text(),
			`grep "Run": 1 raw hits -> 1 symbols/sites (rg)
  Use  api: services/api/u.go:42  hits=1
`,
		},
		{
			"dependents",
			(&DependentsAnswer{
				Anchor:     "core",
				Total:      1,
				Dependents: []DependentRef{{Kind: "imports", QName: "services/api/u.go", File: "services/api/u.go", Line: 4, Repo: "api"}},
			}).Text(),
			`dependents of core (1):
  imports    api: services/api/u.go:4  services/api/u.go
`,
		},
		{
			"deps",
			(&DepsAnswer{
				Anchor: "Use",
				Sections: []DepSection{{Label: "calls", Total: 2, Deps: []DepRef{
					{Kind: "call", Target: "Run", DefFile: "core/r.go", DefLine: 3, Line: 42, Repo: "core"},
					// No resolved definition — the prefix still attributes the
					// row, exactly as in the callees case above.
					{Kind: "call", Target: "Ext", Line: 43, Repo: "api"},
				}}},
			}).Text(),
			`calls (2):
  call       Run (core: core/r.go:3)  @42
  call       api: Ext  @43
`,
		},
		{
			"enclosing",
			(&EnclosingAnswer{
				File: "services/api/u.go", Start: 40, End: 44,
				Symbols: []EnclosingRef{{Name: "Use", Kind: "func", File: "services/api/u.go", StartLine: 40, EndLine: 44, Callers: 2, ExternalCallers: 1, Repo: "api"}},
			}).Text(),
			`sym  Use  func  api: services/api/u.go:40-44  callers=2 external=1
`,
		},
		{
			"nav",
			(&NavAnswer{
				Anchor:       "Run",
				Definitions:  []DefRef{{QName: "Run", File: "core/r.go", Line: 3, Signature: "func Run()", Repo: "core"}},
				Matches:      []FindRef{{QName: "Run", Kind: "func", File: "core/r.go", Line: 3, Match: "exact", Repo: "core"}},
				CallersTotal: 1,
				Callers:      []CallerRef{{QName: "Use", File: "services/api/u.go", Line: 42, Repo: "api"}},
				FilesTotal:   1,
				Files:        []string{"services/api/u.go"},
			}).Text(),
			`nav Run: 1 definition(s), 1 caller(s), 1 referencing file(s)
def  Run  core: core/r.go:3  func Run()
matches:
  Run  func  core: core/r.go:3  [exact]
callers (1):
  api: services/api/u.go:42  Use
referenced in 1 file(s): services/api/u.go
`,
		},
		{
			"impact",
			(&ImpactAnswer{
				Anchor:          "Run",
				Coverage:        "call + import/extends/implements edges; type-usage references not included; workspace clause appended here",
				Definitions:     []DefRef{{QName: "Run", File: "core/r.go", Line: 3, Signature: "func Run()", Repo: "core"}},
				CallersTotal:    1,
				Callers:         []CallerRef{{QName: "Use", File: "services/api/u.go", Line: 42, Repo: "api"}},
				DependentsTotal: 1,
				Dependents:      []DependentRef{{Kind: "imports", QName: "services/api/u.go", File: "services/api/u.go", Line: 4, Repo: "api"}},
				CalleesTotal:    1,
				Callees:         []CalleeRef{{QName: "Helper", DefFile: "core/h.go", DefLine: 9, Repo: "core"}},
			}).Text(),
			`impact of Run: 1 definition(s), 1 caller(s), 1 callee(s), 1 dependent(s)
(coverage: call + import/extends/implements edges; type-usage references not included; workspace clause appended here)

def  Run  core: core/r.go:3  func Run()

callers — these break if Run's behavior/signature changes:
  api: services/api/u.go:42  Use

dependents — who imports/extends/implements Run:
  imports    api: services/api/u.go:4  services/api/u.go

callees — what Run depends on:
  Helper  core: core/h.go:9
`,
		},
	}
	for _, tc := range cases {
		if tc.text != tc.want {
			t.Errorf("%s text mismatch:\n--- got ---\n%s\n--- want ---\n%s", tc.name, tc.text, tc.want)
		}
	}
}

// The renderers key on the boolean only — they never see the overlay's
// "exact"/"inferred" vocabulary — and the tag sits where ambigTag sits.
func TestInferredTagRendersLikeAmbiguousTag(t *testing.T) {
	callers := (&CallersAnswer{
		Anchor:       "Run",
		Definitions:  []DefRef{{QName: "Run", File: "core/r.go", Line: 3, Signature: "func Run()"}},
		CallersTotal: 1,
		Callers:      []CallerRef{{QName: "Use", File: "services/api/u.go", Line: 42, Repo: "api", Inferred: true}},
	}).Text()
	want := `def  Run  core/r.go:3  func Run()
callers (1):
  api: services/api/u.go:42  Use  [inferred]
referenced in 0 file(s):
`
	if callers != want {
		t.Errorf("callers inferred:\n--- got ---\n%s\n--- want ---\n%s", callers, want)
	}

	callees := (&CalleesAnswer{
		Anchor:  "Use",
		Total:   1,
		Callees: []CalleeRef{{QName: "Run", CallLine: 42, DefFile: "core/r.go", DefLine: 3, Repo: "core", Inferred: true}},
	}).Text()
	wantCallees := `callees of Use (1):
  Run  -> core: core/r.go:3  @call:42  [inferred]
`
	if callees != wantCallees {
		t.Errorf("callees inferred:\n--- got ---\n%s\n--- want ---\n%s", callees, wantCallees)
	}
}

// omitempty is the whole non-regression mechanism: an unset Repo/Inferred must
// not appear in the marshaled object, and a set one must.
func TestRepoAndInferredAreOmittedWhenZero(t *testing.T) {
	zero, err := json.Marshal(CallerRef{QName: "Use", File: "u.go", Line: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(zero), `{"name":"","qname":"Use","file":"u.go","line":1}`; got != want {
		t.Errorf("zero CallerRef JSON:\n got %s\nwant %s", got, want)
	}
	set, err := json.Marshal(CallerRef{QName: "Use", File: "u.go", Line: 1, Repo: "api", Inferred: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(set), `{"name":"","qname":"Use","file":"u.go","line":1,"inferred":true,"repo":"api"}`; got != want {
		t.Errorf("set CallerRef JSON:\n got %s\nwant %s", got, want)
	}
	for name, v := range map[string]any{
		"DefRef":       DefRef{Repo: "api"},
		"CalleeRef":    CalleeRef{Repo: "api"},
		"DependentRef": DependentRef{Repo: "api"},
		"FindRef":      FindRef{Repo: "api"},
		"GrepRef":      GrepRef{Repo: "api"},
		"DepRef":       DepRef{Repo: "api"},
		"EnclosingRef": EnclosingRef{Repo: "api"},
	} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if !jsonHasRepo(string(b)) {
			t.Errorf("%s: expected a repo key in %s", name, b)
		}
	}
}

func jsonHasRepo(s string) bool {
	for i := 0; i+8 <= len(s); i++ {
		if s[i:i+8] == `"repo":"` {
			return true
		}
	}
	return false
}
