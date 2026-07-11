// Package tsjs is the tree-sitter TypeScript/JavaScript adapter: named
// functions, classes, methods, and named arrow-function bindings as symbols;
// call and constructor sites as raw name-based call edges. Anonymous callbacks
// are deliberately not symbols (agents anchor on named things).
package tsjs

import (
	"context"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"codeindex/internal/adapter"
	"codeindex/internal/adapter/common"
	"codeindex/internal/graph"
)

// Adapter parses TS/TSX/JS/JSX with the grammar matching the file extension.
type Adapter struct{}

func (Adapter) Extensions() []string { return []string{".ts", ".tsx", ".js", ".jsx"} }

func language(path string) *sitter.Language {
	switch filepath.Ext(path) {
	case ".ts":
		return typescript.GetLanguage()
	case ".tsx", ".jsx": // tsx is a superset that handles JSX in both
		return tsx.GetLanguage()
	default:
		return javascript.GetLanguage()
	}
}

func (Adapter) Parse(path string, src []byte) (*graph.ParsedFile, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(language(path))
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	var spans []common.SymbolSpan
	var calls []common.RawCall
	var deps []common.DepSite

	addDep := func(n *sitter.Node, kind graph.EdgeKind, target string) {
		if target != "" {
			deps = append(deps, common.DepSite{
				Kind: kind, Target: target,
				Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
		}
	}

	// class threads the lexically enclosing class name: method parents, and
	// `this.x()` qualification.
	var walk func(n *sitter.Node, class string)
	walk = func(n *sitter.Node, class string) {
		switch n.Type() {
		case "function_declaration", "generator_function_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), "", graph.KindFunc, "body"))
			}
		case "class_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), "", graph.KindType, "body"))
				class = name.Content(src)
			}
		case "extends_clause", "extends_type_clause":
			// class A extends B / interface A extends B — identifier-ish children
			for i := 0; i < int(n.NamedChildCount()); i++ {
				c := n.NamedChild(i)
				if c.Type() == "identifier" || c.Type() == "type_identifier" {
					addDep(n, graph.KindExtends, c.Content(src))
				}
			}
		case "implements_clause":
			for i := 0; i < int(n.NamedChildCount()); i++ {
				c := n.NamedChild(i)
				if c.Type() == "type_identifier" || c.Type() == "identifier" {
					addDep(n, graph.KindImplements, c.Content(src))
				}
			}
		case "import_statement":
			// import X, {a, b as c} from '...' — named + default specifiers
			var collect func(m *sitter.Node)
			collect = func(m *sitter.Node) {
				switch m.Type() {
				case "import_specifier":
					if nm := m.ChildByFieldName("name"); nm != nil {
						addDep(n, graph.KindImports, nm.Content(src))
					}
					return
				case "identifier": // default import binding
					addDep(n, graph.KindImports, m.Content(src))
					return
				case "namespace_import": // import * as ns — alias, not a repo symbol
					return
				}
				for i := 0; i < int(m.NamedChildCount()); i++ {
					collect(m.NamedChild(i))
				}
			}
			if clause := n.NamedChild(0); clause != nil && clause.Type() == "import_clause" {
				collect(clause)
			}
		case "method_definition":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), class, graph.KindMethod, "body"))
			}
		case "variable_declarator":
			// const f = () => {...} / const f = function(...) {...}
			name := n.ChildByFieldName("name")
			value := n.ChildByFieldName("value")
			if name != nil && value != nil && name.Type() == "identifier" &&
				(value.Type() == "arrow_function" || value.Type() == "function" ||
					value.Type() == "function_expression") {
				spans = append(spans, common.Span(value, src, path, name.Content(src), "", graph.KindFunc, "body"))
			}
		case "call_expression":
			if fn := n.ChildByFieldName("function"); fn != nil {
				if callee := calleeName(fn, src); callee != "" {
					calls = append(calls, common.RawCall{
						Callee: callee, Qualifier: qualifier(fn, src, class),
						Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
				}
			}
		case "new_expression":
			if ctor := n.ChildByFieldName("constructor"); ctor != nil && ctor.Type() == "identifier" {
				calls = append(calls, common.RawCall{
					Callee: ctor.Content(src), Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i), class)
		}
	}
	walk(tree.RootNode(), "")
	return common.Assemble(path, spans, calls, deps), nil
}

// calleeName takes the final name: `foo()` -> foo; `a.b.c()` -> c.
func calleeName(fn *sitter.Node, src []byte) string {
	switch fn.Type() {
	case "identifier":
		return fn.Content(src)
	case "member_expression":
		if prop := fn.ChildByFieldName("property"); prop != nil {
			return prop.Content(src)
		}
	}
	return ""
}

// qualifier extracts a lexical owner-type hint: `this.x()` -> the enclosing
// class; `Foo.x()` with an uppercase bare identifier -> candidate `Foo`
// (validated by the resolver, harmless when wrong).
func qualifier(fn *sitter.Node, src []byte, class string) string {
	if fn.Type() != "member_expression" {
		return ""
	}
	obj := fn.ChildByFieldName("object")
	if obj == nil {
		return ""
	}
	switch obj.Type() {
	case "this":
		return class
	case "identifier":
		name := obj.Content(src)
		if name != "" && name[0] >= 'A' && name[0] <= 'Z' {
			return name
		}
	}
	return ""
}

func init() { adapter.Register(Adapter{}) }
