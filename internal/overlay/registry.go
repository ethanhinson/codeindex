package overlay

import (
	"database/sql"
	"fmt"
	"strings"

	"codeindex/internal/config"
)

// ReplaceRegistry mirrors ws into the overlay as-built, in manifest order, and
// prunes every row belonging to a member the manifest no longer names. It runs
// in one transaction.
//
// The registry is a mirror, so the three registry tables are deleted and
// re-inserted wholesale rather than diffed. Every *declared* member is stored,
// including members absent from disk: presence is a runtime condition reported
// by config.Resolve, and a persisted presence column would be stale the moment
// someone checked a member out.
//
// Duplicate namespaces and deps are de-duplicated in Go, preserving
// first-occurrence order, and are not an error: config.validate never inspects
// those slices, so a duplicate is legal loader input that would otherwise abort
// this transaction on the composite primary keys. De-duplication is done in Go
// rather than with INSERT OR IGNORE so the ord sequence stays contiguous.
//
// Pruning is set-based against the new id set. For a member the manifest drops,
// a dropped member is gone as consumer and as owner alike, so its
// dep_suppressions rows go on *either* column — deliberately unlike
// ReplaceMemberEdges, whose delete scope is consumer-only.
func (s *Store) ReplaceRegistry(ws *config.Workspace) error {
	if ws == nil {
		return fmt.Errorf("overlay: ReplaceRegistry: nil workspace")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, t := range []string{"members", "member_namespaces", "member_deps"} {
		if _, err := tx.Exec(`DELETE FROM ` + t); err != nil {
			return err
		}
	}
	if err := insertMembers(tx, ws.Members); err != nil {
		return err
	}
	if err := pruneOrphans(tx, ws.Members); err != nil {
		return err
	}
	return tx.Commit()
}

func insertMembers(tx *sql.Tx, members []config.Member) error {
	insMember, err := tx.Prepare(`INSERT INTO members (id, root, ord) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insMember.Close()
	insNS, err := tx.Prepare(`INSERT INTO member_namespaces (member_id, namespace, ord) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insNS.Close()
	insDep, err := tx.Prepare(`INSERT INTO member_deps (member_id, dep_id, ord) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insDep.Close()

	for i, m := range members {
		if _, err := insMember.Exec(m.ID, m.Root, i); err != nil {
			return fmt.Errorf("overlay: member %q: %w", m.ID, err)
		}
		for j, ns := range dedupe(m.Namespaces) {
			if _, err := insNS.Exec(m.ID, ns, j); err != nil {
				return fmt.Errorf("overlay: member %q namespace %q: %w", m.ID, ns, err)
			}
		}
		for j, dep := range dedupe(m.Deps) {
			if _, err := insDep.Exec(m.ID, dep, j); err != nil {
				return fmt.Errorf("overlay: member %q dep %q: %w", m.ID, dep, err)
			}
		}
	}
	return nil
}

// dedupe returns in's distinct values in first-occurrence order.
func dedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// pruneStmt is one prune delete in two forms: the unconditional form used when
// the new manifest is empty (every row is an orphan), and the conditional form
// whose %s verbs each take the id-list placeholders.
type pruneStmt struct {
	all  string
	cond string
	// lists is how many times the id list appears in cond.
	lists int
}

// pruneStmts runs candidates-before-ambiguities so the ambiguity ids still
// resolve when the orphaned candidate rows are selected.
var pruneStmts = []pruneStmt{
	{
		all: `DELETE FROM cross_ambiguity_candidates`,
		cond: `DELETE FROM cross_ambiguity_candidates
		       WHERE member_id NOT IN (%s)
		          OR ambiguity_id IN (SELECT id FROM cross_ambiguities WHERE src_member NOT IN (%s))`,
		lists: 2,
	},
	{
		all:   `DELETE FROM cross_ambiguities`,
		cond:  `DELETE FROM cross_ambiguities WHERE src_member NOT IN (%s)`,
		lists: 1,
	},
	{
		all:   `DELETE FROM cross_edges`,
		cond:  `DELETE FROM cross_edges WHERE src_member NOT IN (%s) OR dst_member NOT IN (%s)`,
		lists: 2,
	},
	{
		all:   `DELETE FROM member_stamps`,
		cond:  `DELETE FROM member_stamps WHERE member_id NOT IN (%s)`,
		lists: 1,
	},
	{
		all:   `DELETE FROM dep_suppressions`,
		cond:  `DELETE FROM dep_suppressions WHERE consumer_member NOT IN (%s) OR owner_member NOT IN (%s)`,
		lists: 2,
	},
}

// pruneOrphans deletes every dependent row naming a member the new manifest no
// longer declares. An empty manifest means "delete everything" and takes the
// unconditional form, since a NOT IN () list is not valid SQL.
func pruneOrphans(tx *sql.Tx, members []config.Member) error {
	for _, st := range pruneStmts {
		if len(members) == 0 {
			if _, err := tx.Exec(st.all); err != nil {
				return err
			}
			continue
		}
		list := strings.TrimSuffix(strings.Repeat("?,", len(members)), ",")
		verbs := make([]any, st.lists)
		args := make([]any, 0, len(members)*st.lists)
		for i := range verbs {
			verbs[i] = list
			for _, m := range members {
				args = append(args, m.ID)
			}
		}
		if _, err := tx.Exec(fmt.Sprintf(st.cond, verbs...), args...); err != nil {
			return err
		}
	}
	return nil
}

// Registry returns the registry in manifest order, as config.Member values —
// not a structurally identical shadow type, which would only force callers to
// convert. Namespaces and deps come back in their own recorded order.
func (s *Store) Registry() ([]config.Member, error) {
	rows, err := s.db.Query(`SELECT id, root FROM members ORDER BY ord`)
	if err != nil {
		return nil, err
	}
	members := []config.Member{}
	index := map[string]int{}
	for rows.Next() {
		var m config.Member
		if err := rows.Scan(&m.ID, &m.Root); err != nil {
			rows.Close()
			return nil, err
		}
		index[m.ID] = len(members)
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(members) == 0 {
		return members, nil
	}

	if err := scanSide(s.db, `SELECT member_id, namespace FROM member_namespaces ORDER BY member_id, ord`,
		func(i int, v string) { members[i].Namespaces = append(members[i].Namespaces, v) }, index); err != nil {
		return nil, err
	}
	if err := scanSide(s.db, `SELECT member_id, dep_id FROM member_deps ORDER BY member_id, ord`,
		func(i int, v string) { members[i].Deps = append(members[i].Deps, v) }, index); err != nil {
		return nil, err
	}
	return members, nil
}

// scanSide reads a (member_id, value) side table and appends each value to its
// member via add. Rows naming an unknown member cannot occur — the side tables
// are rewritten with members in one transaction — and are skipped defensively.
func scanSide(db *sql.DB, query string, add func(i int, v string), index map[string]int) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, val string
		if err := rows.Scan(&id, &val); err != nil {
			return err
		}
		if i, ok := index[id]; ok {
			add(i, val)
		}
	}
	return rows.Err()
}
