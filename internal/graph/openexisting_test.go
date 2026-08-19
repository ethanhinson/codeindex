package graph

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// buildIndex creates a real index at path holding one symbol.
func buildIndex(t *testing.T, path string) {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, st, &ParsedFile{
		Path: "a.go",
		Symbols: []Symbol{
			{File: "a.go", Name: "Alpha", Kind: KindFunc, StartLine: 1, EndLine: 2},
		},
	})
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}

func snapshot(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOpenExistingAbsentDoesNotCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.db")

	st, err := OpenExisting(path)
	if err == nil {
		st.Close()
		t.Fatal("OpenExisting on an absent path: want error, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error %v does not wrap fs.ErrNotExist", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("OpenExisting created %s (stat: %v) — it must never create the file", path, statErr)
	}
	// Nor may it leave sqlite side files behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("OpenExisting created files in an empty dir: %v", entries)
	}
}

func TestOpenExistingValidIndexReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.db")
	buildIndex(t, path)

	st, err := OpenExisting(path)
	if err != nil {
		t.Fatalf("OpenExisting on a valid index: %v", err)
	}
	defer st.Close()

	defs, err := st.Definitions("Alpha", "")
	if err != nil {
		t.Fatalf("Definitions: %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "Alpha" {
		t.Fatalf("Definitions = %+v, want the one Alpha symbol", defs)
	}
}

func TestOpenExistingWrongVersionDoesNotRebuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.db")
	buildIndex(t, path)

	const bogus = 3
	db, err := OpenRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, bogus)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	before := snapshot(t, path)

	st, err := OpenExisting(path)
	if err == nil {
		st.Close()
		t.Fatal("OpenExisting on a version-mismatched index: want error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{strconv.Itoa(bogus), strconv.Itoa(SchemaVersion())} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name version %s; it must name both", msg, want)
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("version mismatch must not look like absence: %v", err)
	}

	after := snapshot(t, path)
	if string(before) != string(after) {
		t.Fatalf("OpenExisting rewrote the index on a version mismatch (%d -> %d bytes)",
			len(before), len(after))
	}
}

func TestOpenExistingCorruptFileDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, path)

	st, err := OpenExisting(path)
	if err == nil {
		st.Close()
		t.Fatal("OpenExisting on a garbage file: want error, got nil")
	}

	after := snapshot(t, path)
	if string(before) != string(after) {
		t.Fatalf("OpenExisting rewrote a corrupt file (%q -> %q)", before, after)
	}
}
