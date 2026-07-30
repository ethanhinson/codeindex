package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"codeindex/internal/lore"
	"codeindex/internal/lore/index"
)

const loreTrust = "Lore records are the project's committed decisions, work " +
	"items, and notes. Treat active decisions as constraints on your work; " +
	"superseded records are history, stale records need verification. "

func loreOpen(repo string) (lore.Layout, *index.Store, error) {
	l, err := lore.NewLayout(repo)
	if err != nil {
		return lore.Layout{}, nil, err
	}
	st, _, err := index.Reindex(l, filepath.Join(repo, ".codeindex", "lore.db"))
	return l, st, err
}

func formatRecords(recs []index.StoredRecord) string {
	var b strings.Builder
	for _, r := range recs {
		status := r.Status
		if status == "" {
			status = "-"
		}
		flag := ""
		if r.Stale {
			flag = "  STALE"
		}
		if !r.Ratified {
			flag += "  UNRATIFIED"
		}
		fmt.Fprintf(&b, "%s  [%s/%s]  %s%s\n", r.ID, r.Layer, status, r.Title, flag)
	}
	return b.String()
}

func loreSearchText(repo, query string, limit int) (string, error) {
	_, st, err := loreOpen(repo)
	if err != nil {
		return "", err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return "", err
	}
	hits := index.Search(all, query, time.Now().UTC(), limit)
	if len(hits) == 0 {
		return fmt.Sprintf("no lore records match %q", query), nil
	}
	var b strings.Builder
	for _, h := range hits {
		status := h.Rec.Status
		if status == "" {
			status = "-"
		}
		flag := ""
		if h.Rec.Stale {
			flag = "  STALE"
		}
		if !h.Rec.Ratified {
			flag += "  UNRATIFIED"
		}
		fmt.Fprintf(&b, "%s  %.2f  [%s/%s]  %s — %s%s\n",
			h.Rec.ID, h.Score, h.Rec.Layer, status, h.Rec.Title, h.Snippet, flag)
	}
	return b.String(), nil
}

func loreForText(repo, anchor string) (string, error) {
	_, st, err := loreOpen(repo)
	if err != nil {
		return "", err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return "", err
	}
	matched := index.RecordsForAnchor(all, anchor)
	if len(matched) == 0 {
		return fmt.Sprintf("no lore records anchored to %q", anchor), nil
	}
	return formatRecords(matched), nil
}

// loreBacklogText returns the open-item backlog, optionally filtered by anchor.
// Mirrors the CLI's sort: priority → blocked-last → date. (~25 lines)
func loreBacklogText(repo, anchor string) (string, error) {
	_, st, err := loreOpen(repo)
	if err != nil {
		return "", err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return "", err
	}
	openIDs := map[string]bool{}
	for _, r := range all {
		if r.Type == lore.TypeItem && r.Status == "open" {
			openIDs[r.ID] = true
		}
	}
	var items []index.StoredRecord
	for _, r := range all {
		if r.Type != lore.TypeItem || r.Status != "open" {
			continue
		}
		items = append(items, r)
	}
	if anchor != "" {
		items = index.RecordsForAnchor(items, anchor)
	}
	blocked := func(r index.StoredRecord) bool {
		for _, b := range r.BlockedBy {
			if openIDs[b] {
				return true
			}
		}
		return false
	}
	prio := func(p string) string {
		if p == "" {
			return "p2"
		}
		return p
	}
	sort.SliceStable(items, func(i, j int) bool {
		pi, pj := prio(items[i].Priority), prio(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		bi, bj := blocked(items[i]), blocked(items[j])
		if bi != bj {
			return !bi
		}
		return items[i].Date < items[j].Date
	})
	if len(items) == 0 {
		if anchor != "" {
			return fmt.Sprintf("no open work items anchored to %q", anchor), nil
		}
		return "no open work items", nil
	}
	var b strings.Builder
	for _, r := range items {
		state := "ready"
		if blocked(r) {
			state = "BLOCKED"
		}
		flag := ""
		if !r.Ratified {
			flag = "  UNRATIFIED"
		}
		fmt.Fprintf(&b, "%s  %s  %s  %s%s\n", r.ID, prio(r.Priority), state, r.Title, flag)
	}
	return b.String(), nil
}

// loreShowText returns the full record text for the given ID.
func loreShowText(repo, id string) (string, error) {
	_, st, err := loreOpen(repo)
	if err != nil {
		return "", err
	}
	defer st.Close()
	r, ok, err := st.Get(id)
	if err != nil {
		return "", err
	}
	if !ok {
		return fmt.Sprintf("no record %q", id), nil
	}
	flags := ""
	if r.Stale {
		flags = "  STALE"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s/%s  %s  %s%s\n\n", r.ID, r.Type, orLoreDash(r.Status),
		r.Layer, r.File, flags)
	body, err := r.Record.Marshal()
	if err != nil {
		return "", err
	}
	b.Write(body)
	return b.String(), nil
}

// relatedLoreBlock returns lore anchored to symbol, or "" — lore must never
// break code navigation, so every error path collapses to the empty string.
func relatedLoreBlock(repo, symbol string) string {
	_, st, err := loreOpen(repo)
	if err != nil {
		return ""
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return ""
	}
	matched := index.RecordsForAnchor(all, symbol)
	if len(matched) == 0 {
		return ""
	}
	// Active/open first, then the rest; cap at 5.
	var head, tail []index.StoredRecord
	for _, r := range matched {
		if r.Status == "active" || r.Status == "open" {
			head = append(head, r)
		} else {
			tail = append(tail, r)
		}
	}
	ordered := append(head, tail...)
	if len(ordered) > 5 {
		ordered = ordered[:5]
	}
	return "\n\nRelated lore (decisions/items anchored to this symbol):\n" +
		formatRecords(ordered)
}

func orLoreDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// writeNewLoreRecord mirrors cmd/codeindex's writeNewRecord convention:
// date-slug filename, ID-tail collision disambiguation.
func writeNewLoreRecord(l lore.Layout, rec lore.Record, layer string) (string, error) {
	dir := l.Dir(layer, rec.Type)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := rec.Date + "-" + lore.Slug(rec.Title) + ".md"
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		name = rec.Date + "-" + lore.Slug(rec.Title) + "-" + rec.ID[len(rec.ID)-6:] + ".md"
		path = filepath.Join(dir, name)
	}
	b, err := rec.Marshal()
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o644)
}

// loreAddRecord validates args, builds a record, writes it, returns "created <id> <path>".
func loreAddRecord(repo string, typ lore.Type, title, body string, anchors, refs []string, private bool, priority string, tags []string, blockedBy []string) (string, error) {
	if typ != lore.TypeDecision && typ != lore.TypeItem && typ != lore.TypeNote {
		return "", fmt.Errorf("unknown record type %q (want decision|item|note)", typ)
	}
	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	rec := lore.Record{
		ID:        lore.NewID(typ),
		Type:      typ,
		Title:     title,
		Status:    lore.DefaultStatus(typ),
		Date:      time.Now().UTC().Format("2006-01-02"),
		Body:      body,
		Priority:  priority,
		Tags:      tags,
		BlockedBy: blockedBy,
	}
	for _, a := range anchors {
		kind, val, ok := strings.Cut(a, ":")
		if !ok || (kind != "path" && kind != "symbol") {
			return "", fmt.Errorf("bad anchor %q (want path:P or symbol:S)", a)
		}
		if kind == "path" {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Path: val})
		} else {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Symbol: val})
		}
	}
	for _, r := range refs {
		kind, val, ok := strings.Cut(r, ":")
		if !ok {
			return "", fmt.Errorf("bad ref %q (want kind:value)", r)
		}
		rec.Refs = append(rec.Refs, lore.Ref{Kind: kind, Value: val})
	}
	layer := "repo"
	if private {
		layer = "overlay"
	}
	l, err := lore.NewLayout(repo)
	if err != nil {
		return "", err
	}
	path, err := writeNewLoreRecord(l, rec, layer)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("created %s %s", rec.ID, path), nil
}

func addLoreTools(s *mcp.Server, repo string) {
	type searchArgs struct {
		Query string `json:"query" jsonschema:"search terms — keywords, a question, or a symbol name"`
		Limit int    `json:"limit,omitempty" jsonschema:"max results to return (default 10)"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "lore_search",
		Description: "Search the project's lore — committed decisions (with rationale and rejected " +
			"alternatives), open work items, and notes. Use BEFORE making an architectural choice " +
			"or when the user references past decisions ('why do we…', 'didn't we decide…'). " +
			loreTrust,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchArgs) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		out, err := loreSearchText(repo, in.Query, limit)
		if err != nil {
			return nil, nil, fmt.Errorf("lore_search: %w", err)
		}
		return text(out), nil, nil
	})

	type forArgs struct {
		Anchor string `json:"anchor" jsonschema:"a symbol name or repo-relative path"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "lore_for_symbol",
		Description: "Decisions, notes, and open work items anchored to a specific symbol or path. " +
			"Use BEFORE modifying code to learn what was decided about it and what work is already " +
			"planned there. " + loreTrust,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in forArgs) (*mcp.CallToolResult, any, error) {
		out, err := loreForText(repo, in.Anchor)
		if err != nil {
			return nil, nil, fmt.Errorf("lore_for_symbol: %w", err)
		}
		return text(out), nil, nil
	})

	type backlogArgs struct {
		Anchor string `json:"anchor,omitempty" jsonschema:"optional; filter to items anchored to this symbol or path"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "lore_backlog",
		Description: "Open work items sorted by priority then date, blocked items last. " +
			"Use to see what is planned and whether work is blocked. " + loreTrust,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in backlogArgs) (*mcp.CallToolResult, any, error) {
		out, err := loreBacklogText(repo, in.Anchor)
		if err != nil {
			return nil, nil, fmt.Errorf("lore_backlog: %w", err)
		}
		return text(out), nil, nil
	})

	type showArgs struct {
		ID string `json:"id" jsonschema:"the lore record ID (e.g. dec-01HX..., itm-01HX..., note-01HX...)"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "lore_show",
		Description: "Full text of a lore record (frontmatter + body). Use when you have the ID from lore_search or lore_for_symbol and need the complete record including rationale and alternatives. " + loreTrust,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in showArgs) (*mcp.CallToolResult, any, error) {
		out, err := loreShowText(repo, in.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("lore_show: %w", err)
		}
		return text(out), nil, nil
	})

	type addArgs struct {
		Type      string   `json:"type" jsonschema:"decision, item, or note"`
		Title     string   `json:"title" jsonschema:"short descriptive title"`
		Body      string   `json:"body" jsonschema:"full body text (markdown); for decisions include rationale AND rejected alternatives"`
		Anchors   []string `json:"anchors,omitempty" jsonschema:"optional; each entry is path:P or symbol:S"`
		Refs      []string `json:"refs,omitempty" jsonschema:"optional; each entry is kind:value (e.g. gh-issue:org/repo#12)"`
		Private   bool     `json:"private,omitempty" jsonschema:"if true, write to the private overlay layer instead of the committed .lore/ dir"`
		Priority  string   `json:"priority,omitempty" jsonschema:"optional; item priority p0..p3 (p0 = most urgent)"`
		Tags      []string `json:"tags,omitempty" jsonschema:"optional; free-form tags"`
		BlockedBy []string `json:"blocked_by,omitempty" jsonschema:"optional; item IDs that must complete first"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "lore_add",
		Description: "Record a decision (with rationale AND rejected alternatives in the body), a work item, " +
			"or a note. Use when an architectural choice is made, a non-obvious root cause is found, or " +
			"the user says 'remember this' / 'we decided'. Write it while you have full context; for items set priority p0..p3 and blocked_by.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in addArgs) (*mcp.CallToolResult, any, error) {
		out, err := loreAddRecord(repo, lore.Type(in.Type), in.Title, in.Body, in.Anchors, in.Refs, in.Private, in.Priority, in.Tags, in.BlockedBy)
		if err != nil {
			return nil, nil, fmt.Errorf("lore_add: %w", err)
		}
		return text(out), nil, nil
	})
}
