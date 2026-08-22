package golang

import (
	"testing"

	"codeindex/internal/graph"
)

const sample = `package p

func Helper(x int) int {
	return x + 1
}

func Caller() int {
	y := Helper(2)
	return y
}

type Widget struct{ n int }

func (w Widget) Grow() int {
	return Helper(w.n)
}
`

func TestParseSymbols(t *testing.T) {
	pf, err := (Adapter{}).Parse("p.go", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Helper": false, "Caller": false, "Widget": false, "Grow": false}
	for _, s := range pf.Symbols {
		if _, ok := want[s.Name]; ok {
			want[s.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing symbol %q; got %+v", name, pf.Symbols)
		}
	}
}

func TestCallsAttributedToEnclosing(t *testing.T) {
	pf, err := (Adapter{}).Parse("p.go", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	// Both Caller and Grow call Helper; attribute each call to its enclosing symbol.
	callsFrom := map[string][]string{}
	for _, c := range pf.Calls {
		from := "<top>"
		if c.EnclosingIdx >= 0 {
			from = pf.Symbols[c.EnclosingIdx].Name
		}
		callsFrom[from] = append(callsFrom[from], c.Callee)
	}
	if !contains(callsFrom["Caller"], "Helper") {
		t.Errorf("Caller should call Helper; got %v", callsFrom["Caller"])
	}
	if !contains(callsFrom["Grow"], "Helper") {
		t.Errorf("Grow should call Helper; got %v", callsFrom["Grow"])
	}
}

func TestDepsExtraction(t *testing.T) {
	src := `package p

import (
	"fmt"
	"codeindex/internal/graph"
)

type Base struct{ n int }

type Widget struct {
	*Base
	name string
}
`
	pf, err := (Adapter{}).Parse("p.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var imports, extends []string
	for _, d := range pf.Deps {
		switch d.Kind {
		case graph.KindImports:
			imports = append(imports, d.Target)
		case graph.KindExtends:
			extends = append(extends, d.Target)
			if d.EnclosingIdx < 0 || pf.Symbols[d.EnclosingIdx].Name != "Widget" {
				t.Errorf("embedding should originate from Widget; got idx %d", d.EnclosingIdx)
			}
		}
	}
	if !contains(imports, "fmt") || !contains(imports, "codeindex/internal/graph") {
		t.Errorf("imports missing: %v", imports)
	}
	if !contains(extends, "Base") {
		t.Errorf("embedding Base missing: %v", extends)
	}
}

// TestImportDepCarriesSource pins spec item 1: an import dep's Source is the
// verbatim import path, for both an implicit import and an explicit alias. The
// hint channel for subtype edges is the edge-local Source, so an empty Source
// here silently disables the import-mediated resolution rung.
func TestImportDepCarriesSource(t *testing.T) {
	src := `package p

import (
	"fmt"
	al "codeindex/internal/graph"
)
`
	pf, err := (Adapter{}).Parse("p.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range pf.Deps {
		if d.Kind == graph.KindImports {
			got[d.Target] = d.Source
		}
	}
	for _, ipath := range []string{"fmt", "codeindex/internal/graph"} {
		if _, ok := got[ipath]; !ok {
			t.Fatalf("import dep for %q missing; got %v", ipath, got)
		}
		if got[ipath] != ipath {
			t.Errorf("import %q: Source = %q, want %q", ipath, got[ipath], ipath)
		}
	}
}

// TestImportAliasExclusions pins that `_` and `.` imports stay out of the
// aliases map (unchanged behavior). aliases has no direct seam, so this asserts
// through the call path it feeds: a call qualified by a name that was never
// registered carries no namespace hint.
func TestImportAliasExclusions(t *testing.T) {
	src := `package p

import (
	"fmt"
	_ "codeindex/internal/blank"
	. "codeindex/internal/dot"
)

func use() {
	fmt.Println()
	blank.F()
	dot.F()
}
`
	pf, err := (Adapter{}).Parse("p.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	nF := 0
	for _, c := range pf.Calls {
		switch c.Callee {
		case "Println":
			if c.NsHint != "fmt" {
				t.Errorf("plain import should register an alias: NsHint = %q, want %q",
					c.NsHint, "fmt")
			}
		case "F":
			nF++
			if c.NsHint != "" {
				t.Errorf("`_`/`.` imports must not register aliases: NsHint = %q, want %q",
					c.NsHint, "")
			}
		}
	}
	if nF != 2 {
		t.Fatalf("expected both blank.F() and dot.F() calls; got %d", nF)
	}
	// Both `_` and `.` imports still emit their import dep.
	var paths []string
	for _, d := range pf.Deps {
		if d.Kind == graph.KindImports {
			paths = append(paths, d.Target)
		}
	}
	if !contains(paths, "codeindex/internal/blank") || !contains(paths, "codeindex/internal/dot") {
		t.Errorf("`_`/`.` imports should still emit deps; got %v", paths)
	}
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
