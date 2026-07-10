// Package python is the tree-sitter Python adapter: module-level functions,
// classes, and methods (functions lexically inside a class) as symbols; call
// sites as raw name-based call edges. Lambdas are not symbols.
package python

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"
	tspython "github.com/smacker/go-tree-sitter/python"

	"codeindex/internal/adapter"
	"codeindex/internal/adapter/common"
	"codeindex/internal/graph"
)

// Adapter parses Python source.
type Adapter struct{}

func (Adapter) Extensions() []string { return []string{".py"} }

func (Adapter) Parse(path string, src []byte) (*graph.ParsedFile, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(tspython.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	var spans []common.SymbolSpan
	var calls []common.RawCall

	// inClass tracks lexical class nesting so function_definition becomes a
	// method inside a class body and a func elsewhere.
	var walk func(n *sitter.Node, inClass bool)
	walk = func(n *sitter.Node, inClass bool) {
		enterClass := inClass
		switch n.Type() {
		case "class_definition":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), graph.KindType, "body"))
			}
			enterClass = true
		case "function_definition":
			if name := n.ChildByFieldName("name"); name != nil {
				kind := graph.KindFunc
				if inClass {
					kind = graph.KindMethod
				}
				spans = append(spans, common.Span(n, src, path, name.Content(src), kind, "body"))
			}
			enterClass = false // nested defs inside a method are plain funcs
		case "call":
			if fn := n.ChildByFieldName("function"); fn != nil {
				if callee := calleeName(fn, src); callee != "" {
					calls = append(calls, common.RawCall{
						Callee: callee, Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i), enterClass)
		}
	}
	walk(tree.RootNode(), false)
	return common.Assemble(path, spans, calls), nil
}

// calleeName takes the final name: `helper()` -> helper; `self.save()` -> save.
func calleeName(fn *sitter.Node, src []byte) string {
	switch fn.Type() {
	case "identifier":
		return fn.Content(src)
	case "attribute":
		if attr := fn.ChildByFieldName("attribute"); attr != nil {
			return attr.Content(src)
		}
	}
	return ""
}

func init() { adapter.Register(Adapter{}) }
