package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"codeindex/internal/config"
)

// fanOutWorkspace writes a manifest declaring ids in order, creating a
// directory for each id NOT named in absent. The workspace root itself carries
// the manifest, so engine.DetectRootKind classifies it as a workspace.
func fanOutWorkspace(t *testing.T, ids []string, absent map[string]bool) string {
	t.Helper()
	wsRoot := t.TempDir()
	members := make([]config.Member, 0, len(ids))
	for _, id := range ids {
		members = append(members, config.Member{ID: id, Root: "./" + id, Namespaces: []string{}})
		if absent[id] {
			continue
		}
		if err := os.MkdirAll(filepath.Join(wsRoot, id), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.SaveWorkspace(wsRoot, &config.Workspace{Version: 1, Members: members}); err != nil {
		t.Fatal(err)
	}
	return wsRoot
}

// A failing member must not abort the pass, and the aggregate error must name
// EVERY failed member — two failures, because a single-failure fixture cannot
// distinguish "names every failure" from "names the first failure".
func TestFanOutContinuesAndAggregateNamesEveryFailedMember(t *testing.T) {
	ids := []string{"api", "shared", "web", "docs"}
	wsRoot := fanOutWorkspace(t, ids, nil)

	var visited []string
	var out bytes.Buffer
	err := fanOut("build", wsRoot, &out, func(m config.ResolvedMember) error {
		visited = append(visited, m.Member.ID)
		if m.Member.ID == "api" || m.Member.ID == "web" {
			return fmt.Errorf("boom in %s", m.Member.ID)
		}
		return nil
	})

	// Continue-on-failure: every member ran, in manifest order, even though
	// the first one failed.
	if got := strings.Join(visited, ","); got != "api,shared,web,docs" {
		t.Fatalf("fan-out did not continue past failures: visited %q", got)
	}
	if err == nil {
		t.Fatal("want an aggregate error, got nil")
	}
	for _, id := range []string{"api", "web"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("aggregate error %q does not name failed member %q", err, id)
		}
	}
	for _, id := range []string{"shared", "docs"} {
		if strings.Contains(err.Error(), id) {
			t.Errorf("aggregate error %q names succeeding member %q", err, id)
		}
	}
	// Each failure is also reported against its own id on the output stream,
	// so the operator can see which members made it.
	for _, want := range []string{"api: build failed: boom in api", "web: build failed: boom in web"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q\n---\n%s", want, out.String())
		}
	}
}

// A member declared by the manifest but absent from disk is reported by id,
// never skipped silently.
func TestFanOutReportsMissingMemberByID(t *testing.T) {
	wsRoot := fanOutWorkspace(t, []string{"api", "gone", "web"}, map[string]bool{"gone": true})

	var visited []string
	var out bytes.Buffer
	if err := fanOut("status", wsRoot, &out, func(m config.ResolvedMember) error {
		visited = append(visited, m.Member.ID)
		return nil
	}); err != nil {
		t.Fatalf("missing member must not fail the pass: %v", err)
	}
	if got := strings.Join(visited, ","); got != "api,web" {
		t.Fatalf("visited %q, want the present members only", got)
	}
	line := ""
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(l, "gone:") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("missing member silently skipped; output:\n%s", out.String())
	}
	if !strings.Contains(line, "missing") {
		t.Errorf("missing member line %q does not say it is missing", line)
	}
}

// A malformed manifest is a CONFIGURATION fault at the CLI boundary, not a
// refusal listing an empty member set: RefuseWorkspaceRoot propagates
// config.LoadWorkspace's error unchanged and refuseWorkspaceRoot must not
// swallow or reshape it.
func TestRefuseWorkspaceRootSurfacesMalformedManifestAsConfigFault(t *testing.T) {
	wsRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsRoot, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsRoot, config.WorkspaceFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := refuseWorkspaceRoot("export", wsRoot)
	if err == nil {
		t.Fatal("want an error for a malformed manifest")
	}
	if strings.Contains(err.Error(), "is a workspace, not a repo") {
		t.Errorf("malformed manifest reported as a refusal, not a config fault: %v", err)
	}
	if !strings.Contains(err.Error(), "workspace.json") {
		t.Errorf("config fault does not name the manifest: %v", err)
	}
}

// A repo root is never refused.
func TestRefuseWorkspaceRootPassesRepoRoots(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"export", "import", "ingest", "depmap", "serve"} {
		if err := refuseWorkspaceRoot(verb, repo); err != nil {
			t.Errorf("%s on a repo root: %v", verb, err)
		}
	}
}

var buildOnce struct {
	sync.Once
	bin string
	err error
}

// codeindexBinary builds the CLI once per test run so the refusing verbs can
// be observed the only way an exit code can be observed: as a process.
func codeindexBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "codeindex-cli")
		if err != nil {
			buildOnce.err = err
			return
		}
		bin := filepath.Join(dir, "codeindex")
		cmd := exec.Command("go", "build", "-tags", "nollama", "-o", bin, "./cmd/codeindex")
		cmd.Dir = repoRootForTest(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildOnce.err = fmt.Errorf("go build: %v\n%s", err, out)
			return
		}
		buildOnce.bin = bin
	})
	if buildOnce.err != nil {
		t.Fatal(buildOnce.err)
	}
	return buildOnce.bin
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd() // .../cmd/codeindex
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

// Every refusing verb prints the per-repo refusal naming the members and exits
// 1 — the single exit code this slice uses. Exit 2 stays the pre-dispatch
// usage code; a workspace root is a well-formed argument.
func TestRefusingVerbsMessageAndExitCode(t *testing.T) {
	bin := codeindexBinary(t)
	wsRoot := fanOutWorkspace(t, []string{"api", "shared"}, nil)

	cases := []struct {
		verb string
		args []string
	}{
		{"export", []string{"out.db"}},
		{"import", []string{"artifact.db"}},
		{"ingest", nil},
		{"depmap", []string{"--namespace", "N", "--version", "1", "-o", "o.db"}},
		{"serve", nil},
		{"search", []string{"some concept"}},
	}
	for _, tc := range cases {
		t.Run(tc.verb, func(t *testing.T) {
			args := append([]string{tc.verb, wsRoot}, tc.args...)
			// A deadline, because a guard that does not fire does not
			// necessarily error: `serve` would bind a port and block
			// forever, and a hung test reads as an infrastructure problem
			// rather than the missing refusal it actually is.
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Env = append(os.Environ(), "CODEINDEX_DISABLED=")
			out, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("%s did not refuse the workspace root within the deadline", tc.verb)
			}
			code := cmd.ProcessState.ExitCode()
			if err == nil || code != 1 {
				t.Fatalf("exit code %d (err %v), want 1\n%s", code, err, out)
			}
			s := string(out)
			if !strings.Contains(s, "codeindex "+tc.verb+":") {
				t.Errorf("refusal does not name the verb:\n%s", s)
			}
			if !strings.Contains(s, "is a workspace, not a repo") {
				t.Errorf("refusal missing the fixed sentence:\n%s", s)
			}
			if !strings.Contains(s, "members: api, shared") {
				t.Errorf("refusal does not list the members:\n%s", s)
			}
			if tc.verb == "search" && !strings.Contains(s, "frozen non-goal") {
				t.Errorf("search must refuse for its own reason:\n%s", s)
			}
		})
	}
}

// The anchor prefix is documented on the CLI usage banner — the only surface
// that documents it (MCP's tool surface is frozen).
func TestUsageBannerDocumentsAnchorPrefix(t *testing.T) {
	if !strings.Contains(usageVerbs, "<member-id>:") {
		t.Errorf("usage banner does not document the anchor prefix:\n%s", usageVerbs)
	}
}
