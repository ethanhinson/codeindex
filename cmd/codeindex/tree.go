package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"codeindex/internal/graph"
	"codeindex/internal/progress"
	"codeindex/internal/query"
	tuitree "codeindex/internal/tui/tree"
)

// runTree freshens the index and explores it: interactive Bubble Tea UI on a
// TTY, static indented tree otherwise.
func runTree(root string) error {
	if _, err := query.Fresh(root); err != nil {
		return err
	}
	st, err := graph.Open(filepath.Join(root, ".codeindex", "graph.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	syms, err := st.ProjectSymbols()
	if err != nil {
		return err
	}
	if len(syms) == 0 {
		fmt.Println("index is empty: no symbols found")
		return nil
	}
	node := tuitree.BuildTree(syms)

	if !progress.IsTTY(os.Stdout) {
		fmt.Print(tuitree.Static(node))
		return nil
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	m := tuitree.NewModel(filepath.Base(abs), node, len(syms), st)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		// IsTTY's char-device heuristic false-positives on /dev/null and
		// similar sinks; if the TUI can't start, degrade to static output.
		fmt.Print(tuitree.Static(node))
	}
	return nil
}
