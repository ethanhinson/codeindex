package php

import (
	"testing"

	"codeindex/internal/graph"
)

const sample = `<?php

function helper($x) {
    return $x + 1;
}

class Widget {
    public function grow($n) {
        return helper($this->scale($n));
    }

    private function scale($n) {
        return $n * 2;
    }
}

function top() {
    $w = new Widget();
    return $w->grow(2) + Widget::create();
}
`

func TestPHPDeps(t *testing.T) {
	src := `<?php
use App\Support\Helper;
use App\Contracts\Sizeable;

class Widget extends BaseWidget implements Sizeable {
    use Trackable;
}
`
	pf, err := (Adapter{}).Parse("d.php", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[graph.EdgeKind][]string{}
	for _, d := range pf.Deps {
		kinds[d.Kind] = append(kinds[d.Kind], d.Target)
	}
	want := func(kind graph.EdgeKind, name string) {
		for _, got := range kinds[kind] {
			if got == name {
				return
			}
		}
		t.Errorf("%s %s missing: %v", kind, name, kinds[kind])
	}
	want(graph.KindImports, "Helper")
	want(graph.KindImports, "Sizeable")
	want(graph.KindExtends, "BaseWidget")
	want(graph.KindImplements, "Sizeable")
	want(graph.KindImplements, "Trackable")
}

func TestPHPSymbolsAndCalls(t *testing.T) {
	pf, err := (Adapter{}).Parse("w.php", []byte(sample))
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
			t.Errorf("%s: kind=%q want %q (all: %v)", name, kinds[name], want, kinds)
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
		"grow": {"helper", "scale"},          // helper(...) + $this->scale(...)
		"top":  {"Widget", "grow", "create"}, // new Widget + ->grow + ::create
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
