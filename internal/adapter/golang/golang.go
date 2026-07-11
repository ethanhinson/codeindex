// Package golang is the tree-sitter Go language adapter for the walking skeleton.
// It extracts function/method/type definitions as symbols and call sites as raw
// name-based call edges. Resolution (name -> symbol) happens later in the engine.
package golang

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	tsgolang "github.com/smacker/go-tree-sitter/golang"

	"codeindex/internal/adapter"
	"codeindex/internal/graph"
)

const maxSignature = 160

// Adapter parses Go source with tree-sitter.
type Adapter struct{}

func (Adapter) Extensions() []string { return []string{".go"} }

// symbolSpan tracks a symbol plus its byte range so call sites can be attributed
// to their enclosing definition.
type symbolSpan struct {
	sym        graph.Symbol
	start, end uint32
}

// Parse walks the tree once to collect symbols (with byte ranges) and once to
// collect call sites, then attributes each call to its innermost enclosing symbol.
func (Adapter) Parse(path string, src []byte) (*graph.ParsedFile, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(tsgolang.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()
	root := tree.RootNode()

	var spans []symbolSpan
	var rawCalls []struct {
		callee    string
		qualifier string
		line      int
		at        uint32
	}
	var rawDeps []struct {
		kind   graph.EdgeKind
		target string
		line   int
		at     uint32
	}
	addDep := func(n *sitter.Node, kind graph.EdgeKind, target string) {
		if target != "" {
			rawDeps = append(rawDeps, struct {
				kind   graph.EdgeKind
				target string
				line   int
				at     uint32
			}{kind, target, int(n.StartPoint().Row) + 1, n.StartByte()})
		}
	}

	// recvCtx carries the enclosing method's receiver (variable name -> type)
	// so calls through the receiver variable qualify to the receiver type —
	// Go's lexical equivalent of `this`.
	type recvCtx struct{ varName, typeName string }

	var walk func(n *sitter.Node, recv recvCtx)
	walk = func(n *sitter.Node, recv recvCtx) {
		switch n.Type() {
		case "function_declaration", "method_declaration":
			if name := n.ChildByFieldName("name"); name != nil {
				kind := graph.KindFunc
				parent := ""
				if n.Type() == "method_declaration" {
					kind = graph.KindMethod
					var rv string
					rv, parent = receiver(n, src)
					recv = recvCtx{varName: rv, typeName: parent}
				} else {
					recv = recvCtx{}
				}
				spans = append(spans, symbolSpan{
					sym: graph.Symbol{
						File:      path,
						Name:      name.Content(src),
						Parent:    parent,
						Kind:      kind,
						Signature: signature(n, src),
						StartLine: int(n.StartPoint().Row) + 1,
						EndLine:   int(n.EndPoint().Row) + 1,
					},
					start: n.StartByte(),
					end:   n.EndByte(),
				})
			}
		case "import_spec":
			// import "path/to/pkg" — verbatim path, file-level, unresolved
			if pth := n.ChildByFieldName("path"); pth != nil {
				addDep(n, graph.KindImports, strings.Trim(pth.Content(src), "\"`"))
			}
		case "field_declaration":
			// struct embedding: a field with a type but no name is an
			// extends-like edge from the enclosing struct type.
			if n.ChildByFieldName("name") == nil {
				if t := n.ChildByFieldName("type"); t != nil {
					addDep(n, graph.KindExtends, embeddedTypeName(t, src))
				}
			}
		case "type_spec":
			if name := n.ChildByFieldName("name"); name != nil {
				spans = append(spans, symbolSpan{
					sym: graph.Symbol{
						File:      path,
						Name:      name.Content(src),
						Kind:      graph.KindType,
						Signature: clip(firstLine(n.Content(src))),
						StartLine: int(n.StartPoint().Row) + 1,
						EndLine:   int(n.EndPoint().Row) + 1,
					},
					start: n.StartByte(),
					end:   n.EndByte(),
				})
			}
		case "call_expression":
			if fn := n.ChildByFieldName("function"); fn != nil {
				if callee := calleeName(fn, src); callee != "" {
					qual := ""
					// w.scale() inside func (w Widget) ... -> qualifier Widget
					if fn.Type() == "selector_expression" {
						if op := fn.ChildByFieldName("operand"); op != nil &&
							op.Type() == "identifier" && recv.varName != "" &&
							op.Content(src) == recv.varName {
							qual = recv.typeName
						}
					}
					rawCalls = append(rawCalls, struct {
						callee    string
						qualifier string
						line      int
						at        uint32
					}{callee, qual, int(n.StartPoint().Row) + 1, n.StartByte()})
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i), recv)
		}
	}
	walk(root, recvCtx{})

	pf := &graph.ParsedFile{Path: path}
	for _, s := range spans {
		pf.Symbols = append(pf.Symbols, s.sym)
	}
	for _, c := range rawCalls {
		pf.Calls = append(pf.Calls, graph.RawCall{
			EnclosingIdx: enclosing(spans, c.at),
			Callee:       c.callee,
			Qualifier:    c.qualifier,
			Line:         c.line,
		})
	}
	for _, d := range rawDeps {
		pf.Deps = append(pf.Deps, graph.RawDep{
			EnclosingIdx: enclosing(spans, d.at),
			Kind:         d.kind,
			Target:       d.target,
			Line:         d.line,
		})
	}
	return pf, nil
}

// embeddedTypeName reduces an embedded field's type to its type name:
// `B` -> B, `*B` -> B, `pkg.B` -> B, generics stripped.
func embeddedTypeName(t *sitter.Node, src []byte) string {
	for t != nil {
		switch t.Type() {
		case "pointer_type":
			t = t.NamedChild(0)
		case "generic_type":
			t = t.ChildByFieldName("type")
		case "qualified_type":
			t = t.ChildByFieldName("name")
		case "type_identifier":
			return t.Content(src)
		default:
			return ""
		}
	}
	return ""
}

// receiver extracts a method's receiver variable name and type name
// (`func (w *Widget) ...` -> "w", "Widget"; pointer and generic brackets
// stripped). Empty strings when absent (e.g. `func (*Widget) ...`).
func receiver(n *sitter.Node, src []byte) (varName, typeName string) {
	recv := n.ChildByFieldName("receiver")
	if recv == nil || recv.NamedChildCount() == 0 {
		return "", ""
	}
	decl := recv.NamedChild(0) // parameter_declaration
	if nm := decl.ChildByFieldName("name"); nm != nil {
		varName = nm.Content(src)
	}
	t := decl.ChildByFieldName("type")
	for t != nil {
		switch t.Type() {
		case "pointer_type":
			t = t.NamedChild(0)
		case "generic_type":
			t = t.ChildByFieldName("type")
		case "type_identifier":
			return varName, t.Content(src)
		default:
			return varName, ""
		}
	}
	return varName, ""
}

// calleeName extracts the called name: the identifier for `Foo()`, or the field
// for `x.Method()` / `pkg.Foo()` (name-based resolution keys on the final name).
func calleeName(fn *sitter.Node, src []byte) string {
	switch fn.Type() {
	case "identifier":
		return fn.Content(src)
	case "selector_expression":
		if field := fn.ChildByFieldName("field"); field != nil {
			return field.Content(src)
		}
	}
	return ""
}

// enclosing returns the index of the innermost symbol whose byte range contains
// pos, or -1 if none (top-level call, e.g. a package-level var initializer).
func enclosing(spans []symbolSpan, pos uint32) int {
	best, bestSize := -1, ^uint32(0)
	for i, s := range spans {
		if s.start <= pos && pos < s.end {
			if size := s.end - s.start; size < bestSize {
				best, bestSize = i, size
			}
		}
	}
	return best
}

// signature returns a func/method declaration up to its body ("func Foo(a int) error").
func signature(n *sitter.Node, src []byte) string {
	if body := n.ChildByFieldName("body"); body != nil {
		return clip(strings.TrimSpace(string(src[n.StartByte():body.StartByte()])))
	}
	return clip(firstLine(n.Content(src)))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxSignature {
		return s[:maxSignature]
	}
	return s
}

func init() { adapter.Register(Adapter{}) }
