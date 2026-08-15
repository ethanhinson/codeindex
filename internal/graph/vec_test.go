package graph

import (
	"math"
	"math/rand"
	"path/filepath"
	"sort"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func unit(v []float32) []float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	n := math.Sqrt(s)
	for i := range v {
		v[i] = float32(float64(v[i]) / n)
	}
	return v
}

// TestTopKByVec checks the sqlite-vec int8 kernel ranks like a float32
// cosine reference on random unit vectors.
func TestTopKByVec(t *testing.T) {
	st := openTestStore(t)
	rng := rand.New(rand.NewSource(42))
	const n, dims = 200, 64

	vecs := make([][]float32, n)
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := range vecs {
		v := make([]float32, dims)
		for j := range v {
			v[j] = float32(rng.NormFloat64())
		}
		vecs[i] = unit(v)
		hash := string(rune('a'+i%26)) + string(rune('0'+i/26)) // unique per i
		sid, err := st.insertSymbol(tx, "f.go", "sym", "", "ns", 0, "func", "", i+1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.PutSymbolVec(tx, sid, hash); err != nil {
			t.Fatal(err)
		}
		if err := st.PutVec(tx, hash, "m1", QuantizeInt8(vecs[i])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	q := make([]float32, dims)
	for j := range q {
		q[j] = float32(rng.NormFloat64())
	}
	unit(q)

	hits, err := st.TopKByVec(q, "m1", 10)
	if err != nil {
		t.Fatalf("TopKByVec: %v", err)
	}
	if len(hits) != 10 {
		t.Fatalf("got %d hits", len(hits))
	}

	// Float32 reference ranking.
	type ref struct {
		i   int
		sim float64
	}
	refs := make([]ref, n)
	for i, v := range vecs {
		var s float64
		for j := range v {
			s += float64(v[j]) * float64(q[j])
		}
		refs[i] = ref{i, s}
	}
	sort.Slice(refs, func(a, b int) bool { return refs[a].sim > refs[b].sim })

	// Rank-order equivalence within quantization noise: the int8 score at
	// each rank must sit within tolerance of the float score at that rank.
	for k, h := range hits {
		if d := math.Abs(h.Score - refs[k].sim); d > 0.05 {
			t.Fatalf("rank %d: int8 score %.4f vs float %.4f (Δ%.4f)", k, h.Score, refs[k].sim, d)
		}
	}
}

// TestVecLifecycle covers mapping deletion with the file graph, missing-vec
// work lists, pruning, and model invalidation.
func TestVecLifecycle(t *testing.T) {
	st := openTestStore(t)

	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	sid, err := st.insertSymbol(tx, "a.go", "Foo", "", "ns", 0, "func", "func Foo()", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutSymbolVec(tx, sid, "h1"); err != nil {
		t.Fatal(err)
	}
	if err := st.PutVec(tx, "h1", "m1", []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutVecModelStamp(tx, "m1"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if n, _ := st.VecCount("m1"); n != 1 {
		t.Fatalf("VecCount = %d, want 1", n)
	}
	if stamp, _ := st.VecModelStamp(); stamp != "m1" {
		t.Fatalf("stamp = %q", stamp)
	}

	// Missing work list: h1 present, h2 missing.
	missing, err := st.MissingVecs([]string{"h1", "h2", "h2"}, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != "h2" {
		t.Fatalf("missing = %v", missing)
	}
	// Different model: everything is missing (model swap invalidation).
	missing, err = st.MissingVecs([]string{"h1"}, "m2")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 {
		t.Fatalf("m2 missing = %v", missing)
	}

	// Deleting the file's graph removes the mapping in the same tx; pruning
	// then drops the orphaned content vector.
	tx, err = st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteFile(tx, "a.go"); err != nil {
		t.Fatal(err)
	}
	var mapped int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM symvec`).Scan(&mapped); err != nil {
		t.Fatal(err)
	}
	if mapped != 0 {
		t.Fatalf("symvec rows after delete = %d", mapped)
	}
	if err := st.PruneVecs(tx); err != nil {
		t.Fatal(err)
	}
	var cached int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM vecs`).Scan(&cached); err != nil {
		t.Fatal(err)
	}
	if cached != 0 {
		t.Fatalf("vecs rows after prune = %d", cached)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if n, _ := st.VecCount("m1"); n != 0 {
		t.Fatalf("VecCount after delete = %d", n)
	}
}
