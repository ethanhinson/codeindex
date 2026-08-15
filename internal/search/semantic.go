package search

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"codeindex/internal/embed"
	"codeindex/internal/graph"
)

// SemanticOpts parameterize a hybrid search.
type SemanticOpts struct {
	Hints     []string // client-LLM concept→identifier expansion tokens
	Limit     int      // symbols in the final answer (default 20)
	Root      string   // repo root; enables the literal lane ("" disables)
	ErrorText string   // stack trace / quoted error output (max literal authority)
}

// Scored is one fused result with its explanation inputs.
type Scored struct {
	Sym     graph.Symbol
	Callers int
	Score   float64
	Lanes   string // "lex", "vec", or "lex+vec" (explainability)
}

// Cluster is one call-graph-connected group of results: the feature-map unit.
// Entry is the member with the most callers (the place to start reading).
type Cluster struct {
	Label   string
	Entry   Scored
	Members []Scored // excluding Entry, ordered by fused score
}

// SemanticResult is a full hybrid search answer.
type SemanticResult struct {
	Clusters []Cluster
	Flat     []Scored
	Degraded string // non-empty: why the vector lane was absent
	ObsNote  string // non-empty: observed runtime evidence participated (provenance/age)
}

const (
	lexRrfK  = 60 // lexical lane: the find ladder already tiers quality coarsely
	vecRrfK  = 10 // vector lane: steep — cosine rank IS the semantic signal
	vecLaneK = 50 // vector-lane candidates before fusion
	lexLaneK = 50 // lexical-lane candidates before fusion

	// boostGamma compresses graph boosts (~0.4–3.9× raw) so usage breaks
	// ties among semantic peers instead of letting a 200-caller generic
	// outrank the semantically-right answer (measured on the gin concept
	// class: raw boosts inverted the vector lane's top ranks).
	boostGamma = 0.35
)

// Semantic runs the hybrid pipeline: lexical lane (find ladder over query +
// hints) and vector lane (query embedding vs card vectors) fused by RRF,
// graph-boosted, exact-name pinned first, then clustered by call-graph
// connectivity. prov may be nil (or the index unembedded): the result
// degrades to the lexical lane and says so.
func Semantic(st *graph.Store, prov embed.Provider, query string, opts SemanticOpts) (*SemanticResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	// Lexical lane: the deterministic ladder, plus a hint-expanded pass
	// merged by best rank per symbol.
	lexRank := map[int64]int{}
	lex, err := Find(st, query, Opts{Limit: lexLaneK})
	if err != nil {
		return nil, err
	}
	for i, r := range lex {
		lexRank[r.Sym.ID] = i
	}
	if len(opts.Hints) > 0 {
		hintQ := strings.Join(opts.Hints, " ")
		hres, err := Find(st, hintQ, Opts{Limit: lexLaneK})
		if err != nil {
			return nil, err
		}
		for i, r := range hres {
			if cur, ok := lexRank[r.Sym.ID]; !ok || i < cur {
				lexRank[r.Sym.ID] = i
			}
		}
	}

	// Vector lane. Absence is disclosed, never fatal.
	vecRank := map[int64]int{}
	degraded := ""
	if prov == nil {
		degraded = "no embedding provider in this build"
	} else {
		n, err := st.VecCount(prov.ID())
		switch {
		case err != nil:
			return nil, err
		case n == 0:
			degraded = "index has no embeddings for the active model — run `codeindex build` to backfill"
		default:
			qtext := query
			if len(opts.Hints) > 0 {
				qtext += " " + strings.Join(opts.Hints, " ")
			}
			vecs, err := prov.Embed(context.Background(), []string{qtext})
			if err != nil {
				degraded = "query embedding failed: " + err.Error()
			} else {
				hits, err := st.TopKByVec(vecs[0], prov.ID(), vecLaneK)
				if err != nil {
					return nil, err
				}
				for i, h := range hits {
					vecRank[h.SymbolID] = i
				}
			}
		}
	}

	// Fusion + graph boosts over the union of lanes.
	syms, callers, err := st.AllSymbolsWithCallers()
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*graph.Symbol, len(syms))
	for i := range syms {
		byID[syms[i].ID] = &syms[i]
	}

	// Observed heat (design D6): executed code outranks never-executed
	// declarations (interfaces, doc stubs) inside the SAME compressed boost
	// envelope as caller counts — evidence shades, never floods. Loaded
	// before fusion so it participates in ranking, not just entry selection
	// (the runtime gate caught this half of D6 unimplemented).
	var heat map[int64]graph.ObsHeat
	if has, herr := st.HasObs(); herr == nil && has {
		if hk, herr := st.ObsHeatByKey(); herr == nil {
			keyToID := make(map[string]int64, len(syms))
			for i := range syms {
				s := &syms[i]
				if s.Tier == 0 {
					keyToID[graph.ObsKey(s.File, s.Parent, s.Name)] = s.ID
				}
			}
			heat = make(map[int64]graph.ObsHeat, len(hk))
			for k, h := range hk {
				if id, ok := keyToID[k]; ok {
					heat[id] = h
				}
			}
		}
	}
	heatBoost := func(id int64) float64 {
		if heat == nil {
			return 1
		}
		return 1 + math.Log10(1+float64(heat[id].Total))
	}
	// Literal lane (third lane): grep-attributed content evidence,
	// self-weighted. Symptom language lives in string literals that names
	// and cards never carry.
	lit := buildLiteralLane(st, opts.Root, query, opts.ErrorText, len(syms))

	qLower := strings.ToLower(strings.TrimSpace(query))
	ids := union(lexRank, vecRank)
	for id := range lit.rank {
		ids[id] = struct{}{}
	}
	fusedScore := map[int64]float64{}
	lanesOf := map[int64]string{}
	for id := range ids {
		s := byID[id]
		if s == nil {
			continue // vector mapping raced a deletion; skip
		}
		score := 0.0
		lanes := []string{}
		if r, ok := lexRank[id]; ok {
			score += 1.0 / float64(lexRrfK+r)
			lanes = append(lanes, "lex")
		}
		if r, ok := vecRank[id]; ok {
			score += 1.0 / float64(vecRrfK+r)
			lanes = append(lanes, "vec")
		}
		if r, ok := lit.rank[id]; ok {
			score += lit.conf / float64(litK+r)
			lanes = append(lanes, "lit")
		}
		score *= math.Pow(boosts(s, callers[id])*heatBoost(id), boostGamma)
		fusedScore[id] = score
		lanesOf[id] = strings.Join(lanes, "+")
	}
	// Verbatim-phrase pins label as literal evidence; the score pin itself
	// applies AFTER the diffusion blend (next to exact-name) so it cannot
	// distort seed normalization.
	for _, id := range lit.pins {
		if byID[id] != nil && !strings.Contains(lanesOf[id], "lit") {
			if lanesOf[id] != "" {
				lanesOf[id] += "+"
			}
			lanesOf[id] += "lit"
		}
	}

	// Diffusion (design D1): the top fused hits seed a bounded PPR over the
	// symbol graph; final score blends fused and diffused mass. Relevance
	// flows to graph neighbors — including symbols no lane retrieved.
	seeds := topIDs(fusedScore, diffusionSeedK)
	seedScores := make(map[int64]float64, len(seeds))
	for _, id := range seeds {
		seedScores[id] = fusedScore[id]
	}
	// The subgraph always feeds clustering (D4); the PPR itself only runs
	// when its mass participates in ranking.
	var static [][2]int64
	if len(seeds) > 0 {
		static, err = st.NeighborhoodEdges(seeds, subgraphNodes, subgraphDegree)
		if err != nil {
			return nil, err
		}
	}
	pairs := make([]wedge, 0, len(static))
	subNodes := map[int64]bool{}
	for _, p := range static {
		pairs = append(pairs, wedge{p[0], p[1], 1})
		subNodes[p[0]] = true
		subNodes[p[1]] = true
	}
	for _, id := range seeds {
		subNodes[id] = true
	}

	// Observed runtime edges (additive evidence, D6/D7): union in edges that
	// touch the subgraph, keyed by content key -> live symbol id (stale keys
	// simply don't match). Weight compressed — evidence shades flow, never
	// floods it.
	obsNote := ""
	if has, err := st.HasObs(); err == nil && has {
		obsEdges, err := st.ObsEdges()
		if err != nil {
			return nil, err
		}
		keyToID := make(map[string]int64, len(syms))
		for i := range syms {
			s := &syms[i]
			if s.Tier == 0 {
				keyToID[graph.ObsKey(s.File, s.Parent, s.Name)] = s.ID
			}
		}
		used := 0
		for _, e := range obsEdges {
			a, aok := keyToID[e.SrcKey]
			b, bok := keyToID[e.DstKey]
			if !aok || !bok || a == b {
				continue
			}
			if !subNodes[a] && !subNodes[b] {
				continue
			}
			pairs = append(pairs, wedge{a, b, 1 + math.Log10(1+float64(e.Weight))})
			subNodes[a], subNodes[b] = true, true
			used++
		}
		if used > 0 {
			// heat was loaded before fusion (D6 ranking); disclose here.
			if end, _, err := st.ObsProvenance(); err == nil && end > 0 {
				obsNote = obsAge(end)
			} else {
				obsNote = "observed evidence active"
			}
		}
	}

	var diffused map[int64]float64
	if diffusionLambda > 0 {
		diffused = diffuse(pairs, seedScores, diffusionAlpha, diffusionIters)
	}

	maxFused := maxVal(fusedScore)
	maxDiff := maxVal(diffused)
	final := map[int64]float64{}
	for id, f := range fusedScore {
		final[id] = (1 - diffusionLambda) * f / maxFused
	}
	for id, d := range diffused {
		if byID[id] == nil {
			continue
		}
		final[id] += diffusionLambda * d / maxDiff
	}
	// Exactness ladder, applied above the blend: exact symbol name (rung 1)
	// beats a verbatim phrase-in-span match (rung 2) beats everything fused.
	for _, id := range lit.pins {
		if _, ok := final[id]; ok {
			final[id] += phrasePin
		} else if byID[id] != nil {
			final[id] = phrasePin
		}
	}
	for id := range final {
		s := byID[id]
		if strings.ToLower(s.Name) == qLower || strings.ToLower(s.QName()) == qLower {
			final[id] += 1000
		}
	}

	var results []Scored
	for id, score := range final {
		s := byID[id]
		lanes := lanesOf[id]
		if lanes == "" {
			lanes = "graph" // entered via diffusion only
		}
		results = append(results, Scored{Sym: *s, Callers: callers[id], Score: score, Lanes: lanes})
	}
	sort.Slice(results, func(a, b int) bool {
		if results[a].Score != results[b].Score {
			return results[a].Score > results[b].Score
		}
		if results[a].Sym.Name != results[b].Sym.Name {
			return results[a].Sym.Name < results[b].Sym.Name
		}
		if results[a].Sym.File != results[b].Sym.File {
			return results[a].Sym.File < results[b].Sym.File
		}
		return results[a].Sym.StartLine < results[b].Sym.StartLine
	})
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	clusters := clusterOverSubgraph(results, pairs, diffused, heat)
	return &SemanticResult{Clusters: clusters, Flat: results, Degraded: degraded, ObsNote: obsNote}, nil
}

// obsAge renders provenance age for disclosure ("observed 3d ago").
func obsAge(newestEnd int64) string {
	age := time.Since(time.Unix(newestEnd, 0))
	switch {
	case age < time.Hour:
		return "observed <1h ago"
	case age < 48*time.Hour:
		return fmt.Sprintf("observed %dh ago", int(age.Hours()))
	default:
		return fmt.Sprintf("observed %dd ago", int(age.Hours()/24))
	}
}

// topIDs returns the k highest-scored ids, deterministic tie-break by id.
func topIDs(scores map[int64]float64, k int) []int64 {
	ids := make([]int64, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool {
		if scores[ids[a]] != scores[ids[b]] {
			return scores[ids[a]] > scores[ids[b]]
		}
		return ids[a] < ids[b]
	})
	if len(ids) > k {
		ids = ids[:k]
	}
	return ids
}

func maxVal(m map[int64]float64) float64 {
	max := 0.0
	for _, v := range m {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return 1
	}
	return max
}

func union(a, b map[int64]int) map[int64]struct{} {
	u := make(map[int64]struct{}, len(a)+len(b))
	for k := range a {
		u[k] = struct{}{}
	}
	for k := range b {
		u[k] = struct{}{}
	}
	return u
}

// clusterOverSubgraph groups results by connectivity over the DIFFUSION
// subgraph (design D4): two results joined only through an intermediate
// symbol still share a cluster. Entry selection prefers externally-invoked
// observed symbols (runtime ground truth), then diffused mass — the
// query-conditioned centrality — then raw caller count.
func clusterOverSubgraph(results []Scored, pairs []wedge, diffused map[int64]float64, heat map[int64]graph.ObsHeat) []Cluster {
	if len(results) == 0 {
		return nil
	}
	// Union-find over ALL subgraph nodes plus the results.
	parent := map[int64]int64{}
	var find func(int64) int64
	find = func(x int64) int64 {
		p, ok := parent[x]
		if !ok || p == x {
			parent[x] = x
			return x
		}
		root := find(p)
		parent[x] = root
		return root
	}
	union := func(a, b int64) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	for _, p := range pairs {
		union(p.a, p.b)
	}

	groups := map[int64][]Scored{}
	for _, r := range results {
		groups[find(r.Sym.ID)] = append(groups[find(r.Sym.ID)], r)
	}
	var clusters []Cluster
	for _, members := range groups {
		// Members arrive score-ordered (results was sorted); entry =
		// runtime-observed external entry first, then diffused mass, then
		// callers.
		entry := 0
		for i, m := range members {
			mh, bh := heat[m.Sym.ID].Entry, heat[members[entry].Sym.ID].Entry
			me, be := diffused[m.Sym.ID], diffused[members[entry].Sym.ID]
			switch {
			case mh > bh:
				entry = i
			case mh == bh && me > be:
				entry = i
			case mh == bh && me == be && m.Callers > members[entry].Callers:
				entry = i
			}
		}
		c := Cluster{Entry: members[entry], Label: clusterLabel(members)}
		for i, m := range members {
			if i != entry {
				c.Members = append(c.Members, m)
			}
		}
		clusters = append(clusters, c)
	}
	sort.Slice(clusters, func(a, b int) bool {
		sa, sb := bestScore(clusters[a]), bestScore(clusters[b])
		if sa != sb {
			return sa > sb
		}
		return clusters[a].Entry.Sym.File < clusters[b].Entry.Sym.File
	})
	return clusters
}

func bestScore(c Cluster) float64 {
	best := c.Entry.Score
	for _, m := range c.Members {
		if m.Score > best {
			best = m.Score
		}
	}
	return best
}

// clusterLabel names a cluster by the members' shared path prefix and their
// dominant name token.
func clusterLabel(members []Scored) string {
	prefix := members[0].Sym.File
	for _, m := range members[1:] {
		prefix = sharedDirPrefix(prefix, m.Sym.File)
	}
	counts := map[string]int{}
	for _, m := range members {
		for _, t := range Tokenize(m.Sym.Name) {
			counts[t]++
		}
	}
	tok, best := "", 0
	for t, n := range counts {
		if n > best || (n == best && t < tok) {
			tok, best = t, n
		}
	}
	switch {
	case prefix != "" && tok != "":
		return prefix + " · " + tok
	case prefix != "":
		return prefix
	default:
		return tok
	}
}

func sharedDirPrefix(a, b string) string {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	n := 0
	for n < len(as)-1 && n < len(bs)-1 && as[n] == bs[n] {
		n++
	}
	return strings.Join(as[:n], "/")
}
