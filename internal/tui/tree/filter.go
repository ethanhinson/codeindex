package tree

import "strings"

// FilterTree returns a pruned copy of root: nodes that match q, plus their
// ancestors (expanded, so every match is visible). A matching node keeps its
// whole subtree, collapsed. Returns nil when nothing matches; returns root
// itself when q is empty.
func FilterTree(root *Node, q string) *Node {
	if strings.TrimSpace(q) == "" {
		return root
	}
	return filterNode(root, strings.ToLower(q))
}

func filterNode(n *Node, needle string) *Node {
	var kids []*Node
	for _, c := range n.Children {
		if matchesLower(c, needle) {
			cp := *c
			cp.Expanded = false
			cp.Children = append([]*Node(nil), c.Children...)
			kids = append(kids, &cp)
			continue
		}
		if fc := filterNode(c, needle); fc != nil {
			kids = append(kids, fc)
		}
	}
	if kids == nil {
		return nil
	}
	cp := *n
	cp.Children = kids
	cp.Expanded = true
	return &cp
}

// Matches reports whether the node matches the query (case-insensitive
// substring on the label, or on Parent.Name for symbols).
func Matches(n *Node, q string) bool {
	return matchesLower(n, strings.ToLower(q))
}

func matchesLower(n *Node, needle string) bool {
	if strings.Contains(strings.ToLower(n.Label), needle) {
		return true
	}
	if n.Kind == KindSymbol && n.SymParent != "" {
		return strings.Contains(strings.ToLower(n.SymParent+"."+n.SymName), needle)
	}
	return false
}
