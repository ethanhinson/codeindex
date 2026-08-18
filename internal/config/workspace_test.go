package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// absTestRoot is an absolute member root, JSON-escaped for embedding in a
// manifest literal (Windows separators would otherwise break the string).
var absTestRoot = func() string {
	b, err := json.Marshal(filepath.Join(string(os.PathSeparator), "abs", "member"))
	if err != nil {
		panic(err)
	}
	return strings.Trim(string(b), `"`)
}()

// writeWorkspace writes body as the workspace manifest under a fresh temp
// workspace root and returns that root.
func writeWorkspace(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, WorkspaceFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadWorkspaceValid(t *testing.T) {
	root := writeWorkspace(t, `{
	  "version": 1,
	  "members": [
	    {"id": "api", "root": "services/api", "namespaces": ["example.com/api"]},
	    {"id": "web-1.0_x", "root": "services/web", "namespaces": ["@acme/web"], "deps": ["api"]}
	  ]
	}`)
	ws, err := LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if ws.Version != 1 || len(ws.Members) != 2 {
		t.Fatalf("got version %d, %d members", ws.Version, len(ws.Members))
	}
	if ws.Members[1].ID != "web-1.0_x" || ws.Members[1].Root != "services/web" {
		t.Fatalf("member 1 = %+v", ws.Members[1])
	}
	if len(ws.Members[1].Deps) != 1 || ws.Members[1].Deps[0] != "api" {
		t.Fatalf("deps = %v", ws.Members[1].Deps)
	}
	if len(ws.Members[0].Namespaces) != 1 || ws.Members[0].Namespaces[0] != "example.com/api" {
		t.Fatalf("namespaces = %v", ws.Members[0].Namespaces)
	}
}

// A "../" root is the D1 dedicated-workspace-directory shape and is accepted.
func TestLoadWorkspaceParentRelativeRootAccepted(t *testing.T) {
	root := writeWorkspace(t, `{
	  "version": 1,
	  "members": [{"id": "symfony", "root": "../symfony", "namespaces": []}]
	}`)
	ws, err := LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if ws.Members[0].Root != "../symfony" {
		t.Fatalf("root = %q", ws.Members[0].Root)
	}
}

// Load validates shape only: it must never stat a member root, or the frozen
// coverage-clause scenario (one member unavailable) becomes unreachable.
func TestLoadWorkspaceNeverStatsMemberRoot(t *testing.T) {
	root := writeWorkspace(t, `{
	  "version": 1,
	  "members": [
	    {"id": "gone", "root": "../nope/does-not-exist", "namespaces": ["a"]},
	    {"id": "also-gone", "root": "missing/too", "namespaces": ["b"]}
	  ]
	}`)
	if _, err := LoadWorkspace(root); err != nil {
		t.Fatalf("LoadWorkspace with nonexistent member roots: %v", err)
	}
}

// Unknown fields are ignored (plain encoding/json), matching Config's posture.
func TestLoadWorkspaceIgnoresUnknownFields(t *testing.T) {
	root := writeWorkspace(t, `{
	  "version": 1,
	  "future": {"overlay": true},
	  "members": [{"id": "a", "root": "a", "namespaces": [], "weight": 3}]
	}`)
	if _, err := LoadWorkspace(root); err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
}

func TestLoadWorkspaceMissingFile(t *testing.T) {
	_, err := LoadWorkspace(t.TempDir())
	if err == nil {
		t.Fatal("want error for missing manifest")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("errors.Is(err, fs.ErrNotExist) = false; err = %v", err)
	}
}

func TestLoadWorkspaceMalformedJSON(t *testing.T) {
	root := writeWorkspace(t, `{"version": 1, "members": [`)
	if _, err := LoadWorkspace(root); err == nil {
		t.Fatal("want error for malformed JSON")
	}
}

// Version 0 gets its own message. runtime.go:66-67 fuses its version-0 case
// with an unmarshal failure; that fusion is deliberately not copied here.
func TestLoadWorkspaceVersionAbsentIsNotFusedWithParseFailure(t *testing.T) {
	root := writeWorkspace(t, `{"members": [{"id": "a", "root": "a", "namespaces": []}]}`)
	_, err := LoadWorkspace(root)
	if err == nil {
		t.Fatal("want error for absent version")
	}
	if strings.Contains(err.Error(), "unsupported workspace version") {
		t.Fatalf("absent version reused the unsupported-version message: %v", err)
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("message does not diagnose the missing version field: %v", err)
	}
	// Must be diagnosable on its own, not reported as a parse failure.
	if _, perr := LoadWorkspace(writeWorkspace(t, `{"version": `)); perr != nil {
		if perr.Error() == err.Error() {
			t.Fatalf("absent-version error is fused with the parse failure: %v", err)
		}
	}
}

func TestLoadWorkspaceValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unsupported version",
			body: `{"version": 2, "members": [{"id": "a", "root": "a", "namespaces": []}]}`,
			want: "unsupported workspace version 2",
		},
		{
			name: "members empty",
			body: `{"version": 1, "members": []}`,
			want: "members",
		},
		{
			name: "members absent",
			body: `{"version": 1}`,
			want: "members",
		},
		{
			name: "id empty",
			body: `{"version": 1, "members": [{"id": "", "root": "a", "namespaces": []}]}`,
			want: "id",
		},
		{
			name: "id bad rune",
			body: `{"version": 1, "members": [{"id": "a:b", "root": "a", "namespaces": []}]}`,
			want: "id",
		},
		{
			name: "id slash",
			body: `{"version": 1, "members": [{"id": "a/b", "root": "a", "namespaces": []}]}`,
			want: "id",
		},
		{
			name: "duplicate id",
			body: `{"version": 1, "members": [{"id": "a", "root": "x", "namespaces": []}, {"id": "a", "root": "y", "namespaces": []}]}`,
			want: "duplicate",
		},
		{
			name: "root empty",
			body: `{"version": 1, "members": [{"id": "a", "root": "", "namespaces": []}]}`,
			want: "root",
		},
		{
			name: "root absolute",
			body: `{"version": 1, "members": [{"id": "a", "root": "` + absTestRoot + `", "namespaces": []}]}`,
			want: "root",
		},
		{
			name: "duplicate root after clean",
			body: `{"version": 1, "members": [{"id": "a", "root": "x/y", "namespaces": []}, {"id": "b", "root": "x/./y", "namespaces": []}]}`,
			want: "duplicate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadWorkspace(writeWorkspace(t, tc.body))
			if err == nil {
				t.Fatalf("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}
