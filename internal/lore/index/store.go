// Package index maintains the derived lore search index in
// .codeindex/lore.db. Records on disk are the source of truth; this database
// can always be deleted and rebuilt.
package index

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"codeindex/internal/lore"
)

const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS lore_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS lore_files (path TEXT PRIMARY KEY, hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS lore_records (
  id TEXT PRIMARY KEY, type TEXT NOT NULL, title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT '', date TEXT NOT NULL DEFAULT '',
  layer TEXT NOT NULL, file TEXT NOT NULL,
  priority TEXT NOT NULL DEFAULT '',
  supersedes TEXT NOT NULL DEFAULT '', superseded_by TEXT NOT NULL DEFAULT '',
  stale INTEGER NOT NULL DEFAULT 0, confidence REAL NOT NULL DEFAULT 0,
  body TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS idx_lore_records_file ON lore_records(file);
CREATE TABLE IF NOT EXISTS lore_anchors (
  record_id TEXT NOT NULL, path TEXT NOT NULL DEFAULT '', symbol TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS idx_lore_anchors_rec ON lore_anchors(record_id);
CREATE TABLE IF NOT EXISTS lore_refs (
  record_id TEXT NOT NULL, kind TEXT NOT NULL, value TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_lore_refs_rec ON lore_refs(record_id);
CREATE TABLE IF NOT EXISTS lore_blocked (record_id TEXT NOT NULL, blocked_by TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS lore_tags (record_id TEXT NOT NULL, tag TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_lore_blocked_rec ON lore_blocked(record_id);
CREATE INDEX IF NOT EXISTS idx_lore_tags_rec ON lore_tags(record_id);
`

type Store struct{ db *sql.DB }

// StoredRecord is a record plus its index-side metadata.
type StoredRecord struct {
	lore.Record
	Layer      string
	File       string
	Stale      bool
	Confidence float64
}

func Open(path string) (*Store, error) {
	// cross-process contention is real (long-lived MCP server + CLI + Stop-hook captures share lore.db);
	// busy_timeout waits instead of failing SQLITE_BUSY, WAL lets readers proceed during writes.
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	var ver string
	err = db.QueryRow(`SELECT value FROM lore_meta WHERE key='schema'`).Scan(&ver)
	if err == sql.ErrNoRows {
		_, err = db.Exec(`INSERT INTO lore_meta(key,value) VALUES('schema',?)`,
			fmt.Sprint(schemaVersion))
	} else if err == nil && ver != fmt.Sprint(schemaVersion) {
		// Derived data: on mismatch, wipe and let the next reindex rebuild.
		for _, t := range []string{"lore_files", "lore_records", "lore_anchors",
			"lore_refs", "lore_blocked", "lore_tags"} {
			if _, err = db.Exec("DELETE FROM " + t); err != nil {
				break
			}
		}
		if err == nil {
			_, err = db.Exec(`UPDATE lore_meta SET value=? WHERE key='schema'`,
				fmt.Sprint(schemaVersion))
		}
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Upsert(r lore.Record, layer, file string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// stale and confidence are deliberately absent from the upsert: they are
	// index-side derived state, owned by SetStale (and, later, the lifecycle
	// signals pass) — a record file changing must not reset them.
	if _, err := tx.Exec(`INSERT INTO lore_records
		(id,type,title,status,date,layer,file,priority,supersedes,superseded_by,body)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET type=excluded.type,title=excluded.title,
		status=excluded.status,date=excluded.date,layer=excluded.layer,
		file=excluded.file,priority=excluded.priority,supersedes=excluded.supersedes,
		superseded_by=excluded.superseded_by,body=excluded.body`,
		r.ID, string(r.Type), r.Title, r.Status, r.Date, layer, file,
		r.Priority, r.Supersedes, r.SupersededBy, r.Body); err != nil {
		return err
	}
	for _, t := range []string{"lore_anchors", "lore_refs", "lore_blocked", "lore_tags"} {
		if _, err := tx.Exec("DELETE FROM "+t+" WHERE record_id=?", r.ID); err != nil {
			return err
		}
	}
	for _, a := range r.Anchors {
		if _, err := tx.Exec(`INSERT INTO lore_anchors(record_id,path,symbol) VALUES(?,?,?)`,
			r.ID, a.Path, a.Symbol); err != nil {
			return err
		}
	}
	for _, ref := range r.Refs {
		if _, err := tx.Exec(`INSERT INTO lore_refs(record_id,kind,value) VALUES(?,?,?)`,
			r.ID, ref.Kind, ref.Value); err != nil {
			return err
		}
	}
	for _, b := range r.BlockedBy {
		if _, err := tx.Exec(`INSERT INTO lore_blocked(record_id,blocked_by) VALUES(?,?)`,
			r.ID, b); err != nil {
			return err
		}
	}
	for _, tag := range r.Tags {
		if _, err := tx.Exec(`INSERT INTO lore_tags(record_id,tag) VALUES(?,?)`,
			r.ID, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteByFile(file string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM lore_records WHERE file=?`, file)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		for _, t := range []string{"lore_records", "lore_anchors", "lore_refs",
			"lore_blocked", "lore_tags"} {
			col := "record_id"
			if t == "lore_records" {
				col = "id"
			}
			if _, err := tx.Exec("DELETE FROM "+t+" WHERE "+col+"=?", id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) All() ([]StoredRecord, error) {
	rows, err := s.db.Query(`SELECT id,type,title,status,date,layer,file,priority,
		supersedes,superseded_by,stale,confidence,body
		FROM lore_records ORDER BY date DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredRecord
	for rows.Next() {
		var r StoredRecord
		var typ string
		var stale int
		if err := rows.Scan(&r.ID, &typ, &r.Title, &r.Status, &r.Date, &r.Layer,
			&r.File, &r.Priority, &r.Supersedes, &r.SupersededBy, &stale,
			&r.Confidence, &r.Body); err != nil {
			return nil, err
		}
		r.Type, r.Stale = lore.Type(typ), stale != 0
		if err := s.loadChildren(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) loadChildren(r *StoredRecord) error {
	rows, err := s.db.Query(`SELECT path,symbol FROM lore_anchors WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a lore.Anchor
		if err := rows.Scan(&a.Path, &a.Symbol); err != nil {
			rows.Close()
			return err
		}
		r.Anchors = append(r.Anchors, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = s.db.Query(`SELECT kind,value FROM lore_refs WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ref lore.Ref
		if err := rows.Scan(&ref.Kind, &ref.Value); err != nil {
			rows.Close()
			return err
		}
		r.Refs = append(r.Refs, ref)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = s.db.Query(`SELECT blocked_by FROM lore_blocked WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		r.BlockedBy = append(r.BlockedBy, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = s.db.Query(`SELECT tag FROM lore_tags WHERE record_id=?`, r.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var tg string
		if err := rows.Scan(&tg); err != nil {
			rows.Close()
			return err
		}
		r.Tags = append(r.Tags, tg)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Store) Get(id string) (StoredRecord, bool, error) {
	var r StoredRecord
	var typ string
	var stale int
	err := s.db.QueryRow(`SELECT id,type,title,status,date,layer,file,priority,
		supersedes,superseded_by,stale,confidence,body
		FROM lore_records WHERE id=?`, id).Scan(&r.ID, &typ, &r.Title, &r.Status,
		&r.Date, &r.Layer, &r.File, &r.Priority, &r.Supersedes, &r.SupersededBy,
		&stale, &r.Confidence, &r.Body)
	if err == sql.ErrNoRows {
		return StoredRecord{}, false, nil
	}
	if err != nil {
		return StoredRecord{}, false, err
	}
	r.Type, r.Stale = lore.Type(typ), stale != 0
	if err := s.loadChildren(&r); err != nil {
		return StoredRecord{}, false, err
	}
	return r, true, nil
}

func (s *Store) FileHashes() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT path,hash FROM lore_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var p, h string
		if err := rows.Scan(&p, &h); err != nil {
			return nil, err
		}
		m[p] = h
	}
	return m, rows.Err()
}

func (s *Store) SetFileHash(path, hash string) error {
	_, err := s.db.Exec(`INSERT INTO lore_files(path,hash) VALUES(?,?)
		ON CONFLICT(path) DO UPDATE SET hash=excluded.hash`, path, hash)
	return err
}

func (s *Store) DeleteFileHash(path string) error {
	_, err := s.db.Exec(`DELETE FROM lore_files WHERE path=?`, path)
	return err
}

func (s *Store) SetStale(id string, stale bool) error {
	v := 0
	if stale {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE lore_records SET stale=? WHERE id=?`, v, id)
	return err
}
