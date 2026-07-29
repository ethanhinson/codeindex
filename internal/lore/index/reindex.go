package index

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
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
			if stored[p] == h {
				continue
			}
			rec, err := lore.Parse(b, src.typ)
			if err != nil {
				rep.Errors = append(rep.Errors, FileError{p, err})
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
	return s, rep, nil
}
