// Package merkle provides content-hash change detection: a file walk, per-file
// hashing with a mandatory size+mtime fast path, and a diff against stored state
// that identifies exactly which files must be re-parsed.
package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"

	"codeindex/internal/adapter"
	"codeindex/internal/graph"
)

// skipDir reports directories excluded from the walk (vendored/generated/VCS).
func skipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "testdata", ".codeindex":
		return true
	}
	return false
}

// Walk returns repo-relative paths of source files under root whose extension
// has a registered language adapter (the registry is the single source of
// truth — adding a language never touches the walk).
func Walk(root string) ([]string, error) { return WalkWith(root, nil) }

// WalkWith additionally admits files the extra callback claims — the
// content-sniffing hook for extensionless/odd-extension source (PHP can be
// .inc, .module, anything). extra runs only for files the registry does not
// already cover.
func WalkWith(root string, extra func(rel string, d fs.DirEntry) bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if adapter.Indexable(rel) || (extra != nil && extra(rel, d)) {
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}

// FileWork is a file that must be (re)parsed, with its fresh metadata and bytes
// (already read during hashing, so the engine need not read it again).
type FileWork struct {
	Meta graph.FileMeta
	Src  []byte
}

// Change is the result of a diff between the working tree and stored state.
type Change struct {
	Changed []FileWork       // added or modified -> re-parse
	Refresh []graph.FileMeta // content identical, only mtime moved -> refresh merkle
	Deleted []string         // removed since last index
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Detect walks root and diffs against stored metadata. The size+mtime fast path
// avoids reading/hashing files whose stat is unchanged.
func Detect(root string, stored map[string]graph.FileMeta) (*Change, error) {
	return DetectWith(root, stored, nil)
}

// DetectWith is Detect with a walk-time sniffing hook (see WalkWith).
func DetectWith(root string, stored map[string]graph.FileMeta, extra func(rel string, d fs.DirEntry) bool) (*Change, error) {
	paths, err := WalkWith(root, extra)
	if err != nil {
		return nil, err
	}
	ch := &Change{}
	seen := make(map[string]struct{}, len(paths))
	for _, rel := range paths {
		seen[rel] = struct{}{}
		fi, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		size, mtime := fi.Size(), fi.ModTime().UnixNano()

		if prev, ok := stored[rel]; ok && prev.Size == size && prev.Mtime == mtime {
			continue // fast path: stat unchanged -> no hashing, no work
		}

		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		h := hashBytes(src)
		meta := graph.FileMeta{Path: rel, Hash: h, Size: size, Mtime: mtime}

		if prev, ok := stored[rel]; ok && prev.Hash == h {
			ch.Refresh = append(ch.Refresh, meta) // content same, mtime moved
			continue
		}
		ch.Changed = append(ch.Changed, FileWork{Meta: meta, Src: src})
	}
	for path := range stored {
		if _, ok := seen[path]; !ok {
			ch.Deleted = append(ch.Deleted, path)
		}
	}
	return ch, nil
}
