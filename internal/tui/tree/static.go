package tree

import (
	"fmt"
	"strings"
)

// Static renders the fully expanded tree as plain indented text — the
// non-TTY output of the tree command.
func Static(root *Node) string {
	var b strings.Builder
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		for _, c := range n.Children {
			b.WriteString(strings.Repeat("  ", depth))
			switch c.Kind {
			case KindDir:
				b.WriteString(c.Label + "/")
			case KindFile:
				b.WriteString(c.Label)
			case KindSymbol:
				fmt.Fprintf(&b, "%s  %s  :%d", c.Label, c.SymKind, c.Line)
			}
			b.WriteByte('\n')
			walk(c, depth+1)
		}
	}
	walk(root, 0)
	return b.String()
}
