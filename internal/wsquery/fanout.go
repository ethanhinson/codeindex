package wsquery

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeindex/internal/query"
)

// The fan-out verbs (§3.2) and enclosing's member routing (§2.3).
//
// find and grep run the EXISTING per-repo query against each present member in
// MANIFEST ORDER and CONCATENATE — complete sets, within-member order
// preserved, no rank-merge and no scoring. That is not a simplification to be
// improved on later: D4 froze it and re-ranking is a frozen non-goal.
//
// # Concatenation does not define the aggregates, so they are defined here
//
//   - FindAnswer.Total    = the SUM of the per-member totals.
//   - GrepAnswer.RawHits  = the SUM of the per-member raw-hit counts.
//   - GrepAnswer.Backend  = the single agreed backend verbatim, or the DISTINCT
//     backends joined by "+" in manifest order (see joinBackends).
//
// `limit` bounds the CONCATENATED list, on the same reasoning as §3.5's anchor
// verbs.
//
// # The per-member call takes the CALLER'S limit, not `unlimited`
//
// Unlike the anchor verbs' graph half, these two are retrieval lanes whose own
// *Total is itself limit-capped in repo mode: query.Find sets Total from the
// bounded result set, and query.Grep hands `limit` to search.Grep. Passing
// `unlimited` per member would therefore make a ONE-MEMBER workspace's Total
// differ from the single-repo answer's, which is D7's frozen §7.4
// non-regression bar. So "the sum of the per-member totals" means per-member
// limit THEN sum — the same convention navWorkspace's retrieval half already
// follows, for the same reason.

// grepMember is the per-member grep, behind a package variable ONLY so tests
// can drive the backend-DISAGREEMENT branch of joinBackends through the real
// grepWorkspace. Which backend search.Grep reports depends on whether ripgrep
// is on PATH, so a fixture cannot produce two different ones on one machine —
// and a same-backend fixture cannot catch a first-member fallback, which is
// the actual bug the join guards against. Production code never reassigns it;
// TestGrepMemberSeamDefaultsToQueryGrep fails if the default ever changes.
var grepMember = query.Grep

// joinBackends reduces the consulted members' backends to the single string
// GrepAnswer.Backend carries.
//
// When every member agrees the value is used VERBATIM — the overwhelmingly
// common case, and the one D7's single-member §7.4 bar runs through, where any
// decoration at all would break byte-identity. When they differ the value is
// the DISTINCT backends joined by "+" in MANIFEST ORDER (the input order),
// e.g. "ripgrep+sqlite".
//
// It is never silently the first member's: `GrepAnswer.Text()` prints the
// backend in its "(%s)" tail, so reporting one member's backend for an answer
// composed from several is a quiet lie about how the answer was obtained.
// Empty strings are skipped rather than joined into a leading "+", so a member
// that reported nothing cannot make the field blank on its own.
func joinBackends(backends []string) string {
	var distinct []string
	seen := map[string]bool{}
	for _, b := range backends {
		if b == "" || seen[b] {
			continue
		}
		seen[b] = true
		distinct = append(distinct, b)
	}
	return strings.Join(distinct, "+")
}

// findWorkspace fans find out across the present members and concatenates.
//
// # KNOWN LIMITATION: Total is the sum of the per-member CAPPED totals
//
// The per-member-limit convention above is forced, and so is its residual. Each
// member's query.Find sets Total = len(results) AFTER search.Find applied the
// limit, so Total here is Σ min(n_i, limit) — neither the corpus total nor the
// printed row count, since `limit` then bounds the CONCATENATION too. On a
// three-member workspace with limit 20 where every member overflows,
// FindAnswer.Text prints `find "X" (60)` above TWENTY rows, and FindAnswer has
// no "... (+N more)" line to reconcile the two.
//
// This is CHARACTERIZED, not fixed. Making Total honest would mean either
// per-member `unlimited` — which moves a one-member workspace's Total off the
// single-repo answer's and breaks §7.4's byte-equality bar — or recounting the
// rows, which contradicts §3.2's "sum of the per-member totals". §7.4 and §3.2
// can only both hold this way.
// TestBYDESIGNFindTotalIsTheSumOfPerMemberCappedTotals pins the arithmetic so a
// reader meets the semantics rather than the surprise.
func findWorkspace(s *session, q, kind, path string, limit int) (*query.FindAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()

	total := 0
	var results []query.FindRef
	for _, rm := range u.present {
		id := rm.Member.ID
		if u.store(id) == nil {
			continue
		}
		u.s.consult(id)
		inner, err := query.Find(rm.AbsRoot, q, kind, path, limit)
		if err != nil {
			return nil, err
		}
		total += inner.Total
		for _, r := range inner.Results {
			r.Repo = id
			r.File = u.wsPath(id, r.File)
			results = append(results, r)
		}
	}
	return &query.FindAnswer{
		Query: q,
		Total: total,
		Results: append(make([]query.FindRef, 0, len(results)),
			truncateOwned(s, results, limit, findOwner)...),
	}, nil
}

// grepWorkspace fans grep out across the present members and concatenates.
func grepWorkspace(s *session, pattern string, limit int, word bool) (*query.GrepAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()

	raw := 0
	var (
		groups   []query.GrepRef
		backends []string
	)
	for _, rm := range u.present {
		id := rm.Member.ID
		if u.store(id) == nil {
			continue
		}
		u.s.consult(id)
		inner, err := grepMember(rm.AbsRoot, pattern, limit, word)
		if err != nil {
			return nil, err
		}
		raw += inner.RawHits
		backends = append(backends, inner.Backend)
		for _, g := range inner.Groups {
			g.Repo = id
			g.File = u.wsPath(id, g.File)
			groups = append(groups, g)
		}
	}
	return &query.GrepAnswer{
		Pattern: pattern,
		Word:    word,
		RawHits: raw,
		Backend: joinBackends(backends),
		Groups: append(make([]query.GrepRef, 0, len(groups)),
			truncateOwned(s, groups, limit, grepOwner)...),
	}, nil
}

// enclosingWorkspace routes an enclosing query to the ONE member that owns the
// file (§2.3). enclosing is purely intra-file, so there is nothing to union.
//
// # The match is over RESOLVED ABSOLUTE roots, and that is mandatory
//
// On a workspace root the file argument is workspace-relative. It is resolved
// to an absolute path and the member is selected by LONGEST-PREFIX match over
// each present member's AbsRoot — exactly what config.Workspace.Resolve already
// computes, which is why it is read from there rather than recomputed.
//
// Matching the DECLARED RELATIVE roots instead would be wrong, not merely
// uglier: D1 explicitly sanctions member roots that climb OUT of the workspace
// ("../api"), which is the layout D1 calls the sanctioned shape for scattered
// repos gathered under a dedicated workspace directory. A prefix test against
// the string "../api" fails to match the file paths that actually live there,
// or matches the wrong member. TestEnclosingRoutesToAMemberRootThatClimbsOut
// is what stops a relative-matching rewrite.
//
// Longest prefix, not first prefix, because nested member roots are legal and
// the innermost one owns the file.
func enclosingWorkspace(s *session, file string, start, end int) (*query.EnclosingAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()

	abs := filepath.Join(s.root, file)
	if !filepath.IsAbs(abs) {
		a, aerr := filepath.Abs(abs)
		if aerr != nil {
			return nil, aerr
		}
		abs = a
	}

	best := -1
	for i, rm := range u.present {
		root := u.abs[rm.Member.ID]
		if !underRoot(root, abs) {
			continue
		}
		if best < 0 || len(root) > len(u.abs[u.present[best].Member.ID]) {
			best = i
		}
	}
	if best < 0 {
		return nil, noMemberForFileError(u, file)
	}

	owner := u.present[best]
	id := owner.Member.ID
	rel, err := filepath.Rel(owner.AbsRoot, abs)
	if err != nil {
		return nil, err
	}
	u.s.consult(id)
	inner, err := query.Enclosing(owner.AbsRoot, filepath.ToSlash(rel), start, end)
	if err != nil {
		return nil, err
	}
	a := &query.EnclosingAnswer{
		File:    u.wsPath(id, filepath.ToSlash(rel)),
		Start:   inner.Start,
		End:     inner.End,
		Symbols: make([]query.EnclosingRef, 0, len(inner.Symbols)),
	}
	for _, e := range inner.Symbols {
		e.Repo = id
		e.File = u.wsPath(id, e.File)
		a.Symbols = append(a.Symbols, e)
	}
	return a, nil
}

// underRoot reports whether abs sits inside root, comparing whole path
// components. A raw strings.HasPrefix would put /ws/services/apidocs under
// /ws/services/api.
func underRoot(root, abs string) bool {
	root = filepath.Clean(root)
	abs = filepath.Clean(abs)
	return abs == root || strings.HasPrefix(abs, root+string(filepath.Separator))
}

// noMemberForFileError names the file and lists the member roots, so the
// answer to "why did this not route" is in the message rather than in a second
// command. Both forms are listed: the declared root is what the user would
// edit, the resolved one is what the match actually ran against, and for a
// "../api" member those two look nothing alike.
func noMemberForFileError(u *unionCtx, file string) error {
	roots := make([]string, 0, len(u.present))
	for _, rm := range u.present {
		roots = append(roots, fmt.Sprintf("%s (%s -> %s)", rm.Member.ID, rm.Member.Root, rm.AbsRoot))
	}
	return fmt.Errorf("codeindex enclosing: %s is not under any workspace member root\nmember roots: %s",
		file, strings.Join(roots, ", "))
}
