package tsjs

import (
	"testing"

	"codeindex/internal/graph"
)

const sampleTS = `export function helper(x: number): number {
  return x + 1;
}

export class Widget {
  grow(n: number): number {
    return helper(n);
  }
}

export const fetchAll = async () => {
  const w = new Widget();
  return w.grow(2);
};

items.forEach((item) => process(item));
`

func parse(t *testing.T, path, src string) *graph.ParsedFile {
	t.Helper()
	pf, err := (Adapter{}).Parse(path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return pf
}

func kinds(pf *graph.ParsedFile) map[string]graph.SymbolKind {
	m := map[string]graph.SymbolKind{}
	for _, s := range pf.Symbols {
		m[s.Name] = s.Kind
	}
	return m
}

func callsFrom(pf *graph.ParsedFile) map[string][]string {
	m := map[string][]string{}
	for _, c := range pf.Calls {
		from := "<top>"
		if c.EnclosingIdx >= 0 {
			from = pf.Symbols[c.EnclosingIdx].Name
		}
		m[from] = append(m[from], c.Callee)
	}
	return m
}

func has(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func TestTypeScriptSymbolsAndCalls(t *testing.T) {
	pf := parse(t, "w.ts", sampleTS)
	k := kinds(pf)
	for name, want := range map[string]graph.SymbolKind{
		"helper": graph.KindFunc, "Widget": graph.KindType,
		"grow": graph.KindMethod, "fetchAll": graph.KindFunc,
	} {
		if k[name] != want {
			t.Errorf("%s: kind=%q want %q (symbols: %v)", name, k[name], want, k)
		}
	}
	cf := callsFrom(pf)
	if !has(cf["grow"], "helper") {
		t.Errorf("grow should call helper; got %v", cf)
	}
	if !has(cf["fetchAll"], "Widget") || !has(cf["fetchAll"], "grow") {
		t.Errorf("fetchAll should call Widget (new) and grow; got %v", cf["fetchAll"])
	}
	// anonymous forEach callback is not a symbol
	if _, ok := k[""]; ok {
		t.Error("anonymous symbol leaked")
	}
}

func TestJavaScriptAndTSX(t *testing.T) {
	pf := parse(t, "a.js", "function go() { run(); }\n")
	if kinds(pf)["go"] != graph.KindFunc || !has(callsFrom(pf)["go"], "run") {
		t.Errorf("js parse failed: %+v", pf)
	}
	pf = parse(t, "c.tsx", "export function App() { return <div onClick={() => handle()} />; }\n")
	if kinds(pf)["App"] != graph.KindFunc || !has(callsFrom(pf)["App"], "handle") {
		t.Errorf("tsx parse failed: %+v", pf)
	}
}
