// Package engine orchestrates the walking-skeleton pipeline: walk -> parse
// (concurrently) -> resolve name-based call edges -> persist, for both a full
// build and an incremental patch driven by Merkle change detection.
package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"codeindex/internal/adapter"
	_ "codeindex/internal/adapter/golang" // register the Go adapter
	"codeindex/internal/graph"
	"codeindex/internal/merkle"
)

// Stats summarizes a build or patch.
type Stats struct {
	FilesParsed int
	Symbols     int
	Deleted     int
}

// parseAll parses the given files concurrently using a worker pool.
func parseAll(root string, work []merkle.FileWork) ([]*graph.ParsedFile, error) {
	out := make([]*graph.ParsedFile, len(work))
	errs := make([]error, len(work))
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i, w := range work {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, w merkle.FileWork) {
			defer wg.Done()
			defer func() { <-sem }()
			a := adapter.For(w.Meta.Path)
			if a == nil {
				out[i] = &graph.ParsedFile{Path: w.Meta.Path}
				return
			}
			pf, err := a.Parse(w.Meta.Path, w.Src)
			if err != nil {
				errs[i] = err
				return
			}
			out[i] = pf
		}(i, w)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Build indexes root from scratch into dbPath. All files are inserted, then every
// edge is re-resolved so results are independent of file insertion order.
func Build(root, dbPath string) (Stats, error) {
	store, err := graph.Open(dbPath)
	if err != nil {
		return Stats{}, err
	}
	defer store.Close()

	ch, err := merkle.Detect(root, nil) // nil stored -> everything is "changed"
	if err != nil {
		return Stats{}, err
	}
	parsed, err := parseAll(root, ch.Changed)
	if err != nil {
		return Stats{}, err
	}

	tx, err := store.Begin()
	if err != nil {
		return Stats{}, err
	}
	defer tx.Rollback()

	var st Stats
	for i, pf := range parsed {
		if _, _, err := store.PutFile(tx, pf, ch.Changed[i].Meta); err != nil {
			return Stats{}, err
		}
		st.FilesParsed++
		st.Symbols += len(pf.Symbols)
	}
	if err := tx.Commit(); err != nil {
		return Stats{}, err
	}

	// Second pass: re-resolve all edges against the complete symbol table.
	names, err := store.AllDstNames()
	if err != nil {
		return Stats{}, err
	}
	tx2, err := store.Begin()
	if err != nil {
		return Stats{}, err
	}
	defer tx2.Rollback()
	if err := store.ReResolveNames(tx2, names); err != nil {
		return Stats{}, err
	}
	return st, tx2.Commit()
}

// Patch applies an incremental update to the existing index at dbPath: it detects
// changed/deleted files, re-parses only those, and re-resolves edges for the
// affected names (inbound blast radius). The result is identical to a full rebuild.
func Patch(root, dbPath string) (Stats, error) {
	store, err := graph.Open(dbPath)
	if err != nil {
		return Stats{}, err
	}
	defer store.Close()

	stored, err := store.StoredMeta()
	if err != nil {
		return Stats{}, err
	}
	ch, err := merkle.Detect(root, stored)
	if err != nil {
		return Stats{}, err
	}
	parsed, err := parseAll(root, ch.Changed)
	if err != nil {
		return Stats{}, err
	}

	tx, err := store.Begin()
	if err != nil {
		return Stats{}, err
	}
	defer tx.Rollback()

	affected := map[string]struct{}{}
	add := func(names []string) {
		for _, n := range names {
			affected[n] = struct{}{}
		}
	}

	var st Stats
	for i, pf := range parsed {
		before, after, err := store.PutFile(tx, pf, ch.Changed[i].Meta)
		if err != nil {
			return Stats{}, err
		}
		add(before)
		add(after)
		st.FilesParsed++
		st.Symbols += len(pf.Symbols)
	}
	for _, path := range ch.Deleted {
		names, err := store.DeleteFile(tx, path)
		if err != nil {
			return Stats{}, err
		}
		add(names)
		st.Deleted++
	}
	for _, m := range ch.Refresh {
		if err := store.RefreshMerkle(tx, m); err != nil {
			return Stats{}, err
		}
	}
	if err := store.ReResolveNames(tx, affected); err != nil {
		return Stats{}, err
	}
	return st, tx.Commit()
}

// CountLines counts newline-terminated lines across the walked Go files (a cheap
// throughput denominator for the benchmark).
func CountLines(root string) (int, error) {
	paths, err := merkle.Walk(root)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, rel := range paths {
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return 0, err
		}
		for _, b := range src {
			if b == '\n' {
				total++
			}
		}
	}
	return total, nil
}
