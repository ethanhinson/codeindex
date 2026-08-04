// internal/webserver/server_test.go
package webserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writeRepo creates a temp repo with code files only (no lore).
func writeRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"),
		[]byte("package p\nfunc Helper(x int) int { return x + 1 }\nfunc A() int { return Helper(1) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHealthEndpoint(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != "test" {
		t.Fatalf("body = %+v", body)
	}
}

func TestGraphEndpointSymbolOnly(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/graph?symbol=A")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var g struct {
		SchemaVersion string `json:"schemaVersion"`
		Focus         string `json:"focus"`
		Nodes         []struct {
			ID, Kind, Label string
		} `json:"nodes"`
		Edges []struct {
			Source, Target, Kind string
		} `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.SchemaVersion != "1" {
		t.Fatalf("schemaVersion = %q, want 1", g.SchemaVersion)
	}
	if g.Focus != "sym:A" {
		t.Fatalf("focus = %q, want sym:A", g.Focus)
	}
	// Every node is a symbol; the focus and its callee Helper are present.
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Kind != "symbol" {
			t.Fatalf("non-symbol node kind %q in symbol-only API: %+v", n.Kind, n)
		}
		ids[n.ID] = true
	}
	if !ids["sym:A"] || !ids["sym:Helper"] {
		t.Fatalf("expected sym:A and sym:Helper nodes; got %v", ids)
	}
	// The focus calls Helper.
	var hasCallEdge bool
	for _, e := range g.Edges {
		if e.Source == "sym:A" && e.Target == "sym:Helper" && e.Kind == "calls" {
			hasCallEdge = true
		}
		if e.Kind != "calls" {
			t.Fatalf("non-call edge kind %q in symbol-only API: %+v", e.Kind, e)
		}
	}
	if !hasCallEdge {
		t.Fatalf("expected sym:A -> sym:Helper calls edge; edges=%+v", g.Edges)
	}
}

func TestFullGraphEndpointSymbolOnly(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/graph/full")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var g struct {
		SchemaVersion string `json:"schemaVersion"`
		Nodes         []struct {
			ID, Kind, Label, Group string
		} `json:"nodes"`
		Edges []struct {
			Source, Target, Kind string
		} `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.SchemaVersion != "1" {
		t.Fatalf("schemaVersion = %q, want 1", g.SchemaVersion)
	}
	var hasHelper bool
	for _, n := range g.Nodes {
		if n.Kind != "symbol" {
			t.Fatalf("non-symbol node kind %q in symbol-only full graph: %+v", n.Kind, n)
		}
		if n.Label == "Helper" {
			hasHelper = true
		}
	}
	if !hasHelper {
		t.Errorf("expected a Helper symbol node; nodes=%+v", g.Nodes)
	}
	for _, e := range g.Edges {
		if e.Kind != "calls" {
			t.Fatalf("non-call edge kind %q in symbol-only full graph: %+v", e.Kind, e)
		}
	}
}

func TestGraphEndpointMissingSymbol(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/graph")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRootPath404_NoStaticHosting(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("root path status = %d, want 404 (static hosting must be gone)", resp.StatusCode)
	}
}
