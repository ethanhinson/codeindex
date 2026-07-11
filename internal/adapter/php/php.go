// Package php is the tree-sitter PHP adapter: functions, methods, and
// class/interface/trait declarations as symbols; function, member, scoped, and
// constructor call sites as raw name-based call edges.
package php

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"
	tsphp "github.com/smacker/go-tree-sitter/php"

	"codeindex/internal/adapter"
	"codeindex/internal/adapter/common"
	"codeindex/internal/graph"
)

// Adapter parses PHP source.
type Adapter struct{}

func (Adapter) Extensions() []string { return []string{".php"} }

func (Adapter) Parse(path string, src []byte) (*graph.ParsedFile, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(tsphp.GetLanguage())
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

	addCall := func(n *sitter.Node, name, qual string) {
		if name != "" {
			calls = append(calls, common.RawCall{
				Callee: name, Qualifier: qual,
				Line: int(n.StartPoint().Row) + 1, At: n.StartByte()})
		}
	}

	// class threads the enclosing class/interface/trait name for method
	// parents and $this-> / self:: / static:: qualification ($this in PHP
	// closures inherits the enclosing class, so the context persists).
	var fileNS string
	var walk func(n *sitter.Node, class string)
	walk = func(n *sitter.Node, class string) {
		switch n.Type() {
		case "namespace_definition":
			if nm := n.ChildByFieldName("name"); nm != nil && fileNS == "" {
				fileNS = nm.Content(src)
			}
		case "function_definition":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), "", graph.KindFunc, "body"))
			}
		case "method_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), class, graph.KindMethod, "body"))
			}
		case "class_declaration", "interface_declaration", "trait_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, common.Span(n, src, path, name.Content(src), "", graph.KindType, "body"))
				class = name.Content(src)
			}
		case "base_clause":
			// extends B (class or interface)
			for i := 0; i < int(n.NamedChildCount()); i++ {
				c := n.NamedChild(i)
				if c.Type() == "name" || c.Type() == "qualified_name" {
					addDep(n, graph.KindExtends, finalName(c, src))
				}
			}
		case "class_interface_clause":
			// implements I, J
			for i := 0; i < int(n.NamedChildCount()); i++ {
				c := n.NamedChild(i)
				if c.Type() == "name" || c.Type() == "qualified_name" {
					addDep(n, graph.KindImplements, finalName(c, src))
				}
			}
		case "namespace_use_declaration":
			// use A\B\C; — top-level imports, final segment
			var collect func(m *sitter.Node)
			collect = func(m *sitter.Node) {
				if m.Type() == "namespace_use_clause" {
					for i := 0; i < int(m.NamedChildCount()); i++ {
						c := m.NamedChild(i)
						if c.Type() == "name" || c.Type() == "qualified_name" {
							addDep(n, graph.KindImports, finalName(c, src))
							break
						}
					}
					return
				}
				for i := 0; i < int(m.NamedChildCount()); i++ {
					collect(m.NamedChild(i))
				}
			}
			collect(n)
		case "use_declaration":
			// trait use inside a class body -> implements-like
			for i := 0; i < int(n.NamedChildCount()); i++ {
				c := n.NamedChild(i)
				if c.Type() == "name" || c.Type() == "qualified_name" {
					addDep(n, graph.KindImplements, finalName(c, src))
				}
			}
		case "function_call_expression":
			// helper(...) or Qualified\Name(...) -> final name segment
			if fn := n.ChildByFieldName("function"); fn != nil {
				addCall(n, finalName(fn, src), "")
			}
		case "member_call_expression", "nullsafe_member_call_expression":
			// $this->save() qualifies to the enclosing class; other receivers
			// are dynamic (no lexical type) -> no qualifier.
			if name := n.ChildByFieldName("name"); name != nil {
				qual := ""
				if obj := n.ChildByFieldName("object"); obj != nil &&
					obj.Type() == "variable_name" && obj.Content(src) == "$this" {
					qual = class
				}
				addCall(n, name.Content(src), qual)
			}
		case "scoped_call_expression":
			// Foo::bar() -> Foo; self::/static:: -> enclosing class; parent:: -> none
			if name := n.ChildByFieldName("name"); name != nil {
				qual := ""
				if scope := n.ChildByFieldName("scope"); scope != nil {
					switch scope.Content(src) {
					case "self", "static":
						qual = class
					case "parent":
						qual = ""
					default:
						qual = finalName(scope, src)
					}
				}
				addCall(n, name.Content(src), qual)
			}
		case "object_creation_expression":
			// new Foo(...) -> Foo (first name-ish child)
			for i := 0; i < int(n.ChildCount()); i++ {
				c := n.Child(i)
				if c.Type() == "name" || c.Type() == "qualified_name" {
					addCall(n, finalName(c, src), "")
					break
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i), class)
		}
	}
	walk(tree.RootNode(), "")
	pf := common.Assemble(path, spans, calls, deps)
	pf.Namespace = fileNS
	return pf, nil
}

// finalName reduces a possibly-qualified name to its last segment:
// `App\Support\helper` -> helper.
func finalName(n *sitter.Node, src []byte) string {
	switch n.Type() {
	case "name":
		return n.Content(src)
	case "qualified_name":
		// last `name` child
		var last string
		for i := 0; i < int(n.ChildCount()); i++ {
			if c := n.Child(i); c.Type() == "name" {
				last = c.Content(src)
			}
		}
		return last
	}
	return ""
}

func init() { adapter.Register(Adapter{}) }
