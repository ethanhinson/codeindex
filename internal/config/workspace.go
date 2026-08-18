package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WorkspaceFile is the workspace manifest, relative to the workspace root.
// Note this is a *directory* entry (.codeindex/) — it does not collide with
// FileName, the repo config file at the repo root.
const WorkspaceFile = ".codeindex/workspace.json"

// Workspace is the committed manifest describing a multi-repo workspace: the
// members whose graphs are unioned, and the edges declared between them.
type Workspace struct {
	Version int      `json:"version"`
	Members []Member `json:"members"`
}

// Member is one repo participating in a workspace. Root is relative to the
// workspace root and may climb out of it ("../symfony") — the dedicated
// workspace-directory shape depends on that.
type Member struct {
	ID         string   `json:"id"`
	Root       string   `json:"root"`
	Namespaces []string `json:"namespaces"`
	Deps       []string `json:"deps,omitempty"`
}

// LoadWorkspace reads and validates wsRoot's manifest. A missing file is an
// error wrapping the underlying os error, so callers wanting "no manifest ⇒
// single-repo mode" test errors.Is(err, fs.ErrNotExist) rather than matching
// strings.
//
// Validation is of shape only: LoadWorkspace never stats a member root. A
// workspace must still answer queries while one member is unavailable, so
// absence is reported later by Resolve, never by the load path. Unknown fields
// are ignored, matching Load's posture, so later manifest growth stays
// backward-compatible.
func LoadWorkspace(wsRoot string) (*Workspace, error) {
	path := filepath.Join(wsRoot, WorkspaceFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ws Workspace
	if err := json.Unmarshal(b, &ws); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := ws.validate(path); err != nil {
		return nil, err
	}
	return &ws, nil
}

// validate enforces the manifest's shape rules. Every rule is an error: a
// manifest that is wrong is an authoring mistake to fail loudly on, not a
// degenerate workspace to serve half an answer from.
func (w *Workspace) validate(path string) error {
	// An absent version is diagnosed on its own rather than being folded into
	// the unsupported-version or parse-failure messages: unlike a streamed
	// header, a parsed object's missing field is precisely locatable.
	if w.Version == 0 {
		return fmt.Errorf("%s: missing workspace version field (expected 1)", path)
	}
	if w.Version != 1 {
		return fmt.Errorf("%s: unsupported workspace version %d", path, w.Version)
	}
	if len(w.Members) == 0 {
		return fmt.Errorf("%s: no members declared", path)
	}

	seenID := make(map[string]int, len(w.Members))
	seenRoot := make(map[string]int, len(w.Members))
	for i, m := range w.Members {
		if m.ID == "" {
			return fmt.Errorf("%s: member %d: empty id", path, i)
		}
		// Ids carry an anchor-prefix role ("api:HandleLogin"), so a ':' or '/'
		// in an id would make that grammar ambiguous.
		for _, r := range m.ID {
			if !isMemberIDRune(r) {
				return fmt.Errorf("%s: member %q: invalid id character %q (allowed: A-Z a-z 0-9 . _ -)", path, m.ID, r)
			}
		}
		if j, dup := seenID[m.ID]; dup {
			return fmt.Errorf("%s: member %d: duplicate id %q (already used by member %d)", path, i, m.ID, j)
		}
		seenID[m.ID] = i

		if m.Root == "" {
			return fmt.Errorf("%s: member %q: empty root", path, m.ID)
		}
		// Roots are relative to the workspace root; an absolute root makes the
		// committed manifest non-portable.
		if filepath.IsAbs(m.Root) {
			return fmt.Errorf("%s: member %q: root %q must be relative to the workspace root", path, m.ID, m.Root)
		}
		clean := filepath.Clean(m.Root)
		if j, dup := seenRoot[clean]; dup {
			// Two ids over one tree would double-count every symbol in the
			// union graph.
			return fmt.Errorf("%s: member %q: duplicate root %q (already used by member %q)", path, m.ID, m.Root, w.Members[j].ID)
		}
		seenRoot[clean] = i
	}
	return nil
}

// isMemberIDRune reports whether r is allowed in a member id.
func isMemberIDRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '.', r == '_', r == '-':
		return true
	}
	return false
}
