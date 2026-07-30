package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeindex/internal/lore"
	"codeindex/internal/lore/index"
)

func loreDBPath(root string) string {
	return filepath.Join(root, ".codeindex", "lore.db")
}

// loreReindex is every lore subcommand's preamble: locate layers, patch the
// derived index, hand back the open store. Parse errors are fail-open (the
// report carries them; doctor surfaces them).
func loreReindex(root string) (lore.Layout, *index.Store, index.Report, error) {
	l, err := lore.NewLayout(root)
	if err != nil {
		return lore.Layout{}, nil, index.Report{}, err
	}
	st, rep, err := index.Reindex(l, loreDBPath(root))
	return l, st, rep, err
}

const loreUsage = "usage: codeindex lore <repo-root> " +
	"<add|show|search|for|backlog|promote|supersede|doctor|init> ..."

func runLore(root string, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(loreUsage)
	}
	switch args[0] {
	case "add":
		return loreAdd(root, args[1:], out)
	case "show":
		return loreShow(root, args[1:], out)
	case "search":
		return loreSearch(root, args[1:], out)
	case "for":
		return loreFor(root, args[1:], out)
	default:
		return fmt.Errorf("unknown lore subcommand %q\n%s", args[0], loreUsage)
	}
}

// --- flag helpers (plain args scan, matching main.go's style) ---

func stringFlag(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func multiFlag(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			out = append(out, args[i+1])
		}
	}
	return out
}

func boolIn(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

// --- add ---

func loreAdd(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> add <decision|item|note> --title T ...")
	}
	typ := lore.Type(args[0])
	if typ != lore.TypeDecision && typ != lore.TypeItem && typ != lore.TypeNote {
		return fmt.Errorf("unknown record type %q (want decision|item|note)", args[0])
	}
	title := stringFlag(args, "--title")
	if title == "" {
		return fmt.Errorf("--title is required")
	}
	body := stringFlag(args, "--body")
	if body == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(b)
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	rec := lore.Record{
		ID: lore.NewID(typ), Type: typ, Title: title,
		Status: lore.DefaultStatus(typ),
		Date:   time.Now().UTC().Format("2006-01-02"),
		Body:   body, Priority: stringFlag(args, "--priority"),
		Tags: multiFlag(args, "--tag"),
	}
	for _, a := range multiFlag(args, "--anchor") {
		kind, val, ok := strings.Cut(a, ":")
		if !ok || (kind != "path" && kind != "symbol") {
			return fmt.Errorf("bad --anchor %q (want path:P or symbol:S)", a)
		}
		if kind == "path" {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Path: val})
		} else {
			rec.Anchors = append(rec.Anchors, lore.Anchor{Symbol: val})
		}
	}
	for _, r := range multiFlag(args, "--ref") {
		kind, val, ok := strings.Cut(r, ":")
		if !ok {
			return fmt.Errorf("bad --ref %q (want kind:value)", r)
		}
		rec.Refs = append(rec.Refs, lore.Ref{Kind: kind, Value: val})
	}
	layer := "repo"
	if boolIn(args, "--private") {
		layer = "overlay"
	}
	l, err := lore.NewLayout(root)
	if err != nil {
		return err
	}
	dir := l.Dir(layer, typ)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := rec.Date + "-" + lore.Slug(title) + ".md"
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		// Same-day same-slug collision: disambiguate with the ID tail.
		name = rec.Date + "-" + lore.Slug(title) + "-" + rec.ID[len(rec.ID)-6:] + ".md"
		path = filepath.Join(dir, name)
	}
	b, err := rec.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s %s\n", rec.ID, path)
	return nil
}

// --- show ---

func loreShow(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> show <id>")
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	r, ok, err := st.Get(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no record %q", args[0])
	}
	flags := ""
	if r.Stale {
		flags = "  STALE"
	}
	fmt.Fprintf(out, "%s  %s/%s  %s  %s%s\n\n", r.ID, r.Type, orDash(r.Status),
		r.Layer, r.File, flags)
	b, err := r.Record.Marshal()
	if err != nil {
		return err
	}
	if _, err := out.Write(b); err != nil {
		return err
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// --- JSON output ---

type loreJSON struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"`
	Title   string  `json:"title"`
	Status  string  `json:"status,omitempty"`
	Date    string  `json:"date"`
	Layer   string  `json:"layer"`
	File    string  `json:"file"`
	Stale   bool    `json:"stale,omitempty"`
	Score   float64 `json:"score,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
}

func toJSON(out io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(b))
	return err
}

// --- search ---

func loreSearch(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> search <query> [--limit N] [--json]")
	}
	limit := 10
	if v := stringFlag(args, "--limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return err
	}
	hits := index.Search(all, args[0], time.Now().UTC(), limit)
	if boolIn(args, "--json") {
		js := make([]loreJSON, 0, len(hits))
		for _, h := range hits {
			js = append(js, loreJSON{ID: h.Rec.ID, Type: string(h.Rec.Type),
				Title: h.Rec.Title, Status: h.Rec.Status, Date: h.Rec.Date,
				Layer: h.Rec.Layer, File: h.Rec.File, Stale: h.Rec.Stale,
				Score: h.Score, Snippet: h.Snippet})
		}
		return toJSON(out, js)
	}
	for _, h := range hits {
		fmt.Fprintf(out, "%s  %.2f  [%s/%s]  %s — %s\n",
			h.Rec.ID, h.Score, h.Rec.Layer, orDash(h.Rec.Status), h.Rec.Title, h.Snippet)
	}
	return nil
}

// --- for ---

// anchorMatches reports whether record anchor a covers query anchor q:
// symbols match exactly; paths match on either-direction prefix.
func anchorMatches(a lore.Anchor, q string) bool {
	if a.Symbol != "" {
		return a.Symbol == q
	}
	ap := strings.TrimSuffix(a.Path, "/")
	qp := strings.TrimSuffix(q, "/")
	return ap != "" && (strings.HasPrefix(qp, ap) || strings.HasPrefix(ap, qp))
}

func loreFor(root string, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: codeindex lore <repo> for <path-or-symbol> [--json]")
	}
	_, st, _, err := loreReindex(root)
	if err != nil {
		return err
	}
	defer st.Close()
	all, err := st.All()
	if err != nil {
		return err
	}
	var matched []index.StoredRecord
	for _, r := range all {
		for _, a := range r.Anchors {
			if anchorMatches(a, args[0]) {
				matched = append(matched, r)
				break
			}
		}
	}
	if boolIn(args, "--json") {
		js := make([]loreJSON, 0, len(matched))
		for _, r := range matched {
			js = append(js, loreJSON{ID: r.ID, Type: string(r.Type), Title: r.Title,
				Status: r.Status, Date: r.Date, Layer: r.Layer, File: r.File, Stale: r.Stale})
		}
		return toJSON(out, js)
	}
	for _, r := range matched {
		fmt.Fprintf(out, "%s  [%s/%s]  %s\n", r.ID, r.Layer, orDash(r.Status), r.Title)
	}
	return nil
}
