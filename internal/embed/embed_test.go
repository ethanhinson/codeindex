//go:build !nollama

package embed

import (
	"context"
	"math"
	"testing"
)

// TestBundledModel loads the go:embed'ed weights end-to-end: extraction,
// llama.cpp load, inference, normalization, and semantic sanity.
func TestBundledModel(t *testing.T) {
	p, err := NewLocal("")
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	defer p.Close()

	if p.Dims() != 384 {
		t.Fatalf("dims = %d, want 384 (MiniLM-L6)", p.Dims())
	}
	if id := p.ID(); len(id) == 0 || id[:len(BundledModelName)] != BundledModelName {
		t.Fatalf("ID = %q, want prefix %q", id, BundledModelName)
	}

	vecs, err := p.Embed(context.Background(), []string{
		"user onboarding lifecycle flow",
		"function StartOnboarding begins the host signup flow",
		"binary tree rotation balancing",
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, v := range vecs {
		if len(v) != 384 {
			t.Fatalf("vec %d: len %d", i, len(v))
		}
		var s float64
		for _, x := range v {
			s += float64(x) * float64(x)
		}
		if math.Abs(s-1) > 1e-3 {
			t.Fatalf("vec %d not unit length: %f", i, s)
		}
	}
	related, unrelated := dot(vecs[0], vecs[1]), dot(vecs[0], vecs[2])
	if related <= unrelated {
		t.Fatalf("semantic sanity: onboarding~onboarding %.3f <= onboarding~btree %.3f", related, unrelated)
	}

	// Reproducibility: same text re-embedded must agree within batch-packing
	// wobble (~2e-4 cosine — the ggml graph differs when a text is packed
	// with different neighbors; int8 storage quantization is ~8e-3, so this
	// noise never survives into the index).
	again, err := p.Embed(context.Background(), []string{"user onboarding lifecycle flow"})
	if err != nil {
		t.Fatalf("Embed again: %v", err)
	}
	if d := dot(vecs[0], again[0]); d < 0.999 {
		t.Fatalf("reproducibility: self-similarity %.6f", d)
	}
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}
