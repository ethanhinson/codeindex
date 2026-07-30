package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"codeindex/internal/query"
)

const fileA = `package p
func Helper(x int) int { return x + 1 }
func A() int { return Helper(1) }
`

const fileB = `package p
func B() int { return Helper(2) }
`

func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{"a.go": fileA, "b.go": fileB} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// connect spins up the server over in-memory transports and returns a session.
func connect(t *testing.T, repo string) *mcp.ClientSession {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	srv := New(repo, "test")
	go func() { _ = srv.Run(context.Background(), st) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	sess, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

// Task 4.4: handshake + tools/list + a callers call against a fixture repo.
func TestHandshakeListAndCall(t *testing.T) {
	repo := fixtureRepo(t)
	sess := connect(t, repo)
	ctx := context.Background()

	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	branchOut := map[string]bool{"impact": true, "callers": true, "callees": true, "dependents": true}
	// lore_* tools use a different trust model (committed files, not a derived
	// code-graph index) — they carry loreTrust, not the "COMPLETE" graph trust.
	loreTools := map[string]bool{
		"lore_search": true, "lore_for_symbol": true, "lore_backlog": true,
		"lore_show": true, "lore_add": true,
	}
	for _, tl := range tools.Tools {
		names[tl.Name] = true
		if !loreTools[tl.Name] && !strings.Contains(tl.Description, "COMPLETE") {
			t.Errorf("tool %s description missing trust language", tl.Name)
		}
		// Branch-out tools carry the anchor rule; locate tools (find/grep)
		// carry routing instead — they ARE the sanctioned locate path.
		if branchOut[tl.Name] && !strings.Contains(tl.Description, "NOT for locating") {
			t.Errorf("tool %s description missing anchor language", tl.Name)
		}
	}
	for _, want := range []string{"impact", "callers", "callees", "dependents", "find", "grep"} {
		if !names[want] {
			t.Errorf("missing tool %q; got %v", want, names)
		}
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{
		Name: "callers", Arguments: map[string]any{"symbol": "Helper"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(out, "def  Helper  a.go:2") || !strings.Contains(out, "callers (2)") {
		t.Errorf("unexpected callers output:\n%s", out)
	}
}

// Task 4.2: concurrent tool calls while the working tree has a pending edit —
// exactly one patches, both answer from the patched index, no SQLite error.
func TestConcurrentCallsDuringEdit(t *testing.T) {
	repo := fixtureRepo(t)
	if _, err := query.Fresh(repo); err != nil { // initial index
		t.Fatal(err)
	}
	// pending edit: new caller of Helper in a new file
	if err := os.WriteFile(filepath.Join(repo, "c.go"),
		[]byte("package p\nfunc C() int { return Helper(3) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess := connect(t, repo)
	ctx := context.Background()
	const n = 6
	outs := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := sess.CallTool(ctx, &mcp.CallToolParams{
				Name: "callers", Arguments: map[string]any{"symbol": "Helper"},
			})
			if err != nil {
				errs[i] = err
				return
			}
			outs[i] = res.Content[0].(*mcp.TextContent).Text
		}(i)
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("concurrent call %d errored: %v", i, errs[i])
		}
		if !strings.Contains(outs[i], "callers (3)") { // A, B, and the new C
			t.Errorf("call %d did not see the patched index:\n%s", i, outs[i])
		}
	}
}
