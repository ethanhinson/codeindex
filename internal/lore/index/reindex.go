package index

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codeindex/internal/lore"
)

type FileError struct {
	Path string
	Err  error
}

type Report struct {
	Indexed, Removed int
	Errors           []FileError
	// Duplicates holds entries formatted "<id>: <path1>, <path2>" for IDs
	// found in more than one file during this reindex run.
	Duplicates []string
}

// source is one directory to scan: its layer tag and the record type its
// files parse as.
type source struct {
	dir   string
	layer string
	typ   lore.Type
}

func sources(l lore.Layout) []source {
	var out []source
	for _, t := range []lore.Type{lore.TypeDecision, lore.TypeItem, lore.TypeNote} {
		out = append(out,
			source{l.Dir("repo", t), "repo", t},
			source{l.Dir("overlay", t), "overlay", t})
	}
	out = append(out, source{l.SessionsDir(), "session", lore.TypeNote})
	return out
}

// Reindex opens (or creates) the lore index and patches it to match the
// record files on disk. Malformed files are reported, never fatal.
func Reindex(l lore.Layout, dbPath string) (*Store, Report, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, Report{}, err
	}
	s, err := Open(dbPath)
	if err != nil {
		return nil, Report{}, err
	}
	var rep Report
	stored, err := s.FileHashes()
	if err != nil {
		s.Close()
		return nil, rep, err
	}

	// idToPath tracks which file each ID was seen in during this run. Every
	// parseable file is tracked — including unchanged ones, which are parsed
	// for ID visibility but not re-upserted (lore_records is keyed by ID, so
	// it cannot hold two paths for one ID and cannot seed this map). A
	// collision (same ID, different paths) is recorded in rep.Duplicates.
	idToPath := make(map[string]string)
	// dupPaths accumulates all paths seen for an ID once a collision is found.
	dupPaths := make(map[string][]string)

	recordDup := func(id, path string) {
		if prev, ok := idToPath[id]; ok && prev != path {
			// Collision.
			if _, already := dupPaths[id]; !already {
				dupPaths[id] = []string{prev}
			}
			dupPaths[id] = append(dupPaths[id], path)
		}
	}

	seen := map[string]bool{}

	for _, src := range sources(l) {
		entries, err := os.ReadDir(src.dir)
		if err != nil {
			continue // missing layer dir: fine
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") ||
				strings.EqualFold(e.Name(), "README.md") {
				continue
			}
			p := filepath.Join(src.dir, e.Name())
			seen[p] = true
			b, err := os.ReadFile(p)
			if err != nil {
				rep.Errors = append(rep.Errors, FileError{p, err})
				continue
			}
			h := fmt.Sprintf("%x", sha256.Sum256(b))
			unchanged := stored[p] == h
			rec, err := lore.Parse(b, src.typ)
			if err != nil {
				if !unchanged {
					// Only report parse errors for changed files; unchanged
					// files that were previously indexed are left as-is.
					rep.Errors = append(rep.Errors, FileError{p, err})
				}
				continue
			}
			// Track the ID for every valid file regardless of whether it changed,
			// so we can detect duplicates across unchanged+changed files.
			recordDup(rec.ID, p)
			idToPath[rec.ID] = p
			if unchanged {
				continue
			}
			if err := s.Upsert(rec, src.layer, p); err != nil {
				s.Close()
				return nil, rep, err
			}
			if err := s.SetFileHash(p, h); err != nil {
				s.Close()
				return nil, rep, err
			}
			rep.Indexed++
		}
	}

	for p := range stored {
		if seen[p] {
			continue
		}
		if err := s.DeleteByFile(p); err != nil {
			s.Close()
			return nil, rep, err
		}
		if err := s.DeleteFileHash(p); err != nil {
			s.Close()
			return nil, rep, err
		}
		rep.Removed++
	}

	// Build Duplicates slice: "<id>: <path1>, <path2>, ..." (sorted for
	// deterministic output).
	for id, paths := range dupPaths {
		rep.Duplicates = append(rep.Duplicates, id+": "+strings.Join(paths, ", "))
	}
	sort.Strings(rep.Duplicates)

	return s, rep, nil
}
