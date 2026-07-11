// Package tree renders the index as an explorable directory → file → symbol
// tree: pure tree logic here, the Bubble Tea UI in model.go/view.go.
package tree

import (
	"sort"
	"strings"

	"codeindex/internal/graph"
)

type NodeKind int

const (
	KindDir NodeKind = iota
	KindFile
	KindSymbol
)

type Node struct {
	Label     string
	Kind      NodeKind
	SymKind   string
	File      string
	Line      int
	Signature string
	SymName   string
	SymParent string
	Children  []*Node
	Expanded  bool
}

// BuildTree arranges symbols into directories → files → symbols, nesting a
// method under its parent type when that type is defined in the same file.
func BuildTree(syms []graph.Symbol) *Node {
	root := &Node{Kind: KindDir, Expanded: true}
	dirs := map[string]*Node{"": root}
	files := map[string]*Node{}

	var dirFor func(path string) *Node
	dirFor = func(path string) *Node {
		if n, ok := dirs[path]; ok {
			return n
		}
		parent, base := splitPath(path)
		n := &Node{Label: base, Kind: KindDir}
		dirs[path] = n
		p := dirFor(parent)
		p.Children = append(p.Children, n)
		return n
	}
	fileFor := func(path string) *Node {
		if n, ok := files[path]; ok {
			return n
		}
		parent, base := splitPath(path)
		n := &Node{Label: base, Kind: KindFile, File: path}
		files[path] = n
		d := dirFor(parent)
		d.Children = append(d.Children, n)
		return n
	}

	// First pass: top-level symbols; remember them for member nesting.
	byFileName := map[string]*Node{}
	for i := range syms {
		s := &syms[i]
		if s.Parent != "" {
			continue
		}
		n := symbolNode(s)
		fileFor(s.File).Children = append(fileFor(s.File).Children, n)
		byFileName[s.File+"\x00"+s.Name] = n
	}
	// Second pass: members nest under their type when it exists in the file.
	for i := range syms {
		s := &syms[i]
		if s.Parent == "" {
			continue
		}
		n := symbolNode(s)
		if t, ok := byFileName[s.File+"\x00"+s.Parent]; ok {
			t.Children = append(t.Children, n)
		} else {
			n.Label = s.Parent + "." + s.Name
			fileFor(s.File).Children = append(fileFor(s.File).Children, n)
		}
	}

	sortTree(root)
	return root
}

func symbolNode(s *graph.Symbol) *Node {
	return &Node{
		Label: s.Name, Kind: KindSymbol, SymKind: string(s.Kind),
		File: s.File, Line: s.StartLine, Signature: s.Signature,
		SymName: s.Name, SymParent: s.Parent,
	}
}

// splitPath splits a repo-relative slash path into parent dir and base name.
func splitPath(p string) (dir, base string) {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i], p[i+1:]
	}
	return "", p
}

// sortTree orders children: dirs, then files (each alphabetical), then
// symbols by line.
func sortTree(n *Node) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Kind == KindSymbol {
			return a.Line < b.Line
		}
		return a.Label < b.Label
	})
	for _, c := range n.Children {
		sortTree(c)
	}
}
