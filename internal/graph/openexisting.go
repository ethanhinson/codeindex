package graph

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
)

// VersionMismatchError reports an index whose embedded schema version is not the
// one this binary requires. It names both versions so an operator can act on it:
// the fix is to rebuild that index, which is deliberately NOT something
// OpenExisting will do on their behalf.
type VersionMismatchError struct {
	Path        string
	FileVersion int
	WantVersion int
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("graph: index %s has schema version %d, this binary requires version %d (rebuild the index)",
		e.Path, e.FileVersion, e.WantVersion)
}

// OpenExisting opens an existing index read-only in the sense that matters: it
// never creates the file and never deletes-and-rebuilds on a version mismatch.
// Absence returns an error wrapping fs.ErrNotExist; a mismatch returns a
// *VersionMismatchError naming both versions; an unreadable or corrupt file
// returns the underlying open/pragma error. Open remains the build path — a
// caller that wants an index built or repaired must ask for that explicitly.
//
// The returned *Store is a fully usable read surface (its handle is opened with
// SQLite's mode=ro, so a stray write path fails loudly rather than mutating a
// member's index).
func OpenExisting(path string) (*Store, error) {
	// Stat first: every later step must be reached only for a file that already
	// exists, and this is what makes absence an fs.ErrNotExist for the caller.
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("graph: cannot open index %s: %w", path, err)
	}
	version, err := FileSchemaVersion(path)
	if err != nil {
		return nil, fmt.Errorf("graph: reading schema version of %s: %w", path, err)
	}
	if version != SchemaVersion() {
		return nil, &VersionMismatchError{
			Path:        path,
			FileVersion: version,
			WantVersion: SchemaVersion(),
		}
	}
	db, err := sql.Open("sqlite3", readOnlyDSN(path))
	if err != nil {
		return nil, fmt.Errorf("graph: opening index %s: %w", path, err)
	}
	// sql.Open is lazy; force a connection so an unreadable file is an error
	// here rather than at the first query.
	var check int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&check); err != nil {
		db.Close()
		return nil, fmt.Errorf("graph: opening index %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// readOnlyDSN renders path as a SQLite URI opened read-only. The URI form is
// what carries mode=ro, and url.URL escapes characters ("?", "#", spaces) that
// would otherwise be read as URI syntax.
func readOnlyDSN(path string) string {
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	return u.String()
}
