package tree

import (
	tea "github.com/charmbracelet/bubbletea"

	"codeindex/internal/graph"
)

// CountSource supplies caller/callee counts for the detail pane.
// *graph.Store satisfies it.
type CountSource interface {
	Callers(name, parent string) ([]graph.Caller, error)
	Callees(name, parent string) ([]graph.Callee, error)
}

type symCounts struct {
	callers, callees int
	err              bool
}

// Model is the Bubble Tea model for the tree explorer.
type Model struct {
	repoName string
	root     *Node
	total    int
	source   CountSource

	rows   []Row
	cursor int
	offset int
	width  int
	height int

	filtering bool
	query     string
	filtered  *Node // non-nil while a filter is applied

	counts map[string]symCounts
}

func NewModel(repoName string, root *Node, total int, source CountSource) Model {
	m := Model{repoName: repoName, root: root, total: total, source: source,
		counts: map[string]symCounts{}}
	m.refresh()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scrollIntoView()
		return m, nil
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateNav(msg)
	}
	return m, nil
}

func (m Model) updateNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		m.scrollIntoView()
		m.fetchCounts()
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
		m.scrollIntoView()
		m.fetchCounts()
	case "right", "l":
		if n := m.current(); n != nil && len(n.Children) > 0 && !n.Expanded {
			n.Expanded = true
			m.refresh()
		}
	case "left", "h":
		if n := m.current(); n != nil {
			if n.Expanded {
				n.Expanded = false
				m.refresh()
			} else if p := ParentIndex(m.rows, m.cursor); p >= 0 {
				m.cursor = p
				m.scrollIntoView()
			}
		}
	case "enter":
		if n := m.current(); n != nil && len(n.Children) > 0 {
			n.Expanded = !n.Expanded
			m.refresh()
		}
	case "/":
		m.filtering = true
	case "esc":
		if m.query != "" {
			m.clearFilter()
		}
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filtering = false
		m.clearFilter()
	case "enter":
		m.filtering = false
	case "backspace":
		if len(m.query) > 0 {
			r := []rune(m.query)
			m.query = string(r[:len(r)-1])
			m.applyFilter()
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.query += string(msg.Runes)
			m.applyFilter()
		}
	}
	return m, nil
}

// current returns the node under the cursor, nil when there are no rows.
func (m *Model) current() *Node {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].Node
}

// refresh recomputes visible rows from the active tree and re-clamps
// cursor, scroll, and counts.
func (m *Model) refresh() {
	src := m.root
	if m.filtered != nil {
		src = m.filtered
	}
	m.rows = Visible(src)
	if m.cursor > len(m.rows)-1 {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.scrollIntoView()
	m.fetchCounts()
}

func (m *Model) applyFilter() {
	if m.query == "" {
		m.filtered = nil
	} else if f := FilterTree(m.root, m.query); f != nil {
		m.filtered = f
	} else {
		m.filtered = &Node{Kind: KindDir, Expanded: true} // no matches: empty tree
	}
	m.cursor, m.offset = 0, 0
	m.refresh()
}

func (m *Model) clearFilter() {
	m.query = ""
	m.filtered = nil
	m.cursor, m.offset = 0, 0
	m.refresh()
}

// treeHeight is the drawable row count: total height minus header, footer,
// and the pane's top/bottom borders.
func (m *Model) treeHeight() int {
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) scrollIntoView() {
	h := m.treeHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// fetchCounts lazily loads caller/callee counts for the selected symbol.
// Failures are cached as err so the detail pane shows "—" without retry loops.
func (m *Model) fetchCounts() {
	n := m.current()
	if n == nil || n.Kind != KindSymbol || m.source == nil {
		return
	}
	key := n.SymParent + "\x00" + n.SymName
	if _, ok := m.counts[key]; ok {
		return
	}
	callers, err1 := m.source.Callers(n.SymName, n.SymParent)
	callees, err2 := m.source.Callees(n.SymName, n.SymParent)
	m.counts[key] = symCounts{callers: len(callers), callees: len(callees),
		err: err1 != nil || err2 != nil}
}

