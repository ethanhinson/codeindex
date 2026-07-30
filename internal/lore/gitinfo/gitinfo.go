// Package gitinfo provides injectable git access for lore lifecycle signals.
// It wraps git shell-outs behind a Runner func so tests can supply canned
// output without touching the filesystem or requiring a live git binary.
package gitinfo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Runner is the function type for executing git subcommands. dir is the
// working directory; args are passed directly to git (without "git" itself).
// The default Runner shells out to the system git binary.
type Runner func(dir string, args ...string) (string, error)

// defaultRunner executes git in dir with the given args.
func defaultRunner(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// Git provides git introspection for a single repository root.
type Git struct {
	root   string
	runner Runner
}

// New returns a Git that uses the system git binary.
func New(repoRoot string) *Git {
	return &Git{root: repoRoot, runner: defaultRunner}
}

// NewWithRunner returns a Git using the supplied Runner — intended for tests.
func NewWithRunner(repoRoot string, r Runner) *Git {
	return &Git{root: repoRoot, runner: r}
}

// Available reports whether git is on PATH and the repo root contains a .git
// entry (directory for normal repos, file for worktrees). It uses os.Stat so
// both forms are accepted.
func (g *Git) Available() bool {
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(g.root, ".git"))
	return err == nil
}

// DefaultBranch returns the name of the remote default branch by parsing
// `git symbolic-ref refs/remotes/origin/HEAD`. Any error falls back to "main".
func (g *Git) DefaultBranch() string {
	out, err := g.runner(g.root, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "main"
	}
	ref := strings.TrimSpace(out)
	const prefix = "refs/remotes/origin/"
	if after, ok := strings.CutPrefix(ref, prefix); ok {
		return after
	}
	return "main"
}

// FileOnBranch reports whether relPath exists on branch using
// `git cat-file -e <branch>:<relPath>`. An error (exit code 1 or binary not
// found) means the file does not exist on that branch.
func (g *Git) FileOnBranch(branch, relPath string) bool {
	_, err := g.runner(g.root, "cat-file", "-e", fmt.Sprintf("%s:%s", branch, relPath))
	return err == nil
}

// Head returns the full SHA of HEAD.
func (g *Git) Head() (string, error) {
	out, err := g.runner(g.root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Commit holds a single git commit's metadata including per-file line stats.
type Commit struct {
	SHA     string
	Subject string
	Files   map[string][2]int // path -> [added, deleted]
}

// CommitsSince returns commits newer than sinceSHA (exclusive), oldest first,
// with per-file added/deleted line counts.
//
// When sinceSHA is empty all reachable commits are returned, capped at limit
// via -n <limit>. Note: git applies -n before --reverse, so when capping it
// takes the newest N commits and then reverses them — the oldest of those N
// are dropped, not the newest. This is acceptable cap semantics.
//
// Binary files appear in numstat as "-\t-\t<path>" and are silently skipped.
//
// The format uses %H%x00%s where %x00 is the git format escape for a NUL byte.
// Passing the literal four characters "%x00" as the arg avoids placing an
// actual NUL byte in the exec argument string (which POSIX rejects).
func (g *Git) CommitsSince(sinceSHA string, limit int) ([]Commit, error) {
	// git log --reverse --numstat --format=%H%x00%s [range]
	// %x00 is interpreted by git as NUL in its output; it is safe as an exec arg.
	args := []string{"log", "--reverse", "--numstat", "--format=%H%x00%s"}
	if sinceSHA != "" {
		args = append(args, sinceSHA+"..HEAD")
	} else {
		// Cap all-reachable output. git applies -n before --reverse:
		// it takes the newest N commits then reverses them.
		args = append(args, "-n", strconv.Itoa(limit))
	}

	out, err := g.runner(g.root, args...)
	if err != nil {
		return nil, err
	}
	return parseNumstatLog(out), nil
}

// parseNumstatLog parses the output of:
//
//	git log --reverse --numstat --format=%H%x00%s
//
// git emits each commit as:
//
//	<sha>\x00<subject>\n
//	\n
//	<added>\t<deleted>\t<path>\n   (repeated)
//	\n
//
// The NUL byte (git expands %x00) separates SHA from subject. A blank line
// appears between the header and the numstat lines for the same commit, and
// also between each commit's numstat and the next commit's header. We therefore
// flush the current commit only when a new header line is detected (not on
// every blank line). Binary files produce "-\t-\t<path>" and are skipped.
func parseNumstatLog(raw string) []Commit {
	var commits []Commit
	var cur *Commit

	for _, line := range strings.Split(raw, "\n") {
		// Header line: contains a NUL byte (0x00) separating SHA and subject.
		// git expands %x00 in --format to a literal NUL byte in its output.
		if idx := strings.IndexByte(line, '\x00'); idx >= 0 {
			// A new header means the previous commit is complete.
			if cur != nil {
				commits = append(commits, *cur)
			}
			cur = &Commit{
				SHA:     line[:idx],
				Subject: line[idx+1:],
				Files:   make(map[string][2]int),
			}
			continue
		}

		if line == "" {
			// Blank lines are separators within and between commit blocks;
			// we rely on header detection above to flush, so skip here.
			continue
		}

		// Numstat line: "<added>\t<deleted>\t<path>"
		if cur != nil {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) == 3 {
				addedStr, deletedStr, path := parts[0], parts[1], parts[2]
				// Skip binary files — both fields are "-".
				if addedStr == "-" || deletedStr == "-" {
					continue
				}
				added, err1 := strconv.Atoi(addedStr)
				deleted, err2 := strconv.Atoi(deletedStr)
				if err1 != nil || err2 != nil {
					continue
				}
				cur.Files[path] = [2]int{added, deleted}
			}
		}
	}

	// Flush the last commit.
	if cur != nil {
		commits = append(commits, *cur)
	}

	return commits
}
