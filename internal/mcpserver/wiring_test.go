package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"codeindex/internal/query"
	"codeindex/internal/wsquery"
)

// callText calls one tool and returns the single TextContent it produced.
func callText(t *testing.T, sess *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("call %s returned an error result: %+v", name, res.Content)
	}
	return res.Content[0].(*mcp.TextContent).Text
}

// TestRepoModeToolTextIsByteIdentical is the non-regression bar for this
// wiring, measured rather than assumed.
//
// Before this slice each handler called query.<Verb>Text; after it each calls
// wsquery.<Verb>Text, whose RootRepo branch is a verbatim tail-call into the
// same query function. So "byte-identical to before" is exactly "byte-identical
// to internal/query's own answer with the handler's default limits", and that
// is what is asserted here — for every one of the eight tools, including the
// default-limit arithmetic each handler applies.
func TestRepoModeToolTextIsByteIdentical(t *testing.T) {
	repo := fixtureRepo(t)
	if _, err := query.Fresh(repo); err != nil {
		t.Fatal(err)
	}
	// Drain the cold-build disclosure so the one-shot banner text() prepends
	// cannot land on whichever tool happens to be called first.
	query.ConsumeColdBuild()

	sess := connect(t, repo)

	cases := []struct {
		tool string
		args map[string]any
		want func() (string, error)
	}{
		{"impact", map[string]any{"symbol": "Helper"},
			func() (string, error) { return query.ImpactText(repo, "Helper", 50) }},
		{"nav", map[string]any{"symbol": "Helper"},
			func() (string, error) { return query.NavText(repo, "Helper", 50) }},
		{"callers", map[string]any{"symbol": "Helper"},
			func() (string, error) { return query.CallersText(repo, "Helper", 50) }},
		{"callees", map[string]any{"symbol": "A"},
			func() (string, error) { return query.CalleesText(repo, "A", 50) }},
		{"dependents", map[string]any{"symbol": "Helper"},
			func() (string, error) { return query.DependentsText(repo, "Helper", 50) }},
		{"find", map[string]any{"query": "Helper"},
			func() (string, error) { return query.FindText(repo, "Helper", "", "", 20) }},
		{"grep", map[string]any{"pattern": "Helper"},
			func() (string, error) { return query.GrepText(repo, "Helper", 30, false) }},
		{"search", map[string]any{"query": "helper increment function", "limit": 5},
			func() (string, error) {
				return query.SearchText(repo, "helper increment function", nil, "", 5, false)
			}},
		// Non-default limits exercise the limitOr / limitOr20 branches too.
		{"callers", map[string]any{"symbol": "Helper", "limit": 1},
			func() (string, error) { return query.CallersText(repo, "Helper", 1) }},
		{"find", map[string]any{"query": "Helper", "kind": "func", "path": "a.go", "limit": 3},
			func() (string, error) { return query.FindText(repo, "Helper", "func", "a.go", 3) }},
		{"grep", map[string]any{"pattern": "Helper", "word": true, "limit": 2},
			func() (string, error) { return query.GrepText(repo, "Helper", 2, true) }},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			want, err := tc.want()
			if err != nil {
				t.Fatalf("internal/query reference answer: %v", err)
			}
			if strings.TrimSpace(want) == "" {
				t.Fatalf("reference answer for %s is empty — the comparison would be vacuous", tc.tool)
			}
			got := callText(t, sess, tc.tool, tc.args)
			if got != want {
				t.Errorf("%s text is not byte-identical to internal/query's.\n--- mcp ---\n%q\n--- query ---\n%q",
					tc.tool, got, want)
			}
		})
	}
}

// wsFixture writes a two-member workspace whose members are real, buildable Go
// repos, so the tools below run the genuine workspace path end to end
// (manifest load, whole-workspace freshen, union answer) rather than a stub.
func wsFixture(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "members": [
    {"id": "web", "root": "services/web", "namespaces": ["App\\Web"]},
    {"id": "api", "root": "services/api", "namespaces": ["App\\Api"]}
  ]
}
`
	if err := os.WriteFile(filepath.Join(ws, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"services/web/a.go": "package web\n\nfunc Helper() {}\n\nfunc WebOne() {\n\tHelper()\n}\n",
		"services/api/a.go": "package api\n\nfunc Helper() {}\n\nfunc ApiOne() {\n\tHelper()\n}\n",
	}
	for rel, content := range files {
		p := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Build each member's index up front: the fan-out reads a member only when
	// its store opens, so an unbuilt fixture would make the assertions vacuous.
	for _, id := range []string{"web", "api"} {
		if _, err := query.Fresh(filepath.Join(ws, "services", id)); err != nil {
			t.Fatalf("building member %s: %v", id, err)
		}
	}
	return ws
}

// TestWorkspaceModeToolTextCarriesMemberPrefix is the other half of §5: on a
// workspace root the tools answer (they do not fail), and the member id arrives
// INSIDE the text they already return — which is how the frozen `repo` field is
// discharged for tools that have no structured result to carry it.
func TestWorkspaceModeToolTextCarriesMemberPrefix(t *testing.T) {
	ws := wsFixture(t)
	sess := connect(t, ws)

	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"find", map[string]any{"query": "Helper"}},
		{"grep", map[string]any{"pattern": "Helper"}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			out := callText(t, sess, tc.tool, tc.args)
			for _, id := range []string{"web: ", "api: "} {
				if !strings.Contains(out, id) {
					t.Errorf("%s output carries no %q member prefix:\n%s", tc.tool, id, out)
				}
			}
		})
	}
}

// TestSearchToolRefusesAWorkspaceRootThroughWsquery pins that the handler does
// NOT test root kind itself: the refusal is wsquery.SearchText's RootWorkspace
// branch (the frozen non-goal), surfaced verbatim through the handler's error
// wrap.
func TestSearchToolRefusesAWorkspaceRootThroughWsquery(t *testing.T) {
	ws := wsFixture(t)
	want := wsquery.RefuseWorkspaceRoot("search", ws)
	if want == nil {
		t.Fatal("RefuseWorkspaceRoot returned nil for a workspace root")
	}

	sess := connect(t, ws)
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search", Arguments: map[string]any{"query": "anything"},
	})
	var got string
	switch {
	case err != nil:
		got = err.Error()
	case res.IsError:
		got = res.Content[0].(*mcp.TextContent).Text
	default:
		t.Fatalf("search answered on a workspace root instead of refusing:\n%s",
			res.Content[0].(*mcp.TextContent).Text)
	}
	if !strings.Contains(got, "frozen non-goal") {
		t.Errorf("refusal does not cite the frozen non-goal (so it is not wsquery's):\n%s", got)
	}
	if !strings.Contains(got, "members: web, api") {
		t.Errorf("refusal does not list the members:\n%s", got)
	}
}
