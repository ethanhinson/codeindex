// internal/readmodel/graph.go
package readmodel

import (
	"path/filepath"

	"codeindex/internal/graph"
	"codeindex/internal/query"
)

// SymbolNeighborhood returns the focus symbol plus its direct callers and
// callees as a node/edge graph.
func SymbolNeighborhood(st *graph.Store, name, parent string) (Graph, error) {
	focusQ := qname(name, parent)
	focusID := symID(focusQ)
	nodes := map[string]Node{focusID: {ID: focusID, Kind: NodeSymbol, Label: focusQ}}

	defs, err := st.Definitions(name, parent)
	if err != nil {
		return Graph{}, err
	}
	if len(defs) > 0 {
		n := nodes[focusID]
		n.File, n.Line, n.Signature = defs[0].File, defs[0].StartLine, defs[0].Signature
		nodes[focusID] = n
	}

	callers, err := st.Callers(name, parent)
	if err != nil {
		return Graph{}, err
	}
	var edges []Edge
	for _, c := range callers {
		id := symID(c.QName())
		if _, ok := nodes[id]; !ok {
			nodes[id] = Node{ID: id, Kind: NodeSymbol, Label: c.QName(), File: c.File, Line: c.Line, Signature: c.Signature}
		}
		edges = append(edges, Edge{Source: id, Target: focusID, Kind: EdgeCalls, Conf: string(c.Conf)})
	}

	callees, err := st.Callees(name, parent)
	if err != nil {
		return Graph{}, err
	}
	for _, c := range callees {
		id := symID(c.QName())
		if _, ok := nodes[id]; !ok {
			nodes[id] = Node{ID: id, Kind: NodeSymbol, Label: c.QName(), File: c.DefFile, Line: c.DefLine}
		}
		edges = append(edges, Edge{Source: focusID, Target: id, Kind: EdgeCalls, Conf: string(c.Conf)})
	}

	g := Graph{Focus: focusID, Edges: edges}
	for _, n := range nodes {
		g.Nodes = append(g.Nodes, n)
	}
	sortGraph(&g)
	return g, nil
}

func openGraph(root string) (*graph.Store, error) {
	if _, err := query.Fresh(root); err != nil {
		return nil, err
	}
	return graph.Open(filepath.Join(root, ".codeindex", "graph.db"))
}
