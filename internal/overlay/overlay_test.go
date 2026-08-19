package overlay

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPath(t *testing.T) {
	got := Path("/tmp/ws")
	want := filepath.Join("/tmp/ws", ".codeindex/workspace.db")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

// tableCount reports how many tables the file at p holds, read outside the
// version-enforcing path.
func tableCount(t *testing.T, p string) int {
	t.Helper()
	db, err := OpenRaw(p)
	if err != nil {
		t.Fatalf("OpenRaw: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&n); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	return n
}

func TestOpenCreatesAndReopens(t *testing.T) {
	p := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file created: %v", err)
	}
	first := tableCount(t, p)
	if first == 0 {
		t.Fatal("expected schema tables after Open")
	}

	// Reopening an up-to-date file is a no-op: same tables, no error.
	s2, err := Open(p)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := tableCount(t, p); got != first {
		t.Fatalf("table count changed on reopen: %d -> %d", first, got)
	}
}

func TestFileSchemaVersionMatches(t *testing.T) {
	p := filepath.Join(t.TempDir(), "workspace.db")
	s, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()
	v, err := FileSchemaVersion(p)
	if err != nil {
		t.Fatalf("FileSchemaVersion: %v", err)
	}
	if v != SchemaVersion() {
		t.Fatalf("FileSchemaVersion = %d, want %d", v, SchemaVersion())
	}
}

func TestOpenRebuildsOnVersionMismatch(t *testing.T) {
	p := filepath.Join(t.TempDir(), "workspace.db")
	db, err := OpenRaw(p)
	if err != nil {
		t.Fatalf("OpenRaw: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE stale (x TEXT)`); err != nil {
		t.Fatalf("create stale: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	db.Close()

	s, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	raw, err := OpenRaw(p)
	if err != nil {
		t.Fatalf("OpenRaw: %v", err)
	}
	defer raw.Close()
	var n int
	if err := raw.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='stale'`).Scan(&n); err != nil {
		t.Fatalf("query stale: %v", err)
	}
	if n != 0 {
		t.Fatal("stale table survived a version mismatch; expected delete-and-rebuild")
	}
	var v int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if v != SchemaVersion() {
		t.Fatalf("user_version = %d, want %d", v, SchemaVersion())
	}
}

func TestOpenTablelessV0FileIsSilent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "workspace.db")
	db, err := OpenRaw(p)
	if err != nil {
		t.Fatalf("OpenRaw: %v", err)
	}
	// Touch the file into existence at v0 with no tables.
	if _, err := db.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE probe (x TEXT)`); err != nil {
		t.Fatalf("create probe: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE probe`); err != nil {
		t.Fatalf("drop probe: %v", err)
	}
	db.Close()
	if got := tableCount(t, p); got != 0 {
		t.Fatalf("precondition: expected a table-less file, got %d tables", got)
	}

	stderr := captureStderr(t, func() {
		s, err := Open(p)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		s.Close()
	})
	if stderr != "" {
		t.Fatalf("expected no warning for a table-less v0 file, got %q", stderr)
	}
	if tableCount(t, p) == 0 {
		t.Fatal("expected schema tables after Open")
	}
}

// captureStderr swaps os.Stderr for a pipe around fn and returns what fn wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stderr = saved
	w.Close()
	out := <-done
	r.Close()
	return out
}
