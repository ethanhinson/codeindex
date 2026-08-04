// internal/readmodel/model.go
// Package readmodel converts the codeindex call graph into a single
// JSON-serializable node/edge graph consumed by the headless graph API and CLI.
package readmodel

import "sort"

// SchemaVersion is the top-level version pinned on every graph API response so
// external consumers are insulated from internal shape changes.
const SchemaVersion = "1"

type NodeKind string

const NodeSymbol NodeKind = "symbol"

type EdgeKind string

const EdgeCalls EdgeKind = "calls"

type Node struct {
	ID        string   `json:"id"`
	Kind      NodeKind `json:"kind"`
	Label     string   `json:"label"`
	File      string   `json:"file,omitempty"`
	Line      int      `json:"line,omitempty"`
	Signature string   `json:"signature,omitempty"`
	Group     string   `json:"group,omitempty"` // cluster key (package dir)
}

type Edge struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Kind   EdgeKind `json:"kind"`
	Conf   string   `json:"conf,omitempty"`
}

type Graph struct {
	Focus string `json:"focus"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

func symID(qn string) string { return "sym:" + qn }

func qname(name, parent string) string {
	if parent != "" {
		return parent + "." + name
	}
	return name
}

// sortGraph orders nodes by ID and edges by (source,target,kind) for
// deterministic output.
func sortGraph(g *Graph) {
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Kind < b.Kind
	})
}
