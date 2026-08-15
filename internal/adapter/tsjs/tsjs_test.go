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

func TestTSDeps(t *testing.T) {
	src := `import Default, {helper, other as alias} from './utils';
import * as ns from './ns';

interface Base {}
export class Widget extends BaseWidget implements Base, Sizeable {
  grow(): number { return 1; }
}
`
	pf := parse(t, "d.ts", src)
	kinds := map[graph.EdgeKind][]string{}
	for _, d := range pf.Deps {
		kinds[d.Kind] = append(kinds[d.Kind], d.Target)
	}
	for _, want := range []string{"Default", "helper", "other"} {
		if !has(kinds[graph.KindImports], want) {
			t.Errorf("import %s missing: %v", want, kinds[graph.KindImports])
		}
	}
	if has(kinds[graph.KindImports], "ns") {
		t.Errorf("namespace import should be skipped: %v", kinds[graph.KindImports])
	}
	if !has(kinds[graph.KindExtends], "BaseWidget") {
		t.Errorf("extends BaseWidget missing: %v", kinds[graph.KindExtends])
	}
	if !has(kinds[graph.KindImplements], "Base") || !has(kinds[graph.KindImplements], "Sizeable") {
		t.Errorf("implements missing: %v", kinds[graph.KindImplements])
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

// Abstract classes and enums are distinct tree-sitter node types; both must
// index (the nest ModuleRef gap, FINDINGS-corpus-expansion).
func TestAbstractClassAndEnum(t *testing.T) {
	src := `
export abstract class ModuleRef {
  abstract get<T>(token: any): T;
  resolve(): void {}
}
export enum Scope {
  DEFAULT,
  TRANSIENT,
}
`
	pf := parse(t, "moduleref.ts", src)
	byName := map[string]string{}
	parents := map[string]string{}
	for _, s := range pf.Symbols {
		byName[s.Name] = string(s.Kind)
		parents[s.Name] = s.Parent
	}
	if byName["ModuleRef"] != "type" {
		t.Fatalf("abstract class not indexed as type: %v", byName)
	}
	if byName["Scope"] != "type" {
		t.Fatalf("enum not indexed as type: %v", byName)
	}
	if parents["resolve"] != "ModuleRef" {
		t.Fatalf("method parent inside abstract class = %q, want ModuleRef", parents["resolve"])
	}
}
