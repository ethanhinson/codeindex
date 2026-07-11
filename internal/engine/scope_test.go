package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/graph"
)

// Two packages each define validate(); a caller in pkg a must resolve to its
// OWN package's validate, unambiguous (previously ambiguous across both).
func TestSamePackageCollapse(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/util.go": "package a\nfunc validate() int { return 1 }\n",
		"a/run.go":  "package a\nfunc Run() int { return validate() }\n",
		"b/util.go": "package b\nfunc validate() int { return 2 }\n",
	})
	db := filepath.Join(dir, "g.db")
	if _, err := Build(dir, db); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cs, err := st.Callees("Run", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Name == "validate" {
			if c.Conf != graph.ConfUnambiguous || c.DefFile != "a/util.go" {
				t.Fatalf("same-package call should resolve to a/util.go unambiguously; got %+v", c)
			}
			return
		}
	}
	t.Fatal("validate callee missing")
}

// No same-scope candidate: behavior identical to before (global, ambiguous).
func TestScopeNeverWorse(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/run.go": "package a\nfunc Run() int { return helper() }\n",
		"b/one.go": "package b\nfunc helper() int { return 1 }\n",
		"c/two.go": "package c\nfunc helper() int { return 2 }\n",
	})
	db := filepath.Join(dir, "g.db")
	if _, err := Build(dir, db); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cs, err := st.Callees("Run", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Name == "helper" {
			if c.Conf != graph.ConfAmbiguous {
				t.Fatalf("cross-package collision should stay ambiguous; got %+v", c)
			}
			return
		}
	}
	t.Fatal("helper callee missing")
}

// Moving a definition INTO the caller's scope must flip resolution on an
// incremental patch identically to a full rebuild.
func TestIncrementalEqualsFull_ScopeShift(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/run.go": "package a\nfunc Run() int { return helper() }\n",
		"b/one.go": "package b\nfunc helper() int { return 1 }\n",
	})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	// Add a same-package helper — scope preference must now pick a/'s.
	os.WriteFile(filepath.Join(dir, "a", "local.go"),
		[]byte("package a\nfunc helper() int { return 3 }\n"), 0o644)
	assertIncrementalEqualsFull(t, dir)

	st, err := graph.Open(filepath.Join(dir, "inc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cs, _ := st.Callees("Run", "")
	for _, c := range cs {
		if c.Name == "helper" && (c.Conf != graph.ConfUnambiguous || !strings.HasPrefix(c.DefFile, "a/")) {
			t.Fatalf("after scope shift, helper should resolve into a/ unambiguously; got %+v", c)
		}
	}
}
