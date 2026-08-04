// internal/webserver/server.go
// Package webserver serves the codeindex symbol read model over HTTP as a
// headless, versioned JSON API. Read-only; bind to loopback only. No static
// content is hosted.
package webserver

import (
	"encoding/json"
	"log"
	"net/http"

	"codeindex/internal/readmodel"
)

// graphResponse wraps a symbol graph with the top-level schemaVersion pinned on
// every graph API response.
type graphResponse struct {
	SchemaVersion string `json:"schemaVersion"`
	readmodel.Graph
}

func newGraphResponse(g readmodel.Graph) graphResponse {
	return graphResponse{SchemaVersion: readmodel.SchemaVersion, Graph: g}
}

// New returns the HTTP handler for the read-only headless graph API.
func New(root, version string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "version": version, "root": root,
		})
	})

	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			http.Error(w, "missing required query param: symbol", http.StatusBadRequest)
			return
		}
		parent := r.URL.Query().Get("parent")
		st, err := openGraph(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer st.Close()
		g, err := readmodel.SymbolNeighborhood(st, symbol, parent)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, newGraphResponse(g))
	})

	mux.HandleFunc("/api/graph/full", func(w http.ResponseWriter, _ *http.Request) {
		g, err := readmodel.FullGraph(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, newGraphResponse(g))
	})

	return mux
}

// Run serves on addr until the process is stopped. addr must be loopback.
func Run(root, addr, version string) error {
	log.Printf("codeindex serve: http://%s (root %s)", addr, root)
	return http.ListenAndServe(addr, New(root, version))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
