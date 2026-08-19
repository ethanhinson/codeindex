package main

import (
	"errors"
	"fmt"
	"io/fs"

	"codeindex/internal/config"
	"codeindex/internal/workspace"
)

// runInitWorkspace implements `codeindex init-workspace <workspace-root>
// --scan [--force]`: it discovers member namespaces (and, on a monorepo root,
// members themselves), merges the results into the workspace manifest with
// authored values winning, and writes it back.
//
// args is os.Args[3:] — the flags after the workspace root. It is passed in
// rather than read from the global so the verb is testable end-to-end.
//
// Every failure here returns an error for main's fatal() to report, which exits
// 1: exit 2 stays exclusive to main's arg-count guard.
func runInitWorkspace(root string, args []string) error {
	var scan, force bool
	for _, a := range args {
		switch a {
		case "--scan":
			scan = true
		case "--force":
			force = true
		}
	}
	// --scan is required rather than defaulted so that a future non-scanning
	// mode of this verb cannot silently change what an existing invocation does.
	if !scan {
		return fmt.Errorf("usage: codeindex init-workspace <workspace-root> --scan [--force]")
	}

	ws, err := config.LoadWorkspace(root)
	switch {
	case err == nil:
		if !force {
			return fmt.Errorf("%s/%s already exists; pass --force to rescan and fill gaps (authored values are preserved)",
				root, config.WorkspaceFile)
		}
	case errors.Is(err, fs.ErrNotExist):
		// No manifest yet: the scan authors one from what it discovers.
		ws = &config.Workspace{Version: 1}
	default:
		// A manifest that exists but does not load is reported, never silently
		// replaced — --force overwrites a manifest we understood, not one we
		// failed to read.
		return err
	}

	scanned, err := workspace.Scan(root, ws.Members)
	if err != nil {
		return err
	}
	ws.Members = workspace.Merge(ws.Members, scanned)
	if err := config.SaveWorkspace(root, ws); err != nil {
		return err
	}
	fmt.Printf("wrote %s/%s: %d members\n", root, config.WorkspaceFile, len(ws.Members))
	return nil
}
