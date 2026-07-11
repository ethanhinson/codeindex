package engine

import (
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
)

const drupalModule = `<?php
function mymod_help() { return mymod_format(); }
function mymod_format() { return "x"; }
`

// *.module routed to PHP: symbols index and calls resolve across .php and
// .module files identically.
func TestAssociationRoutesDrupalModule(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".codeindex.json": `{"associations": {"*.module": "php"}}`,
		"mymod.module":    drupalModule,
		"caller.php":      "<?php\nfunction run() { return mymod_help(); }\n",
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
	cs, err := st.Callers("mymod_help", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) == 0 {
		t.Fatal("no callers of mymod_help — .module file was not indexed")
	}
	for _, c := range cs {
		if c.File == "caller.php" {
			return
		}
	}
	t.Fatalf("caller.php -> mymod_help edge missing: %+v", cs)
}

// Adding an association after the initial build must patch identically to a
// full rebuild (newly covered files appear as additions).
func TestIncrementalEqualsFull_AssociationAdd(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"mymod.module": drupalModule,
		"caller.php":   "<?php\nfunction run() { return mymod_help(); }\n",
	})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".codeindex.json"),
		[]byte(`{"associations": {"*.module": "php"}}`), 0o644)
	assertIncrementalEqualsFull(t, dir)
}

// Removing the association must drop the covered files' symbols, again
// identically to a rebuild.
func TestIncrementalEqualsFull_AssociationRemove(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".codeindex.json": `{"associations": {"*.module": "php"}}`,
		"mymod.module":    drupalModule,
		"caller.php":      "<?php\nfunction run() { return mymod_help(); }\n",
	})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, ".codeindex.json"))
	assertIncrementalEqualsFull(t, dir)
}

// A typo in the language name must fail the build, naming valid languages.
func TestUnknownAssociationLanguageFailsLoudly(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".codeindex.json": `{"associations": {"*.inc": "hpp"}}`,
		"a.php":           "<?php\nfunction f() {}\n",
	})
	_, err := Build(dir, filepath.Join(dir, "g.db"))
	if err == nil {
		t.Fatal("unknown language must fail the build")
	}
}

// Broadened defaults: each new extension parses via the right grammar.
func TestBroadenedDefaultExtensions(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"t.phtml": "<?php\nfunction phtml_fn() {}\n",
		"m.mjs":   "export function mjs_fn() { return 1 }\n",
		"c.cts":   "export function cts_fn(): number { return 1 }\n",
		"s.pyi":   "def pyi_fn(x: int) -> int: ...\n",
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
	for _, name := range []string{"phtml_fn", "mjs_fn", "cts_fn", "pyi_fn"} {
		defs, err := st.Definitions(name, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(defs) != 1 {
			t.Fatalf("%s: want 1 definition, got %d", name, len(defs))
		}
	}
}

// The zero-config promise: a Drupal-shaped repo indexes correctly with NO
// .codeindex.json — content sniffing routes *.module/*.inc/extensionless
// scripts by their heads.
func TestSniffZeroConfigDrupal(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"mymod.module": drupalModule,
		"util.inc":     "<?php\nfunction inc_helper() { return 1; }\n",
		"bin/tool":     "#!/usr/bin/env php\n<?php\nfunction cli_main() { return inc_helper(); }\n",
		"script":       "#!/usr/bin/env python3\ndef py_main():\n    return 1\n",
		"notes.txt":    "just text mentioning <?php in prose later on\n",
		"caller.php":   "<?php\nfunction run() { return mymod_help(); }\n",
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
	for _, name := range []string{"mymod_help", "inc_helper", "cli_main", "py_main"} {
		defs, err := st.Definitions(name, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(defs) != 1 {
			t.Fatalf("%s: want 1 definition via sniffing, got %d", name, len(defs))
		}
	}
	// Cross-boundary resolution: caller.php -> mymod_help (module file).
	cs, err := st.Callers("mymod_help", "")
	if err != nil || len(cs) == 0 {
		t.Fatalf("caller.php -> mymod_help missing (err=%v)", err)
	}
	// Prose file must NOT have been indexed.
	if defs, _ := st.Definitions("notes", ""); len(defs) != 0 {
		t.Fatal("notes.txt should not be indexed")
	}
}

// A file GAINING a php head after the initial build must be picked up by an
// incremental patch identically to a rebuild (sniff cache invalidates on
// size/mtime change).
func TestIncrementalEqualsFull_SniffTransition(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"data.inc":   "plain text, not php yet\n",
		"caller.php": "<?php\nfunction run() { return late_fn(); }\n",
	})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "data.inc"),
		[]byte("<?php\nfunction late_fn() { return 1; }\n"), 0o644)
	assertIncrementalEqualsFull(t, dir)

	st, err := graph.Open(filepath.Join(dir, "inc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if defs, _ := st.Definitions("late_fn", ""); len(defs) != 1 {
		t.Fatal("late_fn should be indexed after the sniff transition")
	}
}

// Explicit associations beat sniffing: config can force a file the sniffer
// would route elsewhere.
func TestAssociationOverridesSniff(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".codeindex.json": `{"associations": {"*.inc": "python"}}`,
		"odd.inc":         "def forced_py():\n    return 1\n",
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
	if defs, _ := st.Definitions("forced_py", ""); len(defs) != 1 {
		t.Fatal("association-routed .inc should index as python")
	}
}
