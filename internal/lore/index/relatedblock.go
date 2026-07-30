package index

import (
	"fmt"
	"sort"
	"strings"
)

const relatedBlockCap = 5

// RelatedLoreBlock renders the "Related lore" block for a code query: records
// anchored to symbol (distance 0) plus their graph neighbors out to depth
// (depth < 0 = full trace). Returns "" when nothing is anchored. Ordering is
// distance ascending, then active/open first. Capped at relatedBlockCap entries.
func RelatedLoreBlock(recs []StoredRecord, symbol string, depth int) string {
	roots := RecordsForAnchor(recs, symbol)
	if len(roots) == 0 {
		return ""
	}
	byID := map[string]StoredRecord{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	dist := map[string]int{}
	for _, r := range roots {
		dist[r.ID] = 0
	}
	for _, r := range roots {
		for _, reached := range Trace(recs, r.ID, TraceOpts{Depth: depth}) {
			if d, ok := dist[reached.ID]; !ok || reached.Distance < d {
				dist[reached.ID] = reached.Distance
			}
		}
	}
	type entry struct {
		r StoredRecord
		d int
	}
	var entries []entry
	for id, d := range dist {
		if r, ok := byID[id]; ok {
			entries = append(entries, entry{r, d})
		}
	}
	rank := func(status string) int {
		if status == "active" || status == "open" {
			return 0
		}
		return 1
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].d != entries[j].d {
			return entries[i].d < entries[j].d
		}
		if rank(entries[i].r.Status) != rank(entries[j].r.Status) {
			return rank(entries[i].r.Status) < rank(entries[j].r.Status)
		}
		return entries[i].r.ID < entries[j].r.ID
	})
	total := len(entries)
	if len(entries) > relatedBlockCap {
		entries = entries[:relatedBlockCap]
	}
	var b strings.Builder
	b.WriteString("\n\nRelated lore (decisions/items/notes for this symbol and its links):\n")
	for _, e := range entries {
		status := e.r.Status
		if status == "" {
			status = "-"
		}
		flag := ""
		if e.r.Stale {
			flag = "  STALE"
		}
		hop := ""
		if e.d > 0 {
			hop = fmt.Sprintf("  (+%d)", e.d)
		}
		fmt.Fprintf(&b, "%s  [%s/%s]  %s%s%s\n", e.r.ID, e.r.Layer, status, e.r.Title, hop, flag)
	}
	if total > relatedBlockCap {
		fmt.Fprintf(&b, "(+%d more)\n", total-relatedBlockCap)
	}
	return b.String()
}
