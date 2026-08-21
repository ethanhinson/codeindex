package wsquery

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"codeindex/internal/engine"
	"codeindex/internal/query"
)

// fixtureRepo mirrors internal/query's own fixture so the identity assertions
// below run against the same shapes the query goldens are pinned on.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a.go": `package p

func Target() {}

func CallerOne() {
	Target()
}
`,
		"b.go": `package p

func CallerTwo() {
	Target()
	Target()
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fixtureWorkspace writes a three-member manifest. The member roots exist so
// the fixture is realistic, but RefuseWorkspaceRoot never stats them.
func fixtureWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "members": [
    {"id": "api", "root": "services/api", "namespaces": ["App\\Api"]},
    {"id": "shared", "root": "services/shared", "namespaces": ["App\\Shared"]},
    {"id": "web", "root": "services/web", "namespaces": ["App\\Web"]}
  ]
}
`
	if err := os.WriteFile(filepath.Join(dir, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRootKindDetectsBothShapes(t *testing.T) {
	repo := fixtureRepo(t)
	kind, err := RootKind(repo)
	if err != nil {
		t.Fatal(err)
	}
	if kind != engine.RootRepo {
		t.Errorf("repo root classified as %v, want RootRepo", kind)
	}

	ws := fixtureWorkspace(t)
	kind, err = RootKind(ws)
	if err != nil {
		t.Fatal(err)
	}
	if kind != engine.RootWorkspace {
		t.Errorf("workspace root classified as %v, want RootWorkspace", kind)
	}
}

// TestRepoModeIdentity is the non-regression bar in executable form: on a repo
// root every wsquery entry point must produce exactly what the corresponding
// internal/query entry point produces, because the RootRepo branch is a
// verbatim tail-call.
func TestRepoModeIdentity(t *testing.T) {
	root := fixtureRepo(t)
	// Build the index once up front so no assertion below races the cold
	// build or reads a half-built store.
	if _, err := query.Fresh(root); err != nil {
		t.Fatal(err)
	}

	answers := []struct {
		name string
		got  func() (any, error)
		want func() (any, error)
	}{
		{"Callers", func() (any, error) { return Callers(root, "Target", 50) }, func() (any, error) { return query.Callers(root, "Target", 50) }},
		{"Callees", func() (any, error) { return Callees(root, "CallerOne", 50) }, func() (any, error) { return query.Callees(root, "CallerOne", 50) }},
		{"Impact", func() (any, error) { return Impact(root, "Target", 50) }, func() (any, error) { return query.Impact(root, "Target", 50) }},
		{"Nav", func() (any, error) { return Nav(root, "Target", 50) }, func() (any, error) { return query.Nav(root, "Target", 50) }},
		{"Find", func() (any, error) { return Find(root, "Target", "", "", 50) }, func() (any, error) { return query.Find(root, "Target", "", "", 50) }},
		{"Grep", func() (any, error) { return Grep(root, "Target", 50, true) }, func() (any, error) { return query.Grep(root, "Target", 50, true) }},
		{"Dependents", func() (any, error) { return Dependents(root, "Target", 50) }, func() (any, error) { return query.Dependents(root, "Target", 50) }},
		{"Deps", func() (any, error) { return Deps(root, "CallerOne", 50) }, func() (any, error) { return query.Deps(root, "CallerOne", 50) }},
		{"Enclosing", func() (any, error) { return Enclosing(root, "a.go", 5, 7) }, func() (any, error) { return query.Enclosing(root, "a.go", 5, 7) }},
	}
	for _, tc := range answers {
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := tc.got()
			want, wantErr := tc.want()
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("error disagreement: wsquery err=%v, query err=%v", gotErr, wantErr)
			}
			if gotErr != nil {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("wsquery.%s answer differs from query.%s\n got: %+v\nwant: %+v", tc.name, tc.name, got, want)
			}
		})
	}

	texts := []struct {
		name string
		got  func() (string, error)
		want func() (string, error)
	}{
		{"CallersText", func() (string, error) { return CallersText(root, "Target", 50) }, func() (string, error) { return query.CallersText(root, "Target", 50) }},
		{"CalleesText", func() (string, error) { return CalleesText(root, "CallerOne", 50) }, func() (string, error) { return query.CalleesText(root, "CallerOne", 50) }},
		{"ImpactText", func() (string, error) { return ImpactText(root, "Target", 50) }, func() (string, error) { return query.ImpactText(root, "Target", 50) }},
		{"NavText", func() (string, error) { return NavText(root, "Target", 50) }, func() (string, error) { return query.NavText(root, "Target", 50) }},
		{"FindText", func() (string, error) { return FindText(root, "Target", "", "", 50) }, func() (string, error) { return query.FindText(root, "Target", "", "", 50) }},
		{"GrepText", func() (string, error) { return GrepText(root, "Target", 50, true) }, func() (string, error) { return query.GrepText(root, "Target", 50, true) }},
		{"DependentsText", func() (string, error) { return DependentsText(root, "Target", 50) }, func() (string, error) { return query.DependentsText(root, "Target", 50) }},
		{"DepsText", func() (string, error) { return DepsText(root, "CallerOne", 50) }, func() (string, error) { return query.DepsText(root, "CallerOne", 50) }},
		{"SearchText", func() (string, error) { return SearchText(root, "Target", nil, "", 10, false) }, func() (string, error) { return query.SearchText(root, "Target", nil, "", 10, false) }},
	}
	for _, tc := range texts {
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := tc.got()
			want, wantErr := tc.want()
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("error disagreement: wsquery err=%v, query err=%v", gotErr, wantErr)
			}
			if gotErr != nil {
				return
			}
			if got != want {
				t.Errorf("wsquery.%s bytes differ from query.%s\n got:\n%s\nwant:\n%s", tc.name, tc.name, got, want)
			}
		})
	}
}

func TestRefuseWorkspaceRootMessageShape(t *testing.T) {
	root := fixtureWorkspace(t)
	cases := map[string]string{
		"export": "codeindex export: " + root + " is a workspace, not a repo. Run it per member, e.g.\n" +
			"  codeindex export " + root + "/services/api out.db\n" +
			"members: api, shared, web",
		"import": "codeindex import: " + root + " is a workspace, not a repo. Run it per member, e.g.\n" +
			"  codeindex import " + root + "/services/api artifact.db\n" +
			"members: api, shared, web",
		"ingest": "codeindex ingest: " + root + " is a workspace, not a repo. Run it per member, e.g.\n" +
			"  codeindex ingest " + root + "/services/api\n" +
			"members: api, shared, web",
		"depmap": "codeindex depmap: " + root + " is a workspace, not a repo. Run it per member, e.g.\n" +
			"  codeindex depmap " + root + "/services/api --namespace <ns> --version <v> -o out.db\n" +
			"members: api, shared, web",
		"serve": "codeindex serve: " + root + " is a workspace, not a repo. Run it per member, e.g.\n" +
			"  codeindex serve " + root + "/services/api\n" +
			"members: api, shared, web",
	}
	for verb, want := range cases {
		t.Run(verb, func(t *testing.T) {
			err := RefuseWorkspaceRoot(verb, root)
			if err == nil {
				t.Fatalf("RefuseWorkspaceRoot(%q) returned nil", verb)
			}
			if err.Error() != want {
				t.Errorf("message drifted\n got:\n%s\nwant:\n%s", err.Error(), want)
			}
		})
	}
}

// TestSearchRefusalNamesTheFrozenNonGoal asserts search's DISTINCT wording:
// it is refused because cross-workspace semantic search is a frozen non-goal
// (vectors stay per-repo), not because of a mechanical limitation a later
// slice might lift. The other five verbs must NOT carry that sentence.
func TestSearchRefusalNamesTheFrozenNonGoal(t *testing.T) {
	root := fixtureWorkspace(t)
	err := RefuseWorkspaceRoot("search", root)
	if err == nil {
		t.Fatal("RefuseWorkspaceRoot(\"search\") returned nil")
	}
	want := "codeindex search: " + root + " is a workspace, not a repo. " +
		"Cross-workspace semantic search is a frozen non-goal — vectors stay per-repo. Run it per member, e.g.\n" +
		"  codeindex search " + root + "/services/api \"<query>\"\n" +
		"members: api, shared, web"
	if err.Error() != want {
		t.Errorf("search refusal drifted\n got:\n%s\nwant:\n%s", err.Error(), want)
	}

	exportErr := RefuseWorkspaceRoot("export", root)
	if exportErr == nil {
		t.Fatal("RefuseWorkspaceRoot(\"export\") returned nil")
	}
	if exportErr.Error() == want {
		t.Fatal("export and search must not share a message")
	}
	if got := exportErr.Error(); got == "" || containsFrozenNonGoal(got) {
		t.Errorf("export refusal must not cite the frozen non-goal: %s", got)
	}
}

func containsFrozenNonGoal(s string) bool {
	const needle = "frozen non-goal"
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestSearchTextRefusesWorkspaceRootItself pins that wsquery.SearchText — not
// its call sites — is where the search refusal happens.
func TestSearchTextRefusesWorkspaceRootItself(t *testing.T) {
	root := fixtureWorkspace(t)
	out, err := SearchText(root, "anything", nil, "", 10, false)
	if err == nil {
		t.Fatal("SearchText on a workspace root returned nil error")
	}
	if out != "" {
		t.Errorf("SearchText returned output alongside a refusal: %q", out)
	}
	if !containsFrozenNonGoal(err.Error()) {
		t.Errorf("SearchText refusal must cite the frozen non-goal, got: %s", err.Error())
	}
}
