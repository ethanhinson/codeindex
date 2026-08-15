package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeindex/internal/graph"
	codeindexruntime "codeindex/internal/runtime"
)

// runIngest folds cxprof profiles into the repo's index. With no target it
// sweeps the spool dir (.codeindex/runtime/); with --check it validates
// conformance without ingesting.
func runIngest(root string, args []string) error {
	check := false
	target := ""
	for _, a := range args {
		if a == "--check" {
			check = true
		} else if !strings.HasPrefix(a, "--") {
			target = a
		}
	}

	if check {
		if target == "" {
			return fmt.Errorf("usage: codeindex ingest <repo> <profile.cxprof.jsonl> --check")
		}
		p, err := codeindexruntime.Parse(target)
		if err != nil {
			return err
		}
		fmt.Printf("conforming cxprof v%d: lang=%s stacks=%d skipped-lines=%d\n",
			p.Header.Version, p.Header.Lang, len(p.Stacks), p.Skipped)
		return nil
	}

	st, err := graph.Open(filepath.Join(root, ".codeindex", "graph.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	report := func(s *codeindexruntime.Stats) {
		if s.Stacks == 0 {
			fmt.Printf("%s: already ingested\n", filepath.Base(s.Path))
			return
		}
		pct := 0.0
		if s.FramesTotal > 0 {
			pct = 100 * float64(s.FramesResolved) / float64(s.FramesTotal)
		}
		fmt.Printf("%s: %d stacks (%d samples) -> %d observed edges; frame resolution %.0f%%\n",
			filepath.Base(s.Path), s.Stacks, s.Samples, s.Edges, pct)
	}

	if target == "" || isDir(target) {
		dir := target
		if dir == "" {
			dir = codeindexruntime.SpoolDir(root)
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		n := 0
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".cxprof.jsonl") {
				continue
			}
			s, err := codeindexruntime.Ingest(st, root, filepath.Join(dir, e.Name()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "skipping %s: %v\n", e.Name(), err)
				continue
			}
			report(s)
			n++
		}
		if n == 0 {
			fmt.Println("no *.cxprof.jsonl profiles found")
		}
		return nil
	}

	s, err := codeindexruntime.Ingest(st, root, target)
	if err != nil {
		return err
	}
	report(s)
	return nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
