package index

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
)

// StaleRecords maps record IDs to staleness: an anchor pointing at a path
// that no longer exists or a symbol no longer in the code graph. graph.db is
// optional — without it, symbol anchors are trusted.
func StaleRecords(repoRoot, graphDBPath string, recs []StoredRecord) (map[string]bool, error) {
	stale := map[string]bool{}
	var db *sql.DB
	if _, err := os.Stat(graphDBPath); err == nil {
		db, err = sql.Open("sqlite3", graphDBPath)
		if err != nil {
			return nil, err
		}
		defer db.Close()
	}
	for _, r := range recs {
		for _, a := range r.Anchors {
			if a.Path != "" {
				p := filepath.Join(repoRoot, strings.TrimSuffix(a.Path, "/"))
				if _, err := os.Stat(p); err != nil {
					stale[r.ID] = true
				}
			}
			if a.Symbol != "" && db != nil {
				var n int
				if err := db.QueryRow(
					`SELECT COUNT(1) FROM symbols WHERE name = ?`, a.Symbol).Scan(&n); err != nil {
					return nil, err
				}
				if n == 0 {
					stale[r.ID] = true
				}
			}
		}
	}
	return stale, nil
}
