package main

import (
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
