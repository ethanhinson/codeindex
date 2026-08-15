// Package mcpserver exposes the codeindex query surface as an MCP stdio server
// for IDE clients (Cursor, Claude Desktop, VS Code, Claude Code).
//
// Tool descriptions deliberately embed the measured consumption law: the anchor
// rule (use to branch OUT from a known symbol, never to locate) and the trust
// instruction (the output is complete — answer from it, don't re-verify).
// MCP tool descriptions are always visible to the client model, which is
// exactly the condition under which this tool measured −62–73% cost (A/B v2/v4);
// lazy/ceremonial packagings measured negative (v3a/v3).
package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"codeindex/internal/query"
)

const trust = "The output is COMPLETE and always fresh (the index self-updates " +
	"before answering): answer directly from the returned path:line references — " +
	"do not re-verify by reading files, except entries flagged [ambiguous]. "

const notFor = "NOT for locating/finding things (where is X defined, which " +
	"files mention Y) — plain text search is cheaper for those."

type symbolArgs struct {
	Symbol string `json:"symbol" jsonschema:"the exact function, method, or type name (a known anchor, e.g. HasDuplicateLabelNames)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max references to return (default 50)"`
}

func text(s string) *mcp.CallToolResult {
	// Disclose an implicit cold build once: the first tool result explains
	// where the latency went (and that it's gone for good).
	if info, ok := query.ConsumeColdBuild(); ok {
		s = fmt.Sprintf(
			"[codeindex: indexed %d files (%d symbols) in %s — first query on this repo; subsequent queries are fast]\n\n%s",
			info.FilesParsed, info.Symbols, info.Duration.Round(time.Millisecond), s)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// New builds the codeindex MCP server rooted at repo.
func New(repo, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "codeindex", Version: version}, nil)

	limitOr := func(l int) int {
		if l <= 0 {
			return 50
		}
		return l
	}
	limitOr20 := func(l int) int {
		if l <= 0 {
			return 20
		}
		return l
	}

	mcp.AddTool(s, &mcp.Tool{
		Name: "impact",
		Description: "Blast-radius summary for a KNOWN symbol (Go, TS/JS, Python, PHP) you are about " +
			"to modify, rename, or delete: its definitions, every caller (what " +
			"breaks), and its callees, counts-first. Use BEFORE changing a " +
			"function/method/type, when assessing refactor impact, or for " +
			"dead-code checks. " + trust + notFor,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in symbolArgs) (*mcp.CallToolResult, any, error) {
		out, err := query.ImpactText(repo, in.Symbol, limitOr(in.Limit))
		if err != nil {
			return nil, nil, fmt.Errorf("impact %q: %w", in.Symbol, err)
		}
		return text(out), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "callers",
		Description: "Who calls a KNOWN symbol (Go, TS/JS, Python, PHP): its definition(s) plus every " +
			"call site as path:line references with the calling function's name. " +
			"Use for 'who calls X / which functions use X / is X dead code'. " +
			trust + notFor,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in symbolArgs) (*mcp.CallToolResult, any, error) {
		out, err := query.CallersText(repo, in.Symbol, limitOr(in.Limit))
		if err != nil {
			return nil, nil, fmt.Errorf("callers %q: %w", in.Symbol, err)
		}
		return text(out), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "callees",
		Description: "What a KNOWN symbol (Go, TS/JS, Python, PHP) calls, each callee resolved to its " +
			"definition (path:line). Use for tracing downward from a function " +
			"('what does X depend on / call into'). " + trust + notFor,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in symbolArgs) (*mcp.CallToolResult, any, error) {
		out, err := query.CalleesText(repo, in.Symbol, limitOr(in.Limit))
		if err != nil {
			return nil, nil, fmt.Errorf("callees %q: %w", in.Symbol, err)
		}
		return text(out), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "dependents",
		Description: "Who imports, extends, or implements a KNOWN symbol/module " +
			"(Go, TS/JS, Python, PHP) — the type-level half of blast radius " +
			"(subclasses, implementers, importers). Go packages match by full " +
			"path or last segment. " + trust + notFor,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in symbolArgs) (*mcp.CallToolResult, any, error) {
		out, err := query.DependentsText(repo, in.Symbol, limitOr(in.Limit))
		if err != nil {
			return nil, nil, fmt.Errorf("dependents %q: %w", in.Symbol, err)
		}
		return text(out), nil, nil
	})

	type findArgs struct {
		Query string `json:"query" jsonschema:"partial or vague symbol name — tokens in any order/convention (e.g. 'config load' matches LoadConfig, load_config, ConfigLoader)"`
		Kind  string `json:"kind,omitempty" jsonschema:"optional; restrict results to one of: func | method | type"`
		Path  string `json:"path,omitempty" jsonschema:"optional; only include results whose file path contains this substring"`
		Limit int    `json:"limit,omitempty"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "find",
		Description: "Ranked symbol search when you only PARTIALLY know the " +
			"name (vague, differently-cased, synonym, or common name) — one call " +
			"replaces iterative grep probing; results ranked by usage (caller " +
			"count). If you know the exact distinctive name, plain text search " +
			"is still cheaper. " + trust,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in findArgs) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		out, err := query.FindText(repo, in.Query, in.Kind, in.Path, limit)
		if err != nil {
			return nil, nil, fmt.Errorf("find %q: %w", in.Query, err)
		}
		return text(out), nil, nil
	})

	type searchArgs struct {
		Query     string   `json:"query" jsonschema:"concept or feature phrase in natural language (e.g. 'host onboarding lifecycle')"`
		Hints     []string `json:"hints,omitempty" jsonschema:"optional identifier-style token guesses for the concept (e.g. ['onboard','signup','listing']) — you know naming conventions; supplying 3-6 guesses sharpens ranking"`
		ErrorText string   `json:"error_text,omitempty" jsonschema:"optional: paste error messages, warning text, or stack-trace lines VERBATIM — literal string evidence is the strongest signal for bug/symptom queries"`
		Limit     int      `json:"limit,omitempty" jsonschema:"max symbols in the answer (default 20)"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "search",
		Description: "Semantic code search for CONCEPT/FEATURE questions ('where " +
			"is the onboarding flow', 'code that handles retry backoff') and " +
			"BUG/SYMPTOM questions — for those, pass the error/warning text " +
			"verbatim via error_text (literal string evidence outranks " +
			"semantic similarity). Embedding + symbol-graph + literal hybrid; " +
			"answers as a feature map: results clustered by call-graph " +
			"connectivity, each cluster led by its most central entry point. " +
			"Routing: concept/feature/symptom question → this tool; known " +
			"symbol → impact/callers/callees; distinctive exact name → plain " +
			"text search. " + trust,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchArgs) (*mcp.CallToolResult, any, error) {
		out, err := query.SearchText(repo, in.Query, in.Hints, in.ErrorText, limitOr20(in.Limit), false)
		if err != nil {
			return nil, nil, fmt.Errorf("search %q: %w", in.Query, err)
		}
		return text(out), nil, nil
	})

	s.AddPrompt(&mcp.Prompt{
		Name: "explore-feature",
		Description: "Understand a feature end-to-end: semantic search to find " +
			"the entry points, then impact on the best entry to map its blast radius.",
		Arguments: []*mcp.PromptArgument{{
			Name: "feature", Description: "the feature/concept to explore", Required: true,
		}},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		feature := req.Params.Arguments["feature"]
		return &mcp.GetPromptResult{
			Description: "explore-feature workflow",
			Messages: []*mcp.PromptMessage{{
				Role: "user",
				Content: &mcp.TextContent{Text: fmt.Sprintf(
					"Explore the %q feature in this codebase:\n"+
						"1. Call the codeindex `search` tool with query %q; add 3-6 identifier-style hint tokens you'd expect developers to have used.\n"+
						"2. Pick the most relevant cluster's entry point (highest callers is usually right).\n"+
						"3. Call `impact` on that entry point to map callers and callees.\n"+
						"4. Answer from the returned path:line references — the output is complete; do not re-read files to verify.\n"+
						"5. If search came back [lexical-only], say so and suggest `codeindex build` to enable semantic matching.",
					feature, feature)},
			}},
		}, nil
	})

	type grepArgs struct {
		Pattern string `json:"pattern" jsonschema:"regex/text pattern to search file contents for"`
		Limit   int    `json:"limit,omitempty"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "grep",
		Description: "Content search with symbol attribution: every hit comes " +
			"back attributed to its enclosing function/method, deduped with " +
			"counts, definitions first — so you learn WHERE and IN WHAT the " +
			"pattern occurs without reading files to attribute lines. Use when " +
			"you need to understand occurrences, not just locate one. " + trust,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in grepArgs) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 30
		}
		out, err := query.GrepText(repo, in.Pattern, limit)
		if err != nil {
			return nil, nil, fmt.Errorf("grep %q: %w", in.Pattern, err)
		}
		return text(out), nil, nil
	})

	return s
}

// Run serves over stdio until the client disconnects.
func Run(ctx context.Context, repo, version string) error {
	return New(repo, version).Run(ctx, &mcp.StdioTransport{})
}
