package tree

// Row is one drawable line of the tree: a node at a depth.
type Row struct {
	Node  *Node
	Depth int
}

// Visible returns the rows a renderer should draw: a preorder walk that
// descends only into expanded nodes. The virtual root is not a row.
func Visible(root *Node) []Row {
	var out []Row
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		for _, c := range n.Children {
			out = append(out, Row{Node: c, Depth: depth})
			if c.Expanded && len(c.Children) > 0 {
				walk(c, depth+1)
			}
		}
	}
	walk(root, 0)
	return out
}

// ParentIndex returns the row index of rows[i]'s parent, or -1 when rows[i]
// is top-level. In a preorder flattening the parent is the nearest earlier
// row with a smaller depth.
func ParentIndex(rows []Row, i int) int {
	for j := i - 1; j >= 0; j-- {
		if rows[j].Depth < rows[i].Depth {
			return j
		}
	}
	return -1
}
