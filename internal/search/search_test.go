package search_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"codeindex/internal/engine"
	"codeindex/internal/graph"
	"codeindex/internal/search"
)

func TestTokenize(t *testing.T) {
	cases := map[string][]string{
		"parseConfig":      {"parse", "config"},
		"load_config":      {"load", "config"},
		"HTTPServerConfig": {"http", "server", "config"},
		"firstOrCreate":    {"first", "or", "create"},
		"NewV4Signer":      {"new", "v", "4", "signer"},
		"a":                {"a"},
	}
	for in, want := range cases {
		if got := search.Tokenize(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Tokenize(%q)=%v want %v", in, got, want)
		}
	}
}

func buildFixture(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	fixtureRoot = dir
	files := map[string]string{
		"config.go": `package p
func LoadConfig() int { return 1 }
func ParseConfig() int { return 2 }
func unrelated() int { return 3 }
`,
		"user.go": `package p
func GetUser() int { return LoadConfig() }
func fetchAll() int {
	a := LoadConfig()
	return a + LoadConfig()
}
`,
		"config_test.go": `package p
func TestLoadConfig(t int) {}
`,
	}
	for name, content := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}
	db := filepath.Join(dir, "g.db")
	if _, err := engine.Build(dir, db); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestFindConventionAndRanking(t *testing.T) {
	st := buildFixture(t)
	// token query, reversed order, lowercase — must match LoadConfig first
	// (2 callers) over ParseConfig (0) and the test symbol (penalized).
	res, err := search.Find(st, "config load", search.Opts{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 || res[0].Sym.Name != "LoadConfig" {
		t.Fatalf("expected LoadConfig first; got %+v", res)
	}
	// synonym: "fetch user" -> GetUser (get∈fetch group)
	res, err = search.Find(st, "fetch user", search.Opts{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range res {
		if r.Sym.Name == "GetUser" {
			found = true
		}
	}
	if !found {
		t.Errorf("synonym expansion should surface GetUser; got %+v", res)
	}
}

func TestGrepAttribution(t *testing.T) {
	st := buildFixture(t)
	// LoadConfig occurrences: 1 def line + 3 call sites across 2 funcs.
	root := fixtureRoot
	groups, raw, _, err := search.Grep(st, root, "LoadConfig", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if raw < 4 {
		t.Fatalf("expected >=4 raw hits, got %d", raw)
	}
	// defs first
	if groups[0].Sym == nil || groups[0].Sym.Name != "LoadConfig" || !groups[0].IsDef {
		t.Errorf("first group should be the definition; got %+v", groups[0])
	}
	// fetchAll has 2 hits deduped into one group
	foundFetch := false
	for _, g := range groups {
		if g.Sym != nil && g.Sym.Name == "fetchAll" {
			foundFetch = true
			if g.Hits != 2 {
				t.Errorf("fetchAll should dedup 2 hits; got %d", g.Hits)
			}
		}
	}
	if !foundFetch {
		t.Error("fetchAll group missing")
	}
}

func TestGrepWordBoundary(t *testing.T) {
	st := buildFixture(t)
	root := fixtureRoot
	// substring mode: "Config" hits LoadConfig/ParseConfig/TestLoadConfig
	groups, _, _, err := search.Grep(st, root, "Config", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) == 0 {
		t.Fatal("substring grep found nothing")
	}
	// word mode: "Config" alone never appears as a whole word in the fixture
	wgroups, wraw, _, err := search.Grep(st, root, "Config", 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if wraw != 0 || len(wgroups) != 0 {
		t.Errorf("word grep should match nothing; got %d raw hits %+v", wraw, wgroups)
	}
	// word mode still matches the exact identifier
	wgroups, wraw, _, err = search.Grep(st, root, "LoadConfig", 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if wraw < 4 {
		t.Errorf("word grep of LoadConfig should keep all exact hits; got %d", wraw)
	}
	// ...but drops the mid-identifier hit inside TestLoadConfig
	for _, g := range wgroups {
		if g.Sym != nil && g.Sym.Name == "TestLoadConfig" && g.IsDef {
			t.Errorf("word grep should not match LoadConfig inside TestLoadConfig")
		}
	}
}

var fixtureRoot string
