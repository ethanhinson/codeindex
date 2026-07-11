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

// A TS file imports {helper} from './utils' and calls it; helper is also
// defined elsewhere. The import must bind the call to utils.ts, unambiguous.
func TestTSNamedImportBinds(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"src/utils.ts": "export function helper(): number { return 1 }\n",
		"src/other.ts": "export function helper(): number { return 2 }\n",
		"src/app.ts":   "import {helper} from './utils'\nexport function run(): number { return helper() }\n",
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
	cs, err := st.Callees("run", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Name == "helper" {
			if c.Conf != graph.ConfUnambiguous || c.DefFile != "src/utils.ts" {
				t.Fatalf("import-bound call should resolve to src/utils.ts unambiguously; got %+v", c)
			}
			return
		}
	}
	t.Fatal("helper callee missing")
}

// A Go file calls util.Clock() through an import alias; Clock is defined in
// pkg/util and in another package. The alias must scope the call.
func TestGoAliasScopedCall(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"pkg/util/clock.go":  "package util\nfunc Clock() int { return 1 }\n",
		"pkg/other/clock.go": "package other\nfunc Clock() int { return 2 }\n",
		"cmd/main.go":        "package main\nimport \"example.com/mod/pkg/util\"\nfunc main() { util.Clock() }\n",
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
	cs, err := st.Callees("main", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Name == "Clock" {
			if c.Conf != graph.ConfUnambiguous || c.DefFile != "pkg/util/clock.go" {
				t.Fatalf("alias-scoped call should resolve to pkg/util unambiguously; got %+v", c)
			}
			return
		}
	}
	t.Fatal("Clock callee missing")
}

// A binding that maps nowhere must fall through the ladder untouched —
// identical to no binding at all.
func TestBindingNeverWorse(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"src/app.ts": "import {helper} from 'external-package'\nexport function run(): number { return helper() }\n",
		"src/one.ts": "export function helper(): number { return 1 }\n",
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
	cs, err := st.Callees("run", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Name == "helper" {
			if c.Conf != graph.ConfUnambiguous || c.DefFile != "src/one.ts" {
				t.Fatalf("unmappable binding must fall through to the plain match; got %+v", c)
			}
			return
		}
	}
	t.Fatal("helper callee missing")
}

// Editing the imported module must re-bind on an incremental patch
// identically to a full rebuild.
func TestIncrementalEqualsFull_BindingShift(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"src/app.ts":   "import {helper} from './utils'\nexport function run(): number { return helper() }\n",
		"src/other.ts": "export function helper(): number { return 2 }\n",
	})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	// The bound module appears — the call must snap to it on patch.
	os.WriteFile(filepath.Join(dir, "src", "utils.ts"),
		[]byte("export function helper(): number { return 1 }\n"), 0o644)
	assertIncrementalEqualsFull(t, dir)

	st, err := graph.Open(filepath.Join(dir, "inc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cs, _ := st.Callees("run", "")
	for _, c := range cs {
		if c.Name == "helper" && (c.Conf != graph.ConfUnambiguous || c.DefFile != "src/utils.ts") {
			t.Fatalf("after binding shift, helper should bind to src/utils.ts; got %+v", c)
		}
	}
}

// PHP: use A\B\Helper binds new Helper/Helper:: references to namespace A\B.
func TestPHPUseBinding(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"src/AB/Helper.php":    "<?php\nnamespace A\\B;\nclass Helper { public function go(): int { return 1; } }\n",
		"src/Other/Helper.php": "<?php\nnamespace Other;\nclass Helper { public function go(): int { return 2; } }\n",
		"src/App/Run.php":      "<?php\nnamespace App;\nuse A\\B\\Helper;\nclass Run { public function main(): int { return Helper::make(); } }\n",
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
	// The use edge itself must resolve to A\B's Helper, not Other's.
	deps, err := st.FileImports("src/App/Run.php")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range deps {
		if d.Target == "Helper" {
			if d.DefFile != "src/AB/Helper.php" {
				t.Fatalf("use-bound import should resolve to A\\B's Helper; got %+v", d)
			}
			return
		}
	}
	t.Fatal("Helper import edge missing")
}
