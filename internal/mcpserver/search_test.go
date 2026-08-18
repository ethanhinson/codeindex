package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The search tool answers over MCP: listed with routing-law description,
// callable with hints, and the explore-feature prompt is served.
func TestSearchToolAndPrompt(t *testing.T) {
	repo := fixtureRepo(t)
	sess := connect(t, repo)
	ctx := context.Background()

	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tool := range tools.Tools {
		if tool.Name == "search" {
			found = true
			if !strings.Contains(tool.Description, "concept/feature/symptom question") ||
				!strings.Contains(tool.Description, "error_text") {
				t.Fatalf("search description missing routing law: %s", tool.Description)
			}
		}
	}
	if !found {
		t.Fatal("search tool not listed")
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{
		Name: "search",
		Arguments: map[string]any{
			"query": "helper increment function",
			"hints": []string{"helper", "increment"},
			"limit": 5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(out, "search \"helper increment function\"") {
		t.Fatalf("unexpected search output:\n%s", out)
	}
	// Semantic surfacing of "Helper" for the 3-token conceptual query depends on
	// embedding capability: the lexical ladder is conjunctive by design, so on a
	// lexical-only build it correctly finds no exact/prefix/substring match across
	// all three tokens. Branch on the disclosure marker to assert the honest
	// contract for whichever capability this build actually has.
	if lexIdx := strings.Index(out, "[lexical-only: "); lexIdx != -1 {
		rest := out[lexIdx+len("[lexical-only: "):]
		closeIdx := strings.Index(rest, "]")
		if closeIdx == -1 {
			t.Fatalf("lexical-only disclosure missing closing bracket:\n%s", out)
		}
		reason := rest[:closeIdx]
		if strings.TrimSpace(reason) == "" {
			t.Fatalf("lexical-only disclosure has empty reason:\n%s", out)
		}
		if !strings.Contains(out, "(no matches — try different concept words, or add hints)") {
			t.Fatalf("lexical-only search missing no-match guidance:\n%s", out)
		}
		t.Logf("lexical-only build (%s): skipping semantic Helper-surfacing assertion", reason)
	} else {
		if !strings.Contains(out, "Helper") {
			t.Fatalf("search did not surface Helper:\n%s", out)
		}
	}

	// Build-independent lexical contract: a bare identifier query hits the
	// "exact" rung of the lexical ladder (matchQuality in internal/search/find.go
	// lowercases and stems both sides), so this must surface "Helper" regardless
	// of whether embeddings are live — a genuine lexical-fallback regression
	// would fail this even when the semantic assertion above is skipped.
	res, err = sess.CallTool(ctx, &mcp.CallToolParams{
		Name: "search",
		Arguments: map[string]any{
			"query": "Helper",
			"limit": 5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	out = res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(out, "Helper") {
		t.Fatalf("bare identifier search did not surface Helper:\n%s", out)
	}

	prompts, err := sess.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts.Prompts) == 0 || prompts.Prompts[0].Name != "explore-feature" {
		t.Fatalf("explore-feature prompt not listed: %+v", prompts.Prompts)
	}
	p, err := sess.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      "explore-feature",
		Arguments: map[string]string{"feature": "user onboarding"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ptext := p.Messages[0].Content.(*mcp.TextContent).Text
	for _, want := range []string{"search", "impact", "user onboarding", "hint"} {
		if !strings.Contains(ptext, want) {
			t.Fatalf("prompt missing %q:\n%s", want, ptext)
		}
	}
}
