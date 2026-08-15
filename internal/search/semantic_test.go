package search

import (
	"context"
	"strings"
	"testing"

	"codeindex/internal/graph"
)

// fakeProvider embeds by token overlap against fixed vocab positions — a
// deterministic stand-in exercising the pipeline without model weights.
type fakeProvider struct{ vocab []string }

func (f *fakeProvider) ID() string   { return "fake@test" }
func (f *fakeProvider) Dims() int    { return len(f.vocab) }
func (f *fakeProvider) Close() error { return nil }
func (f *fakeProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, len(f.vocab))
		low := strings.ToLower(t)
		for j, w := range f.vocab {
			if strings.Contains(low, w) {
				v[j] = 1
			}
		}
		var s float32
		for _, x := range v {
			s += x * x
		}
		if s > 0 {
			// normalize
			inv := 1 / sqrt32(s)
			for j := range v {
				v[j] *= inv
			}
		}
		out[i] = v
	}
	return out, nil
}

func sqrt32(x float32) float32 {
	// Newton iterations are plenty for test purposes.
	g := x / 2
	for i := 0; i < 20; i++ {
		g = (g + x/g) / 2
	}
	return g
}

// seedGraph builds a small two-feature repo graph directly through the store:
// an onboarding cluster (StartOnboarding -> verifyHost -> createListing) and
// an unrelated parser cluster (ParseFile -> readToken).
func seedGraph(t *testing.T) *graph.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := graph.Open(dir + "/graph.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	put := func(file string, syms []graph.Symbol, calls []graph.RawCall) {
		tx, err := st.Begin()
		if err != nil {
			t.Fatal(err)
		}
		pf := &graph.ParsedFile{Path: file, Symbols: syms, Calls: calls}
		if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: file, Hash: file, Size: 1, Mtime: 1}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	put("onboard/start.go", []graph.Symbol{
		{File: "onboard/start.go", Name: "StartOnboarding", Kind: graph.KindFunc, StartLine: 5, EndLine: 20, Signature: "func StartOnboarding()"},
		{File: "onboard/start.go", Name: "verifyHost", Kind: graph.KindFunc, StartLine: 22, EndLine: 30},
	}, []graph.RawCall{
		{EnclosingIdx: 0, Callee: "verifyHost", Line: 7},
		{EnclosingIdx: 0, Callee: "createListing", Line: 9},
	})
	put("onboard/listing.go", []graph.Symbol{
		{File: "onboard/listing.go", Name: "createListing", Kind: graph.KindFunc, StartLine: 3, EndLine: 12},
	}, nil)
	put("parse/parse.go", []graph.Symbol{
		{File: "parse/parse.go", Name: "ParseFile", Kind: graph.KindFunc, StartLine: 4, EndLine: 30},
		{File: "parse/parse.go", Name: "readToken", Kind: graph.KindFunc, StartLine: 32, EndLine: 40},
	}, []graph.RawCall{{EnclosingIdx: 0, Callee: "readToken", Line: 6}})

	// Re-resolve after all files exist (mirrors the build pass).
	names, err := st.AllDstNames()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReResolveNames(tx, names); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return st
}

// embedAll stores card vectors for every symbol via the fake provider using
// name-derived card text.
func embedAll(t *testing.T, st *graph.Store, prov *fakeProvider) {
	t.Helper()
	syms, _, err := st.AllSymbolsWithCallers()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range syms {
		text := strings.Join(Tokenize(s.Name), " ")
		vecs, err := prov.Embed(context.Background(), []string{text})
		if err != nil {
			t.Fatal(err)
		}
		hash := "h-" + s.Name
		if err := st.PutVec(tx, hash, prov.ID(), graph.QuantizeInt8(vecs[0])); err != nil {
			t.Fatal(err)
		}
		if err := st.PutSymbolVec(tx, s.ID, hash); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.PutVecModelStamp(tx, prov.ID()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

var vocab = []string{"start", "onboard", "verify", "host", "create", "listing", "parse", "file", "read", "token", "signup", "flow"}

func TestSemanticConceptQuery(t *testing.T) {
	st := seedGraph(t)
	prov := &fakeProvider{vocab: vocab}
	embedAll(t, st, prov)

	res, err := Semantic(st, prov, "host onboarding signup flow", SemanticOpts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Degraded != "" {
		t.Fatalf("unexpected degradation: %s", res.Degraded)
	}
	if len(res.Flat) == 0 {
		t.Fatal("no results for concept query")
	}
	// Onboarding symbols must lead; parser cluster must not be first.
	if got := res.Flat[0].Sym.Name; got != "StartOnboarding" && got != "verifyHost" {
		t.Fatalf("top result = %s, want onboarding-cluster symbol", got)
	}

	// Clustering: onboarding trio connected, parser pair separate.
	if len(res.Clusters) < 2 {
		t.Fatalf("clusters = %d, want >= 2", len(res.Clusters))
	}
	first := res.Clusters[0]
	names := map[string]bool{first.Entry.Sym.Name: true}
	for _, m := range first.Members {
		names[m.Sym.Name] = true
	}
	if !names["StartOnboarding"] || !names["createListing"] {
		t.Fatalf("first cluster %v missing onboarding members", names)
	}
	if names["ParseFile"] || names["readToken"] {
		t.Fatalf("parser symbols leaked into onboarding cluster: %v", names)
	}
}

func TestSemanticExactNamePrecedence(t *testing.T) {
	st := seedGraph(t)
	prov := &fakeProvider{vocab: vocab}
	embedAll(t, st, prov)

	// "createListing" matches ParseFile-ish vectors weakly everywhere, but an
	// exact name match must rank first regardless of the vector lane.
	res, err := Semantic(st, prov, "createListing", SemanticOpts{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Flat[0].Sym.Name != "createListing" {
		t.Fatalf("top = %s, want exact-name createListing", res.Flat[0].Sym.Name)
	}
}

func TestSemanticHints(t *testing.T) {
	st := seedGraph(t)
	prov := &fakeProvider{vocab: vocab}
	embedAll(t, st, prov)

	// Vague query alone misses lexically; hints carry identifier guesses.
	res, err := Semantic(st, prov, "guest signup", SemanticOpts{Hints: []string{"onboarding", "listing"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range res.Flat {
		if r.Sym.Name == "StartOnboarding" || r.Sym.Name == "createListing" {
			found = true
		}
	}
	if !found {
		t.Fatal("hints did not surface onboarding symbols")
	}
}

// Observed runtime edges bridge statically-disconnected clusters: with an
// obs edge StartOnboarding -> readToken, an onboarding query diffuses mass
// into readToken, the result disclose the evidence, and clustering joins
// the two regions. Without obs rows, behavior is bit-identical to before
// (no-op parity is what every other test in this file keeps verifying).
func TestSemanticObservedEdge(t *testing.T) {
	st := seedGraph(t)
	prov := &fakeProvider{vocab: vocab}
	embedAll(t, st, prov)

	base, err := Semantic(st, prov, "host onboarding signup flow", SemanticOpts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if base.ObsNote != "" {
		t.Fatalf("no-obs run disclosed evidence: %q", base.ObsNote)
	}
	baseHasRead := false
	for _, r := range base.Flat {
		if r.Sym.Name == "readToken" {
			baseHasRead = true
		}
	}

	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddObsEdge(tx,
		graph.ObsKey("onboard/start.go", "", "StartOnboarding"),
		graph.ObsKey("parse/parse.go", "", "readToken"), 500, true); err != nil {
		t.Fatal(err)
	}
	if err := st.AddObsHeat(tx,
		graph.ObsKey("onboard/start.go", "", "StartOnboarding"), 0, 500, 500); err != nil {
		t.Fatal(err)
	}
	if err := st.PutObsLedger(tx, "p.cxprof.jsonl", "h", "go", 1, 2, "", 3, 3, 4); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	res, err := Semantic(st, prov, "host onboarding signup flow", SemanticOpts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.ObsNote == "" {
		t.Fatal("observed evidence participated but was not disclosed")
	}
	// readToken must now carry diffused mass (rank above its no-obs state
	// or appear when it didn't).
	var withRead bool
	var readRank, baseReadRank = -1, -1
	for i, r := range res.Flat {
		if r.Sym.Name == "readToken" {
			withRead, readRank = true, i
		}
	}
	for i, r := range base.Flat {
		if r.Sym.Name == "readToken" {
			baseReadRank = i
		}
	}
	if !withRead {
		t.Fatal("observed edge did not pull readToken into results")
	}
	if baseHasRead && readRank >= baseReadRank {
		t.Fatalf("observed edge did not improve readToken rank (%d -> %d)", baseReadRank, readRank)
	}

	// Clustering joins the regions through the observed edge.
	joined := false
	for _, c := range res.Clusters {
		names := map[string]bool{c.Entry.Sym.Name: true}
		for _, m := range c.Members {
			names[m.Sym.Name] = true
		}
		if names["StartOnboarding"] && names["readToken"] {
			joined = true
		}
	}
	if !joined {
		t.Fatal("observed edge did not join clusters")
	}
}

func TestSemanticDegradation(t *testing.T) {
	st := seedGraph(t) // no vectors stored

	res, err := Semantic(st, &fakeProvider{vocab: vocab}, "StartOnboarding", SemanticOpts{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Degraded == "" {
		t.Fatal("want degradation notice for unembedded index")
	}
	if len(res.Flat) == 0 || res.Flat[0].Sym.Name != "StartOnboarding" {
		t.Fatal("lexical lane must still answer")
	}

	res, err = Semantic(st, nil, "StartOnboarding", SemanticOpts{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Degraded == "" {
		t.Fatal("want degradation notice for nil provider")
	}
}
