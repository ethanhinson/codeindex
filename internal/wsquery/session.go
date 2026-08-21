package wsquery

import (
	"sort"

	"codeindex/internal/config"
	"codeindex/internal/wsfresh"
)

// freshenWorkspace is the whole-workspace freshen pass, behind a package
// variable ONLY so tests can inject the failure of §4.2 — the degrade path is
// the one this slice's D7 bar is defended on, and it cannot be provoked
// reliably from a fixture. Production code never reassigns it, and
// TestFreshenSeamDefaultsToWsfreshFreshen fails if the default is ever
// something other than wsfresh.Freshen.
var freshenWorkspace = wsfresh.Freshen

// session is one workspace query's freshness context: the manifest, the result
// of the freshen pass that ran before anything was read, and the record of
// which members this answer actually consulted. Every RootWorkspace entry
// point opens exactly one, and the coverage clause is built from it at the end.
type session struct {
	// root is the workspace root.
	root string
	// ws is the manifest, loaded EXPLICITLY before the freshen (see newSession).
	ws *config.Workspace
	// report is the freshen pass's report. On the degrade path it is partial
	// and is NOT read as authoritative — see staleMembers.
	report wsfresh.Report
	// freshenErr is Freshen's error, or nil. Non-nil means the query is
	// proceeding against the overlay as it stands (§4.2).
	freshenErr error
	// consulted records member ids whose graph.db was read TO COMPOSE THE
	// ANSWER. The freshen pass is deliberately not recorded here: it opens
	// every present member, so recording it would make the field constant.
	consulted map[string]bool
	// keysUnmapped counts references dropped because their stable key
	// inverted to no symbol (§3.8). Incremented at the re-map site by
	// remapKey, so a drop and its disclosure cannot come apart.
	keysUnmapped int
}

// newSession loads the manifest, then runs the whole-workspace freshen, then
// returns the context the answer is composed against. It is the first thing
// every RootWorkspace entry point does, before reading anything.
//
// # The manifest is loaded EXPLICITLY, and first
//
// An absent, unparseable or invalid manifest comes back from here as an ERROR:
// it is a configuration fault, not a staleness condition, and a query cannot
// degrade past it because there is no declared member set to degrade against.
//
// This ordering is mandatory rather than an optimization. wsfresh.Freshen also
// loads the manifest, but ALL TEN of its error returns are opaque fmt.Errorf
// wraps with no sentinel and no error type, and engine.DetectRootKind only
// STATS the manifest without parsing it. So nothing downstream of Freshen can
// tell a manifest fault from a member fold failure. Loading here first is the
// only thing that makes the exception discriminable — remove it and every
// broken manifest silently becomes a degraded answer that names every member
// stale, which is precisely the quiet-wrongness posture §4.2 exists to avoid.
//
// # The freshen is WHOLE-WORKSPACE
//
// Not a subset of the members this query will consult, and ADR-0012 is NOT the
// argument for that (it governs re-resolution scope only). The argument is:
// Freshen's signature takes no member subset, so a subset freshen would be a
// SECOND freshen implementation and a second enforcement site for the gate; a
// member's cleanliness is unknowable without folding it, so "freshen only what
// we consult" cannot skip the expensive half once anything is dirty; and the
// fan-out verbs reach the whole manifest by definition anyway.
//
// # A freshen error degrades, it does not fail the query
//
// A workspace that refuses to answer while one member's index is briefly
// unopenable is strictly worse than one that answers and discloses what it
// could not verify. The disclosure is what makes that trade honest, and it is
// carried by the clause: freshen_failed plus EVERY declared member named stale.
func newSession(wsRoot string) (*session, error) {
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		return nil, err
	}
	rep, ferr := freshenWorkspace(wsRoot)
	return sessionFrom(wsRoot, ws, rep, ferr), nil
}

// sessionFrom builds a session from an already-obtained manifest and report.
// It is the seam the union tests drive: the Dirty/Resolved windows of §4.3 are
// states of a Report, and asserting on them directly is more honest than
// contriving a fixture that happens to produce one.
func sessionFrom(root string, ws *config.Workspace, rep wsfresh.Report, freshenErr error) *session {
	return &session{
		root:       root,
		ws:         ws,
		report:     rep,
		freshenErr: freshenErr,
		consulted:  make(map[string]bool, len(ws.Members)),
	}
}

// consult records that a member's graph.db was read to compose the answer.
// Callers record as they go — at the point of the read, not by reconstructing
// the set afterwards, which would be a second derivation of the same fact.
//
// A member that was consulted and contributed NOTHING is still recorded: the
// clause answers "what did you look at", not "what did you find".
func (s *session) consult(ids ...string) {
	for _, id := range ids {
		if id == "" {
			continue
		}
		s.consulted[id] = true
	}
}

// clause builds verb's coverage clause. It is called once, after the answer is
// composed, so members_consulted is complete.
func (s *session) clause(verb string) Clause {
	c := Clause{
		MembersConsulted: s.manifestOrder(s.consulted),
		MembersStale:     s.staleMembers(),
		Boundary:         Boundary,
		Layer:            ClauseLayer(verb),
		KeysUnmapped:     s.keysUnmapped,
	}
	if s.freshenErr != nil {
		c.FreshenFailed = s.freshenErr.Error()
	}
	return c
}

// staleMembers is the id set the clause names stale.
//
// # A failed freshen names EVERY DECLARED MEMBER
//
// Not the members the partial Report happens to list. wsfresh.Freshen returns
// a partially-filled report on all of its error paths, and freshen.go's own
// comment at its step 5d says the mid-flight fold error is DELIBERATELY FATAL
// because a racing rebuild "would drop the member out of Dirty, leave its
// stale stamp unread, and let the gate return a quietly WRONG clean verdict."
// Reading that same partial report as authoritative here would reproduce that
// quiet wrong verdict at the query surface — which is the silent staleness the
// D7 gate hard-fails. An over-broad stale list on a rare path is the price,
// and it buys the guarantee.
//
// # Otherwise, a FIVE-WAY SET UNION
//
//  1. Dirty                   — stamp absent or moved from the re-folded root
//  2. StaleStamped            — declared-but-unavailable and still stamped
//  3. MembersMissing          — declared, absent from disk
//  4. MembersFreshenFailedIDs — available, but the member's own Fresh errored
//  5. MembersUnindexedIDs     — declared and present, but the index will not open
//
// Dirty is dropped ONLY WHEN Resolved is true: the whole-pass resolution that
// just ran retired those members' staleness, so naming them stale would be a
// false alarm — and a workspace that always looks stale teaches the agent to
// ignore the field entirely.
//
// DIRTY MUST STAY IN THE UNION. The Dirty-nonempty-with-Resolved-false window
// is REAL: Freshen appends to rep.Dirty at ~freshen.go:255 and sets
// rep.Resolved only at ~:375, and its error returns at ~:292, ~:318 and ~:373
// sit BETWEEN them — precisely the paths the degrade posture above proceeds
// through. An earlier draft of the design claimed the state "cannot occur";
// that claim is WITHDRAWN AS FALSE, and
// TestDirtyStaysInTheStaleUnionWhenResolvedIsFalse is what stops it coming
// back. Do not drop Dirty from the union.
//
// # §4.3's FOUR-WAY phrasing is SUPERSEDED — do not restore it
//
// The spec (§4.3, assumption 6) writes this union as four sets and excludes
// MembersUnindexed, on the stated ground that a present-but-unopenable member
// is "covered by StaleStamped when it previously contributed rows, and by
// boundary when it never did". BOTH HALVES OF THAT COVER ARGUMENT ARE FALSE,
// which is why the letter of §4.3 is not followed here:
//
//   - StaleStamped is ONE-SHOT. Its own doc comment says it is non-empty for at
//     most ONE pass per transition: the pass that reports it trips the gate, and
//     the wsresolve.Resolve that follows prunes the very stamp it fired on. From
//     the next query onward the member is not Dirty (nothing re-folded it), not
//     StaleStamped (no stamp left) and not MembersMissing (still on disk).
//   - Boundary is a FIXED CONSTANT — "symbols outside this workspace are unknown
//     to it" — which says nothing whatever about a DECLARED member INSIDE the
//     workspace whose rows this answer omitted.
//
// So under the four-way rule such a member produced `members_stale: (none)`
// with its rows silently missing, from the second query onward and forever.
// That is the D7 hard fail the gate refuses, and it is not an exotic state: a
// freshly cloned, never-built member reaches it on the VERY FIRST query.
//
// The double-count §4.3 worried about is handled by SET-UNION semantics, not by
// exclusion: these are five id sets accumulated into one map, so a member that
// is both stale-stamped and unindexed is named exactly once. The union reads
// MembersUnindexedIDs and never the MembersUnindexed COUNT — a count names
// nobody, and inventing ids for it is how a name and a denominator come apart.
//
// DIRTY MUST STAY, and so must this fifth set. A reader tempted to restore
// §4.3's letter should note that
// TestD7FreshnessPropertyUnindexedMemberIsNamedStaleOnEveryPass queries TWICE
// on purpose: a single-pass test cannot catch a one-shot disclosure, which is
// exactly how the four-way rule survived review once already.
func (s *session) staleMembers() []string {
	if s.freshenErr != nil {
		return s.declaredIDs()
	}
	stale := make(map[string]bool)
	if !s.report.Resolved {
		for _, id := range s.report.Dirty {
			stale[id] = true
		}
	}
	for _, id := range s.report.StaleStamped {
		stale[id] = true
	}
	for _, id := range s.report.MembersMissing {
		stale[id] = true
	}
	for _, id := range s.report.MembersFreshenFailedIDs {
		stale[id] = true
	}
	for _, id := range s.report.MembersUnindexedIDs {
		stale[id] = true
	}
	return s.manifestOrder(stale)
}

// manifestOrder projects an id set onto manifest order. Ids the manifest does
// not declare cannot normally appear — every set the clause draws from is
// declared-scoped — but if one ever does it is appended, sorted, rather than
// dropped: silently losing a stale id is the one failure mode this whole
// clause exists to prevent.
func (s *session) manifestOrder(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	seen := make(map[string]bool, len(set))
	for _, m := range s.ws.Members {
		if set[m.ID] {
			out = append(out, m.ID)
			seen[m.ID] = true
		}
	}
	var undeclared []string
	for id := range set {
		if !seen[id] {
			undeclared = append(undeclared, id)
		}
	}
	sort.Strings(undeclared)
	return append(out, undeclared...)
}

// declaredIDs is every member the manifest names, in manifest order.
func (s *session) declaredIDs() []string {
	out := make([]string, 0, len(s.ws.Members))
	for _, m := range s.ws.Members {
		out = append(out, m.ID)
	}
	return out
}
