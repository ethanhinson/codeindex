package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/config"
)

// writeFile creates a file (and its parents) under dir.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// wsShape builds the dedicated-workspace-directory shape: an empty workspace
// dir whose single member lives at ../go-member, a Go module.
func wsShape(t *testing.T) (wsRoot string) {
	t.Helper()
	base := t.TempDir()
	wsRoot = filepath.Join(base, "ws")
	if err := os.MkdirAll(wsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(base, "go-member", "go.mod"), "module example.com/gomember\n\ngo 1.22\n")
	return wsRoot
}

func authoredSkeleton(t *testing.T, wsRoot string) {
	t.Helper()
	if err := config.SaveWorkspace(wsRoot, &config.Workspace{
		Version: 1,
		Members: []config.Member{{ID: "go-member", Root: "../go-member", Namespaces: []string{}}},
	}); err != nil {
		t.Fatal(err)
	}
}

// End-to-end: --scan --force over an authored skeleton fills namespaces.
func TestInitWorkspaceScanFillsNamespaces(t *testing.T) {
	wsRoot := wsShape(t)
	authoredSkeleton(t, wsRoot)

	if err := runInitWorkspace(wsRoot, []string{"--scan", "--force"}); err != nil {
		t.Fatalf("runInitWorkspace: %v", err)
	}

	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if ws.Version != 1 || len(ws.Members) != 1 {
		t.Fatalf("got %+v", ws)
	}
	m := ws.Members[0]
	if m.ID != "go-member" || m.Root != "../go-member" {
		t.Fatalf("identity rewritten: %+v", m)
	}
	if len(m.Namespaces) != 1 || m.Namespaces[0] != "example.com/gomember" {
		t.Fatalf("namespaces not filled: %+v", m.Namespaces)
	}
}

// --scan is required.
func TestInitWorkspaceRequiresScan(t *testing.T) {
	wsRoot := wsShape(t)
	authoredSkeleton(t, wsRoot)

	err := runInitWorkspace(wsRoot, []string{})
	if err == nil {
		t.Fatal("want usage error without --scan")
	}
	if !strings.Contains(err.Error(), "--scan") {
		t.Fatalf("message must name --scan: %v", err)
	}
}

// An existing manifest without --force is refused, and left untouched.
func TestInitWorkspaceRefusesExistingWithoutForce(t *testing.T) {
	wsRoot := wsShape(t)
	authoredSkeleton(t, wsRoot)
	before, err := os.ReadFile(filepath.Join(wsRoot, config.WorkspaceFile))
	if err != nil {
		t.Fatal(err)
	}

	err = runInitWorkspace(wsRoot, []string{"--scan"})
	if err == nil {
		t.Fatal("want refusal when a manifest already exists")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("message must point at --force: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(wsRoot, config.WorkspaceFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("refused run rewrote the manifest")
	}
}

// No manifest at all: the scan writes one from discovered members (a monorepo
// root declaring its members via go.work).
func TestInitWorkspaceNoManifestWritesDiscovered(t *testing.T) {
	wsRoot := t.TempDir()
	writeFile(t, filepath.Join(wsRoot, "go.work"), "go 1.22\n\nuse ./svc\n")
	writeFile(t, filepath.Join(wsRoot, "svc", "go.mod"), "module example.com/svc\n\ngo 1.22\n")

	if err := runInitWorkspace(wsRoot, []string{"--scan"}); err != nil {
		t.Fatalf("runInitWorkspace: %v", err)
	}
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if ws.Version != 1 {
		t.Fatalf("version = %d, want 1", ws.Version)
	}
	if len(ws.Members) != 1 || ws.Members[0].ID != "svc" {
		t.Fatalf("got %+v", ws.Members)
	}
	if len(ws.Members[0].Namespaces) != 1 || ws.Members[0].Namespaces[0] != "example.com/svc" {
		t.Fatalf("namespaces = %+v", ws.Members[0].Namespaces)
	}
}

// A workspace with nothing to find is an error, not an empty manifest.
func TestInitWorkspaceEmptyResultIsError(t *testing.T) {
	wsRoot := t.TempDir()
	if err := runInitWorkspace(wsRoot, []string{"--scan"}); err == nil {
		t.Fatal("want the found-nothing error")
	}
	if _, err := os.Stat(filepath.Join(wsRoot, config.WorkspaceFile)); !os.IsNotExist(err) {
		t.Fatalf("a failed scan wrote a manifest: %v", err)
	}
}

// The usage verb list advertises the new verb.
func TestUsageListsInitWorkspace(t *testing.T) {
	if !strings.Contains(usageVerbs, "init-workspace") {
		t.Fatalf("usage string omits init-workspace: %q", usageVerbs)
	}
}
