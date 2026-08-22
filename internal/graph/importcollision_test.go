package graph

import (
	"path/filepath"
	"testing"
)

// putImportCollisionFixture builds the 6b mirror of task 6a's call-collision
// fixture: a single-segment stdlib import ("log") whose Target contains no '/'
// — so store.go DOES call resolve() on the import dep — against a decoy symbol
// named `log` in a namespace ending `/log`, plus an in-package method of the
// same name that the srcNS rung would otherwise pick.
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

// TestMEASUREDDELTAGoImportEdgeMovesToTheHintedNamespace pins spec item 6b, and
// it pins a BEHAVIOR CHANGE — read the name literally. This is not a
// no-regression pass.
//
// Task 6a's remedy (Go import deps no longer populate the file-level `bind`
// map, via the h == d.Target self-binding skip in store.go) does NOT cover this
// path. The import edge's own hint comes from the dep loop's edge-local
// normalizeHint(d.Source, d.Target, pf.Path), not from `bind`, so Edit 1's new
// RawDep.Source makes the import edge's hint non-empty regardless of 6a. Per
// plan constraint 5 a non-empty hint PREEMPTS the srcNS same-scope rung, so the
// import edge's chosen dst MOVED:
//
//	BEFORE (measured): dst = app/server.go, Server.log, unambiguous, hint ""
//	                   — decided by the srcNS rung (namespace "app").
//	AFTER  (measured): dst = internal/log/log.go, unambiguous, hint "log"
//	                   — decided by the boundIDs rung, nsMatch("internal/log",
//	                     "log") matching by suffix.
//
// Baseline method: a throwaway internal/engine probe over a temp tree of the
// two files above, run through the real adapter twice — once at branch HEAD,
// once with internal/adapter/golang/golang.go and internal/graph/store.go
// checked out at the pre-change base 2c8b9c3 — recording the imports edge each
// time. The probe was deleted and the tree restored clean afterwards.
//
// Whether this delta is ACCEPTABLE is a judgment above task 6b and is reported
// up, not decided here; task 6b deliberately does not improvise a second remedy
// on top of 6a's. Note that neither dst is a package: `import "log"` has no
// symbol to point at, and the pre-change answer (an unexported method in the
// IMPORTING package) was arguably the more wrong of the two. If the delta is
// later ruled unacceptable, this test is the thing to flip — change the wants
// below back to app/server.go and rename it, do not delete it.
func TestMEASUREDDELTAGoImportEdgeMovesToTheHintedNamespace(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	putImportCollisionFixture(t, st)

	dstFile, dstNS, conf, hint := resolvedDepEdge(t, st, "app/server.go", string(KindImports))

	// The mechanism: the import edge carries its own edge-local hint. 6a's
	// bind skip cannot suppress this one.
	if hint != "log" {
		t.Errorf("import edge hint = %q, want %q (edge-local Source, not bind)", hint, "log")
	}
	// The behavior, as measured post-change. Pre-change this was
	// "app/server.go" / "app".
	if dstFile != "internal/log/log.go" {
		t.Errorf("import edge dst = %q, want %q", dstFile, "internal/log/log.go")
	}
	if dstNS != "internal/log" {
		t.Errorf("import edge dst namespace = %q, want %q", dstNS, "internal/log")
	}
	if conf != string(ConfUnambiguous) {
		t.Errorf("import edge confidence = %q, want %q", conf, ConfUnambiguous)
	}
}
