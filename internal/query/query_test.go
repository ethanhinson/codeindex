package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Text() renders are a published contract: agent prompts and the bench
// formatters (bench/scout/formatters.py) parse them. These goldens pin the
// exact bytes; change them only deliberately, with the consumers.

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

func TestCallersTextGolden(t *testing.T) {
	root := fixtureRepo(t)
	got, err := CallersText(root, "Target", 50)
	if err != nil {
		t.Fatal(err)
	}
	want := `def  Target  a.go:3  func Target()
callers (3):
  a.go:6  CallerOne
  b.go:4  CallerTwo
  b.go:5  CallerTwo
referenced in 2 file(s): a.go b.go
`
	if got != want {
		t.Errorf("callers text drifted from the published contract\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestCallersTruncationAndJSON(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Callers(root, "Target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.CallersTotal != 3 || len(a.Callers) != 1 {
		t.Fatalf("want total=3 shown=1, got total=%d shown=%d", a.CallersTotal, len(a.Callers))
	}
	if !strings.Contains(a.Text(), "  ... (+2 more; raise limit)\n") {
		t.Errorf("truncation line missing:\n%s", a.Text())
	}

	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["callers_total"].(float64) != 3 {
		t.Errorf("json callers_total = %v, want 3", m["callers_total"])
	}
	first := m["callers"].([]any)[0].(map[string]any)
	if first["qname"] != "CallerOne" || first["file"] != "a.go" || first["line"].(float64) != 6 {
		t.Errorf("unexpected first caller: %v", first)
	}
}

func TestMissingSymbolJSONEmptyArrays(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Callers(root, "NoSuchSymbol", 50)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Machine consumers get [] for empty result sets, never null.
	for _, want := range []string{`"definitions":[]`, `"callers":[]`, `"referenced_files":[]`} {
		if !strings.Contains(s, want) {
			t.Errorf("json missing %s: %s", want, s)
		}
	}
	if !strings.Contains(a.Text(), "def  NoSuchSymbol: (not found in index)\n") {
		t.Errorf("not-found text drifted:\n%s", a.Text())
	}
}

func TestCalleesTextGolden(t *testing.T) {
	root := fixtureRepo(t)
	got, err := CalleesText(root, "CallerTwo", 50)
	if err != nil {
		t.Fatal(err)
	}
	want := `callees of CallerTwo (2):
  Target  -> a.go:3  @call:4
  Target  -> a.go:3  @call:5
`
	if got != want {
		t.Errorf("callees text drifted from the published contract\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestEnclosingText(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Enclosing(root, "a.go", 5, 6)
	if err != nil {
		t.Fatal(err)
	}
	want := "sym  CallerOne  func  a.go:5-7  callers=0 external=0\n"
	if a.Text() != want {
		t.Errorf("enclosing text drifted\ngot:\n%q\nwant:\n%q", a.Text(), want)
	}
}

func TestNavTextGolden(t *testing.T) {
	root := fixtureRepo(t)
	got, err := NavText(root, "Target", 50)
	if err != nil {
		t.Fatal(err)
	}
	want := `nav Target: 1 definition(s), 3 caller(s), 2 referencing file(s)
def  Target  a.go:3  func Target()
matches:
  Target  func  a.go:3  callers=3  [exact]
callers (3):
  a.go:6  CallerOne
  b.go:4  CallerTwo
  b.go:5  CallerTwo
referenced in 2 file(s): a.go b.go
`
	if got != want {
		t.Errorf("nav text drifted from the published contract\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestNavGrepStandIn(t *testing.T) {
	// A symbol with zero call edges but content references: grep attribution
	// stands in for callers (the measured callers/grep blur).
	root := fixtureRepo(t)
	if err := os.WriteFile(filepath.Join(root, "c.go"), []byte(`package p

// unrelated mentions Lonely in a comment: Lonely
func unrelated() {}

func Lonely() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Nav(root, "Lonely", 50)
	if err != nil {
		t.Fatal(err)
	}
	if !a.CallersFromGrep {
		t.Fatalf("expected grep stand-in for a call-edge-less symbol: %+v", a)
	}
	if len(a.Definitions) == 0 || a.Definitions[0].File != "c.go" {
		t.Errorf("nav should still carry the definition: %+v", a.Definitions)
	}
}

func TestGrepTextAndJSON(t *testing.T) {
	root := fixtureRepo(t)
	a, err := Grep(root, "Target", 30, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.Text(), `grep "Target": `) {
		t.Errorf("grep header drifted: %q", a.Text())
	}
	var hasDef bool
	for _, g := range a.Groups {
		if g.QName == "Target" && g.IsDef {
			hasDef = true
		}
	}
	if !hasDef {
		t.Errorf("grep groups missing the Target definition: %+v", a.Groups)
	}
}
