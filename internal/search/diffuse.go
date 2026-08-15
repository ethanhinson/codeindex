package search

import (
	"os"
	"sort"
	"strconv"
)

// Diffusion parameters, FROZEN 2026-07-11 after the registered tuning
// iterations (bench/engine/FINDINGS-diffusion-contrast.md): lambda 0.3 with
// neighbor-free contrast cards won the tuning grid (all four repos over the
// 65% bar). The lambda env override remains for experiments; changing the
// default requires a new registered gate.
var (
	diffusionLambda = envFloat("CODEINDEX_DIFFUSION_LAMBDA", 0.3)
	diffusionAlpha  = 0.85
	diffusionIters  = 12
	diffusionSeedK  = 50
	subgraphNodes   = 2000
	subgraphDegree  = 64
)

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// wedge is a weighted undirected edge for diffusion. Static edges carry
// weight 1; observed runtime edges carry compressed sample weight (the
// boost-domination lesson: evidence strength shades flow, never floods it).
type wedge struct {
	a, b int64
	w    float64
}

// diffuse runs bounded personalized PageRank over a weighted undirected
// edge list: restart mass proportional to seed scores, weight-normalized
// propagation, fixed iterations. All accumulation walks nodes in sorted
// order — float addition is non-associative, so map-order iteration would
// wobble results at the ULP level and break the determinism the spec
// requires.
func diffuse(pairs []wedge, seedScores map[int64]float64, alpha float64, iters int) map[int64]float64 {
	if len(seedScores) == 0 {
		return nil
	}
	type nb struct {
		id int64
		w  float64
	}
	adj := map[int64][]nb{}
	wsum := map[int64]float64{}
	nodeSet := map[int64]bool{}
	for _, p := range pairs {
		adj[p.a] = append(adj[p.a], nb{p.b, p.w})
		adj[p.b] = append(adj[p.b], nb{p.a, p.w})
		wsum[p.a] += p.w
		wsum[p.b] += p.w
		nodeSet[p.a] = true
		nodeSet[p.b] = true
	}
	for id := range seedScores {
		nodeSet[id] = true
	}
	nodes := make([]int64, 0, len(nodeSet))
	for id := range nodeSet {
		nodes = append(nodes, id)
	}
	sort.Slice(nodes, func(a, b int) bool { return nodes[a] < nodes[b] })

	// Restart vector: seed scores normalized to sum 1 (seed order sorted).
	seedIDs := make([]int64, 0, len(seedScores))
	for id := range seedScores {
		seedIDs = append(seedIDs, id)
	}
	sort.Slice(seedIDs, func(a, b int) bool { return seedIDs[a] < seedIDs[b] })
	var total float64
	for _, id := range seedIDs {
		total += seedScores[id]
	}
	if total == 0 {
		return nil
	}
	restart := make(map[int64]float64, len(seedIDs))
	for _, id := range seedIDs {
		restart[id] = seedScores[id] / total
	}

	p := make(map[int64]float64, len(restart))
	for _, id := range seedIDs {
		p[id] = restart[id]
	}
	for it := 0; it < iters; it++ {
		next := make(map[int64]float64, len(p))
		for _, id := range seedIDs {
			next[id] += (1 - alpha) * restart[id]
		}
		var dangling float64
		for _, u := range nodes {
			mass := p[u]
			if mass == 0 {
				continue
			}
			ns := adj[u]
			if len(ns) == 0 || wsum[u] == 0 {
				dangling += mass
				continue
			}
			flow := alpha * mass / wsum[u]
			for _, v := range ns {
				next[v.id] += flow * v.w
			}
		}
		if dangling > 0 {
			// Dangling mass restarts (keeps the total conserved).
			for _, id := range seedIDs {
				next[id] += alpha * dangling * restart[id]
			}
		}
		p = next
	}
	return p
}
