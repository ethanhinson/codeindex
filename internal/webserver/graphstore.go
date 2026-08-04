// internal/webserver/graphstore.go
package webserver

import (
	"path/filepath"

	"codeindex/internal/graph"
	"codeindex/internal/query"
)

// openGraph freshens the index for root and opens its symbol graph store. The
// caller owns Close.
func openGraph(root string) (*graph.Store, error) {
	if _, err := query.Fresh(root); err != nil {
		return nil, err
	}
	return graph.Open(filepath.Join(root, ".codeindex", "graph.db"))
}
