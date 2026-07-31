package readmodel

// SeedFocus is a suggested starting point for exploration — a lore record the
// UI can focus on landing so the canvas is never empty.
type SeedFocus struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// Seed returns up to limit starting focuses drawn from lore, preferring
// records that anchor to code or block other items (they produce a non-trivial
// neighborhood) over standalone notes. Records come back date-descending from
// the store, so the newest anchored record leads.
func Seed(root string, limit int) ([]SeedFocus, error) {
	recs, err := openLore(root)
	if err != nil {
		return nil, err
	}
	var anchored, rest []SeedFocus
	for _, r := range recs {
		sf := SeedFocus{ID: r.ID, Label: r.Title, Kind: string(r.Type)}
		if len(r.Anchors) > 0 || len(r.BlockedBy) > 0 {
			anchored = append(anchored, sf)
		} else {
			rest = append(rest, sf)
		}
	}
	out := append(anchored, rest...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
