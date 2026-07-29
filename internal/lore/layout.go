package lore

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Layout locates the two record layers for a repo.
type Layout struct {
	RepoRoot   string // absolute repo root
	RepoDir    string // <root>/.lore (committed)
	OverlayDir string // <home>/.codeindex/lore/<repo-id> (private)
}

func NewLayout(root string) (Layout, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, err
	}
	home := os.Getenv("CODEINDEX_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return Layout{}, err
		}
		home = filepath.Join(h, ".codeindex")
	}
	return Layout{
		RepoRoot:   abs,
		RepoDir:    filepath.Join(abs, ".lore"),
		OverlayDir: filepath.Join(home, "lore", RepoID(abs)),
	}, nil
}

func typeDir(t Type) string {
	switch t {
	case TypeDecision:
		return "decisions"
	case TypeItem:
		return "items"
	}
	return "notes"
}

// Dir returns the directory for a layer ("repo" | "overlay") and record type.
func (l Layout) Dir(layer string, t Type) string {
	if layer == "overlay" {
		return filepath.Join(l.OverlayDir, typeDir(t))
	}
	return filepath.Join(l.RepoDir, typeDir(t))
}

func (l Layout) SessionsDir() string { return filepath.Join(l.OverlayDir, "sessions") }

// RepoID identifies a repo across clones and worktrees: the normalized origin
// remote hashed, so every checkout of one repo shares one overlay. Repos
// without an origin fall back to their absolute path.
func RepoID(root string) string {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Dir = root
	out, err := cmd.Output()
	key := strings.TrimSpace(string(out))
	slug := filepath.Base(root)
	if err == nil && key != "" {
		key = normalizeOrigin(key)
		if i := strings.LastIndex(key, "/"); i >= 0 {
			slug = key[i+1:]
		}
	} else {
		abs, _ := filepath.Abs(root)
		key = abs
	}
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%s-%x", Slug(slug), sum[:6])
}

// normalizeOrigin maps SSH and HTTPS remote forms to "host/org/repo".
func normalizeOrigin(u string) string {
	u = strings.TrimSuffix(u, ".git")
	if rest, ok := strings.CutPrefix(u, "git@"); ok { // git@host:org/repo
		return strings.Replace(rest, ":", "/", 1)
	}
	for _, p := range []string{"https://", "http://", "ssh://git@", "ssh://"} {
		if rest, ok := strings.CutPrefix(u, p); ok {
			return rest
		}
	}
	return u
}
