package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// Readers used by the workspace resolution ladder (openspec §3.3). They are
// pure reads over the `edges` and `symbols` views; nothing here writes.

// UnresolvedEdge is an edge whose destination could not be resolved inside its
// own repo, carried with enough of its source symbol to name that end with a
// stable key.
type UnresolvedEdge struct {
	SrcFile      string
	SrcName      string
	SrcParent    string
	DstName      string
	DstQualifier string
	DstNS        string
	Kind         string
	Line         int
}

// UnresolvedEdges returns every edge with dst_symbol_id = 0 whose source is a
// real symbol (src_symbol_id != 0), ordered by
// (src_file, src_name, src_parent, dst_name, kind, line, dst_qualifier,
// dst_ns, id).
//
// The order is total by construction: edges_t has no unique constraint, so two
// calls to the same name on the same line differing only in qualifier or
// namespace hint tie on every other key, and the trailing dst_qualifier,
// dst_ns and id break that tie rather than leaving it to SQLite.
//
// File-level import edges carry src_symbol_id = 0 and are excluded: they have
// no source symbol, so no stable key can name their source end. Their
// information is not lost — those imports are exactly the namespace hints
// (dst_ns) the ladder's import-mediated rung consumes.
//
// The `edges` view exposes src_file but neither src_name nor src_parent, so
// this is a join to `symbols` on src_symbol_id, not a single-table select.
func (s *Store) UnresolvedEdges() ([]UnresolvedEdge, error) {
	rows, err := s.db.Query(
		`SELECT e.src_file, y.name, y.parent, e.dst_name, e.dst_qualifier,
		        e.dst_ns, e.kind, e.line
		 FROM edges e JOIN symbols y ON y.id = e.src_symbol_id
		 WHERE e.dst_symbol_id = 0 AND e.src_symbol_id != 0
		 ORDER BY e.src_file, y.name, y.parent, e.dst_name, e.kind, e.line,
		          e.dst_qualifier, e.dst_ns, e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnresolvedEdge
	for rows.Next() {
		var e UnresolvedEdge
		if err := rows.Scan(&e.SrcFile, &e.SrcName, &e.SrcParent, &e.DstName,
			&e.DstQualifier, &e.DstNS, &e.Kind, &e.Line); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// TierOneEdge is an UnresolvedEdge-shaped record for an edge that IS resolved,
// but resolved to a tier-1 (attached dependency) symbol, carried with that
// target's namespace.
//
// It embeds UnresolvedEdge rather than repeating its eight fields: the source
// key and dst_* hint columns are the same columns with the same meaning, so
// embedding keeps the two readers visibly symmetric and lets a caller pass the
// .UnresolvedEdge through to any key-building code task 2 already feeds.
type TierOneEdge struct {
	UnresolvedEdge
	// DstNamespace is the namespace of the resolved tier-1 TARGET symbol (not
	// the source's). Task 7 matches the suppression set against this.
	DstNamespace string
}

// TierOneEdges returns every edge whose resolved destination is a tier-1
// symbol, ordered by (src_file, src_name, src_parent, dst_name, kind, line,
// dst_qualifier, dst_ns, id) — total for the same reason UnresolvedEdges'
// order is.
//
// Same source-symbol requirement as UnresolvedEdges: src_symbol_id != 0, so
// file-level import edges are excluded for the same reason (no source symbol,
// hence no stable key for the source end). Edges resolved to a tier-0 symbol
// are excluded by the tier filter on the destination join, and unresolved
// edges (dst_symbol_id = 0) never match that join at all.
func (s *Store) TierOneEdges() ([]TierOneEdge, error) {
	rows, err := s.db.Query(
		`SELECT e.src_file, y.name, y.parent, e.dst_name, e.dst_qualifier,
		        e.dst_ns, e.kind, e.line, d.namespace
		 FROM edges e
		 JOIN symbols y ON y.id = e.src_symbol_id
		 JOIN symbols d ON d.id = e.dst_symbol_id
		 WHERE e.src_symbol_id != 0 AND d.tier = 1
		 ORDER BY e.src_file, y.name, y.parent, e.dst_name, e.kind, e.line,
		          e.dst_qualifier, e.dst_ns, e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TierOneEdge
	for rows.Next() {
		var e TierOneEdge
		if err := rows.Scan(&e.SrcFile, &e.SrcName, &e.SrcParent, &e.DstName,
			&e.DstQualifier, &e.DstNS, &e.Kind, &e.Line, &e.DstNamespace); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ProjectDefs returns the tier-0 definitions of name, optionally restricted to
// parent (empty means no parent restriction), ordered by
// (file, start_line, name).
//
// The tier filter is load-bearing: admitting tier-1 rows would let a vendored
// dependency snapshot become a cross-repo resolution target, the exact failure
// member-over-dep precedence exists to prevent. Definitions cannot be reused —
// it neither filters tier nor returns Tier/Namespace, and Symbol.QName() on a
// full row is what supplies a stable key's QName.
func (s *Store) ProjectDefs(name, parent string) ([]Symbol, error) {
	q := `SELECT id, file, name, parent, namespace, tier, kind, signature,
	             start_line, end_line
	      FROM symbols WHERE name = ? AND tier = 0`
	args := []any{name}
	if parent != "" {
		q += ` AND parent = ?`
		args = append(args, parent)
	}
	q += ` ORDER BY file, start_line, name`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Symbol
	for rows.Next() {
		var sy Symbol
		var kind string
		if err := rows.Scan(&sy.ID, &sy.File, &sy.Name, &sy.Parent, &sy.Namespace,
			&sy.Tier, &kind, &sy.Signature, &sy.StartLine, &sy.EndLine); err != nil {
			return nil, err
		}
		sy.Kind = SymbolKind(kind)
		out = append(out, sy)
	}
	return out, rows.Err()
}

// MemberMerkleRoot folds a member index's content state into one repo-level token:
// sha256 over "path\x00hash\n" for every merkle row in path order, then a
// "\x01depfiles\n" separator, then "path\x00namespace\x00version\x00curhash\n" for every
// depfiles row in path order. The value is opaque to the overlay, which compares it for
// equality only.
func (s *Store) MemberMerkleRoot() (string, error) {
	h := sha256.New()

	rows, err := s.db.Query(`SELECT path, hash FROM merkle ORDER BY path`)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			rows.Close()
			return "", err
		}
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write([]byte(hash))
		h.Write([]byte{'\n'})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	h.Write([]byte("\x01depfiles\n"))

	deps, err := s.DepFiles()
	if err != nil {
		return "", err
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Path < deps[j].Path })
	for _, d := range deps {
		h.Write([]byte(d.Path))
		h.Write([]byte{0})
		h.Write([]byte(d.Namespace))
		h.Write([]byte{0})
		h.Write([]byte(d.Version))
		h.Write([]byte{0})
		h.Write([]byte(d.CurHash))
		h.Write([]byte{'\n'})
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
