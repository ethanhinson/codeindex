// Package ghsync provides injectable GitHub CLI access for lore lifecycle signals.
// It wraps gh shell-outs behind a Runner func so tests can supply canned output
// without touching the filesystem or requiring a live gh binary.
package ghsync

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Runner is the function type for executing gh subcommands. dir is the working
// directory; args are passed directly to gh (without "gh" itself). The default
// Runner shells out to the system gh binary.
type Runner func(dir string, args ...string) (string, error)

// defaultRunner executes gh in dir with the given args.
func defaultRunner(dir string, args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// GH provides GitHub CLI access for a single repository.
type GH struct {
	runner Runner
}

// New returns a GH that uses the system gh binary.
func New() *GH {
	return &GH{runner: defaultRunner}
}

// NewWithRunner returns a GH using the supplied Runner — intended for tests.
func NewWithRunner(r Runner) *GH {
	return &GH{runner: r}
}

// Available reports whether gh is on PATH.
func (g *GH) Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// ParseRef parses a gh-issue ref into (repo, issueNum, error).
// Accepted forms:
//   - "owner/repo#N"  → repo="owner/repo", num="N"
//   - "#N"            → repo="", num="N"
//   - "N"             → repo="", num="N"  (only when N is all digits)
//
// Any other form returns an error.
func ParseRef(ref string) (repo, num string, err error) {
	if strings.Contains(ref, "#") {
		// Could be "owner/repo#N" or "#N"
		idx := strings.LastIndex(ref, "#")
		repo = ref[:idx]
		num = ref[idx+1:]
		if num == "" {
			return "", "", fmt.Errorf("invalid gh-issue ref %q: empty issue number", ref)
		}
		return repo, num, nil
	}
	// No '#': must be all digits for a bare numeric ref.
	if _, err := strconv.Atoi(ref); err == nil {
		return "", ref, nil
	}
	return "", "", fmt.Errorf("invalid gh-issue ref %q: expected owner/repo#N, #N, or bare N", ref)
}

// IssueState returns the state ("OPEN" or "CLOSED") for the given issue ref.
// ref may be "owner/repo#N", "#N", or bare "N". For qualified refs, --repo is
// passed to gh; for bare refs, gh infers the repo from the repoDir.
//
// Any gh error (including auth errors) is returned as a real error.
func (g *GH) IssueState(repoDir, ref string) (string, error) {
	repo, num, err := ParseRef(ref)
	if err != nil {
		return "", err
	}
	args := []string{"issue", "view", num, "--json", "state"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := g.runner(repoDir, args...)
	if err != nil {
		return "", fmt.Errorf("gh issue view %s: %w (check gh auth status)", ref, err)
	}
	var payload struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		return "", fmt.Errorf("gh issue view %s: malformed JSON output: %w", ref, err)
	}
	if payload.State == "" {
		return "", fmt.Errorf("gh issue view %s: missing state field in JSON", ref)
	}
	return payload.State, nil
}

// CreateIssue creates a new GitHub issue with the given title and body, and
// returns the issue URL. The body should include the "lore: <id>" backlink.
//
// Any gh error (including auth errors) is returned as a real error.
func (g *GH) CreateIssue(repoDir, title, body string) (string, error) {
	args := []string{"issue", "create", "--title", title, "--body", body}
	out, err := g.runner(repoDir, args...)
	if err != nil {
		return "", fmt.Errorf("gh issue create: %w (check gh auth status)", err)
	}
	// gh prints the URL as the last (or only) non-empty line.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line, nil
		}
	}
	return "", fmt.Errorf("gh issue create: no URL in output (check gh auth status)")
}
