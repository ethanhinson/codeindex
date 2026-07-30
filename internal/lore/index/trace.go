package index

import "codeindex/internal/lore"

// Reached is one record found by Trace: how far from the start, along which
// edge, and from which parent record.
type Reached struct {
	ID        string
	Distance  int
	ViaEdge   string // "related" | "supersedes" | "blocked_by"
	ViaParent string
}

// TraceOpts bounds a walk. Depth < 0 is unbounded (full trace). Cap <= 0
// defaults to 200 total reached records.
type TraceOpts struct {
	Depth int
	Cap   int
}

// ResolveID maps an id-or-slug to a canonical record id. Exact id wins; else an
// exact, unambiguous match against lore.Slug(title). Returns ("", false) when
// missing or ambiguous.
func ResolveID(recs []StoredRecord, value string) (string, bool) {
	for _, r := range recs {
		if r.ID == value {
			return r.ID, true
		}
	}
	var hit string
	n := 0
	for _, r := range recs {
		if lore.Slug(r.Title) == value {
			hit = r.ID
			n++
		}
	}
	if n == 1 {
		return hit, true
	}
	return "", false
}

// neighbors returns the undirected record-graph neighbors of rec, each tagged
// with the edge type it was reached by. related is treated as bidirectional;
// supersedes/superseded_by and blocked_by (and their reverses) are included.
func neighbors(recs []StoredRecord, byID map[string]StoredRecord, rec StoredRecord) []Reached {
	var out []Reached
	add := func(target, edge string) {
		if id, ok := ResolveID(recs, target); ok {
			out = append(out, Reached{ID: id, ViaEdge: edge, ViaParent: rec.ID})
		}
	}
	for _, rel := range rec.Related {
		add(rel, "related")
	}
	if rec.Supersedes != "" {
		add(rec.Supersedes, "supersedes")
	}
	if rec.SupersededBy != "" {
		add(rec.SupersededBy, "supersedes")
	}
	for _, b := range rec.BlockedBy {
		add(b, "blocked_by")
	}
	// Reverse edges: any record pointing at rec.
	for _, other := range recs {
		if other.ID == rec.ID {
			continue
		}
		for _, rel := range other.Related {
			if id, ok := ResolveID(recs, rel); ok && id == rec.ID {
				out = append(out, Reached{ID: other.ID, ViaEdge: "related", ViaParent: rec.ID})
			}
		}
		if other.Supersedes == rec.ID || other.SupersededBy == rec.ID {
			out = append(out, Reached{ID: other.ID, ViaEdge: "supersedes", ViaParent: rec.ID})
		}
		for _, b := range other.BlockedBy {
			if b == rec.ID {
				out = append(out, Reached{ID: other.ID, ViaEdge: "blocked_by", ViaParent: rec.ID})
			}
		}
	}
	return out
}

// Trace walks the record graph breadth-first from startID, cycle-safe. The
// start node is excluded from the result; shortest distance wins.
func Trace(recs []StoredRecord, startID string, opts TraceOpts) []Reached {
	cap := opts.Cap
	if cap <= 0 {
		cap = 200
	}
	byID := map[string]StoredRecord{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	if _, ok := byID[startID]; !ok {
		return nil
	}
	seen := map[string]bool{startID: true}
	var out []Reached
	frontier := []Reached{{ID: startID, Distance: 0}}
	for len(frontier) > 0 && len(out) < cap {
		var next []Reached
		for _, cur := range frontier {
			if opts.Depth >= 0 && cur.Distance >= opts.Depth {
				continue
			}
			for _, nb := range neighbors(recs, byID, byID[cur.ID]) {
				if seen[nb.ID] {
					continue
				}
				seen[nb.ID] = true
				reached := Reached{ID: nb.ID, Distance: cur.Distance + 1,
					ViaEdge: nb.ViaEdge, ViaParent: cur.ID}
				out = append(out, reached)
				next = append(next, reached)
				if len(out) >= cap {
					return out
				}
			}
		}
		frontier = next
	}
	return out
}

// Backlinks returns records that directly reference id (depth-1 inbound):
// via related, supersedes/superseded_by, or blocked_by.
func Backlinks(recs []StoredRecord, id string) []StoredRecord {
	var out []StoredRecord
	for _, r := range recs {
		hit := r.Supersedes == id || r.SupersededBy == id
		for _, rel := range r.Related {
			if rid, ok := ResolveID(recs, rel); ok && rid == id {
				hit = true
			}
		}
		for _, b := range r.BlockedBy {
			if b == id {
				hit = true
			}
		}
		if hit {
			out = append(out, r)
		}
	}
	return out
}
