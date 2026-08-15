package query

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"codeindex/internal/config"
	"codeindex/internal/embed"
	"codeindex/internal/search"
)

// Long-lived surfaces (MCP server) search repeatedly; loading GGUF weights
// per query would dominate latency, so the provider is cached per config
// signature and swapped when .codeindex.json changes.
var (
	provMu   sync.Mutex
	provider embed.Provider
	provKey  string
	provNote string
)

func sharedProvider(root string) (embed.Provider, string) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, "config: " + err.Error()
	}
	key := strings.Join([]string{cfg.Embed.Provider, cfg.Embed.Model, cfg.Embed.APIBase, cfg.Embed.APIKeyEnv}, "\x00")
	provMu.Lock()
	defer provMu.Unlock()
	if provider != nil && provKey == key {
		return provider, provNote
	}
	if provider != nil {
		provider.Close()
		provider = nil
	}
	p, note, err := embed.New(embed.Config{
		Provider:  cfg.Embed.Provider,
		Model:     cfg.Embed.Model,
		APIBase:   cfg.Embed.APIBase,
		APIKeyEnv: cfg.Embed.APIKeyEnv,
	})
	if err != nil {
		if errors.Is(err, embed.ErrUnavailable) {
			provKey, provNote = key, err.Error()
			return nil, provNote
		}
		return nil, err.Error()
	}
	provider, provKey, provNote = p, key, note
	return provider, note
}

// SearchText is the hybrid concept search: lexical + vector + literal lanes
// fused, graph-ranked, clustered into a feature map (flat=true for the plain
// ranked list). Degradation to lexical-only is stated on the first line.
// errorText carries stack traces / quoted error output (maximum literal-lane
// authority).
func SearchText(root, q string, hints []string, errorText string, limit int, flat bool) (string, error) {
	st, err := open(root)
	if err != nil {
		return "", err
	}
	defer st.Close()

	prov, note := sharedProvider(root)
	res, err := search.Semantic(st, prov, q, search.SemanticOpts{
		Hints: hints, Limit: limit, Root: root, ErrorText: errorText})
	if err != nil {
		return "", err
	}
	if res.Degraded == "" && prov == nil {
		res.Degraded = note
	}

	var b strings.Builder
	head := fmt.Sprintf("search %q (%d symbols, %d clusters)", q, len(res.Flat), len(res.Clusters))
	if res.Degraded != "" {
		head += " [lexical-only: " + res.Degraded + "]"
	}
	if res.ObsNote != "" {
		head += " [" + res.ObsNote + "]"
	}
	b.WriteString(head + ":\n")
	if len(res.Flat) == 0 {
		b.WriteString("  (no matches — try different concept words, or add hints)\n")
		return b.String(), nil
	}

	if flat {
		for _, r := range res.Flat {
			writeScored(&b, "  ", r)
		}
		return b.String(), nil
	}
	for i, c := range res.Clusters {
		fmt.Fprintf(&b, "cluster %d — %s:\n", i+1, c.Label)
		writeScored(&b, "  entry: ", c.Entry)
		for _, m := range c.Members {
			writeScored(&b, "         ", m)
		}
	}
	return b.String(), nil
}

func writeScored(b *strings.Builder, prefix string, r search.Scored) {
	// Signatures can span lines (TS/PHP); the output contract is one result
	// per line, so collapse all internal whitespace runs.
	sig := strings.Join(strings.Fields(r.Sym.Signature), " ")
	if len(sig) > 72 {
		sig = sig[:69] + "..."
	}
	if sig != "" {
		sig = "  " + sig
	}
	callers := ""
	if r.Callers > 0 {
		callers = fmt.Sprintf("  callers=%d", r.Callers)
	}
	fmt.Fprintf(b, "%s%s  %s  %s:%d%s%s  [%s]\n",
		prefix, r.Sym.QName(), r.Sym.Kind, r.Sym.File, r.Sym.StartLine, callers, sig, r.Lanes)
}
