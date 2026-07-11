package python

import (
	"testing"

	"codeindex/internal/graph"
)

const sample = `def helper(x):
    return x + 1

class Widget:
    def grow(self, n):
        return helper(self.scale(n))

    def scale(self, n):
        return n * 2

def top():
    w = Widget()
    return w.grow(2)
`

func TestPythonDeps(t *testing.T) {
	src := `import os.path
from utils import helper, other as alias

class Widget(Base, mixins.Sizeable):
    pass
`
	pf, err := (Adapter{}).Parse("d.py", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[graph.EdgeKind][]string{}
	for _, d := range pf.Deps {
		kinds[d.Kind] = append(kinds[d.Kind], d.Target)
	}
	for _, want := range []string{"os.path", "helper", "other"} {
		found := false
		for _, got := range kinds[graph.KindImports] {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("import %s missing: %v", want, kinds[graph.KindImports])
		}
	}
	found := false
	for _, got := range kinds[graph.KindExtends] {
		if got == "Base" {
			found = true
		}
	}
	if !found {
		t.Errorf("extends Base missing: %v", kinds[graph.KindExtends])
	}
}

func TestPythonSymbolsAndCalls(t *testing.T) {
	pf, err := (Adapter{}).Parse("w.py", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]graph.SymbolKind{}
	for _, s := range pf.Symbols {
		kinds[s.Name] = s.Kind
	}
	for name, want := range map[string]graph.SymbolKind{
		"helper": graph.KindFunc, "Widget": graph.KindType,
		"grow": graph.KindMethod, "scale": graph.KindMethod, "top": graph.KindFunc,
	} {
		if kinds[name] != want {
			t.Errorf("%s: kind=%q want %q", name, kinds[name], want)
		}
	}
	calls := map[string][]string{}
	for _, c := range pf.Calls {
		from := "<top>"
		if c.EnclosingIdx >= 0 {
			from = pf.Symbols[c.EnclosingIdx].Name
		}
		calls[from] = append(calls[from], c.Callee)
	}
	want := map[string][]string{
		"grow": {"helper", "scale"}, // helper(...) and self.scale(...)
		"top":  {"Widget", "grow"},  // constructor call + method call
	}
	for from, needs := range want {
		for _, n := range needs {
			found := false
			for _, got := range calls[from] {
				if got == n {
					found = true
				}
			}
			if !found {
				t.Errorf("%s should call %s; got %v", from, n, calls[from])
			}
		}
	}
}
