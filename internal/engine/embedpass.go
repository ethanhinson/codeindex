package engine

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codeindex/internal/config"
	"codeindex/internal/embed"
	"codeindex/internal/graph"
	"codeindex/internal/progress"
	"codeindex/internal/search"
)

// Card is one symbol's embedding input: deterministic text plus its content
// hash. Vectors are stored by hash, so unchanged text never re-embeds even
// though symbol ids churn on every re-parse.
type Card struct {
	SymbolID int64
	Hash     string
	Text     string
}

// Contrast constants (design D3, frozen before measurement): suppression
// applies in families of >= minFamily members to words present in >= supDF
// of them.
const (
	minFamily = 5
	supDF     = 0.8
)

// BuildCards computes cards for tier-0 symbols. files (repo-relative) limits
// the set to symbols defined in those files; nil means every symbol. Doc
// context is the contiguous comment block directly above the definition —
// read here, not parsed by adapters, so all four languages get it for free.
//
// Cards are contrast-weighted against the symbol's structural families
// (same parent type; same directory module): family-common doc words are
// suppressed, family-unique tokens emphasized in a `distinct:` field.
// Family stats are always computed over the FULL symbol table so patch and
// full-build card text agree. Cross-member drift (a sibling's doc change
// shifts this symbol's suppression set without re-carding it) is accepted,
// same class as the neighbor-name drift, refreshed by full builds.
func BuildCards(store *graph.Store, root string, files []string) ([]Card, error) {
	syms, _, err := store.AllSymbolsWithCallers()
	if err != nil {
		return nil, err
	}
	var fileSet map[string]bool
	if files != nil {
		fileSet = make(map[string]bool, len(files))
		for _, f := range files {
			fileSet[f] = true
		}
	}

	comments := map[string][]string{} // file -> line-indexed source (lazy)
	docs := make([]string, len(syms))
	docOf := func(i int) string {
		s := &syms[i]
		lines, ok := comments[s.File]
		if !ok {
			lines = readLines(filepath.Join(root, s.File))
			comments[s.File] = lines
		}
		doc := docComment(lines, s.StartLine)
		if doc == "" && strings.HasSuffix(s.File, ".py") {
			doc = pyDocstring(lines, s.StartLine, s.EndLine)
		}
		return doc
	}
	for i := range syms {
		if syms[i].Tier == 0 {
			docs[i] = docOf(i)
		}
	}
	con := buildContrast(syms, docs)

	var cards []Card
	for i := range syms {
		s := &syms[i]
		if s.Tier != 0 || (fileSet != nil && !fileSet[s.File]) {
			continue
		}
		text := cardText(s, con.filterDoc(i, docs[i]), con.distinct(i))
		cards = append(cards, Card{
			SymbolID: s.ID,
			Hash:     fmt.Sprintf("%x", sha256.Sum256([]byte(text))),
			Text:     text,
		})
	}
	return cards, nil
}

// contrast holds family-relative statistics: which doc words are family
// boilerplate (suppressed) and which tokens are family-unique (emphasized).
type contrast struct {
	famOf   [][]string                // symbol idx -> its family keys
	famDF   map[string]map[string]int // family -> doc word -> member count
	famSize map[string]int
	famTok  map[string]map[string]int // family -> token -> member count
	toks    []map[string]bool         // symbol idx -> own token set (name+doc)
}

func famKeys(s *graph.Symbol) []string {
	keys := make([]string, 0, 2)
	if s.Parent != "" {
		keys = append(keys, "p:"+s.Parent)
	}
	dir := ""
	if i := strings.LastIndexByte(s.File, '/'); i >= 0 {
		dir = s.File[:i]
	}
	keys = append(keys, "d:"+dir)
	return keys
}

func docWords(doc string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(doc)) {
		w = strings.Trim(w, ".,;:()[]{}\"'`!?")
		if len(w) >= 3 {
			out[w] = true
		}
	}
	return out
}

func buildContrast(syms []graph.Symbol, docs []string) *contrast {
	c := &contrast{
		famOf:   make([][]string, len(syms)),
		famDF:   map[string]map[string]int{},
		famSize: map[string]int{},
		famTok:  map[string]map[string]int{},
		toks:    make([]map[string]bool, len(syms)),
	}
	for i := range syms {
		s := &syms[i]
		if s.Tier != 0 {
			continue
		}
		keys := famKeys(s)
		c.famOf[i] = keys
		words := docWords(docs[i])
		tokens := map[string]bool{}
		for _, t := range search.Tokenize(s.Name) {
			tokens[t] = true
		}
		for w := range words {
			tokens[w] = true
		}
		c.toks[i] = tokens
		for _, k := range keys {
			c.famSize[k]++
			if c.famDF[k] == nil {
				c.famDF[k] = map[string]int{}
				c.famTok[k] = map[string]int{}
			}
			for w := range words {
				c.famDF[k][w]++
			}
			for t := range tokens {
				c.famTok[k][t]++
			}
		}
	}
	return c
}

// filterDoc removes family-boilerplate words from a doc (word appears in
// >= supDF of a >= minFamily-member family the symbol belongs to).
func (c *contrast) filterDoc(i int, doc string) string {
	if doc == "" || c.famOf[i] == nil {
		return doc
	}
	suppressed := func(w string) bool {
		lw := strings.Trim(strings.ToLower(w), ".,;:()[]{}\"'`!?")
		if len(lw) < 3 {
			return false
		}
		for _, k := range c.famOf[i] {
			if n := c.famSize[k]; n >= minFamily {
				if float64(c.famDF[k][lw]) >= supDF*float64(n) {
					return true
				}
			}
		}
		return false
	}
	fields := strings.Fields(doc)
	kept := fields[:0]
	for _, w := range fields {
		if !suppressed(w) {
			kept = append(kept, w)
		}
	}
	return strings.Join(kept, " ")
}

// distinct returns the symbol's family-unique tokens (in no other member of
// any of its families), deterministic order, capped.
func (c *contrast) distinct(i int) []string {
	if c.famOf[i] == nil {
		return nil
	}
	var out []string
	for t := range c.toks[i] {
		unique := true
		for _, k := range c.famOf[i] {
			if c.famSize[k] >= 2 && c.famTok[k][t] > 1 {
				unique = false
				break
			}
		}
		if unique {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// cardText renders the deterministic embedding input. Sentence-shaped (the
// bundled model is a sentence encoder), name tokens spelled out so concept
// words align with identifier conventions, family-distinct tokens emphasized
// early (they separate a symbol from its boilerplate siblings). Neighbor
// names were REMOVED at the diffusion-contrast freeze: query-time diffusion
// carries neighborhood semantics, measured strictly better than baking a
// fixed neighbor radius into the vectors (paired D2 run).
func cardText(s *graph.Symbol, doc string, distinct []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", s.QName(), s.Kind)
	if toks := search.Tokenize(s.Name); len(toks) > 0 {
		fmt.Fprintf(&b, ". name: %s", strings.Join(toks, " "))
	}
	if len(distinct) > 0 {
		fmt.Fprintf(&b, ". distinct: %s", strings.Join(distinct, " "))
	}
	if s.Signature != "" {
		fmt.Fprintf(&b, ". signature: %s", s.Signature)
	}
	fmt.Fprintf(&b, ". in %s", strings.Join(pathTokens(s.File), " "))
	if doc != "" {
		fmt.Fprintf(&b, ". doc: %s", doc)
	}
	return b.String()
}

// pathTokens splits a repo path into embedding-friendly words (segments and
// their convention tokens, extension dropped).
func pathTokens(path string) []string {
	path = strings.TrimSuffix(path, filepath.Ext(path))
	var out []string
	for _, seg := range strings.Split(path, "/") {
		toks := search.Tokenize(seg)
		if len(toks) == 0 {
			out = append(out, seg)
			continue
		}
		out = append(out, toks...)
	}
	return out
}

// readLines loads a source file for doc-comment extraction ([] on error —
// cards degrade to no doc, never fail the pass).
func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// docCommentScan bounds how far above a definition the comment block may
// start (long JSDoc blocks routinely run tens of lines).
const docCommentScan = 40

// docComment extracts the doc block above startLine (1-based) and returns its
// leading DESCRIPTION: lines are collected from the TOP of the block down,
// stopping at the first @tag — JSDoc/docblock summaries live at the top, and
// the lines adjacent to the definition are @param/@see/@publicApi tails that
// would poison the card (measured on nest: tail-extraction read '@see https
// docs...' instead of the summary and cost 15+ recall points).
func docComment(lines []string, startLine int) string {
	// Walk up to the block's first line.
	end := startLine - 2 // index of the line above the definition
	first := -1
	for i := end; i >= 0 && i > end-docCommentScan; i-- {
		t := strings.TrimSpace(lines[i])
		if !isCommentLine(t) {
			break
		}
		first = i
		if strings.HasPrefix(t, "/*") { // reached the block opener
			break
		}
	}
	if first < 0 {
		return ""
	}
	// Collect description lines top-down; @tag lines end the description.
	var parts []string
	for i := first; i <= end && len(parts) < 4; i++ {
		body := stripCommentMarker(strings.TrimSpace(lines[i]))
		if strings.HasPrefix(body, "@") {
			break
		}
		if body != "" {
			parts = append(parts, body)
		}
	}
	doc := strings.Join(parts, " ")
	if len(doc) > 240 {
		doc = doc[:240]
	}
	return strings.TrimSpace(doc)
}

func isCommentLine(t string) bool {
	return strings.HasPrefix(t, "//") ||
		(strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#!")) ||
		strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*")
}

func stripCommentMarker(t string) string {
	switch {
	case strings.HasPrefix(t, "//"):
		t = strings.TrimPrefix(strings.TrimPrefix(t, "//"), "/")
	case strings.HasPrefix(t, "#"):
		t = strings.TrimPrefix(t, "#")
	case strings.HasPrefix(t, "/*"):
		t = strings.TrimPrefix(strings.TrimPrefix(t, "/**"), "/*")
	case strings.HasPrefix(t, "*"):
		t = strings.TrimPrefix(t, "*")
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t), "*/"))
}

// pyDocstring extracts a Python docstring: the first triple-quoted block in
// the definition's body (Python documents below the def line, not above it).
func pyDocstring(lines []string, startLine, endLine int) string {
	for i := startLine; i < endLine && i < len(lines) && i < startLine+3; i++ {
		t := strings.TrimSpace(lines[i])
		q := ""
		switch {
		case strings.HasPrefix(t, `"""`):
			q = `"""`
		case strings.HasPrefix(t, "'''"):
			q = "'''"
		default:
			continue
		}
		body := strings.TrimPrefix(t, q)
		if j := strings.Index(body, q); j >= 0 { // one-liner
			return strings.TrimSpace(body[:j])
		}
		parts := []string{strings.TrimSpace(body)}
		for k := i + 1; k < len(lines) && len(parts) < 4; k++ {
			t := strings.TrimSpace(lines[k])
			if j := strings.Index(t, q); j >= 0 {
				parts = append(parts, strings.TrimSpace(t[:j]))
				break
			}
			parts = append(parts, t)
		}
		doc := strings.TrimSpace(strings.Join(parts, " "))
		if len(doc) > 240 {
			doc = doc[:240]
		}
		return doc
	}
	return ""
}

// embedBatch is the number of cards per provider call (bounds memory; the
// local provider serializes internally anyway).
const embedBatch = 32

// EmbedPass embeds cards and stores vectors + mappings. files scopes the pass
// (nil = full build: also prunes orphans and swaps models). Unavailable
// embedding (nollama build, corrupt model) is not an error: the pass reports
// and search degrades to lexical-only.
func EmbedPass(store *graph.Store, root string, files []string, rep progress.Reporter) (embedded int, err error) {
	cfg, err := config.Load(root)
	if err != nil {
		return 0, err
	}
	prov, note, err := embed.New(embed.Config{
		Provider:  cfg.Embed.Provider,
		Model:     cfg.Embed.Model,
		APIBase:   cfg.Embed.APIBase,
		APIKeyEnv: cfg.Embed.APIKeyEnv,
	})
	if err != nil {
		if errors.Is(err, embed.ErrUnavailable) {
			fmt.Fprintf(os.Stderr, "codeindex: embedding unavailable (%v); semantic search will be lexical-only\n", err)
			return 0, nil
		}
		return 0, err
	}
	defer prov.Close()
	if note != "" {
		fmt.Fprintf(os.Stderr, "codeindex: %s\n", note)
	}

	cards, err := BuildCards(store, root, files)
	if err != nil {
		return 0, err
	}
	if len(cards) == 0 {
		return 0, nil
	}

	hashes := make([]string, len(cards))
	textByHash := map[string]string{}
	for i, c := range cards {
		hashes[i] = c.Hash
		textByHash[c.Hash] = c.Text
	}
	missing, err := store.MissingVecs(hashes, prov.ID())
	if err != nil {
		return 0, err
	}

	tx, err := store.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Model swap: stamp mismatch invalidates every cached vector up front so
	// coverage counting stays truthful.
	if stamp, err := store.VecModelStamp(); err == nil && stamp != "" && stamp != prov.ID() {
		if err := store.DropVecsForOtherModels(tx, prov.ID()); err != nil {
			return 0, err
		}
	}

	for start := 0; start < len(missing); start += embedBatch {
		end := start + embedBatch
		if end > len(missing) {
			end = len(missing)
		}
		texts := make([]string, end-start)
		for i, h := range missing[start:end] {
			texts[i] = textByHash[h]
		}
		vecs, err := prov.Embed(context.Background(), texts)
		if err != nil {
			return 0, fmt.Errorf("embed pass: %w", err)
		}
		for i, h := range missing[start:end] {
			if err := store.PutVec(tx, h, prov.ID(), graph.QuantizeInt8(vecs[i])); err != nil {
				return 0, err
			}
		}
		embedded = end
		if rep != nil {
			rep.Report(progress.Event{Phase: "embed", Done: end, Total: len(missing)})
		}
	}
	for _, c := range cards {
		if err := store.PutSymbolVec(tx, c.SymbolID, c.Hash); err != nil {
			return 0, err
		}
	}
	if err := store.PutVecModelStamp(tx, prov.ID()); err != nil {
		return 0, err
	}
	if files == nil { // full build: drop content vectors nothing maps to anymore
		if err := store.PruneVecs(tx); err != nil {
			return 0, err
		}
	}
	return len(missing), tx.Commit()
}
