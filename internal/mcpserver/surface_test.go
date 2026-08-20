package mcpserver

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateSurface = flag.Bool("update-surface", false, "rewrite the pinned MCP surface golden")

const surfaceGolden = "testdata/tool-surface.json"

// surfaceEntry is one advertised tool or prompt, reduced to exactly the parts
// owner ruling 2 freezes: the name, the description, and the argument schema.
type surfaceEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

// TestToolSurfaceIsFrozen pins the advertised MCP surface — the eight tool
// names, their descriptions, their input schemas, and the explore-feature
// prompt — byte for byte.
//
// Owner ruling 2 freezes this surface: the workspace slice wires the handlers
// to wsquery and adds nothing to what the client sees. In particular no `repo`
// field appears in any schema (these tools return TextContent, so the member id
// rides inside the text) and the anchor prefix is deliberately unadvertised.
// A future well-meaning edit to a description or a schema turns this red, which
// is the point: the freeze has no compiler behind it.
func TestToolSurfaceIsFrozen(t *testing.T) {
	sess := connect(t, fixtureRepo(t))
	ctx := context.Background()

	var got []surfaceEntry
	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		schema, err := json.Marshal(tl.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, surfaceEntry{Name: tl.Name, Description: tl.Description, Schema: schema})
	}
	prompts, err := sess.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range prompts.Prompts {
		args, err := json.Marshal(p.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, surfaceEntry{Name: p.Name, Description: p.Description, Arguments: args})
	}

	// Guard the guard: if the surface ever shrinks to nothing the comparison
	// below would pass vacuously.
	if len(tools.Tools) != 8 {
		t.Errorf("advertised %d tools, want the frozen 8: %+v", len(tools.Tools), tools.Tools)
	}
	if len(prompts.Prompts) != 1 {
		t.Errorf("advertised %d prompts, want 1 (explore-feature)", len(prompts.Prompts))
	}

	enc, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	enc = append(enc, '\n')

	if *updateSurface {
		if err := os.MkdirAll(filepath.Dir(surfaceGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(surfaceGolden, enc, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", surfaceGolden)
		return
	}

	want, err := os.ReadFile(surfaceGolden)
	if err != nil {
		t.Fatalf("read golden (regenerate with -update-surface): %v", err)
	}
	if string(enc) != string(want) {
		t.Errorf("the frozen MCP surface changed.\n--- got ---\n%s\n--- want ---\n%s", enc, want)
	}
}
