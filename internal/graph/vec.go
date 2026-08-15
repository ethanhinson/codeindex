package graph

import (
	"database/sql"
	"strings"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

// sqlite-vec provides the distance kernel; vectors live in ordinary tables so
// they share transactions with the symbol rows they describe.
//
// Layout (v8): symvec maps live symbol ids to card-content hashes; vecs is a
// content-addressed store of int8 vectors per (hash, model). Symbol ids churn
// on every file re-parse (delete + insert), so keying vectors by content hash
// is what makes "re-embed only when the card text changed" hold across
// incremental patches: a re-inserted symbol with an unchanged card reuses its
// cached vector by hash.
func init() { sqlitevec.Auto() }

// VecHit is one vector-lane result: a live symbol and its cosine similarity
// to the query (1 = identical direction).
type VecHit struct {
	SymbolID int64
	Score    float64
}

// QuantizeInt8 maps a unit-normalized float vector to int8 bytes (linear
// ±127). Cosine ranking is scale-invariant, so quantizing both sides
// preserves order up to rounding.
func QuantizeInt8(v []float32) []byte {
	b := make([]byte, len(v))
	for i, x := range v {
		q := int(x * 127)
		if q > 127 {
			q = 127
		}
		if q < -127 {
			q = -127
		}
		b[i] = byte(int8(q))
	}
	return b
}

// PutSymbolVec records symbol -> card hash in tx (mapping row; the vector
// itself may already exist content-addressed in vecs).
func (s *Store) PutSymbolVec(tx *sql.Tx, symbolID int64, hash string) error {
	_, err := tx.Exec(
		`INSERT INTO symvec(symbol_id, hash) VALUES(?,?)
		 ON CONFLICT(symbol_id) DO UPDATE SET hash=excluded.hash`, symbolID, hash)
	return err
}

// PutVec stores one embedded card vector (int8) for model in tx.
func (s *Store) PutVec(tx *sql.Tx, hash, model string, vec []byte) error {
	_, err := tx.Exec(
		`INSERT INTO vecs(hash, model, vec) VALUES(?,?,?)
		 ON CONFLICT(hash, model) DO UPDATE SET vec=excluded.vec`, hash, model, vec)
	return err
}

// MissingVecs returns the subset of hashes with no stored vector for model —
// the embedding work list. Chunked to stay under statement limits.
func (s *Store) MissingVecs(hashes []string, model string) ([]string, error) {
	have := map[string]bool{}
	for i := 0; i < len(hashes); i += 500 {
		end := min(i+500, len(hashes))
		args := make([]any, 0, end-i+1)
		args = append(args, model)
		ph := make([]string, 0, end-i)
		for _, h := range hashes[i:end] {
			args = append(args, h)
			ph = append(ph, "?")
		}
		rows, err := s.db.Query(
			`SELECT hash FROM vecs WHERE model=? AND hash IN (`+strings.Join(ph, ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var h string
			if err := rows.Scan(&h); err != nil {
				rows.Close()
				return nil, err
			}
			have[h] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	var missing []string
	seen := map[string]bool{}
	for _, h := range hashes {
		if !have[h] && !seen[h] {
			missing = append(missing, h)
			seen[h] = true
		}
	}
	return missing, nil
}

// PruneVecs deletes cached vectors whose hash no longer maps from any live
// symbol (run at the end of a build; keeps the content cache bounded).
func (s *Store) PruneVecs(tx *sql.Tx) error {
	_, err := tx.Exec(`DELETE FROM vecs WHERE hash NOT IN (SELECT hash FROM symvec)`)
	return err
}

// TopKByVec is the vector lane: cosine similarity of the query against every
// mapped symbol's card vector for model, best k. Brute force through
// sqlite-vec's int8 kernel — the same budget class as find's in-memory scan.
func (s *Store) TopKByVec(query []float32, model string, k int) ([]VecHit, error) {
	rows, err := s.db.Query(
		`SELECT sv.symbol_id,
		        vec_distance_cosine(vec_int8(v.vec), vec_int8(?)) AS d
		 FROM symvec sv JOIN vecs v ON v.hash = sv.hash AND v.model = ?
		 ORDER BY d ASC, sv.symbol_id LIMIT ?`,
		QuantizeInt8(query), model, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VecHit
	for rows.Next() {
		var h VecHit
		var d float64
		if err := rows.Scan(&h.SymbolID, &d); err != nil {
			return nil, err
		}
		h.Score = 1 - d
		out = append(out, h)
	}
	return out, rows.Err()
}

// CallEdgesAmong returns the call edges whose both endpoints are in ids —
// the connectivity the feature-map clustering groups by.
func (s *Store) CallEdgesAmong(ids []int64) (pairs [][2]int64, err error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, 0, 2*len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	for _, id := range ids {
		args = append(args, id)
	}
	in := strings.Join(ph, ",")
	rows, err := s.db.Query(
		`SELECT DISTINCT src_symbol_id, dst_symbol_id FROM edges
		 WHERE kind='calls' AND src_symbol_id IN (`+in+`) AND dst_symbol_id IN (`+in+`)`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]int64{a, b})
	}
	return pairs, rows.Err()
}

// NeighborhoodEdges returns the deduped edges of the seeds' bounded 2-hop
// neighborhood over calls/extends/implements (both directions) — the
// diffusion subgraph. Deterministic: edges scanned in id order; per-node
// degree caps bound hub blowup; expansion stops at maxNodes.
func (s *Store) NeighborhoodEdges(seeds []int64, maxNodes, maxDegree int) ([][2]int64, error) {
	nodes := map[int64]bool{}
	for _, id := range seeds {
		nodes[id] = true
	}
	degree := map[int64]int{}
	seen := map[[2]int64]bool{}
	var pairs [][2]int64

	frontier := append([]int64(nil), seeds...)
	for hop := 0; hop < 2 && len(frontier) > 0 && len(nodes) < maxNodes; hop++ {
		var next []int64
		for start := 0; start < len(frontier); start += 400 {
			end := min(start+400, len(frontier))
			ph := make([]string, 0, end-start)
			args := make([]any, 0, 2*(end-start))
			for _, id := range frontier[start:end] {
				ph = append(ph, "?")
				args = append(args, id)
			}
			in := strings.Join(ph, ",")
			args = append(args, args[:end-start]...)
			rows, err := s.db.Query(
				`SELECT src_symbol_id, dst_symbol_id FROM edges
				 WHERE kind IN ('calls','extends','implements')
				   AND src_symbol_id != 0 AND dst_symbol_id != 0
				   AND (src_symbol_id IN (`+in+`) OR dst_symbol_id IN (`+in+`))
				 ORDER BY id`, args...)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var a, b int64
				if err := rows.Scan(&a, &b); err != nil {
					rows.Close()
					return nil, err
				}
				if a == b {
					continue
				}
				key := [2]int64{min64(a, b), max64(a, b)}
				if seen[key] {
					continue
				}
				if degree[a] >= maxDegree || degree[b] >= maxDegree {
					continue
				}
				if !nodes[a] && !nodes[b] {
					continue // edge between two unknowns can't happen from this query, but guard
				}
				grow := 0
				if !nodes[a] {
					grow++
				}
				if !nodes[b] {
					grow++
				}
				if len(nodes)+grow > maxNodes {
					continue
				}
				seen[key] = true
				pairs = append(pairs, key)
				degree[a]++
				degree[b]++
				if !nodes[a] {
					nodes[a] = true
					next = append(next, a)
				}
				if !nodes[b] {
					nodes[b] = true
					next = append(next, b)
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			rows.Close()
		}
		frontier = next
	}
	return pairs, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// VecCount reports how many live symbols have a stored vector for model —
// zero means the semantic lane is absent (degrade to lexical and disclose).
func (s *Store) VecCount(model string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM symvec sv JOIN vecs v ON v.hash=sv.hash AND v.model=?`,
		model).Scan(&n)
	return n, err
}

// SymvecHashes returns the card hashes currently mapped for the given symbol
// ids (parity checks and tests).
func (s *Store) SymvecHashes(ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	for i := 0; i < len(ids); i += 500 {
		end := min(i+500, len(ids))
		ph := make([]string, 0, end-i)
		args := make([]any, 0, end-i)
		for _, id := range ids[i:end] {
			ph = append(ph, "?")
			args = append(args, id)
		}
		rows, err := s.db.Query(
			`SELECT symbol_id, hash FROM symvec WHERE symbol_id IN (`+strings.Join(ph, ",")+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var h string
			if err := rows.Scan(&id, &h); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = h
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// CallNeighborNames returns, per symbol id, the distinct names of its top
// callers and callees by call count (bounded to topN, deterministic order) —
// the graph context that goes into embedding cards. Two grouped scans, not
// per-symbol queries.
func (s *Store) CallNeighborNames(topN int) (callers, callees map[int64][]string, err error) {
	load := func(q string) (map[int64][]string, error) {
		rows, err := s.db.Query(q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := map[int64][]string{}
		for rows.Next() {
			var id int64
			var name string
			var n int
			if err := rows.Scan(&id, &name, &n); err != nil {
				return nil, err
			}
			if len(out[id]) < topN {
				out[id] = append(out[id], name)
			}
		}
		return out, rows.Err()
	}
	callers, err = load(
		`SELECT e.dst_symbol_id, sc.name, COUNT(*) AS n
		 FROM edges e JOIN symbols sc ON sc.id = e.src_symbol_id
		 WHERE e.kind='calls' AND e.dst_symbol_id != 0 AND e.src_symbol_id != 0
		 GROUP BY e.dst_symbol_id, sc.name ORDER BY e.dst_symbol_id, n DESC, sc.name`)
	if err != nil {
		return nil, nil, err
	}
	callees, err = load(
		`SELECT e.src_symbol_id, d.name, COUNT(*) AS n
		 FROM edges e JOIN symbols d ON d.id = e.dst_symbol_id
		 WHERE e.kind='calls' AND e.dst_symbol_id != 0 AND e.src_symbol_id != 0
		 GROUP BY e.src_symbol_id, d.name ORDER BY e.src_symbol_id, n DESC, d.name`)
	if err != nil {
		return nil, nil, err
	}
	return callers, callees, nil
}

// VecModelStamp reads/writes the model the index was last embedded with, so a
// provider change is detected even before any lookup misses.
func (s *Store) VecModelStamp() (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM vecmeta WHERE key='model'`).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// PutVecModelStamp records model as the active embedding model in tx.
func (s *Store) PutVecModelStamp(tx *sql.Tx, model string) error {
	_, err := tx.Exec(
		`INSERT INTO vecmeta(key,value) VALUES('model',?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, model)
	return err
}

// DropVecsForOtherModels removes cached vectors not belonging to model (model
// swap invalidation, run during the build embedding pass).
func (s *Store) DropVecsForOtherModels(tx *sql.Tx, model string) error {
	_, err := tx.Exec(`DELETE FROM vecs WHERE model != ?`, model)
	return err
}
