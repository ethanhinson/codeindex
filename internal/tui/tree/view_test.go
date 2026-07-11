package tree

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestViewSmoke(t *testing.T) {
	m := newTestModel(t, &fakeCounts{})
	out := m.View()
	for _, want := range []string{"codeindex tree", "repo", "6 symbols",
		"internal", "main.go", "q quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestViewDetailPaneShowsSymbol(t *testing.T) {
	m := newTestModel(t, &fakeCounts{})
	m = press(t, m, "right", "down", "right", "down", "right", "down") // onto Store
	out := m.View()
	for _, want := range []string{"type Store struct", "store.go:5",
		"called by", "3", "calls", "1"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail pane missing %q", want)
		}
	}
}

func TestViewNarrowHidesDetailPane(t *testing.T) {
	m := newTestModel(t, &fakeCounts{})
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = upd.(Model)
	m = press(t, m, "right", "down", "right", "down", "right", "down")
	if out := m.View(); strings.Contains(out, "called by") {
		t.Error("detail pane should be hidden below 80 columns")
	}
}

func TestViewTooSmall(t *testing.T) {
	m := newTestModel(t, nil)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	m = upd.(Model)
	if out := m.View(); !strings.Contains(out, "too small") {
		t.Errorf("expected too-small notice, got %q", out)
	}
}

func TestViewFilterPrompt(t *testing.T) {
	m := newTestModel(t, nil)
	m = press(t, m, "/", "f", "r")
	if out := m.View(); !strings.Contains(out, "filter: fr") {
		t.Error("footer should show the live filter prompt")
	}
}

func TestViewNeverPanicsWhileNavigating(t *testing.T) {
	m := newTestModel(t, &fakeCounts{})
	for _, k := range []string{"right", "down", "right", "down", "down",
		"down", "down", "left", "/", "z", "z", "esc", "up"} {
		m = press(t, m, k)
		_ = m.View()
	}
}
