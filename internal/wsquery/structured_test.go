package wsquery

import (
	"encoding/json"
	"testing"

	"codeindex/internal/query"
)

// reservedClauseKeys are D6's three reserved fields. A --json consumer is
// promised this shape in workspace mode; anything less and §4.5's "consumers
// get the reserved shape" is not delivered.
var reservedClauseKeys = []string{"members_consulted", "members_stale", "boundary"}

// structuredCall names one verb's structured entry point so the table below
// can hold every one of the nine to the same bar. A verb missing from this
// table is a verb whose --json consumers silently lost the clause, which is
// the failure Part B exists to close.
type structuredCall struct {
	verb string
	repo func(root string) (Emittable, error)
	ws   func(root string) (Emittable, error)
}

func structuredCalls() []structuredCall {
	const anchor = "Target"
	return []structuredCall{
		{"callers",
			func(r string) (Emittable, error) { return CallersStructured(r, anchor, 50) },
			func(r string) (Emittable, error) { return CallersStructured(r, wsMemberLib+":"+anchor, 50) }},
		{"callees",
			func(r string) (Emittable, error) { return CalleesStructured(r, anchor, 50) },
			func(r string) (Emittable, error) { return CalleesStructured(r, wsMemberLib+":"+anchor, 50) }},
		{"impact",
			func(r string) (Emittable, error) { return ImpactStructured(r, anchor, 50) },
			func(r string) (Emittable, error) { return ImpactStructured(r, wsMemberLib+":"+anchor, 50) }},
		{"nav",
			func(r string) (Emittable, error) { return NavStructured(r, anchor, 50) },
			func(r string) (Emittable, error) { return NavStructured(r, wsMemberLib+":"+anchor, 50) }},
		{"dependents",
			func(r string) (Emittable, error) { return DependentsStructured(r, anchor, 50) },
			func(r string) (Emittable, error) { return DependentsStructured(r, wsMemberLib+":"+anchor, 50) }},
		{"deps",
			func(r string) (Emittable, error) { return DepsStructured(r, anchor, 50) },
			func(r string) (Emittable, error) { return DepsStructured(r, wsMemberLib+":"+anchor, 50) }},
		{"find",
			func(r string) (Emittable, error) { return FindStructured(r, anchor, "", "", 20) },
			func(r string) (Emittable, error) { return FindStructured(r, anchor, "", "", 20) }},
		{"grep",
			func(r string) (Emittable, error) { return GrepStructured(r, anchor, 30, false) },
			func(r string) (Emittable, error) { return GrepStructured(r, anchor, 30, false) }},
		{"enclosing",
			func(r string) (Emittable, error) { return EnclosingStructured(r, "a.go", 1, 5) },
			func(r string) (Emittable, error) { return EnclosingStructured(r, "services/lib/target.go", 1, 5) }},
	}
}

// §4.5: in WORKSPACE mode the --json shape carries the reserved
// `workspace: {members_consulted, members_stale, boundary}` sibling. Before
// this wiring the CLI handed internal/query's concrete answers straight to the
// emitter, so the clause reached the text surface and never reached --json.
func TestStructuredWorkspaceJSONCarriesTheReservedClause(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)

	for _, c := range structuredCalls() {
		t.Run(c.verb, func(t *testing.T) {
			a, err := c.ws(ws)
			if err != nil {
				t.Fatalf("%s: %v", c.verb, err)
			}
			b, err := json.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("%s: %v: %s", c.verb, err, b)
			}
			raw, ok := got["workspace"]
			if !ok {
				t.Fatalf("%s: workspace-mode --json has no workspace clause: %s", c.verb, b)
			}
			var clause map[string]any
			if err := json.Unmarshal(raw, &clause); err != nil {
				t.Fatal(err)
			}
			for _, key := range reservedClauseKeys {
				if _, ok := clause[key]; !ok {
					t.Errorf("%s: clause missing reserved field %q: %s", c.verb, key, raw)
				}
			}
			if b, _ := clause["boundary"].(string); b != Boundary {
				t.Errorf("%s: boundary is %q, want the fixed sentence %q", c.verb, b, Boundary)
			}
			consulted, _ := clause["members_consulted"].([]any)
			if len(consulted) == 0 {
				t.Errorf("%s: members_consulted is empty; the answer read at least one member: %s", c.verb, raw)
			}
		})
	}
}

// THE BAR: repo-mode --json is BYTE-IDENTICAL through the structured wrapper.
// The wrapper must return the inner answer itself in repo mode — an Answer
// with an empty clause would still marshal a "workspace" key and move every
// repo-mode golden Task 1 pinned. If this fails, the wrapper is firing in repo
// mode; that is the bug, never the golden.
func TestStructuredRepoModeJSONIsByteIdenticalToTheBareAnswer(t *testing.T) {
	repo := fixtureRepo(t)

	bare := map[string]func(string) (any, error){
		"callers":    func(r string) (any, error) { return query.Callers(r, "Target", 50) },
		"callees":    func(r string) (any, error) { return query.Callees(r, "Target", 50) },
		"impact":     func(r string) (any, error) { return query.Impact(r, "Target", 50) },
		"nav":        func(r string) (any, error) { return query.Nav(r, "Target", 50) },
		"dependents": func(r string) (any, error) { return query.Dependents(r, "Target", 50) },
		"deps":       func(r string) (any, error) { return query.Deps(r, "Target", 50) },
		"find":       func(r string) (any, error) { return query.Find(r, "Target", "", "", 20) },
		"grep":       func(r string) (any, error) { return query.Grep(r, "Target", 30, false) },
		"enclosing":  func(r string) (any, error) { return query.Enclosing(r, "a.go", 1, 5) },
	}

	for _, c := range structuredCalls() {
		t.Run(c.verb, func(t *testing.T) {
			want, err := bare[c.verb](repo)
			if err != nil {
				t.Fatalf("bare %s: %v", c.verb, err)
			}
			wantJSON, err := json.MarshalIndent(want, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.repo(repo)
			if err != nil {
				t.Fatalf("structured %s: %v", c.verb, err)
			}
			gotJSON, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("%s: repo-mode --json moved through the structured wrapper\n got: %s\nwant: %s",
					c.verb, gotJSON, wantJSON)
			}
			if _, ok := got.(Answer); ok {
				t.Errorf("%s: repo mode returned a workspace Answer; it must return the inner answer", c.verb)
			}
			if got.Text() != want.(interface{ Text() string }).Text() {
				t.Errorf("%s: repo-mode text moved through the structured wrapper", c.verb)
			}
		})
	}
}
