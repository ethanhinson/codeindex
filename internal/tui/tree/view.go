package tree

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width < 40 || m.height < 10 {
		return "terminal too small — enlarge to at least 40×10\n"
	}
	showDetail := m.width >= 80
	treeW := m.width
	if showDetail {
		treeW = m.width * 6 / 10
	}
	h := m.treeHeight()

	// paneStyle's border+padding add 4 columns; Width/Height size the inside.
	treePane := paneStyle.Width(treeW - 4).Height(h).Render(m.renderRows(treeW-4, h))
	body := treePane
	if showDetail {
		detailW := m.width - treeW
		detailPane := paneStyle.Width(detailW - 4).Height(h).
			Render(m.renderDetail(detailW - 4))
		body = lipgloss.JoinHorizontal(lipgloss.Top, treePane, detailPane)
	}
	return m.renderHeader() + "\n" + body + "\n" + m.renderFooter()
}

func (m Model) renderHeader() string {
	left := headerStyle.Render("codeindex tree — " + m.repoName)
	right := countStyle.Render(fmt.Sprintf("%d symbols", m.total))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderFooter() string {
	if m.filtering {
		return footerStyle.Render("filter: " + m.query + "▌  esc clear · enter keep")
	}
	hint := "↑↓ move · ←→ collapse/expand · / filter · enter toggle · q quit"
	if m.query != "" {
		hint = string(rune(0x201c)) + "filtered: " + m.query + string(rune(0x201d)) + " · esc clear · " + hint
	}
	return footerStyle.Render(hint)
}

func (m Model) renderRows(w, h int) string {
	if len(m.rows) == 0 {
		return badgeStyle.Render("no matches")
	}
	var b strings.Builder
	end := m.offset + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(m.rows[i], i == m.cursor, w))
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m Model) renderRow(r Row, selected bool, w int) string {
	aff := "  "
	if len(r.Node.Children) > 0 {
		aff = "▸ "
		if r.Node.Expanded {
			aff = "▾ "
		}
	}
	label := r.Node.Label
	if r.Node.Kind == KindDir {
		label += "/"
	}
	badge := ""
	if r.Node.Kind == KindSymbol {
		badge = r.Node.SymKind
	}
	avail := w
	if badge != "" {
		avail -= len(badge) + 2
	}
	text := truncate(strings.Repeat("  ", r.Depth)+aff+label, avail)

	if selected {
		if badge != "" {
			text += "  " + badge
		}
		return cursorStyle.Width(w).Render(text)
	}
	switch {
	case m.query != "" && Matches(r.Node, m.query):
		text = matchStyle.Render(text)
	case r.Node.Kind == KindDir:
		text = dirStyle.Render(text)
	}
	if badge != "" {
		text += "  " + badgeStyle.Render(badge)
	}
	return text
}

func (m Model) renderDetail(w int) string {
	n := m.current()
	if n == nil {
		return ""
	}
	var b strings.Builder
	switch n.Kind {
	case KindDir:
		b.WriteString(titleStyle.Render(n.Label + "/"))
	case KindFile:
		b.WriteString(titleStyle.Render(n.Label) + "\n")
		b.WriteString(badgeStyle.Render(n.File))
	case KindSymbol:
		name := n.SymName
		if n.SymParent != "" {
			name = n.SymParent + "." + n.SymName
		}
		b.WriteString(titleStyle.Render(truncate(name, w)) + "\n")
		fmt.Fprintf(&b, "%s · %s\n\n", n.SymKind,
			truncate(fmt.Sprintf("%s:%d", n.File, n.Line), w-len(n.SymKind)-3))
		if n.Signature != "" {
			b.WriteString(wrap(n.Signature, w) + "\n\n")
		}
		if c, ok := m.counts[n.SymParent+"\x00"+n.SymName]; ok {
			if c.err {
				b.WriteString("called by  —\ncalls      —")
			} else {
				fmt.Fprintf(&b, "called by  %d\ncalls      %d", c.callers, c.callees)
			}
		}
	}
	return b.String()
}

// truncate shortens s to w display cells with a trailing ellipsis.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// wrap hard-wraps s at w cells (signatures are code: no word wrapping needed).
func wrap(s string, w int) string {
	if w <= 0 {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\n' || col >= w {
			b.WriteByte('\n')
			col = 0
			if r == '\n' {
				continue
			}
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}
