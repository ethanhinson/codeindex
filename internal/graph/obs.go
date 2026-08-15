package graph

import (
	"database/sql"
)

// Observed runtime evidence (runtime-evidence-stack). Rows key on interned
// content keys "file|parent|name" — symbol row-ids churn on every re-parse
// (and rowids can be reused), so id-keyed evidence would dangle or
// mis-attribute. Content keys make staleness safe by construction: a
// renamed/deleted symbol's evidence simply stops matching any live symbol.
//
// Sampled-truth invariant (design D7): observed evidence is ADDITIVE —
// nothing here removes or overrides static conclusions, and absence of
// samples is never evidence of death.

// ObsKey renders the stable content key for a symbol.
func ObsKey(file, parent, name string) string {
	return file + "|" + parent + "|" + name
}

// AddObsEdge accumulates weight on an observed src→dst edge in tx.
func (s *Store) AddObsEdge(tx *sql.Tx, srcKey, dstKey string, weight int64, indirect bool) error {
	sid, err := s.intern(tx, srcKey)
	if err != nil {
		return err
	}
	did, err := s.intern(tx, dstKey)
	if err != nil {
		return err
	}
	ind := 0
	if indirect {
		ind = 1
	}
	_, err = tx.Exec(
		`INSERT INTO obs_edges(src_key_id, dst_key_id, weight, indirect) VALUES(?,?,?,?)
		 ON CONFLICT(src_key_id, dst_key_id, indirect)
		 DO UPDATE SET weight = weight + excluded.weight`,
		sid, did, weight, ind)
	return err
}

// AddObsHeat accumulates a symbol's sample counters in tx.
func (s *Store) AddObsHeat(tx *sql.Tx, key string, leaf, total, entry int64) error {
	kid, err := s.intern(tx, key)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO obs_heat(key_id, leaf_samples, total_samples, entry_samples) VALUES(?,?,?,?)
		 ON CONFLICT(key_id) DO UPDATE SET
		   leaf_samples = leaf_samples + excluded.leaf_samples,
		   total_samples = total_samples + excluded.total_samples,
		   entry_samples = entry_samples + excluded.entry_samples`,
		kid, leaf, total, entry)
	return err
}

// ObsEdge is one observed edge with endpoints as content keys.
type ObsEdge struct {
	SrcKey, DstKey string
	Weight         int64
	Indirect       bool
}

// ObsEdges loads all observed edges (search maps keys to live symbol ids).
func (s *Store) ObsEdges() ([]ObsEdge, error) {
	rows, err := s.db.Query(
		`SELECT ks.s, kd.s, e.weight, e.indirect FROM obs_edges e
		 JOIN strs ks ON ks.id = e.src_key_id
		 JOIN strs kd ON kd.id = e.dst_key_id ORDER BY e.rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ObsEdge
	for rows.Next() {
		var e ObsEdge
		var ind int
		if err := rows.Scan(&e.SrcKey, &e.DstKey, &e.Weight, &ind); err != nil {
			return nil, err
		}
		e.Indirect = ind == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// ObsHeat is a symbol's accumulated sample counters.
type ObsHeat struct {
	Leaf, Total, Entry int64
}

// ObsHeatByKey loads all heat rows keyed by content key.
func (s *Store) ObsHeatByKey() (map[string]ObsHeat, error) {
	rows, err := s.db.Query(
		`SELECT k.s, h.leaf_samples, h.total_samples, h.entry_samples
		 FROM obs_heat h JOIN strs k ON k.id = h.key_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ObsHeat{}
	for rows.Next() {
		var k string
		var h ObsHeat
		if err := rows.Scan(&k, &h.Leaf, &h.Total, &h.Entry); err != nil {
			return nil, err
		}
		out[k] = h
	}
	return out, rows.Err()
}

// ObsLedgerHas reports whether a spool file (path+content hash) was ingested.
func (s *Store) ObsLedgerHas(path, hash string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM obs_ledger WHERE path=? AND hash=?`, path, hash).Scan(&n)
	return n > 0, err
}

// PutObsLedger records an ingested spool file with its resolution stats.
func (s *Store) PutObsLedger(tx *sql.Tx, path, hash, lang string, start, end int64, commit string, framesTotal, framesResolved int, ingestedAt int64) error {
	_, err := tx.Exec(
		`INSERT INTO obs_ledger(path, hash, ingested_at, lang, start, end, commit_id, frames_total, frames_resolved)
		 VALUES(?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(path) DO UPDATE SET hash=excluded.hash, ingested_at=excluded.ingested_at,
		   lang=excluded.lang, start=excluded.start, end=excluded.end, commit_id=excluded.commit_id,
		   frames_total=excluded.frames_total, frames_resolved=excluded.frames_resolved`,
		path, hash, ingestedAt, lang, start, end, commit, framesTotal, framesResolved)
	return err
}

// ObsProvenance summarizes ingested evidence for disclosure: newest profile
// end time and the set of commits it came from.
func (s *Store) ObsProvenance() (newestEnd int64, commits []string, err error) {
	rows, err := s.db.Query(`SELECT COALESCE(MAX(end),0) FROM obs_ledger`)
	if err != nil {
		return 0, nil, err
	}
	if rows.Next() {
		if err := rows.Scan(&newestEnd); err != nil {
			rows.Close()
			return 0, nil, err
		}
	}
	rows.Close()
	crows, err := s.db.Query(
		`SELECT DISTINCT commit_id FROM obs_ledger WHERE commit_id != '' ORDER BY commit_id`)
	if err != nil {
		return 0, nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var c string
		if err := crows.Scan(&c); err != nil {
			return 0, nil, err
		}
		commits = append(commits, c)
	}
	return newestEnd, commits, crows.Err()
}

// HasObs reports whether any observed evidence exists (cheap gate for the
// search path's no-spool no-op parity).
func (s *Store) HasObs() (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM obs_ledger`).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
