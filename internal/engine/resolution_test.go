package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/graph"
)

// Two classes define scale(); $this-> style qualification must collapse the
// collision that plain name-based resolution flags ambiguous.
const collideGo = `package p

type Widget struct{ n int }

func (w Widget) Grow() int { return w.scale(2) }
func (w Widget) scale(n int) int { return w.n * n }

type Gauge struct{ n int }

func (g Gauge) scale(n int) int { return g.n + n }
`

func TestQualifierCollapsesCollision(t *testing.T) {
	dir := writeTree(t, map[string]string{"w.go": collideGo})
	db := filepath.Join(dir, "g.db")
	if _, err := Build(dir, db); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Grow's call w.scale(2) carries qualifier Widget -> unambiguous, and it
	// must resolve to Widget.scale, not Gauge.scale.
	cs, err := st.Callees("Grow", "")
	if err != nil {
		t.Fatal(err)
	}
	var scale *graph.Callee
	for i := range cs {
		if cs[i].Name == "scale" {
			scale = &cs[i]
		}
	}
	if scale == nil {
		t.Fatalf("Grow should call scale; got %+v", cs)
	}
	if scale.Conf != graph.ConfUnambiguous {
		t.Errorf("qualified call should be unambiguous; got %s", scale.Conf)
	}
	if scale.DefParent != "Widget" {
		t.Errorf("scale should resolve to Widget.scale; got parent %q", scale.DefParent)
	}

	// Qualified anchor filters: callers of Widget.scale = Grow; Gauge.scale = none.
	widgetCallers, err := st.Callers("scale", "Widget")
	if err != nil {
		t.Fatal(err)
	}
	if len(widgetCallers) != 1 || widgetCallers[0].Name != "Grow" {
		t.Errorf("Widget.scale callers should be [Grow]; got %+v", widgetCallers)
	}
	gaugeCallers, err := st.Callers("scale", "Gauge")
	if err != nil {
		t.Fatal(err)
	}
	if len(gaugeCallers) != 0 {
		t.Errorf("Gauge.scale should have no callers; got %+v", gaugeCallers)
	}
}

func TestWrongHintFallsBack(t *testing.T) {
	// A call qualified to a type that doesn't define the method must fall back
	// to plain name-based resolution (single def -> unambiguous).
	dir := writeTree(t, map[string]string{"a.go": `package p

type Widget struct{}

func (w Widget) Grow() int { return w.helper() } // Widget has no helper
func helper() int          { return 1 }
`})
	db := filepath.Join(dir, "g.db")
	if _, err := Build(dir, db); err != nil {
		t.Fatal(err)
	}
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cs, err := st.Callees("Grow", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Name == "helper" {
			if c.Conf != graph.ConfUnambiguous || c.DefFile != "a.go" {
				t.Errorf("wrong hint should fall back to plain resolution; got %+v", c)
			}
			return
		}
	}
	t.Fatalf("helper callee not found: %+v", cs)
}

func TestIncrementalEqualsFull_WithQualifiers(t *testing.T) {
	dir := writeTree(t, map[string]string{"w.go": collideGo})
	if _, err := Build(dir, filepath.Join(dir, "inc.db")); err != nil {
		t.Fatal(err)
	}
	// Rename Gauge.scale -> Gauge.rescale: the Widget-qualified edge must stay
	// stable through re-resolution; snapshots (incl. parents+qualifiers) equal.
	edited := strings.Replace(collideGo, "func (g Gauge) scale(", "func (g Gauge) rescale(", 1)
	os.WriteFile(filepath.Join(dir, "w.go"), []byte(edited), 0o644)
	assertIncrementalEqualsFull(t, dir)
}

func TestSchemaVersionRebuild(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": fileA})
	db := filepath.Join(dir, "g.db")
	if _, err := Build(dir, db); err != nil {
		t.Fatal(err)
	}
	// Simulate an old index: downgrade user_version, then reopen.
	st, err := graph.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	// graph.Open sets the current version; use raw sqlite to force a mismatch.
	// (Reuse the store to run the pragma via a fresh open + Exec through Build's
	// path is overkill — write the pragma with a throwaway connection.)
	raw, err := graph.OpenRaw(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	st2, err := graph.Open(db) // must rebuild empty, not error
	if err != nil {
		t.Fatalf("open after version mismatch: %v", err)
	}
	defer st2.Close()
	meta, err := st2.StoredMeta()
	if err != nil {
		t.Fatal(err)
	}
	if len(meta) != 0 {
		t.Errorf("rebuilt index should be empty; got %d merkle rows", len(meta))
	}
}
