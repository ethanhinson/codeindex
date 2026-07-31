// internal/webserver/server_test.go
package webserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/lore"
)

func writeRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("CODEINDEX_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"),
		[]byte("package p\nfunc Helper(x int) int { return x + 1 }\nfunc A() int { return Helper(1) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := lore.Record{
		ID: "dec-A", Type: lore.TypeDecision, Title: "Keep Helper pure",
		Status: "active", Date: "2026-07-29",
		Anchors: []lore.Anchor{{Symbol: "Helper"}},
	}
	b, err := rec.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".lore", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), b, 0o644); err != nil {
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

func TestGraphEndpoint(t *testing.T) {
	srv := httptest.NewServer(New(writeRepo(t), "test"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/graph?focus=sym:Helper")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var g struct {
		Focus string `json:"focus"`
		Edges []struct {
			Source, Target, Kind string
		} `json:"edges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.Focus != "sym:Helper" {
		t.Fatalf("focus = %q", g.Focus)
	}
	var joined bool
	for _, e := range g.Edges {
		if e.Source == "dec-A" && e.Target == "sym:Helper" && e.Kind == "anchors" {
			joined = true
		}
	}
	if !joined {
		t.Fatalf("expected code+lore join edge; edges=%+v", g.Edges)
	}
}

func TestGraphEndpointMissingFocus(t *testing.T) {
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
