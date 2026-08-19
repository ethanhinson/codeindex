package overlay

import (
	"database/sql"
	"fmt"
	"strings"
)

// SymKey is the stable identity of a symbol across member rebuilds: member id
// + member-relative file path + qualified name. Never a rowid — a member's
// graph.db is rebuilt independently and its row ids do not survive.
type SymKey struct{ Member, File, QName string }

// CrossEdge is one cross-member edge, keyed by stable key on both ends.
type CrossEdge struct {
	Src, Dst SymKey
	// Kind is a graph.EdgeKind value, stored verbatim and never validated: the
	// overlay is the resolution ladder's sink, not its judge, and validating
	// here would couple this package to internal/graph for a string.
	Kind string
	// Provenance is "cross_repo_import" | "cross_repo_name".
	Provenance string
	// Confidence is "exact" | "inferred" — the frozen workspace spec's
	// vocabulary, deliberately not graph.Confidence (decision 8). Reconciling
	// the two words is §4.1's problem.
	Confidence string
	Line       int
}

// Ambiguity is a reference the ladder could not resolve to one symbol, with
// its candidates. It is deliberately not stored as N edges: presenting one
// candidate as exact is exactly what the frozen spec forbids.
type Ambiguity struct {
	Src     SymKey
	RefName string
	// RefNS is D3's namespace hint: empty for a rung-2 bare name.
	RefNS      string
	Kind       string
	Line       int
	Candidates []SymKey // in recorded order; deps-named candidate first
	Count      int      // as recorded; len(Candidates) unless truncated upstream
}

// Suppression records a member-over-dep precedence decision, for skew
// reporting.
type Suppression struct {
	ConsumerMember    string // whose tier-1 depmap attachment was suppressed
	Namespace         string
	OwnerMember       string // the member that claims the namespace and won
	SuppressedVersion string // the vendored snapshot's version; "" if unknown
}

// insertCrossEdge upserts with the stronger-class-wins action of decision 17.
// Two D3 rungs can reach the same (src, dst, kind, line) tuple — rung 1 as
// cross_repo_import/exact, rung 2 as cross_repo_name/inferred — and the UNIQUE
// key excludes provenance and confidence so they collide by design. The guarded
// DO UPDATE lets an exact upgrade an inferred, never lets an inferred demote an
// exact, and makes a re-put of identical input a no-op. INSERT OR IGNORE would
// make the stored answer depend on ladder emission order; INSERT OR REPLACE
// would let a rung-2 pass silently demote a rung-1 answer.
const insertCrossEdge = `INSERT INTO cross_edges
  (src_member, src_file, src_qname, dst_member, dst_file, dst_qname,
   kind, provenance, confidence, line)
  VALUES (?,?,?,?,?,?,?,?,?,?)
  ON CONFLICT(src_member, src_file, src_qname, dst_member, dst_file, dst_qname, kind, line)
  DO UPDATE SET provenance = excluded.provenance, confidence = excluded.confidence
  WHERE excluded.confidence = 'exact' AND cross_edges.confidence <> 'exact'`

// crossEdgeCols is the read projection, in CrossEdge field order.
const crossEdgeCols = `src_member, src_file, src_qname, dst_member, dst_file, dst_qname,
  kind, provenance, confidence, line`

// ambiguityKey is the natural key of cross_ambiguities: its surrogate id is not
// stable across a re-put, so every write and delete addresses rows by this.
const ambiguityKey = `src_member = ? AND src_file = ? AND src_qname = ?
  AND ref_name = ? AND kind = ? AND line = ?`

const insertSuppression = `INSERT INTO dep_suppressions
  (consumer_member, namespace, owner_member, suppressed_version) VALUES (?,?,?,?)
  ON CONFLICT(consumer_member, namespace)
  DO UPDATE SET owner_member = excluded.owner_member,
                suppressed_version = excluded.suppressed_version`

// PutCrossEdges writes edges in one transaction, idempotently. Where an edge
// already exists under the same stable-key tuple, the stronger confidence class
// wins regardless of write order (decision 17).
func (s *Store) PutCrossEdges(edges []CrossEdge) error {
	return s.inTx(func(tx *sql.Tx) error { return putCrossEdges(tx, edges) })
}

// PutAmbiguities writes ambiguities and their ranked candidates in one
// transaction, idempotently. Every record whose Count is below its own
// candidate list is rejected before any write, so a bad batch never half
// applies.
func (s *Store) PutAmbiguities(a []Ambiguity) error {
	if err := checkAmbiguities(a); err != nil {
		return err
	}
	return s.inTx(func(tx *sql.Tx) error { return putAmbiguities(tx, a) })
}

// PutSuppressions upserts suppressions on their (consumer_member, namespace)
// primary key, in one transaction.
func (s *Store) PutSuppressions(sup []Suppression) error {
	return s.inTx(func(tx *sql.Tx) error { return putSuppressions(tx, sup) })
}

// ReplaceMemberEdges re-resolves memberID's contribution to the overlay in one
// transaction: it deletes M's prior records, then writes the given ones.
//
// What it deletes:
//
//   - cross_edges incident to M on either end (src_member = M or dst_member =
//     M) — an edge is M's contribution whichever end M sits on.
//   - cross_ambiguities incident to M on either end — sourced from M, or naming
//     M anywhere in their candidate list — together with every candidate row of
//     those ambiguities. Re-resolving M may rename or delete any symbol M owns,
//     so a candidate pointing into M is stale the moment M is replaced, and no
//     other path would ever clear it: ReplaceMemberEdges(N) fires only when N
//     re-resolves, and registry pruning only when M leaves the manifest
//     entirely. A candidate list is never *thinned*: when M appears only as a
//     candidate of an ambiguity sourced from another member N, the whole parent
//     ambiguity is deleted, because dropping M's row alone would leave N's
//     stored candidate_count contradicting its own surviving rows. N re-derives
//     the reference the next time N is re-resolved. Parents and candidates go
//     together inside the one transaction, so decision 18's orphan hazard is
//     closed here as it is in PutAmbiguities.
//   - dep_suppressions where consumer_member = M. Rows where M is merely the
//     owner_member are deliberately left alone (decision 21): a suppression
//     records a consumer's attachment being overridden and is re-derived when
//     that consumer re-resolves, so deleting owner-side rows would drop records
//     for members this call is not re-resolving and cannot rewrite.
//
// A sup entry whose ConsumerMember is not memberID is rejected: it would land
// outside this call's own delete scope. Validation of both that and each
// ambiguity's Count runs before the transaction opens.
func (s *Store) ReplaceMemberEdges(memberID string, edges []CrossEdge, a []Ambiguity, sup []Suppression) error {
	if err := checkAmbiguities(a); err != nil {
		return err
	}
	for _, x := range sup {
		if x.ConsumerMember != memberID {
			return fmt.Errorf(
				"overlay: ReplaceMemberEdges(%q): suppression for namespace %q names consumer_member %q, outside this call's delete scope",
				memberID, x.Namespace, x.ConsumerMember)
		}
	}
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`DELETE FROM cross_edges WHERE src_member = ? OR dst_member = ?`,
			memberID, memberID); err != nil {
			return err
		}
		if err := deleteIncidentAmbiguities(tx, memberID); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`DELETE FROM dep_suppressions WHERE consumer_member = ?`, memberID); err != nil {
			return err
		}
		if err := putCrossEdges(tx, edges); err != nil {
			return err
		}
		if err := putAmbiguities(tx, a); err != nil {
			return err
		}
		return putSuppressions(tx, sup)
	})
}

// ambiguityDeleteChunk bounds how many ids one DELETE binds, staying well under
// any SQLite host-parameter limit for a member with many ambiguities.
const ambiguityDeleteChunk = 400

// deleteIncidentAmbiguities removes every ambiguity incident to memberID on
// either end — sourced from it, or naming it among its candidates — and all of
// their candidate rows.
func deleteIncidentAmbiguities(tx *sql.Tx, memberID string) error {
	return deleteAmbiguitiesMatching(tx,
		`SELECT id FROM cross_ambiguities WHERE src_member = ?
		 UNION
		 SELECT ambiguity_id FROM cross_ambiguity_candidates WHERE member_id = ?`,
		memberID, memberID)
}

// deleteAmbiguitiesMatching deletes every cross_ambiguities row whose id the
// given single-column query returns, together with all of their candidate rows.
// It is shared by the two callers that delete an ambiguity whole rather than
// thinning its candidate list: ReplaceMemberEdges' either-end incidence delete,
// and ReplaceRegistry's either-end orphan prune.
//
// The ids are resolved into Go first rather than driven from a subquery,
// because an either-end query's candidate-side arm reads
// cross_ambiguity_candidates while the same statement would delete from it;
// collecting first makes the two deletes independent of any evaluation-order
// subtlety. Candidates are deleted before their parents so the second delete
// can never strand a row (decision 18's hazard applies here exactly as it does
// in PutAmbiguities), and the whole sequence runs inside the caller's
// transaction, in chunks bounded by ambiguityDeleteChunk.
//
// The query may name an ambiguity id that no longer has a parent row; deleting
// those candidate rows is right either way.
func deleteAmbiguitiesMatching(tx *sql.Tx, query string, args ...any) error {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return err
	}
	var ids []any
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for len(ids) > 0 {
		n := min(len(ids), ambiguityDeleteChunk)
		chunk := ids[:n]
		ids = ids[n:]
		in := "(" + strings.Repeat("?,", n-1) + "?)"
		if _, err := tx.Exec(
			`DELETE FROM cross_ambiguity_candidates WHERE ambiguity_id IN `+in, chunk...); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`DELETE FROM cross_ambiguities WHERE id IN `+in, chunk...); err != nil {
			return err
		}
	}
	return nil
}

// inTx runs fn inside a single transaction, rolling back on any error.
func (s *Store) inTx(fn func(*sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// checkAmbiguities rejects a record whose recorded Count contradicts its own
// candidate list. Count may legally exceed len(Candidates) — upstream may have
// truncated the list — but never fall below it.
func checkAmbiguities(a []Ambiguity) error {
	for _, x := range a {
		if x.Count < len(x.Candidates) {
			return fmt.Errorf(
				"overlay: ambiguity %s %s %s ref %q line %d: Count %d is below len(Candidates) %d",
				x.Src.Member, x.Src.File, x.Src.QName, x.RefName, x.Line, x.Count, len(x.Candidates))
		}
	}
	return nil
}

func putCrossEdges(tx *sql.Tx, edges []CrossEdge) error {
	if len(edges) == 0 {
		return nil
	}
	st, err := tx.Prepare(insertCrossEdge)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, e := range edges {
		if _, err := st.Exec(e.Src.Member, e.Src.File, e.Src.QName,
			e.Dst.Member, e.Dst.File, e.Dst.QName,
			e.Kind, e.Provenance, e.Confidence, e.Line); err != nil {
			return fmt.Errorf("overlay: cross edge %s %s -> %s %s: %w",
				e.Src.Member, e.Src.QName, e.Dst.Member, e.Dst.QName, err)
		}
	}
	return nil
}

// putAmbiguities deletes each record's natural-key row and its candidates
// before inserting (decision 18). INSERT OR REPLACE would delete the
// conflicting row and insert a new one under a *new* rowid, stranding the old
// candidate rows forever: there is no PRAGMA foreign_keys (decision 13) to
// collect them, and every read would still look correct while the candidates
// table grew monotonically.
func putAmbiguities(tx *sql.Tx, a []Ambiguity) error {
	if len(a) == 0 {
		return nil
	}
	delCands, err := tx.Prepare(`DELETE FROM cross_ambiguity_candidates
	  WHERE ambiguity_id IN (SELECT id FROM cross_ambiguities WHERE ` + ambiguityKey + `)`)
	if err != nil {
		return err
	}
	defer delCands.Close()
	delAmbig, err := tx.Prepare(`DELETE FROM cross_ambiguities WHERE ` + ambiguityKey)
	if err != nil {
		return err
	}
	defer delAmbig.Close()
	insAmbig, err := tx.Prepare(`INSERT INTO cross_ambiguities
	  (src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count)
	  VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insAmbig.Close()
	insCand, err := tx.Prepare(`INSERT INTO cross_ambiguity_candidates
	  (ambiguity_id, rank, member_id, file, qname) VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insCand.Close()

	for _, x := range a {
		key := []any{x.Src.Member, x.Src.File, x.Src.QName, x.RefName, x.Kind, x.Line}
		// Candidates first, while the ambiguity id is still resolvable.
		if _, err := delCands.Exec(key...); err != nil {
			return err
		}
		if _, err := delAmbig.Exec(key...); err != nil {
			return err
		}
		res, err := insAmbig.Exec(x.Src.Member, x.Src.File, x.Src.QName,
			x.RefName, x.RefNS, x.Kind, x.Line, x.Count)
		if err != nil {
			return fmt.Errorf("overlay: ambiguity %s ref %q: %w", x.Src.Member, x.RefName, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for rank, c := range x.Candidates {
			if _, err := insCand.Exec(id, rank, c.Member, c.File, c.QName); err != nil {
				return fmt.Errorf("overlay: ambiguity %s ref %q candidate %d: %w",
					x.Src.Member, x.RefName, rank, err)
			}
		}
	}
	return nil
}

func putSuppressions(tx *sql.Tx, sup []Suppression) error {
	if len(sup) == 0 {
		return nil
	}
	st, err := tx.Prepare(insertSuppression)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, x := range sup {
		if _, err := st.Exec(x.ConsumerMember, x.Namespace, x.OwnerMember, x.SuppressedVersion); err != nil {
			return fmt.Errorf("overlay: suppression %s %s: %w", x.ConsumerMember, x.Namespace, err)
		}
	}
	return nil
}

// OutEdges returns every cross-edge leaving src, ordered by destination stable
// key, then kind, then line.
func (s *Store) OutEdges(src SymKey) ([]CrossEdge, error) {
	return s.queryEdges(`SELECT `+crossEdgeCols+` FROM cross_edges
	  WHERE src_member = ? AND src_file = ? AND src_qname = ?
	  ORDER BY dst_member, dst_file, dst_qname, kind, line`,
		src.Member, src.File, src.QName)
}

// InEdges returns every cross-edge arriving at dst, ordered by source stable
// key, then kind, then line.
func (s *Store) InEdges(dst SymKey) ([]CrossEdge, error) {
	return s.queryEdges(`SELECT `+crossEdgeCols+` FROM cross_edges
	  WHERE dst_member = ? AND dst_file = ? AND dst_qname = ?
	  ORDER BY src_member, src_file, src_qname, kind, line`,
		dst.Member, dst.File, dst.QName)
}

// MemberEdges returns every cross-edge incident to memberID on either end,
// ordered by source stable key, then destination stable key, then kind and
// line.
func (s *Store) MemberEdges(memberID string) ([]CrossEdge, error) {
	return s.queryEdges(`SELECT `+crossEdgeCols+` FROM cross_edges
	  WHERE src_member = ? OR dst_member = ?
	  ORDER BY src_member, src_file, src_qname, dst_member, dst_file, dst_qname, kind, line`,
		memberID, memberID)
}

func (s *Store) queryEdges(query string, args ...any) ([]CrossEdge, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CrossEdge{}
	for rows.Next() {
		var e CrossEdge
		if err := rows.Scan(&e.Src.Member, &e.Src.File, &e.Src.QName,
			&e.Dst.Member, &e.Dst.File, &e.Dst.QName,
			&e.Kind, &e.Provenance, &e.Confidence, &e.Line); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AmbiguitiesFor returns the ambiguities *sourced from* memberID — the member
// whose reference could not be resolved — ordered by source stable key, then
// ref name, kind and line, each with its candidates in recorded rank order.
// Ambiguities that merely list memberID among their candidates are read back
// under their own source member, not here. That is a scoping choice about this
// read only: ReplaceMemberEdges deletes ambiguities incident to a member on
// either end, candidate side included.
func (s *Store) AmbiguitiesFor(memberID string) ([]Ambiguity, error) {
	rows, err := s.db.Query(`SELECT id, src_member, src_file, src_qname, ref_name, ref_ns, kind, line, candidate_count
	  FROM cross_ambiguities WHERE src_member = ?
	  ORDER BY src_member, src_file, src_qname, ref_name, kind, line`, memberID)
	if err != nil {
		return nil, err
	}
	out := []Ambiguity{}
	index := map[int64]int{}
	for rows.Next() {
		var id int64
		var x Ambiguity
		if err := rows.Scan(&id, &x.Src.Member, &x.Src.File, &x.Src.QName,
			&x.RefName, &x.RefNS, &x.Kind, &x.Line, &x.Count); err != nil {
			rows.Close()
			return nil, err
		}
		index[id] = len(out)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(out) == 0 {
		return out, nil
	}

	cands, err := s.db.Query(`SELECT c.ambiguity_id, c.member_id, c.file, c.qname
	  FROM cross_ambiguity_candidates c
	  JOIN cross_ambiguities a ON a.id = c.ambiguity_id
	  WHERE a.src_member = ?
	  ORDER BY c.ambiguity_id, c.rank`, memberID)
	if err != nil {
		return nil, err
	}
	defer cands.Close()
	for cands.Next() {
		var id int64
		var k SymKey
		if err := cands.Scan(&id, &k.Member, &k.File, &k.QName); err != nil {
			return nil, err
		}
		if i, ok := index[id]; ok {
			out[i].Candidates = append(out[i].Candidates, k)
		}
	}
	return out, cands.Err()
}

// Suppressions returns every recorded suppression, ordered by consumer member
// then namespace — the primary key, so the order is total.
func (s *Store) Suppressions() ([]Suppression, error) {
	rows, err := s.db.Query(`SELECT consumer_member, namespace, owner_member, suppressed_version
	  FROM dep_suppressions ORDER BY consumer_member, namespace`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Suppression{}
	for rows.Next() {
		var x Suppression
		if err := rows.Scan(&x.ConsumerMember, &x.Namespace, &x.OwnerMember, &x.SuppressedVersion); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
