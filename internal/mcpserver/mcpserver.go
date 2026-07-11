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
		Kind  string `json:"kind,omitempty" jsonschema:"optional filter: func | method | type"`
		Path  string `json:"path,omitempty" jsonschema:"optional file-path substring filter"`
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
