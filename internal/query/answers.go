// Structured answers: every query builds one answer value, then renders it
// twice — Text() is the stable human/agent format (bench formatters and the
// MCP server consume it byte-for-byte), encoding/json is the machine format
// (`--json`). Retrieval happens once; the two renders cannot drift.
package query

import (
	"fmt"
	"strings"

	"codeindex/internal/search"
)

// DefRef is a definition entry.
type DefRef struct {
	Name      string `json:"name"`
	Parent    string `json:"parent,omitempty"`
	QName     string `json:"qname"`
	Kind      string `json:"kind,omitempty"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Signature string `json:"signature,omitempty"`
}

// CallerRef is one calling symbol with its call-site line.
type CallerRef struct {
	Name      string `json:"name"`
	Parent    string `json:"parent,omitempty"`
	QName     string `json:"qname"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Ambiguous bool   `json:"ambiguous,omitempty"`
}

// CalleeRef is one called symbol. DefFile=="" means the call did not resolve
// to a known definition (stdlib/external).
type CalleeRef struct {
	Name        string `json:"name"`
	Parent      string `json:"parent,omitempty"`
	QName       string `json:"qname"`
	CallLine    int    `json:"call_line"`
	DefFile     string `json:"def_file,omitempty"`
	DefLine     int    `json:"def_line,omitempty"`
	Ambiguous   bool   `json:"ambiguous,omitempty"`
	Dep         string `json:"dep,omitempty"` // "namespace@version" provenance
	DepModified bool   `json:"dep_modified,omitempty"`
}

// DependentRef is one importer/extender/implementer.
type DependentRef struct {
	Kind  string `json:"kind"`
	QName string `json:"qname"` // qualified symbol, or the file itself for imports
	File  string `json:"file"`
	Line  int    `json:"line"`
}

// FindRef is one ranked symbol-search result.
type FindRef struct {
	QName       string `json:"qname"`
	Kind        string `json:"kind"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Callers     int    `json:"callers,omitempty"`
	Dep         string `json:"dep,omitempty"`
	DepModified bool   `json:"dep_modified,omitempty"`
	Match       string `json:"match"`
}

// GrepRef is one grouped content hit. QName=="" means the hit sits outside
// any symbol.
type GrepRef struct {
	QName string `json:"qname,omitempty"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Hits  int    `json:"hits"`
	IsDef bool   `json:"is_definition,omitempty"`
}

// DepRef is one outgoing dependency edge.
type DepRef struct {
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	DefFile string `json:"def_file,omitempty"`
	DefLine int    `json:"def_line,omitempty"`
	Line    int    `json:"line"`
}

// EnclosingRef is one symbol overlapping a queried line range.
type EnclosingRef struct {
	Name            string `json:"name"`
	Parent          string `json:"parent,omitempty"`
	Kind            string `json:"kind"`
	File            string `json:"file"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	Callers         int    `json:"callers"`
	ExternalCallers int    `json:"external_callers"`
}

// CallersAnswer is definitions + callers + referencing files for an anchor.
// List fields hold at most the query limit; *Total fields hold full counts.
type CallersAnswer struct {
	Anchor               string      `json:"anchor"`
	Definitions          []DefRef    `json:"definitions"`
	CallersTotal         int         `json:"callers_total"`
	Callers              []CallerRef `json:"callers"`
	ReferencedFilesTotal int         `json:"referenced_files_total"`
	ReferencedFiles      []string    `json:"referenced_files"`
}

func (a *CallersAnswer) Text() string {
	var b strings.Builder
	writeDefs(&b, a.Anchor, a.Definitions)
	fmt.Fprintf(&b, "callers (%d):\n", a.CallersTotal)
	for _, c := range a.Callers {
		fmt.Fprintf(&b, "  %s:%d  %s%s\n", c.File, c.Line, c.QName, ambigTag(c.Ambiguous))
	}
	if a.CallersTotal > len(a.Callers) {
		fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", a.CallersTotal-len(a.Callers))
	}
	fmt.Fprintf(&b, "referenced in %d file(s):", a.ReferencedFilesTotal)
	for _, f := range a.ReferencedFiles {
		fmt.Fprintf(&b, " %s", f)
	}
	if a.ReferencedFilesTotal > len(a.ReferencedFiles) {
		fmt.Fprintf(&b, " ... (+%d more)", a.ReferencedFilesTotal-len(a.ReferencedFiles))
	}
	fmt.Fprintf(&b, "\n")
	return b.String()
}

// CalleesAnswer is what an anchor calls.
type CalleesAnswer struct {
	Anchor  string      `json:"anchor"`
	Total   int         `json:"callees_total"`
	Callees []CalleeRef `json:"callees"`
}

func (a *CalleesAnswer) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "callees of %s (%d):\n", a.Anchor, a.Total)
	for _, c := range a.Callees {
		target := "unresolved"
		if c.DefFile != "" {
			target = fmt.Sprintf("%s:%d", c.DefFile, c.DefLine)
		}
		fmt.Fprintf(&b, "  %s  -> %s  @call:%d%s%s\n",
			c.QName, target, c.CallLine, ambigTag(c.Ambiguous), depTag(c.Dep, c.DepModified))
	}
	if a.Total > len(a.Callees) {
		fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", a.Total-len(a.Callees))
	}
	return b.String()
}

// FindAnswer is ranked symbol search results.
type FindAnswer struct {
	Query   string    `json:"query"`
	Total   int       `json:"total"`
	Results []FindRef `json:"results"`
}

func (a *FindAnswer) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "find %q (%d):\n", a.Query, a.Total)
	for _, r := range a.Results {
		callers := ""
		if r.Callers > 0 {
			callers = fmt.Sprintf("  callers=%d", r.Callers)
		}
		fmt.Fprintf(&b, "  %s  %s  %s:%d%s%s  [%s]\n",
			r.QName, r.Kind, r.File, r.Line, callers, depTag(r.Dep, r.DepModified), r.Match)
	}
	if len(a.Results) == 0 {
		// A multi-token miss is usually a concept/feature phrase, not a
		// symbol name — route it to the tool that answers those.
		if len(search.Tokenize(a.Query)) > 1 {
			fmt.Fprintf(&b, "  (no symbol matches — this looks like a concept query; use `search` for feature/topic questions)\n")
		} else {
			fmt.Fprintf(&b, "  (no symbol matches — try plain grep for content search)\n")
		}
	}
	return b.String()
}

// GrepAnswer is content hits attributed to enclosing symbols.
type GrepAnswer struct {
	Pattern string    `json:"pattern"`
	Word    bool      `json:"word,omitempty"`
	RawHits int       `json:"raw_hits"`
	Backend string    `json:"backend"`
	Groups  []GrepRef `json:"groups"`
}

func (a *GrepAnswer) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "grep %q: %d raw hits -> %d symbols/sites (%s)\n",
		a.Pattern, a.RawHits, len(a.Groups), a.Backend)
	for _, g := range a.Groups {
		name := g.QName
		if name == "" {
			name = "<outside any symbol>"
		}
		def := ""
		if g.IsDef {
			def = "  [definition]"
		}
		fmt.Fprintf(&b, "  %s  %s:%d  hits=%d%s\n", name, g.File, g.Line, g.Hits, def)
	}
	return b.String()
}

// NavAnswer is the one-shot navigation union for an anchor: definitions,
// ranked name matches, callers, and every referencing file, from a single
// over-retrieval pass (callers + find + grep). Measured (bench/scout): the
// union answer matches the per-tool formatter ceiling with zero routing.
type NavAnswer struct {
	Anchor          string      `json:"anchor"`
	Definitions     []DefRef    `json:"definitions"`
	Matches         []FindRef   `json:"matches"`
	CallersTotal    int         `json:"callers_total"`
	Callers         []CallerRef `json:"callers"`
	CallersFromGrep bool        `json:"callers_from_grep,omitempty"`
	FilesTotal      int         `json:"files_total"`
	Files           []string    `json:"files"`
}

func (a *NavAnswer) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "nav %s: %d definition(s), %d caller(s), %d referencing file(s)\n",
		a.Anchor, len(a.Definitions), a.CallersTotal, a.FilesTotal)
	writeDefs(&b, a.Anchor, a.Definitions)
	if len(a.Matches) > 0 {
		fmt.Fprintf(&b, "matches:\n")
		for _, r := range a.Matches {
			callers := ""
			if r.Callers > 0 {
				callers = fmt.Sprintf("  callers=%d", r.Callers)
			}
			fmt.Fprintf(&b, "  %s  %s  %s:%d%s%s  [%s]\n",
				r.QName, r.Kind, r.File, r.Line, callers, depTag(r.Dep, r.DepModified), r.Match)
		}
	}
	if a.CallersFromGrep {
		fmt.Fprintf(&b, "callers (%d, via grep attribution — no call edges in the graph):\n", a.CallersTotal)
	} else {
		fmt.Fprintf(&b, "callers (%d):\n", a.CallersTotal)
	}
	for _, c := range a.Callers {
		fmt.Fprintf(&b, "  %s:%d  %s%s\n", c.File, c.Line, c.QName, ambigTag(c.Ambiguous))
	}
	if a.CallersTotal > len(a.Callers) {
		fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", a.CallersTotal-len(a.Callers))
	}
	fmt.Fprintf(&b, "referenced in %d file(s):", a.FilesTotal)
	for _, f := range a.Files {
		fmt.Fprintf(&b, " %s", f)
	}
	if a.FilesTotal > len(a.Files) {
		fmt.Fprintf(&b, " ... (+%d more)", a.FilesTotal-len(a.Files))
	}
	fmt.Fprintf(&b, "\n")
	return b.String()
}

// DependentsAnswer is who imports/extends/implements the anchor.
type DependentsAnswer struct {
	Anchor     string         `json:"anchor"`
	Total      int            `json:"dependents_total"`
	Dependents []DependentRef `json:"dependents"`
}

func (a *DependentsAnswer) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "dependents of %s (%d):\n", a.Anchor, a.Total)
	for _, d := range a.Dependents {
		fmt.Fprintf(&b, "  %-10s %s:%d  %s\n", d.Kind, d.File, d.Line, d.QName)
	}
	if a.Total > len(a.Dependents) {
		fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", a.Total-len(a.Dependents))
	}
	return b.String()
}

// DepSection is one labeled group of outgoing dependencies.
type DepSection struct {
	Label string   `json:"label"`
	Total int      `json:"total"`
	Deps  []DepRef `json:"deps"`
}

// DepsAnswer is what the anchor depends on, in one or more sections (a symbol
// anchor also lists its defining file's imports).
type DepsAnswer struct {
	Anchor   string       `json:"anchor"`
	Sections []DepSection `json:"sections"`
}

func (a *DepsAnswer) Text() string {
	var b strings.Builder
	for _, s := range a.Sections {
		fmt.Fprintf(&b, "%s (%d):\n", s.Label, s.Total)
		for _, d := range s.Deps {
			target := d.Target
			if d.DefFile != "" {
				target = fmt.Sprintf("%s (%s:%d)", d.Target, d.DefFile, d.DefLine)
			}
			fmt.Fprintf(&b, "  %-10s %s  @%d\n", d.Kind, target, d.Line)
		}
		if s.Total > len(s.Deps) {
			fmt.Fprintf(&b, "  ... (+%d more; raise limit)\n", s.Total-len(s.Deps))
		}
	}
	return b.String()
}

// ImpactAnswer is the counts-first blast-radius summary. Callees may include
// unresolved entries (DefFile==""); Text() omits them but they count toward
// the truncation arithmetic, matching the historical output.
type ImpactAnswer struct {
	Anchor          string         `json:"anchor"`
	Coverage        string         `json:"coverage"`
	Definitions     []DefRef       `json:"definitions"`
	CallersTotal    int            `json:"callers_total"`
	Callers         []CallerRef    `json:"callers"`
	DependentsTotal int            `json:"dependents_total"`
	Dependents      []DependentRef `json:"dependents"`
	CalleesTotal    int            `json:"callees_total"`
	Callees         []CalleeRef    `json:"callees"`
}

func (a *ImpactAnswer) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "impact of %s: %d definition(s), %d caller(s), %d callee(s), %d dependent(s)\n",
		a.Anchor, len(a.Definitions), a.CallersTotal, a.CalleesTotal, a.DependentsTotal)
	fmt.Fprintf(&b, "(coverage: %s)\n\n", a.Coverage)
	writeDefs(&b, a.Anchor, a.Definitions)
	fmt.Fprintf(&b, "\ncallers — these break if %s's behavior/signature changes:\n", a.Anchor)
	for _, c := range a.Callers {
		fmt.Fprintf(&b, "  %s:%d  %s%s\n", c.File, c.Line, c.QName, ambigTag(c.Ambiguous))
	}
	if a.CallersTotal > len(a.Callers) {
		fmt.Fprintf(&b, "  ... (+%d more)\n", a.CallersTotal-len(a.Callers))
	}
	fmt.Fprintf(&b, "\ndependents — who imports/extends/implements %s:\n", a.Anchor)
	for _, d := range a.Dependents {
		fmt.Fprintf(&b, "  %-10s %s:%d  %s\n", d.Kind, d.File, d.Line, d.QName)
	}
	if a.DependentsTotal > len(a.Dependents) {
		fmt.Fprintf(&b, "  ... (+%d more)\n", a.DependentsTotal-len(a.Dependents))
	}
	fmt.Fprintf(&b, "\ncallees — what %s depends on:\n", a.Anchor)
	for _, c := range a.Callees {
		if c.DefFile == "" {
			continue // unresolved (stdlib/external) — noise in an impact summary
		}
		fmt.Fprintf(&b, "  %s  %s:%d%s\n", c.QName, c.DefFile, c.DefLine, depTag(c.Dep, c.DepModified))
	}
	if a.CalleesTotal > len(a.Callees) {
		fmt.Fprintf(&b, "  ... (+%d more)\n", a.CalleesTotal-len(a.Callees))
	}
	return b.String()
}

// EnclosingAnswer is the symbols overlapping a line range. An empty result
// renders as no output (exit 0) — the edit-hook contract.
type EnclosingAnswer struct {
	File    string         `json:"file"`
	Start   int            `json:"start"`
	End     int            `json:"end"`
	Symbols []EnclosingRef `json:"symbols"`
}

func (a *EnclosingAnswer) Text() string {
	var b strings.Builder
	for _, e := range a.Symbols {
		fmt.Fprintf(&b, "sym  %s  %s  %s:%d-%d  callers=%d external=%d\n",
			e.Name, e.Kind, e.File, e.StartLine, e.EndLine, e.Callers, e.ExternalCallers)
	}
	return b.String()
}

func writeDefs(b *strings.Builder, anchor string, defs []DefRef) {
	for _, d := range defs {
		fmt.Fprintf(b, "def  %s  %s:%d  %s\n", d.QName, d.File, d.Line, d.Signature)
	}
	if len(defs) == 0 {
		fmt.Fprintf(b, "def  %s: (not found in index)\n", anchor)
	}
}

func ambigTag(ambiguous bool) string {
	if ambiguous {
		return "  [ambiguous]"
	}
	return ""
}

func depTag(dep string, modified bool) string {
	if dep == "" {
		return ""
	}
	if modified {
		return "  [dep " + dep + " modified]"
	}
	return "  [dep " + dep + "]"
}
