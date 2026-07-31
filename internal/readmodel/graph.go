// internal/readmodel/graph.go
package readmodel

import (
	"codeindex/internal/graph"
	loreindex "codeindex/internal/lore/index"
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

func loreNode(r loreindex.StoredRecord) Node {
	return Node{
		ID:       r.ID,
		Kind:     NodeKind(r.Type),
		Label:    r.Title,
		Status:   r.Status,
		Priority: r.Priority,
	}
}

// RecordNeighborhood returns a lore record as focus with its anchored symbols
// and paths and its blocked_by items.
func RecordNeighborhood(rec loreindex.StoredRecord, all []loreindex.StoredRecord, st *graph.Store) (Graph, error) {
	g := Graph{Focus: rec.ID, Nodes: []Node{loreNode(rec)}}

	for _, a := range rec.Anchors {
		switch {
		case a.Symbol != "":
			name, parent := query.SplitAnchor(a.Symbol)
			defs, err := st.Definitions(name, parent)
			if err != nil {
				return Graph{}, err
			}
			label := a.Symbol
			node := Node{Kind: NodeSymbol, Label: label}
			if len(defs) > 0 {
				label = defs[0].QName()
				node.Label, node.File, node.Line, node.Signature = label, defs[0].File, defs[0].StartLine, defs[0].Signature
			}
			node.ID = symID(label)
			g.Nodes = append(g.Nodes, node)
			g.Edges = append(g.Edges, Edge{Source: rec.ID, Target: node.ID, Kind: EdgeAnchors})
		case a.Path != "":
			id := "path:" + a.Path
			g.Nodes = append(g.Nodes, Node{ID: id, Kind: NodePath, Label: a.Path, File: a.Path})
			g.Edges = append(g.Edges, Edge{Source: rec.ID, Target: id, Kind: EdgeAnchors})
		}
	}

	byID := map[string]loreindex.StoredRecord{}
	for _, r := range all {
		byID[r.ID] = r
	}
	for _, bid := range rec.BlockedBy {
		if br, ok := byID[bid]; ok {
			g.Nodes = append(g.Nodes, loreNode(br))
		} else {
			g.Nodes = append(g.Nodes, Node{ID: bid, Kind: NodeItem, Label: bid})
		}
		g.Edges = append(g.Edges, Edge{Source: rec.ID, Target: bid, Kind: EdgeBlockedBy})
	}

	sortGraph(&g)
	return g, nil
}

// AttachAnchoredLore adds, for every symbol node already in g, the lore records
// anchored to that symbol (as lore nodes) and an anchors edge lore->symbol.
func AttachAnchoredLore(g *Graph, recs []loreindex.StoredRecord) {
	present := map[string]bool{}
	var symbols []Node
	for _, n := range g.Nodes {
		present[n.ID] = true
		if n.Kind == NodeSymbol {
			symbols = append(symbols, n)
		}
	}
	for _, sym := range symbols {
		for _, r := range loreindex.RecordsForAnchor(recs, sym.Label) {
			if !present[r.ID] {
				g.Nodes = append(g.Nodes, loreNode(r))
				present[r.ID] = true
			}
			g.Edges = append(g.Edges, Edge{Source: r.ID, Target: sym.ID, Kind: EdgeAnchors})
		}
	}
	sortGraph(g)
}
