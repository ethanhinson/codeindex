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

// TestEmbedDepCarriesSource pins spec items 2 and 3: a qualified embed resolves
// its package operand through the aliases map and carries the import path as
// the dep's Source, while Target stays the BARE type name. Unqualified and
// generic embeds keep Target bare with an empty Source.
//
// The pointer case is deliberately the QUALIFIED pointer embed `*al.Thing`: a
// bare `*B` never reaches embeddedTypeName's pointer_type leg (tree-sitter-go
// gives the field a plain type_identifier `type` with a sibling `*`), so a
// bare-`*B` pointer test would be vacuous.
func TestEmbedDepCarriesSource(t *testing.T) {
	src := `package p

import (
	"codeindex/internal/tsdb/chunkenc"
	al "codeindex/internal/alias"
)

type B struct{ n int }

type G[T any] struct{ v T }

type S struct {
	chunkenc.Chunk
	*al.Thing
	B
	G[int]
}
`
	pf, err := (Adapter{}).Parse("p.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range pf.Deps {
		if d.Kind == graph.KindExtends {
			if _, dup := got[d.Target]; dup {
				t.Fatalf("duplicate extends target %q", d.Target)
			}
			got[d.Target] = d.Source
		}
	}
	want := map[string]string{
		"Chunk": "codeindex/internal/tsdb/chunkenc", // qualified
		"Thing": "codeindex/internal/alias",         // qualified pointer, aliased
		"B":     "",                                 // unqualified
		"G":     "",                                 // generic
	}
	if len(got) != len(want) {
		t.Fatalf("extends deps = %v, want keys %v", got, want)
	}
	for target, wantSrc := range want {
		src, ok := got[target]
		if !ok {
			t.Errorf("no extends dep with bare Target %q; got %v", target, got)
			continue
		}
		if src != wantSrc {
			t.Errorf("embed %q: Source = %q, want %q", target, src, wantSrc)
		}
	}
}

// TestEmbedDepMissesHintForUnaliasedOperand pins spec item 4's first miss
// class: an embed whose package operand has no `aliases` entry at all (a
// dot-import) still resolves with Target bare and Source empty. This is
// TODAY'S BEHAVIOR, preserved deliberately — `import_spec` excludes `.` (and
// `_`) imports from the aliases map on purpose, so there is nothing for
// embeddedTypeName to look up.
func TestEmbedDepMissesHintForUnaliasedOperand(t *testing.T) {
	src := `package p

import (
	. "codeindex/internal/dotimport"
)

type S struct {
	dotimport.Thing
}
`
	pf, err := (Adapter{}).Parse("p.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range pf.Deps {
		if d.Kind != graph.KindExtends {
			continue
		}
		if d.Target != "Thing" {
			t.Fatalf("unexpected extends target %q", d.Target)
		}
		found = true
		if d.Source != "" {
			t.Errorf("Source = %q, want \"\" (dot-import has no aliases entry)", d.Source)
		}
	}
	if !found {
		t.Fatalf("no extends dep for Thing; got %v", pf.Deps)
	}
}

// TestEmbedDepMissesHintForSegmentMismatch pins spec item 4's second miss
// class — the ACCEPTED MISS CLASS documented in the plan's binding constraint
// 6: `import_spec` registers `aliases[lastPathSegment] = ipath`, so
// `import "gopkg.in/yaml.v2"` registers the key "yaml.v2", not "yaml". An
// embed written as `yaml.MapSlice` looks up "yaml" and misses, yielding
// Source == "". This is TODAY'S BEHAVIOR, not a regression: resolving the
// import's real `package` clause (which here really is `yaml`) would require
// reading the imported package's source, which is cross-file work this
// change deliberately does not take on.
func TestEmbedDepMissesHintForSegmentMismatch(t *testing.T) {
	src := `package p

import (
	"gopkg.in/yaml.v2"
)

type S struct {
	yaml.MapSlice
}
`
	pf, err := (Adapter{}).Parse("p.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range pf.Deps {
		if d.Kind != graph.KindExtends {
			continue
		}
		if d.Target != "MapSlice" {
			t.Fatalf("unexpected extends target %q", d.Target)
		}
		found = true
		if d.Source != "" {
			t.Errorf("Source = %q, want \"\" (aliases holds key %q, not %q)", d.Source, "yaml.v2", "yaml")
		}
	}
	if !found {
		t.Fatalf("no extends dep for MapSlice; got %v", pf.Deps)
	}
}
