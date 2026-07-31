// internal/webserver/server.go
// Package webserver serves the codeindex read model over HTTP and hosts the
// embedded SPA. Read-only; bind to loopback only.
package webserver

import (
	"encoding/json"
	"log"
	"net/http"

	"codeindex/internal/readmodel"
)

// New returns the HTTP handler for the read-only graph API and static SPA.
func New(root, version string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "version": version, "root": root,
		})
	})

	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		focus := r.URL.Query().Get("focus")
		if focus == "" {
			http.Error(w, "missing required query param: focus", http.StatusBadRequest)
			return
		}
		g, err := readmodel.Neighborhood(root, focus)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, g)
	})

	mux.Handle("/", staticHandler())

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
