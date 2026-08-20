package overlay

import (
	"database/sql"
	"time"
)

// Stamp is a member's freshness stamp: the member's merkle root as of its
// last resolution into the overlay. MerkleRoot is opaque here — this package
// never computes, parses, or validates it (decision 9); how staleness is
// decided from it belongs to a later slice.
type Stamp struct {
	MemberID   string
	MerkleRoot string
	ResolvedAt int64
}

const upsertStamp = `INSERT INTO member_stamps (member_id, merkle_root, resolved_at)
  VALUES (?,?,?)
  ON CONFLICT(member_id) DO UPDATE SET
    merkle_root = excluded.merkle_root, resolved_at = excluded.resolved_at`

// PutStamp upserts memberID's stamp with merkleRoot and the current time.
func (s *Store) PutStamp(memberID, merkleRoot string) error {
	_, err := s.db.Exec(upsertStamp, memberID, merkleRoot, time.Now().Unix())
	return err
}

// DeleteStamp removes memberID's stamp. Deleting an absent stamp is not an
// error: the call is idempotent, which is what lets wsresolve.Resolve prune
// unconditionally without a "does it have one?" pre-check.
func (s *Store) DeleteStamp(memberID string) error {
	_, err := s.db.Exec(`DELETE FROM member_stamps WHERE member_id = ?`, memberID)
	return err
}

// Stamp returns memberID's stamp, or (Stamp{}, false, nil) when absent.
func (s *Store) Stamp(memberID string) (Stamp, bool, error) {
	var st Stamp
	err := s.db.QueryRow(`SELECT member_id, merkle_root, resolved_at
	  FROM member_stamps WHERE member_id = ?`, memberID).
		Scan(&st.MemberID, &st.MerkleRoot, &st.ResolvedAt)
	if err == sql.ErrNoRows {
		return Stamp{}, false, nil
	}
	if err != nil {
		return Stamp{}, false, err
	}
	return st, true, nil
}

// Stamps returns every recorded stamp, ordered by member id.
func (s *Store) Stamps() ([]Stamp, error) {
	rows, err := s.db.Query(`SELECT member_id, merkle_root, resolved_at
	  FROM member_stamps ORDER BY member_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stamp{}
	for rows.Next() {
		var st Stamp
		if err := rows.Scan(&st.MemberID, &st.MerkleRoot, &st.ResolvedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
