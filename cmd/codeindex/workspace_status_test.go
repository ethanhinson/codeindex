package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/overlay"
)

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
// The §6 surfaces print rather than return, so the only honest assertion on
// them is on the bytes they emit.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	callErr := fn()
	os.Stdout = prev
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), callErr
}

// statusWS writes a one-member workspace with an overlay carrying two
// suppressions — one versioned, one with NO recorded version. The
// unknown-version record is the point: it is the one a naive renderer drops.
func statusWS(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := config.SaveWorkspace(ws, &config.Workspace{Version: 1, Members: []config.Member{
		{ID: "api", Root: "services/api", Namespaces: []string{}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "services", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	ov, err := overlay.Open(overlay.Path(ws))
	if err != nil {
		t.Fatal(err)
	}
	if err := ov.PutSuppressions([]overlay.Suppression{
		{ConsumerMember: "drupal", Namespace: "symfony/console", OwnerMember: "symfony"},
		{ConsumerMember: "drupal", Namespace: "symfony/http-foundation",
			OwnerMember: "symfony", SuppressedVersion: "v7.1.0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ov.PutStamp("api", "merkle-deadbeef"); err != nil {
		t.Fatal(err)
	}
	if err := ov.Close(); err != nil {
		t.Fatal(err)
	}
	return ws
}

// The verb prints the §6 report, including every skew line, with an empty
// SuppressedVersion rendered as "version unknown" rather than omitted.
func TestWorkspaceStatusVerbPrintsTheReport(t *testing.T) {
	ws := statusWS(t)
	out, err := captureStdout(t, func() error { return dispatchWorkspaceStatus(ws, false) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"api: ",
		"stamped merkle merkle-deadbeef",
		"drupal vendors symfony/console at version unknown; member symfony wins",
		"drupal vendors symfony/http-foundation at v7.1.0; member symfony wins",
		"cross-edges: 0",
		"ambiguities: 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("workspace-status output missing %q\n---\n%s", want, out)
		}
	}
}

// --json emits the structured report on stdout, parseable in one piece.
func TestWorkspaceStatusVerbJSON(t *testing.T) {
	ws := statusWS(t)
	out, err := captureStdout(t, func() error { return dispatchWorkspaceStatus(ws, true) })
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json output is not one JSON document: %v\n%s", err, out)
	}
	for _, key := range []string{"workspace", "overlay_schema_version", "cross_edges",
		"ambiguities", "members", "skew"} {
		if _, ok := got[key]; !ok {
			t.Errorf("--json missing key %q: %s", key, out)
		}
	}
	skew, _ := got["skew"].([]any)
	if len(skew) != 2 {
		t.Fatalf("--json dropped a skew record: got %d, want 2\n%s", len(skew), out)
	}
	for _, raw := range skew {
		s, _ := raw.(map[string]any)
		if _, ok := s["suppressed_version"]; !ok {
			t.Errorf("--json skew record omits suppressed_version: %s", out)
		}
	}
}

// The verb REFUSES a repo root and names the repo-mode `status` verb — the one
// that answers the same question there.
func TestWorkspaceStatusVerbRefusesARepoRootNamingStatus(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a.go"),
		[]byte("package p\n\nfunc Target() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := captureStdout(t, func() error { return dispatchWorkspaceStatus(repo, false) })
	if err == nil {
		t.Fatalf("workspace-status accepted a repo root; output:\n%s", out)
	}
	msg := err.Error()
	for _, want := range []string{"codeindex workspace-status", "is a repo, not a workspace",
		"codeindex status " + repo} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q:\n%s", want, msg)
		}
	}
	if out != "" {
		t.Errorf("refusal still printed a report:\n%s", out)
	}
}

// `status <workspace-root>` prints the per-member fan-out FOLLOWED BY the §6
// block — one command answering both halves.
func TestStatusOnWorkspaceRootPrintsFanOutThenTheWorkspaceBlock(t *testing.T) {
	ws := statusWS(t)
	out, err := captureStdout(t, func() error { return dispatchStatus(ws, false) })
	if err != nil {
		t.Fatal(err)
	}
	fanLine := strings.Index(out, "api: status ")
	block := strings.Index(out, "version skew:")
	if fanLine < 0 {
		t.Fatalf("no per-member fan-out line:\n%s", out)
	}
	if block < 0 {
		t.Fatalf("no workspace-status block:\n%s", out)
	}
	if block < fanLine {
		t.Errorf("workspace block printed BEFORE the fan-out:\n%s", out)
	}
	if !strings.Contains(out, "version unknown") {
		t.Errorf("the fan-out path lost the unknown-version skew line:\n%s", out)
	}
}

// The verb is documented on the usage banner. An undocumented verb is one
// nobody finds.
func TestUsageBannerNamesWorkspaceStatus(t *testing.T) {
	if !strings.Contains(usageVerbs, "workspace-status") {
		t.Errorf("usage banner omits workspace-status:\n%s", usageVerbs)
	}
}
