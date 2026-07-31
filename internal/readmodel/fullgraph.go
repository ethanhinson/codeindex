package readmodel

import (
	"fmt"
	"path"
	"strings"
)

// generatedDirs are path segments whose contents are compiled/vendored, not
// authored source — indexing them (e.g. a minified JS bundle) floods the graph
// with meaningless symbols.
var generatedDirs = []string{"/dist/", "/node_modules/", "/vendor/", "/build/", "/.next/", "/out/", "/.svelte-kit/"}

// isGenerated reports whether a repo-relative file is compiled/vendored output.
func isGenerated(file string) bool {
	p := "/" + file
	for _, seg := range generatedDirs {
		if strings.Contains(p, seg) {
			return true
		}
	}
	return strings.HasSuffix(file, ".min.js") || strings.HasSuffix(file, ".min.css")
}

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

// FullGraph returns the entire project symbol graph (all tier-0 symbols and
// resolved call edges) with the lore layer overlaid: lore records as nodes,
// anchors edges to the symbols they reference, and blocked_by edges between
// records. Symbol nodes carry a Group (package dir) for clustering.
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
	recs, err := openLore(root)
	if err != nil {
		return Graph{}, err
	}

	// Index all symbols by id and by name, but do not emit nodes yet: we only
	// keep symbols that participate in the graph (a resolved call or a lore
	// anchor). Thousands of isolated leaf symbols would otherwise bury the
	// actual call structure.
	present := make(map[int64]bool, len(syms))
	symByID := make(map[int64]graphSym, len(syms))
	byQName := map[string][]int64{}
	byName := map[string][]int64{}
	for _, sy := range syms {
		if isGenerated(sy.File) {
			continue // skip compiled/vendored output (minified bundles, etc.)
		}
		present[sy.ID] = true
		qn := sy.Name
		if sy.Parent != "" {
			qn = sy.Parent + "." + sy.Name
		}
		symByID[sy.ID] = graphSym{qn: qn, file: sy.File, line: sy.StartLine, sig: sy.Signature}
		byQName[qn] = append(byQName[qn], sy.ID)
		byName[sy.Name] = append(byName[sy.Name], sy.ID)
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

	recPresent := make(map[string]bool, len(recs))
	for _, r := range recs {
		recPresent[r.ID] = true
		ln := loreNode(r)
		ln.Group = "lore"
		g.Nodes = append(g.Nodes, ln)
	}
	for _, r := range recs {
		for _, a := range r.Anchors {
			if a.Symbol == "" {
				continue
			}
			targets := byQName[a.Symbol]
			if len(targets) == 0 {
				targets = byName[a.Symbol]
			}
			for _, id := range targets {
				g.Edges = append(g.Edges, Edge{Source: r.ID, Target: symNodeID(id), Kind: EdgeAnchors})
				used[id] = true
			}
		}
		for _, b := range r.BlockedBy {
			if recPresent[b] {
				g.Edges = append(g.Edges, Edge{Source: r.ID, Target: b, Kind: EdgeBlockedBy})
			}
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
