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

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "function_declaration", "generator_function_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), graph.KindFunc, "body"))
			}
		case "class_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), graph.KindType, "body"))
			}
		case "method_definition":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), graph.KindMethod, "body"))
			}
		case "variable_declarator":
			// const f = () => {...} / const f = function(...) {...}
			name := n.ChildByFieldName("name")
			value := n.ChildByFieldName("value")
			if name != nil && value != nil && name.Type() == "identifier" &&
				(value.Type() == "arrow_function" || value.Type() == "function" ||
					value.Type() == "function_expression") {
				spans = append(spans, common.Span(value, src, path, name.Content(src), graph.KindFunc, "body"))
			}
		case "call_expression":
			if fn := n.ChildByFieldName("function"); fn != nil {
				if callee := calleeName(fn, src); callee != "" {
					calls = append(calls, common.RawCall{
						Callee: callee, Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
				}
			}
		case "new_expression":
			if ctor := n.ChildByFieldName("constructor"); ctor != nil && ctor.Type() == "identifier" {
				calls = append(calls, common.RawCall{
					Callee: ctor.Content(src), Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())
	return common.Assemble(path, spans, calls), nil
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

func init() { adapter.Register(Adapter{}) }
