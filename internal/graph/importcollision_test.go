package graph

import (
	"path/filepath"
	"testing"
)

// putImportCollisionFixture builds the 6b mirror of task 6a's call-collision
// fixture: a single-segment stdlib import ("log") whose Target contains no '/'
// — so store.go DOES call resolve() on the import dep — against a decoy symbol
// named `log` in a namespace ending `/log`, plus an in-package method of the
// same name that the srcNS rung picks.
//
// This is the ParsedFile the Go adapter emits (verified by dumping a real
// engine.Build over the equivalent tree) for:
//
//	// app/server.go
//	package app
//	import "log"
//	type Server struct{}
//	func (s *Server) log(msg string) { log.Print(msg) }
//	func Handle(x *Server) { x.log("hi") }
//
//	// internal/log/log.go
//	package log
//	func log() {}
func putImportCollisionFixture(t *testing.T, st *Store) {
	t.Helper()

	// The decoy: a symbol named `log` in a namespace ending `/log`, which is
	// what nsMatch suffix-matches the import-path hint "log" against.
	putFile(t, st, &ParsedFile{
		Path: "internal/log/log.go",
		Symbols: []Symbol{
			{File: "internal/log/log.go", Name: "log", Kind: KindFunc, StartLine: 3, EndLine: 3},
		},
	})

	putFile(t, st, &ParsedFile{
		Path: "app/server.go",
		Symbols: []Symbol{
			{File: "app/server.go", Name: "Server", Kind: KindType, StartLine: 5, EndLine: 5},
			{File: "app/server.go", Name: "log", Parent: "Server", Kind: KindMethod, StartLine: 7, EndLine: 7},
			{File: "app/server.go", Name: "Handle", Kind: KindFunc, StartLine: 9, EndLine: 11},
		},
		Deps: []RawDep{
			{EnclosingIdx: -1, Kind: KindImports, Target: "log", Source: "log", Line: 3},
		},
	})
}

// TestGoImportEdgeKeepsItsPreChangeResolution is plan task 6b's no-regression
// guard, in the form the task originally asked for. It replaces the earlier
// TestMEASUREDDELTAGoImportEdgeMovesToTheHintedNamespace, which pinned a
// behavior change this branch has since closed; the wants below are that
// test's own recorded pre-change baseline, restored.
//
// The delta it used to pin came from an asymmetry, not from Edit 1 alone. Edit 1
// gave Go import deps a RawDep.Source, so the dep loop's edge-local
// normalizeHint(d.Source, d.Target, pf.Path) became non-empty ("log") and, per
// plan constraint 5, PREEMPTED the srcNS same-scope rung — moving the import
// edge's dst to the unrelated repo symbol internal/log/log.go at confidence
// `unambiguous`, the token every downstream consumer trusts. Meanwhile the
// self-binding skip landed at the OTHER site: the file-level `bind` map
// discarded exactly that hint as informationless and hazardous. One invariant,
// two sites, drifted apart. store.go's dep loop now runs the same
// goImportSelfHint predicate as the bind site, so the hint is empty at both and
// the import edge resolves as it did before the change:
//
//	dst = app/server.go, Server.log, unambiguous, hint ""  — the srcNS rung.
//
// Neither answer is a package: `import "log"` has no symbol to point at. The
// point of the guard is not that Server.log is right — it is that this change
// must not INVENT a confident cross-namespace resolution that was not there
// before.
//
// Mutation evidence: delete the goImportSelfHint call from store.go's dep loop
// and this test fails on all four assertions, reporting hint "log" and
// internal/log/log.go / "internal/log". The control probe below is what keeps
// that failure meaningful — it proves the decoy really is reachable through the
// hint rung, so these assertions pin a SUPPRESSED hint rather than a dead
// fixture.
func TestGoImportEdgeKeepsItsPreChangeResolution(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	putImportCollisionFixture(t, st)

	dstFile, dstNS, conf, hint := resolvedDepEdge(t, st, "app/server.go", string(KindImports))

	// The mechanism: a Go import dep's edge-local hint is the self-binding
	// `hint == target`, and the dep loop drops it exactly as the bind site does.
	if hint != "" {
		t.Errorf("import edge hint = %q, want %q (self-binding, dropped at both sites)", hint, "")
	}
	// The behavior, matching the measured pre-change baseline.
	if dstFile != "app/server.go" {
		t.Errorf("import edge dst = %q, want %q", dstFile, "app/server.go")
	}
	if dstNS != "app" {
		t.Errorf("import edge dst namespace = %q, want %q", dstNS, "app")
	}
	if conf != string(ConfUnambiguous) {
		t.Errorf("import edge confidence = %q, want %q", conf, ConfUnambiguous)
	}
}

// TestGoDepHintStillReachesTheDecoyNamespace is the control probe for the guard
// above: the same fixture and the same bare Target "log", but a Source that is
// NOT the Target — so goImportSelfHint does not fire and the hint survives to
// resolve(). It must land on the decoy, beating the in-package Server.log that
// the srcNS rung would otherwise pick.
//
// Without this, the guard above could pass for the wrong reason: a fixture
// whose decoy had become unreachable, or a hint channel switched off wholesale
// rather than narrowed to self-bindings. The dep here is deliberately
// synthetic — a real Go file cannot embed another package's unexported `log` —
// because the point is to hold every input of the guard fixture fixed except
// the one bit the fix keys on, whether Source equals Target.
func TestGoDepHintStillReachesTheDecoyNamespace(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	putImportCollisionFixture(t, st)

	putFile(t, st, &ParsedFile{
		Path: "app/wrap.go",
		Symbols: []Symbol{
			{File: "app/wrap.go", Name: "Wrapper", Kind: KindType, StartLine: 3, EndLine: 5},
		},
		Deps: []RawDep{
			{EnclosingIdx: 0, Kind: KindExtends, Target: "log", Source: "internal/log", Line: 4},
		},
	})

	dstFile, dstNS, conf, hint := resolvedDepEdge(t, st, "app/wrap.go", string(KindExtends))
	if hint != "internal/log" {
		t.Errorf("extends edge hint = %q, want %q (Source != Target, so kept)", hint, "internal/log")
	}
	if dstFile != "internal/log/log.go" {
		t.Errorf("extends edge dst = %q, want %q — the hint rung is not live, so the "+
			"import-edge guard above proves nothing", dstFile, "internal/log/log.go")
	}
	if dstNS != "internal/log" {
		t.Errorf("extends edge dst namespace = %q, want %q", dstNS, "internal/log")
	}
	if conf != string(ConfUnambiguous) {
		t.Errorf("extends edge confidence = %q, want %q", conf, ConfUnambiguous)
	}
}
