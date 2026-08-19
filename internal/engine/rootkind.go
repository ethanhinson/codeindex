package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"codeindex/internal/adapter"
	"codeindex/internal/config"
)

// RootKind classifies what a root path is: a single code repository, or a
// workspace whose manifest names the repos to union.
type RootKind int

const (
	RootRepo RootKind = iota
	RootWorkspace
)

// errFoundIndexable aborts the hasIndexableSource walk on the first admitted
// file. It never escapes the helper.
var errFoundIndexable = errors.New("indexable source found")

// DetectRootKind classifies root. A root holding the workspace manifest is a
// workspace; otherwise a root holding at least one indexable source file is a
// repo; a root with neither is an error naming both possibilities.
//
// The manifest is only stat'ed, never parsed: a malformed manifest is
// LoadWorkspace's error to report, and silently misclassifying it as a
// single repo is the worse failure.
func DetectRootKind(root string) (RootKind, error) {
	switch _, err := os.Stat(filepath.Join(root, config.WorkspaceFile)); {
	case err == nil:
		return RootWorkspace, nil
	case !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR):
		// Unreadable is not the same as absent — do not guess. ENOTDIR is
		// absence too: it fires when .codeindex exists as a regular file
		// rather than a directory, which is not a fault in the repo.
		return RootRepo, err
	}
	filter, err := config.LoadFilter(root)
	if err != nil {
		return RootRepo, err
	}
	// adapter.Indexable is passed by value and only ever read.
	// adapter.SetAssociations is deliberately NOT called: it replaces
	// process-global routes, so a detection running inside a live MCP server
	// would corrupt the serving repo's routing. Callers that need the repo's
	// `associations` rules honored install them once at startup.
	ok, err := hasIndexableSource(root, filter, adapter.Indexable)
	if err != nil {
		return RootRepo, err
	}
	if ok {
		return RootRepo, nil
	}
	return RootRepo, fmt.Errorf(
		"%s: not a code repository (no indexable source) and not a workspace (no %s)",
		root, config.WorkspaceFile)
}

// hasIndexableSource reports whether root contains at least one file filter
// admits and indexable claims, aborting the walk on the first hit — so it is
// O(files-until-first-hit), not a full-tree walk.
//
// The filter and the predicate are parameters rather than loaded here: the
// caller passes the repo's real filter and the real registry predicate, which
// keeps detection in step with the language registry (never a hard-coded
// extension list) and keeps this helper testable without touching adapter's
// process-global state. merkle.WalkWith is not reused: it returns the complete
// path slice with no early-exit hook.
func hasIndexableSource(root string, filter *config.Filter, indexable func(rel string) bool) (bool, error) {
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel != "." && filter.SkipDir(rel, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filter.SkipFile(rel) {
			return nil
		}
		if indexable(rel) {
			return errFoundIndexable
		}
		return nil
	})
	if errors.Is(err, errFoundIndexable) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}
