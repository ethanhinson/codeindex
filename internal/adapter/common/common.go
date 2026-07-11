// Package common holds the language-independent pieces of adapter
// implementations: span bookkeeping, enclosing-symbol attribution, and
// signature formatting. Adapters stay per-language; this stays mechanical.
package common

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"codeindex/internal/graph"
)

const maxSignature = 160

// SymbolSpan tracks a symbol plus its byte range so call sites can be
// attributed to their innermost enclosing definition.
type SymbolSpan struct {
	Sym        graph.Symbol
	Start, End uint32
}

// RawCall is a call site before enclosing attribution. Qualifier is the
// lexical owner-type hint (validated by the resolver; "" = none).
type RawCall struct {
	Callee    string
	Qualifier string
	Line      int
	At        uint32
}

// Enclosing returns the index of the innermost symbol whose byte range
// contains pos, or -1 if none.
func Enclosing(spans []SymbolSpan, pos uint32) int {
	best, bestSize := -1, ^uint32(0)
	for i, s := range spans {
		if s.Start <= pos && pos < s.End {
			if size := s.End - s.Start; size < bestSize {
				best, bestSize = i, size
			}
		}
	}
	return best
}

// DepSite is a dependency fact before enclosing attribution: imports sit at
// top level (attribute to -1 = file); extends/implements sit inside their
// class span (attribute to the class).
type DepSite struct {
	Kind   graph.EdgeKind
	Target string
	Line   int
	At     uint32
}

// Assemble builds a ParsedFile from collected spans, calls, and dep sites.
func Assemble(path string, spans []SymbolSpan, calls []RawCall, deps []DepSite) *graph.ParsedFile {
	pf := &graph.ParsedFile{Path: path}
	for _, s := range spans {
		pf.Symbols = append(pf.Symbols, s.Sym)
	}
	for _, c := range calls {
		pf.Calls = append(pf.Calls, graph.RawCall{
			EnclosingIdx: Enclosing(spans, c.At),
			Callee:       c.Callee,
			Qualifier:    c.Qualifier,
			Line:         c.Line,
		})
	}
	for _, d := range deps {
		pf.Deps = append(pf.Deps, graph.RawDep{
			EnclosingIdx: Enclosing(spans, d.At),
			Kind:         d.Kind,
			Target:       d.Target,
			Line:         d.Line,
		})
	}
	return pf
}

// Signature returns a declaration's text up to its body field ("func Foo(...)"),
// falling back to the first line.
func Signature(n *sitter.Node, src []byte, bodyField string) string {
	if body := n.ChildByFieldName(bodyField); body != nil && body.StartByte() > n.StartByte() {
		return Clip(strings.TrimSpace(string(src[n.StartByte():body.StartByte()])))
	}
	return Clip(FirstLine(n.Content(src)))
}

// FirstLine truncates s at its first newline.
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Clip trims and bounds a signature for display.
func Clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxSignature {
		return s[:maxSignature]
	}
	return s
}

// Span makes a SymbolSpan for a definition node with the given name, parent
// (owner type, "" for top-level), and kind.
func Span(n *sitter.Node, src []byte, path, name, parent string, kind graph.SymbolKind, bodyField string) SymbolSpan {
	return SymbolSpan{
		Sym: graph.Symbol{
			File:      path,
			Name:      name,
			Parent:    parent,
			Kind:      kind,
			Signature: Signature(n, src, bodyField),
			StartLine: int(n.StartPoint().Row) + 1,
			EndLine:   int(n.EndPoint().Row) + 1,
		},
		Start: n.StartByte(),
		End:   n.EndByte(),
	}
}
