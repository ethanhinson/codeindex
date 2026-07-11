package tree

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"codeindex/internal/graph"
)

// fakeCounts is a CountSource with canned results.
type fakeCounts struct{ calls int }

func (f *fakeCounts) Callers(name, parent string) ([]graph.Caller, error) {
	f.calls++
	return make([]graph.Caller, 3), nil
}
func (f *fakeCounts) Callees(name, parent string) ([]graph.Callee, error) {
	return make([]graph.Callee, 1), nil
}

func key(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newTestModel(t *testing.T, src CountSource) Model {
	t.Helper()
	m := NewModel("repo", BuildTree(fixtureSymbols()), 6, src)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return upd.(Model)
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		upd, _ := m.Update(key(k))
		m = upd.(Model)
	}
	return m
}

func TestNavigationMovesCursor(t *testing.T) {
	m := newTestModel(t, nil)
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d", m.cursor)
	}
	m = press(t, m, "down", "j")
	// Only 2 top-level rows: cursor clamps at 1.
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	m = press(t, m, "up")
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
}

func TestExpandCollapseAndParentJump(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "right") // expand internal/
	if len(m.rows) != 4 {
		t.Fatalf("rows after expand = %d, want 4", len(m.rows))
	}
	m = press(t, m, "down") // onto graph/
	m = press(t, m, "left") // collapsed already → jump to parent internal/
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (parent)", m.cursor)
	}
	m = press(t, m, "enter") // toggle internal/ closed
	if len(m.rows) != 2 {
		t.Fatalf("rows after collapse = %d, want 2", len(m.rows))
	}
}

func TestQuitKeys(t *testing.T) {
	m := newTestModel(t, nil)
	if _, cmd := m.Update(key("q")); cmd == nil {
		t.Fatal("q should quit")
	}
	if _, cmd := m.Update(key("ctrl+c")); cmd == nil {
		t.Fatal("ctrl+c should quit")
	}
}

func TestFilterModeLiveNarrowing(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "/")
	if !m.filtering {
		t.Fatal("/ should enter filter mode")
	}
	m = press(t, m, "f", "r", "e", "s", "h")
	got := rowLabels(m.rows)
	if len(got) != 4 || got[3] != "Fresh" {
		t.Fatalf("filtered rows = %v, want path to Fresh", got)
	}
	// enter keeps the filter, exits typing.
	m = press(t, m, "enter")
	if m.filtering || m.query != "fresh" || len(m.rows) != 4 {
		t.Fatalf("after enter: filtering=%v query=%q rows=%d", m.filtering, m.query, len(m.rows))
	}
	// esc in nav mode clears the applied filter.
	m = press(t, m, "esc")
	if m.query != "" || len(m.rows) != 2 {
		t.Fatalf("after esc: query=%q rows=%d", m.query, len(m.rows))
	}
}

func TestFilterEscAndBackspace(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "/", "x", "y", "backspace")
	if m.query != "x" {
		t.Fatalf("query = %q, want x", m.query)
	}
	m = press(t, m, "esc")
	if m.filtering || m.query != "" || len(m.rows) != 2 {
		t.Fatalf("esc should clear: filtering=%v query=%q rows=%d", m.filtering, m.query, len(m.rows))
	}
}

func TestFilterNoMatchShowsEmpty(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "/", "z", "z", "z", "z")
	if len(m.rows) != 0 {
		t.Fatalf("rows = %d, want 0 for no match", len(m.rows))
	}
	// Navigation on an empty row set must not panic.
	m = press(t, m, "down", "up", "enter")
}

func TestCountsFetchedLazilyAndCached(t *testing.T) {
	src := &fakeCounts{}
	m := newTestModel(t, src)
	// Drill to a symbol: expand internal, graph, store.go, then move onto Store.
	m = press(t, m, "right", "down", "right", "down", "right", "down")
	n := m.current()
	if n == nil || n.Kind != KindSymbol {
		t.Fatalf("cursor not on a symbol: %+v", n)
	}
	c, ok := m.counts[n.SymParent+"\x00"+n.SymName]
	if !ok || c.callers != 3 || c.callees != 1 {
		t.Fatalf("counts = %+v ok=%v", c, ok)
	}
	before := src.calls
	m = press(t, m, "up", "down") // revisit same symbol
	if src.calls != before {
		t.Fatalf("counts not cached: %d calls, want %d", src.calls, before)
	}
}
