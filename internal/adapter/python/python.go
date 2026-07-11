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
	var deps []common.DepSite

	addDep := func(n *sitter.Node, kind graph.EdgeKind, target string) {
		if target != "" {
			deps = append(deps, common.DepSite{
				Kind: kind, Target: target,
				Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
		}
	}

	// Two lexical contexts: defClass = the class whose BODY we are directly in
	// (decides method kind/parent; cleared inside function bodies), and
	// selfClass = the class `self`/`cls` refers to (set on entering a method,
	// retained through nested closures — Python closures capture self).
	var walk func(n *sitter.Node, defClass, selfClass string)
	walk = func(n *sitter.Node, defClass, selfClass string) {
		nextDef, nextSelf := defClass, selfClass
		switch n.Type() {
		case "class_definition":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), "", graph.KindType, "body"))
				nextDef = name.Content(src)
			}
			// class A(B, C): bases are extends edges (attributed to A's span)
			if sup := n.ChildByFieldName("superclasses"); sup != nil {
				for i := 0; i < int(sup.NamedChildCount()); i++ {
					c := sup.NamedChild(i)
					if c.Type() == "identifier" {
						addDep(sup, graph.KindExtends, c.Content(src))
					}
				}
			}
		case "import_statement":
			// import a.b, c — full dotted names, unresolved by nature
			for i := 0; i < int(n.NamedChildCount()); i++ {
				c := n.NamedChild(i)
				if c.Type() == "dotted_name" {
					addDep(n, graph.KindImports, c.Content(src))
				} else if c.Type() == "aliased_import" {
					if nm := c.ChildByFieldName("name"); nm != nil {
						addDep(n, graph.KindImports, nm.Content(src))
					}
				}
			}
		case "import_from_statement":
			// from m import a, b — the imported names (resolvable in-repo)
			seenModule := false
			for i := 0; i < int(n.NamedChildCount()); i++ {
				c := n.NamedChild(i)
				if c.Type() == "dotted_name" || c.Type() == "relative_import" {
					if !seenModule {
						seenModule = true // first dotted_name is the module
						continue
					}
					addDep(n, graph.KindImports, c.Content(src))
				} else if c.Type() == "aliased_import" {
					if nm := c.ChildByFieldName("name"); nm != nil {
						addDep(n, graph.KindImports, nm.Content(src))
					}
				}
			}
		case "function_definition":
			if name := n.ChildByFieldName("name"); name != nil {
				kind := graph.KindFunc
				if defClass != "" {
					kind = graph.KindMethod
					nextSelf = defClass // self inside this method = defClass
				}
				spans = append(spans, common.Span(n, src, path, name.Content(src), defClass, kind, "body"))
			}
			nextDef = "" // nested defs inside a function body are plain funcs
		case "call":
			if fn := n.ChildByFieldName("function"); fn != nil {
				if callee := calleeName(fn, src); callee != "" {
					calls = append(calls, common.RawCall{
						Callee: callee, Qualifier: qualifier(fn, src, selfClass),
						Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i), nextDef, nextSelf)
		}
	}
	walk(tree.RootNode(), "", "")
	return common.Assemble(path, spans, calls, deps), nil
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

// qualifier extracts a lexical owner-type hint: `self.x()`/`cls.x()` -> the
// enclosing class; `Foo.x()` with an uppercase identifier -> candidate `Foo`.
func qualifier(fn *sitter.Node, src []byte, class string) string {
	if fn.Type() != "attribute" {
		return ""
	}
	obj := fn.ChildByFieldName("object")
	if obj == nil || obj.Type() != "identifier" {
		return ""
	}
	switch name := obj.Content(src); name {
	case "self", "cls":
		return class
	default:
		if name != "" && name[0] >= 'A' && name[0] <= 'Z' {
			return name
		}
	}
	return ""
}

func init() { adapter.Register(Adapter{}) }
