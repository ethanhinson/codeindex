package graph

import (
	"path/filepath"
	"testing"
)

// TestSelfBindingImportStillBindsOutsideGo pins the LANGUAGE SCOPE of the
// self-binding skip in PutFile's bind loop. The skip exists for one Go-specific
// reason (see goImportSelfHint), and Go's half of the contract is guarded by
// TestGoImportBindDoesNotCaptureMethodCall. This test guards the other half:
// outside Go a hint equal to its target is a real, load-bearing binding and
// must survive.
//
// Both fixtures are shapes the adapters actually emit:
//
//	# app/main.py             internal/adapter/python: Target=name, Source=module
//	from base import base     -> RawDep{Target: "base", Source: "base"}
//	class X(base): ...        -> RawDep{Kind: extends, Target: "base", Source: ""}
//
//	// app/main.ts            internal/adapter/tsjs: Target=binding, Source=spec
//	import Foo from "Foo"     -> RawDep{Target: "Foo", Source: "Foo"}
//	class X extends Foo {}    -> RawDep{Kind: extends, Target: "Foo", Source: ""}
//
// `from X import X` (`from app import app`, `from config import config`,
// `from datetime import datetime`) and a default import whose binding matches
// its module specifier are common, not contrived. Python, TS and PHP all emit
// extends/implements with an empty Source, so the file-level bind map is their
// ONLY hint channel: dropping the self-binding deletes the hint outright.
//
// Non-vacuity: the assertion is that the persisted hint equals the module, and
// the failure mode being guarded persists "" instead — the two are distinct
// because the fixture's Source is non-empty.
func TestSelfBindingImportStillBindsOutsideGo(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, tc := range []struct {
		name       string
		path       string
		target     string // imported name == its module: the self-binding shape
		wantHint   string
		importKind EdgeKind
	}{
		{"python from-import", "app/main.py", "base", "base", KindExtends},
		{"tsjs default import", "app/main.ts", "Foo", "Foo", KindExtends},
		{"php root use", "app/Main.php", "Foo", "Foo", KindImplements},
	} {
		t.Run(tc.name, func(t *testing.T) {
			putFile(t, st, &ParsedFile{
				Path: tc.path,
				Symbols: []Symbol{
					{File: tc.path, Name: "X", Kind: KindType, StartLine: 1, EndLine: 3},
				},
				Deps: []RawDep{
					{EnclosingIdx: -1, Kind: KindImports, Target: tc.target, Source: tc.target, Line: 1},
					{EnclosingIdx: 0, Kind: tc.importKind, Target: tc.target, Source: "", Line: 2},
				},
			})
			if got := depEdgeHint(t, st, tc.path, string(tc.importKind)); got != tc.wantHint {
				t.Errorf("%s: subtype hint = %q, want the import binding %q",
					tc.path, got, tc.wantHint)
			}
		})
	}
}

// TestPhpQualifiedUseBindsParentNamespace covers the PHP `use A\B\C;` leg that
// normalizeHint's backslash rule exists for and that nothing else exercises:
// the adapter emits Target="C" (bare final segment, as RawDep.Target always is)
// with Source="A\B\C", and the hint persisted for a later `implements C` must
// be the PARENT namespace "A\B", not the fully qualified name.
//
// It is deliberately adjacent to the test above: that one pins the root-
// namespace `use Foo;` self-binding, this one pins the qualified form, and
// together they cover both shapes a PHP `use` can take.
func TestPhpQualifiedUseBindsParentNamespace(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	putFile(t, st, &ParsedFile{
		Path: "src/Qualified.php",
		Symbols: []Symbol{
			{File: "src/Qualified.php", Name: "Qualified", Kind: KindType, StartLine: 1, EndLine: 3},
		},
		Deps: []RawDep{
			{EnclosingIdx: -1, Kind: KindImports, Target: "C", Source: `A\B\C`, Line: 1},
			{EnclosingIdx: 0, Kind: KindImplements, Target: "C", Source: "", Line: 2},
		},
	})
	if got := depEdgeHint(t, st, "src/Qualified.php", string(KindImplements)); got != `A\B` {
		t.Errorf(`use A\B\C hint = %q, want %q`, got, `A\B`)
	}
}
