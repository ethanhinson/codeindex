// Package engine orchestrates the walking-skeleton pipeline: walk -> parse
// (concurrently) -> resolve name-based call edges -> persist, for both a full
// build and an incremental patch driven by Merkle change detection.
package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"codeindex/internal/adapter"
	_ "codeindex/internal/adapter/golang" // register language adapters
	_ "codeindex/internal/adapter/php"
	_ "codeindex/internal/adapter/python"
	_ "codeindex/internal/adapter/tsjs"
	"codeindex/internal/config"
	"codeindex/internal/graph"
	"codeindex/internal/merkle"
	"codeindex/internal/progress"
)

// Stats summarizes a build or patch.
type Stats struct {
	FilesParsed int
	Symbols     int
	Deleted     int
	Embedded    int // cards embedded this pass (cache misses only)
}

// parseAll parses the given files concurrently using a worker pool; done, if
// non-nil, is called with the running completion count.
func parseAll(root string, work []merkle.FileWork, done func(int)) ([]*graph.ParsedFile, error) {
	out := make([]*graph.ParsedFile, len(work))
	errs := make([]error, len(work))
	sem := make(chan struct{}, runtime.NumCPU())
	var completed int64
	var wg sync.WaitGroup
	for i, w := range work {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, w merkle.FileWork) {
			defer wg.Done()
			defer func() {
				<-sem
				if done != nil {
					done(int(atomic.AddInt64(&completed, 1)))
				}
			}()
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
	return BuildWithProgress(root, dbPath, nil)
}

// BuildWithProgress is Build with progress reporting. Independent of rep, the
// engine maintains a best-effort status.json sidecar next to the index so
// any surface (status verb, IDE status bar) can observe the build.
func BuildWithProgress(root, dbPath string, rep progress.Reporter) (Stats, error) {
	if err := loadAssociations(root); err != nil {
		return Stats{}, err
	}
	side := progress.NewSidecar(sidecarPath(dbPath), "building")
	rep = progress.Multi(rep, side)

	store, err := graph.Open(dbPath)
	if err != nil {
		return Stats{}, err
	}
	defer store.Close()

	rep.Report(progress.Event{Phase: "walk"})
	sniff, flushSniff := newSniffer(root, store)
	ch, err := merkle.DetectWith(root, nil, sniff) // nil stored -> everything is "changed"
	if err != nil {
		return Stats{}, err
	}
	if err := flushSniff(); err != nil {
		return Stats{}, err
	}
	total := len(ch.Changed)
	parsed, err := parseAll(root, ch.Changed, func(done int) {
		rep.Report(progress.Event{Phase: "parse", Done: done, Total: total})
	})
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
		rep.Report(progress.Event{Phase: "write", Done: i + 1, Total: total})
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
	if err := store.ReResolveNamesP(tx2, names, func(done, totalNames int) {
		rep.Report(progress.Event{Phase: "resolve", Done: done, Total: totalNames})
	}); err != nil {
		return Stats{}, err
	}
	if err := tx2.Commit(); err != nil {
		return Stats{}, err
	}
	// Embedding runs after resolution so cards see final caller/callee edges.
	st.Embedded, err = EmbedPass(store, root, nil, rep)
	if err != nil {
		return Stats{}, err
	}
	side.FinishCounts(st.FilesParsed, st.Symbols)
	return st, nil
}

// loadAssociations applies the repo's committed file-type associations
// (.codeindex.json) before any walk — loaded fresh on every build/patch so
// a long-lived MCP server picks up config edits on the next query.
func loadAssociations(root string) error {
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	return adapter.SetAssociations(cfg.Associations)
}

// newSniffer returns a walk hook that content-sniffs files no extension or
// association covers (PHP open tags, shebangs), with verdicts cached in the
// index keyed by size+mtime so unchanged files never get re-read. flush
// installs the discovered per-path routes (so parsing finds the right
// adapter) and persists new verdicts — call it after the walk, before parse.
func newSniffer(root string, store *graph.Store) (func(rel string, d fs.DirEntry) bool, func() error) {
	// Routes are strictly per-run: a leftover set from a previous build in
	// this process would make Indexable() claim a file at walk time and then
	// vanish before parse (walk sniffs only what the registry doesn't cover).
	adapter.SetExactRoutes(nil)
	cache, err := store.SniffCache()
	if err != nil {
		cache = map[string]graph.SniffEntry{} // degraded: sniff everything
	}
	routes := map[string]string{}
	var updates []graph.SniffEntry

	sniff := func(rel string, d fs.DirEntry) bool {
		fi, err := d.Info()
		if err != nil {
			return false
		}
		size, mtime := fi.Size(), fi.ModTime().UnixNano()
		if e, ok := cache[rel]; ok && e.Size == size && e.Mtime == mtime {
			if e.Lang != "" {
				routes[rel] = e.Lang
				return true
			}
			return false
		}
		lang := ""
		if f, err := os.Open(filepath.Join(root, rel)); err == nil {
			head := make([]byte, 1024)
			n, _ := f.Read(head)
			f.Close()
			lang = adapter.SniffLang(head[:n])
		}
		updates = append(updates, graph.SniffEntry{Path: rel, Size: size, Mtime: mtime, Lang: lang})
		if lang != "" {
			routes[rel] = lang
			return true
		}
		return false
	}
	flush := func() error {
		adapter.SetExactRoutes(routes)
		return store.PutSniffEntries(updates)
	}
	return sniff, flush
}

func sidecarPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "status.json")
}

// Patch applies an incremental update to the existing index at dbPath: it detects
// changed/deleted files, re-parses only those, and re-resolves edges for the
// affected names (inbound blast radius). The result is identical to a full rebuild.
func Patch(root, dbPath string) (Stats, error) {
	return PatchWithProgress(root, dbPath, nil)
}

// PatchWithProgress is Patch with progress reporting (and the same status
// sidecar as builds — state "patching").
func PatchWithProgress(root, dbPath string, rep progress.Reporter) (Stats, error) {
	if err := loadAssociations(root); err != nil {
		return Stats{}, err
	}
	side := progress.NewSidecar(sidecarPath(dbPath), "patching")
	rep = progress.Multi(rep, side)

	store, err := graph.Open(dbPath)
	if err != nil {
		return Stats{}, err
	}
	defer store.Close()

	stored, err := store.StoredMeta()
	if err != nil {
		return Stats{}, err
	}
	rep.Report(progress.Event{Phase: "walk"})
	sniff, flushSniff := newSniffer(root, store)
	ch, err := merkle.DetectWith(root, stored, sniff)
	if err != nil {
		return Stats{}, err
	}
	if err := flushSniff(); err != nil {
		return Stats{}, err
	}
	total := len(ch.Changed)
	parsed, err := parseAll(root, ch.Changed, func(done int) {
		rep.Report(progress.Event{Phase: "parse", Done: done, Total: total})
	})
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
		rep.Report(progress.Event{Phase: "write", Done: i + 1, Total: total})
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
	if err := store.ReResolveNamesP(tx, affected, func(done, totalNames int) {
		rep.Report(progress.Event{Phase: "resolve", Done: done, Total: totalNames})
	}); err != nil {
		return Stats{}, err
	}
	if err := tx.Commit(); err != nil {
		return Stats{}, err
	}
	// Re-embed only symbols from re-parsed files: their mappings died with
	// PutFile's delete, and unchanged card text re-attaches via the
	// content-addressed cache without touching the model. No changed files =
	// no provider load — the per-query patch path stays cheap.
	if len(ch.Changed) > 0 {
		changedPaths := make([]string, len(ch.Changed))
		for i, w := range ch.Changed {
			changedPaths[i] = w.Meta.Path
		}
		st.Embedded, err = EmbedPass(store, root, changedPaths, rep)
		if err != nil {
			return Stats{}, err
		}
	}
	side.FinishCounts(st.FilesParsed, st.Symbols)
	return st, nil
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
