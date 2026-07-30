// Package lore models decision/item/note records: Markdown files with YAML
// frontmatter, stored in a committed .lore/ layer and a private overlay.
package lore

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"
	"gopkg.in/yaml.v3"
)

type Type string

const (
	TypeDecision Type = "decision"
	TypeItem     Type = "item"
	TypeNote     Type = "note"
)

// Anchor ties a record to code: exactly one of Path or Symbol is set.
type Anchor struct{ Path, Symbol string }

// Ref is a typed external reference (gh-issue, jira, commit, url, ...).
type Ref struct{ Kind, Value string }

type Record struct {
	ID           string
	Type         Type
	Title        string
	Status       string
	Date         string // YYYY-MM-DD (UTC)
	Supersedes   string
	SupersededBy string
	Priority     string // items: p0..p3
	BlockedBy    []string
	Related      []string
	Tags         []string
	Anchors      []Anchor
	Refs         []Ref
	Body         string
	Extra        map[string]any // unknown frontmatter keys preserved for forward-compat
}

func NewID(t Type) string {
	prefix := map[Type]string{TypeDecision: "dec-", TypeItem: "itm-", TypeNote: "note-"}[t]
	return prefix + ulid.Make().String()
}

func DefaultStatus(t Type) string {
	switch t {
	case TypeDecision:
		return "active"
	case TypeItem:
		return "open"
	}
	return ""
}

// wire is the YAML frontmatter shape. Anchors/refs are single-key maps so the
// file reads naturally: `- path: internal/engine/` / `- gh-issue: org/repo#12`.
type wire struct {
	ID           string              `yaml:"id"`
	Title        string              `yaml:"title"`
	Status       string              `yaml:"status,omitempty"`
	Date         string              `yaml:"date"`
	Supersedes   string              `yaml:"supersedes,omitempty"`
	SupersededBy string              `yaml:"superseded_by,omitempty"`
	Priority     string              `yaml:"priority,omitempty"`
	BlockedBy    []string            `yaml:"blocked_by,omitempty,flow"`
	Related      []string            `yaml:"related,omitempty,flow"`
	Tags         []string            `yaml:"tags,omitempty,flow"`
	Anchors      []map[string]string `yaml:"anchors,omitempty"`
	Refs         []map[string]string `yaml:"refs,omitempty"`
}

// knownKeys lists the yaml tag names of all wire struct fields. This must stay
// in sync with wire (enforced by TestKnownKeysCoversWireFields).
var knownKeys = []string{
	"id",
	"title",
	"status",
	"date",
	"supersedes",
	"superseded_by",
	"priority",
	"blocked_by",
	"related",
	"tags",
	"anchors",
	"refs",
}

// Parse splits frontmatter from body and decodes. The type comes from the
// caller (derived from the directory), never from the file.
func Parse(b []byte, t Type) (Record, error) {
	rest, ok := bytes.CutPrefix(b, []byte("---\n"))
	if !ok {
		return Record{}, fmt.Errorf("missing frontmatter (no leading ---)")
	}
	fm, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return Record{}, fmt.Errorf("unterminated frontmatter")
	}
	var w wire
	if err := yaml.Unmarshal(fm, &w); err != nil {
		return Record{}, fmt.Errorf("frontmatter: %w", err)
	}

	// Second pass: capture unknown keys into Extra.
	var all map[string]any
	if err := yaml.Unmarshal(fm, &all); err != nil {
		return Record{}, fmt.Errorf("frontmatter (extra pass): %w", err)
	}
	for _, k := range knownKeys {
		delete(all, k)
	}
	var extra map[string]any
	if len(all) > 0 {
		extra = all
	}

	bodyStr := string(body)
	bodyStr, _ = strings.CutPrefix(bodyStr, "\n")
	r := Record{
		ID: w.ID, Type: t, Title: w.Title, Status: w.Status, Date: w.Date,
		Supersedes: w.Supersedes, SupersededBy: w.SupersededBy,
		Priority: w.Priority, BlockedBy: w.BlockedBy, Related: w.Related, Tags: w.Tags,
		Body: bodyStr, Extra: extra,
	}
	for _, m := range w.Anchors {
		r.Anchors = append(r.Anchors, Anchor{Path: m["path"], Symbol: m["symbol"]})
	}
	for _, m := range w.Refs {
		for k, v := range m {
			r.Refs = append(r.Refs, Ref{Kind: k, Value: v})
		}
	}
	return r, nil
}

func (r Record) Marshal() ([]byte, error) {
	w := wire{
		ID: r.ID, Title: r.Title, Status: r.Status, Date: r.Date,
		Supersedes: r.Supersedes, SupersededBy: r.SupersededBy,
		Priority: r.Priority, BlockedBy: r.BlockedBy, Related: r.Related, Tags: r.Tags,
	}
	for _, a := range r.Anchors {
		m := map[string]string{}
		if a.Path != "" {
			m["path"] = a.Path
		}
		if a.Symbol != "" {
			m["symbol"] = a.Symbol
		}
		w.Anchors = append(w.Anchors, m)
	}
	for _, ref := range r.Refs {
		w.Refs = append(w.Refs, map[string]string{ref.Kind: ref.Value})
	}
	fm, err := yaml.Marshal(w)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fm)
	if len(r.Extra) > 0 {
		// Marshal-side collision guard: filter Extra to exclude keys in knownKeys.
		// This prevents deliberate API misuse like r.Extra["title"] from creating
		// duplicate YAML keys where the last one silently wins.
		filteredExtra := make(map[string]any)
		knownKeysMap := make(map[string]bool)
		for _, k := range knownKeys {
			knownKeysMap[k] = true
		}
		for k, v := range r.Extra {
			if !knownKeysMap[k] {
				filteredExtra[k] = v
			}
		}
		if len(filteredExtra) > 0 {
			extraBytes, err := yaml.Marshal(filteredExtra)
			if err != nil {
				return nil, err
			}
			buf.Write(extraBytes)
		}
	}
	buf.WriteString("---\n")
	buf.WriteString("\n")
	buf.WriteString(r.Body)
	return buf.Bytes(), nil
}

// Slug converts a title to a filename fragment: lowercase alnum runs joined
// by hyphens, capped at 48 chars.
func Slug(title string) string {
	var b strings.Builder
	prevDash := true
	for _, c := range strings.ToLower(title) {
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			b.WriteRune(c)
			prevDash = false
		case !prevDash:
			b.WriteByte('-')
			prevDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}
