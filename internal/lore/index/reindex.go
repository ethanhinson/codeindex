package index

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"codeindex/internal/lore"
	"codeindex/internal/lore/gitinfo"
)

// closesRe matches "closes <item-id>" in commit subjects (case-insensitive).
// The character class [0-9A-HJKMNP-TV-Z] is Crockford base32 (excludes I, L, O, U).
var closesRe = regexp.MustCompile(`(?i)\bcloses\s+(itm-[0-9A-HJKMNP-TV-Z]{26})\b`)

// newGit is the test seam for gitinfo construction. Tests override this to
// inject a fake runner without touching the filesystem or a live git binary.
var newGit func(root string) *gitinfo.Git = gitinfo.New

type FileError struct {
	Path string
	Err  error
}

type Report struct {
	Indexed, Removed int
	Errors           []FileError
	// Duplicates holds entries formatted "<id>: <path1>, <path2>" for IDs
	// found in more than one file during this reindex run.
	Duplicates []string
	// Closed holds the IDs of items whose status was flipped to "done" by a
	// "closes <id>" commit subject during this reindex run.
	Closed []string
}

// source is one directory to scan: its layer tag and the record type its
// files parse as.
type source struct {
	dir   string
	layer string
	typ   lore.Type
}

func sources(l lore.Layout) []source {
	var out []source
	for _, t := range []lore.Type{lore.TypeDecision, lore.TypeItem, lore.TypeNote} {
		out = append(out,
			source{l.Dir("repo", t), "repo", t},
			source{l.Dir("overlay", t), "overlay", t})
	}
	out = append(out, source{l.SessionsDir(), "session", lore.TypeNote})
	return out
}

// Reindex opens (or creates) the lore index and patches it to match the
// record files on disk. Malformed files are reported, never fatal.
func Reindex(l lore.Layout, dbPath string) (*Store, Report, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, Report{}, err
	}
	s, err := Open(dbPath)
	if err != nil {
		return nil, Report{}, err
	}
	var rep Report
	stored, err := s.FileHashes()
	if err != nil {
		s.Close()
		return nil, rep, err
	}

	// idToPath tracks which file each ID was seen in during this run. Every
	// parseable file is tracked — including unchanged ones, which are parsed
	// for ID visibility but not re-upserted (lore_records is keyed by ID, so
	// it cannot hold two paths for one ID and cannot seed this map). A
	// collision (same ID, different paths) is recorded in rep.Duplicates.
	idToPath := make(map[string]string)
	// dupPaths accumulates all paths seen for an ID once a collision is found.
	dupPaths := make(map[string][]string)

	recordDup := func(id, path string) {
		if prev, ok := idToPath[id]; ok && prev != path {
			// Collision.
			if _, already := dupPaths[id]; !already {
				dupPaths[id] = []string{prev}
			}
			dupPaths[id] = append(dupPaths[id], path)
		}
	}

	seen := map[string]bool{}

	for _, src := range sources(l) {
		entries, err := os.ReadDir(src.dir)
		if err != nil {
			continue // missing layer dir: fine
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") ||
				strings.EqualFold(e.Name(), "README.md") {
				continue
			}
			p := filepath.Join(src.dir, e.Name())
			seen[p] = true
			b, err := os.ReadFile(p)
			if err != nil {
				rep.Errors = append(rep.Errors, FileError{p, err})
				continue
			}
			h := fmt.Sprintf("%x", sha256.Sum256(b))
			unchanged := stored[p] == h
			rec, err := lore.Parse(b, src.typ)
			if err != nil {
				if !unchanged {
					// Only report parse errors for changed files; unchanged
					// files that were previously indexed are left as-is.
					rep.Errors = append(rep.Errors, FileError{p, err})
				}
				continue
			}
			// Track the ID for every valid file regardless of whether it changed,
			// so we can detect duplicates across unchanged+changed files.
			recordDup(rec.ID, p)
			idToPath[rec.ID] = p
			if unchanged {
				continue
			}
			if err := s.Upsert(rec, src.layer, p); err != nil {
				s.Close()
				return nil, rep, err
			}
			if err := s.SetFileHash(p, h); err != nil {
				s.Close()
				return nil, rep, err
			}
			rep.Indexed++
		}
	}

	for p := range stored {
		if seen[p] {
			continue
		}
		if err := s.DeleteByFile(p); err != nil {
			s.Close()
			return nil, rep, err
		}
		if err := s.DeleteFileHash(p); err != nil {
			s.Close()
			return nil, rep, err
		}
		rep.Removed++
	}

	// Build Duplicates slice: "<id>: <path1>, <path2>, ..." (sorted for
	// deterministic output).
	for id, paths := range dupPaths {
		rep.Duplicates = append(rep.Duplicates, id+": "+strings.Join(paths, ", "))
	}
	sort.Strings(rep.Duplicates)

	// Ratification pass: for each repo-layer record, determine whether the file
	// is present on origin/<default-branch>. Overlay and session records are
	// always ratified (their default is 1).
	//
	// Guard: only run this pass when origin exists. Without an origin ref,
	// FileOnBranch would return false for every file and mark everything
	// unratified — which is wrong for repos that have no remote yet. We check
	// HasRef("refs/remotes/origin/HEAD") as a reliable proxy for "origin
	// exists and has been fetched." This is intentional: if origin is not
	// configured, we trust the current state and leave all records ratified.
	g := newGit(l.RepoRoot)
	if g.Available() && g.HasRef("refs/remotes/origin/HEAD") {
		branch := "origin/" + g.DefaultBranch()
		all, err := s.All()
		if err != nil {
			s.Close()
			return nil, rep, err
		}
		for _, r := range all {
			if r.Layer != "repo" {
				continue // overlay/session always ratified
			}
			relPath, err := filepath.Rel(l.RepoRoot, r.File)
			if err != nil {
				continue
			}
			ratified := g.FileOnBranch(branch, relPath)
			if err := s.SetRatified(r.ID, ratified); err != nil {
				s.Close()
				return nil, rep, err
			}
		}
	}

	// Closes-transition + survival signals pass: scan commits since the last
	// reindex. CommitsSince is called ONCE; both passes iterate the same slice.
	//
	// Guard: requires git available (g.Available()) AND a readable HEAD. Unlike
	// the ratification pass, this does NOT require an origin ref — closes
	// transitions work in origin-less repos.
	if g.Available() {
		head, err := g.Head()
		if err == nil && head != "" {
			since, _ := s.Meta("last_scanned_commit")
			commits, err := g.CommitsSince(since, 500)
			if err == nil {
				// Closes-transition sub-pass.
				for _, c := range commits {
					matches := closesRe.FindAllStringSubmatch(c.Subject, -1)
					for _, m := range matches {
						itemID := m[1]
						row, found, err := s.Get(itemID)
						if err != nil || !found {
							continue // unknown ID — skip silently
						}
						if string(row.Type) != string(lore.TypeItem) || row.Status != "open" {
							continue // not an open item — skip silently
						}
						// Durable rewrite: Parse → mutate → Marshal → WriteFile.
						data, err := os.ReadFile(row.File)
						if err != nil {
							rep.Errors = append(rep.Errors, FileError{row.File, err})
							continue
						}
						rec, err := lore.Parse(data, lore.TypeItem)
						if err != nil {
							rep.Errors = append(rep.Errors, FileError{row.File, err})
							continue
						}
						rec.Status = "done"
						shortSHA := c.SHA
						if len(shortSHA) > 7 {
							shortSHA = shortSHA[:7]
						}
						rec.Refs = append(rec.Refs, lore.Ref{Kind: "commit", Value: shortSHA})
						newData, err := rec.Marshal()
						if err != nil {
							rep.Errors = append(rep.Errors, FileError{row.File, err})
							continue
						}
						if err := os.WriteFile(row.File, newData, 0o644); err != nil {
							rep.Errors = append(rep.Errors, FileError{row.File, err})
							continue
						}
						// Re-upsert and update file hash so next reindex sees it as unchanged.
						if err := s.Upsert(rec, row.Layer, row.File); err != nil {
							rep.Errors = append(rep.Errors, FileError{row.File, err})
							continue
						}
						newHash := fmt.Sprintf("%x", sha256.Sum256(newData))
						if err := s.SetFileHash(row.File, newHash); err != nil {
							rep.Errors = append(rep.Errors, FileError{row.File, err})
							continue
						}
						rep.Closed = append(rep.Closed, itemID)
					}
				}

				// Survival/churn signals sub-pass (repo-layer records with path anchors only).
				// For each commit: if an anchored path prefix-matches a changed file path
				// AND the record's own file is NOT among the commit's changed files
				// → survived+1, churnLines += added+deleted for matching anchored files.
				all, aerr := s.All()
				if aerr == nil {
					for _, c := range commits {
						for _, r := range all {
							if r.Layer != "repo" {
								continue
							}
							// Collect path anchors only.
							var pathAnchors []string
							for _, a := range r.Anchors {
								if a.Path != "" {
									pathAnchors = append(pathAnchors, a.Path)
								}
							}
							if len(pathAnchors) == 0 {
								continue
							}
							// Determine if the record's own file is in this commit.
							// Compare relative repo path prefix.
							ownRelPath, err := filepath.Rel(l.RepoRoot, r.File)
							if err != nil {
								continue
							}
							ownInCommit := false
							for changedPath := range c.Files {
								if changedPath == ownRelPath {
									ownInCommit = true
									break
								}
							}
							if ownInCommit {
								// Record moved with the code — no survival credit.
								continue
							}
							// Check if any anchored path prefix-matches a changed file.
							survivedDelta := 0
							churnDelta := 0
							for _, anchor := range pathAnchors {
								for changedPath, stats := range c.Files {
									if strings.HasPrefix(changedPath, anchor) {
										if survivedDelta == 0 {
											survivedDelta = 1
										}
										churnDelta += stats[0] + stats[1]
									}
								}
							}
							if survivedDelta > 0 || churnDelta > 0 {
								if err := s.AddSignals(r.ID, survivedDelta, churnDelta); err != nil {
									rep.Errors = append(rep.Errors, FileError{r.File, err})
								}
							}
						}
					}
				}
			}
			// Advance last_scanned_commit even when zero matches, so the next
			// reindex doesn't re-scan the same commits.
			if err := s.SetMeta("last_scanned_commit", head); err != nil {
				rep.Errors = append(rep.Errors, FileError{"lore_meta", err})
			}
		}
	}

	return s, rep, nil
}
