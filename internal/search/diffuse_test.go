package search

import (
	"math"
	"testing"
)

// A seed's direct neighbor must receive meaningful mass; a node two hops
// out gets less; an unconnected node gets none.
func TestDiffuseNeighborPullIn(t *testing.T) {
	// 1 -- 2 -- 3    4 (isolated)
	pairs := []wedge{{1, 2, 1}, {2, 3, 1}}
	p := diffuse(pairs, map[int64]float64{1: 1.0}, 0.85, 12)

	if p[2] <= p[3] {
		t.Fatalf("1-hop mass %f should exceed 2-hop mass %f", p[2], p[3])
	}
	if p[3] <= 0 {
		t.Fatal("2-hop neighbor received no mass")
	}
	if p[4] != 0 {
		t.Fatalf("isolated node received mass %f", p[4])
	}
	// NOTE: with high alpha on a path, the connective middle node can carry
	// more mass than the seed itself — that is correct PPR behavior; seed
	// identity is preserved by the fused-score half of the blend, not here.
	if p[1] <= p[3] {
		t.Fatalf("seed mass %f should exceed 2-hop mass %f", p[1], p[3])
	}
}

// Mass is conserved (sums to ~1) so ranking scales are stable.
func TestDiffuseConservation(t *testing.T) {
	pairs := []wedge{{1, 2, 1}, {2, 3, 1}, {3, 4, 1}, {4, 1, 1}}
	p := diffuse(pairs, map[int64]float64{1: 2.0, 3: 1.0}, 0.85, 12)
	var sum float64
	for _, v := range p {
		sum += v
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Fatalf("mass sum = %f, want 1", sum)
	}
	// Restart proportionality: the heavier seed keeps more mass.
	if p[1] <= p[3] {
		t.Fatalf("seed 1 (weight 2) mass %f should exceed seed 3 (weight 1) %f", p[1], p[3])
	}
}

// A high-degree hub connected to one seed must not out-mass a dedicated
// low-degree neighbor: row normalization splits the hub's outflow.
func TestDiffuseHubContainment(t *testing.T) {
	pairs := []wedge{{1, 2, 1}} // dedicated neighbor
	// hub 100 connects to the seed AND to 50 unrelated nodes.
	pairs = append(pairs, wedge{1, 100, 1})
	for i := int64(200); i < 250; i++ {
		pairs = append(pairs, wedge{100, i, 1})
	}
	p := diffuse(pairs, map[int64]float64{1: 1.0}, 0.85, 12)
	// The hub itself receives the same inflow as the dedicated neighbor
	// (both are 1 hop from the seed), but its onward flood is diluted:
	// each unrelated node must get a tiny share.
	if p[200] >= p[2]/10 {
		t.Fatalf("hub satellite mass %f not contained vs dedicated neighbor %f", p[200], p[2])
	}
}

func TestDiffuseDeterministic(t *testing.T) {
	pairs := []wedge{{1, 2, 1}, {2, 3, 1}, {3, 4, 1}, {2, 5, 1}, {5, 6, 1}}
	seeds := map[int64]float64{1: 0.7, 5: 0.3}
	a := diffuse(pairs, seeds, 0.85, 12)
	b := diffuse(pairs, seeds, 0.85, 12)
	for id, v := range a {
		if b[id] != v {
			t.Fatalf("node %d: %f != %f", id, v, b[id])
		}
	}
}

func TestDiffuseEmpty(t *testing.T) {
	if p := diffuse(nil, map[int64]float64{1: 1}, 0.85, 12); p[1] == 0 {
		t.Fatal("seed with no edges should keep restart mass")
	}
	if p := diffuse([]wedge{{1, 2, 1}}, nil, 0.85, 12); p != nil {
		t.Fatal("no seeds should yield nil")
	}
}
