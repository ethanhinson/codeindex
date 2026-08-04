package graph

import (
	"testing"
)

// putFile inserts a parsed file into a fresh store inside one transaction.
func putFile(t *testing.T, st *Store, pf *ParsedFile) {
	t.Helper()
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	meta := FileMeta{Path: pf.Path, Hash: "h-" + pf.Path, Size: 1, Mtime: 1}
	if _, _, err := st.PutFile(tx, pf, meta); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
