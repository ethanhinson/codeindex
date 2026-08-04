package readmodel

import (
	"fmt"
	"path"
)

// pkgOf derives a cluster key from a file path: its directory, e.g.
// "internal/graph/store.go" -> "internal/graph". Root files group as "(root)".
func pkgOf(file string) string {
	d := path.Dir(file)
	if d == "." || d == "" {
		return "(root)"
	}
	return d
}

func symNodeID(id int64) string { return fmt.Sprintf("sym#%d", id) }

// FullGraph returns the entire project symbol graph: all tier-0 symbols that
// participate in a resolved call edge, plus those call edges. Symbol nodes carry
// a Group (package dir) for clustering. Isolated leaf symbols (no resolved call)
// are omitted so the call structure is not buried.
func FullGraph(root string) (Graph, error) {
	st, err := openGraph(root)
	if err != nil {
		return Graph{}, err
	}
	defer st.Close()

	syms, err := st.GraphNodes()
	if err != nil {
		return Graph{}, err
	}
	callEdges, err := st.GraphCallEdges()
	if err != nil {
		return Graph{}, err
	}

	present := make(map[int64]bool, len(syms))
	symByID := make(map[int64]graphSym, len(syms))
	for _, sy := range syms {
		present[sy.ID] = true
		qn := sy.Name
		if sy.Parent != "" {
			qn = sy.Parent + "." + sy.Name
		}
		symByID[sy.ID] = graphSym{qn: qn, file: sy.File, line: sy.StartLine, sig: sy.Signature}
	}

	g := Graph{}
	used := map[int64]bool{}

	for _, e := range callEdges {
		if present[e.Src] && present[e.Dst] && e.Src != e.Dst {
			g.Edges = append(g.Edges, Edge{Source: symNodeID(e.Src), Target: symNodeID(e.Dst), Kind: EdgeCalls})
			used[e.Src] = true
			used[e.Dst] = true
		}
	}

	for id := range used {
		sy := symByID[id]
		g.Nodes = append(g.Nodes, Node{
			ID:        symNodeID(id),
			Kind:      NodeSymbol,
			Label:     sy.qn,
			File:      sy.file,
			Line:      sy.line,
			Signature: sy.sig,
			Group:     pkgOf(sy.file),
		})
	}

	sortGraph(&g)
	return g, nil
}

type graphSym struct {
	qn, file, sig string
	line          int
}
