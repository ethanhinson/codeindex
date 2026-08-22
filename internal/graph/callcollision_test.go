package graph

import (
	"path/filepath"
	"testing"
)

// TestGoImportBindDoesNotCaptureMethodCall is the no-regression guard that
// stands in for the graph goldens this repo does not have (plan constraint 4).
//
// The Go adapter now sets RawDep.Source on every import dep, keyed on the full
// import path, which made the file-level `bind` map newly non-empty for Go
// files. Go calls fall back to bind[c.Callee], and calleeName yields the
// SELECTOR FIELD for x.log() — so a lowercase single-segment stdlib import
// path ("log", "path", "context") has exactly the shape of an unexported Go
// method name and collides. nsMatch widens it once more: the candidate
// namespace internal/log matches the hint log by suffix. Per plan constraint 5
// a newly non-empty hint PREEMPTS the srcNS same-scope rung that decides this
// edge today, so the collision is a target change, not a narrowing.
//
// The fixture is the ParsedFile the Go adapter actually emits (verified by
// dumping Adapter.Parse) for:
//
//	// app/server.go
//	package app
//	import "log"
//	type Server struct{}
//	func (s *Server) log(msg string) {}
//	func Handle(x *Server) { x.log("hi") }
//
//	// internal/log/log.go
//	package log
//	func log() {}
//
// Baseline, established empirically end-to-end through the real adapter with
// the import-dep Source temporarily reverted: the call resolves to
// Server.log in app/server.go, unambiguous, with no hint. That is what this
// test pins. The hint assertion is the mechanism (bind must not capture the
// callee); the target assertion is the behavior.
func TestGoImportBindDoesNotCaptureMethodCall(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// The decoy: a symbol named `log` in a namespace ending `/log`, which is
	// what nsMatch would suffix-match the import path "log" against.
	putFile(t, st, &ParsedFile{
		Path: "internal/log/log.go",
		Symbols: []Symbol{
			{File: "internal/log/log.go", Name: "log", Kind: KindFunc, StartLine: 3, EndLine: 3},
		},
	})

	// The importing file: imports "log" and calls the method x.log().
	putFile(t, st, &ParsedFile{
		Path: "app/server.go",
		Symbols: []Symbol{
			{File: "app/server.go", Name: "Server", Kind: KindType, StartLine: 5, EndLine: 5},
			{File: "app/server.go", Name: "log", Parent: "Server", Kind: KindMethod, StartLine: 7, EndLine: 7},
			{File: "app/server.go", Name: "Handle", Kind: KindFunc, StartLine: 9, EndLine: 11},
		},
		Calls: []RawCall{
			{EnclosingIdx: 2, Callee: "log", Qualifier: "", NsHint: "", Line: 10},
		},
		Deps: []RawDep{
			{EnclosingIdx: -1, Kind: KindImports, Target: "log", Source: "log", Line: 3},
		},
	})

	dstFile, _, conf, hint := resolvedDepEdge(t, st, "app/server.go", string(KindCalls))
	if hint != "" {
		t.Errorf("the import dep must not bind the callee name: call hint = %q, want %q", hint, "")
	}
	if dstFile != "app/server.go" {
		t.Errorf("x.log() must still resolve to the in-package method: dst = %q, want %q",
			dstFile, "app/server.go")
	}
	if conf != string(ConfUnambiguous) {
		t.Errorf("x.log() confidence = %q, want %q", conf, ConfUnambiguous)
	}
}
