package wsquery

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codeindex/internal/config"
	"codeindex/internal/graph"
	"codeindex/internal/overlay"
	"codeindex/internal/query"
)

// The anchor union verbs (§3.1, §3.3-§3.5).
//
//	callers(X)  = X's own member's callers  ∪  overlay.InEdges(X's key)
//	callees(X)  = X's own member's callees  ∪  overlay.OutEdges(X's key)
//	dependents  = own ∪ InEdges  restricted to import/extends/implements
//	deps        = own ∪ OutEdges restricted to extends/implements
//	impact      = definitions + callers + callees + dependents, AT DEPTH 1
//	nav         = the callers union, plus retrieval components that fan out
//
// dependents is a union verb by FORCE, not by taste: ImpactAnswer embeds
// DependentsTotal/Dependents and impact crosses member boundaries by owner
// ruling, so a per-repo dependents would make `codeindex dependents` and the
// dependents block of `codeindex impact` report different numbers for the same
// anchor from the same root — one invariant, two sites, guaranteed drift.
// TestDependentsAndImpactBlockAgree reads the two sites against each other.
// deps is unioned for directional symmetry with dependents (assumption 1a).
//
// # impact is DEPTH-1 and stays there
//
// query.Impact composes definitions + callers + callees + dependents at depth
// 1; it is not a transitive closure today. "Crosses member boundaries by
// default" means the depth-1 neighbourhood INCLUDES CROSS-EDGES WITH NO FLAG.
// It does not authorize a closure the single-repo path lacks — building one
// would silently change the answer shape and blow the non-regression bar for a
// capability nobody asked for. TestImpactStaysDepthOne pins it.
//
// # limit bounds the UNIONED list; *Total counts the UNIONED set
//
// Every own-half read below passes `unlimited` to internal/query and the
// caller's limit is applied ONCE, to the concatenated union. Applying the limit
// per member and then concatenating makes `CallersTotal - len(Callers)` a lie
// in both directions, and the renderers print that difference literally
// ("  ... (+%d more; raise limit)"). It is explicitly NOT claimed that a
// truncated workspace head equals the single-repo head: §3.6's suppression
// filter legitimately removes own-member intra-repo edges.

// unlimited is the limit handed to the per-repo query when reading a member's
// own half. See the *Total note above: the per-member call must return the
// COMPLETE set or the union's arithmetic is computed over a truncated input.
const unlimited = math.MaxInt

// impactCoverage is internal/query's impactCoverage sentence, which is
// unexported there and which the workspace answer must carry verbatim before
// the coverage clause is appended to it (§4.5).
//
// It is duplicated rather than exported from internal/query because the ground
// rule for this slice is that internal/query is edited only for the additive
// reference fields and the conditional renderer branches — widening its API for
// a display string is not on that list. The duplication is a drift site, so it
// is guarded: TestImpactCoverageSentenceMatchesRepoMode compares this constant
// against a real repo-mode answer and fails the moment the two diverge.
const impactCoverage = "call + import/extends/implements edges; type-usage references not included"

// depKinds are the edge kinds `dependents` counts, mirroring
// (*graph.Store).Dependents' own WHERE clause. Keeping the workspace cross half
// on the same kind set as the repo half is what makes the union a union rather
// than two differently-shaped answers glued together.
var depKinds = map[string]bool{"imports": true, "extends": true, "implements": true}

// symbolDepKinds are the kinds (*graph.Store).SymbolDeps reads — `deps`' own
// half — and therefore the kinds its cross half must read too.
var symbolDepKinds = map[string]bool{"extends": true, "implements": true}

// callKinds is the single kind `callees` reads, mirroring
// (*graph.Store).Callees' `e.kind = 'calls'`.
var callKinds = map[string]bool{"calls": true}

// unionCtx is one workspace answer's read context: the overlay, the present
// members, and the member stores opened along the way. One is opened per entry
// point and closed when the answer is composed.
type unionCtx struct {
	s       *session
	ov      *overlay.Store
	present []config.ResolvedMember
	// idx maps a member id to its MANIFEST index — the sort key that projects
	// the overlay's alphabetical src_member ordering back onto manifest order.
	idx map[string]int
	// root maps a member id to its declared (workspace-relative) root, for
	// rewriting member-relative paths onto the workspace.
	root map[string]string
	// abs maps a member id to its resolved absolute root.
	abs map[string]string
	// stores memoizes graph.OpenExisting per member. A nil value is a member
	// whose index would not open; see store().
	stores map[string]*graph.Store
	// sup is overlay.Suppressions(), read once. Almost always empty.
	sup []overlay.Suppression
	// dropped memoizes the §3.6 suppression filter's dropped set per member.
	dropped map[string][]graph.TierOneEdge
	// fileOwner maps a workspace-relative path in a FILE LIST back to the
	// member that contributed it. The file lists are []string, so unlike every
	// other unioned list they carry no Repo field of their own — and without an
	// owner the truncation disclosure could count their withheld rows but never
	// name a member. Populated wherever a file list is built.
	fileOwner map[string]string
}

// openUnion opens the read context, VERSION-GATING the overlay first.
//
// # Why the gate is here and not left to overlay.Open
//
// overlay.Open MUTATES: on a schema-version mismatch it DELETES the file and
// recreates it empty. WorkspaceStatus already refuses to reach that path for
// exactly that reason (see status.go's file comment), and a read-only QUERY has
// the same obligation — more so on §4.2's degrade path, where Freshen errored
// possibly before touching the overlay and the query is supposed to proceed
// "against the overlay as it stands". Rebuilding it here would destroy the very
// state the degraded answer is being read from.
//
// So a mismatched overlay is read as EMPTY (ov == nil; every cross-edge read
// below yields nothing) AND DISCLOSED through the coverage clause. The
// disclosure is not optional: a silently empty overlay is an answer missing
// every cross-member edge with nothing saying so, which is the silent staleness
// the D7 gate hard-fails.
//
// An ABSENT overlay is left to overlay.Open, which creates it empty. That
// create is not a rebuild — there is no recorded state to destroy — and it is
// the state a workspace sits in before its first refresh.
func openUnion(s *session) (*unionCtx, error) {
	present, _, err := s.ws.Resolve(s.root)
	if err != nil {
		return nil, err
	}
	ovPath := overlay.Path(s.root)
	var (
		ov  *overlay.Store
		sup []overlay.Suppression
	)
	switch ver, verr := overlay.FileSchemaVersion(ovPath); {
	case verr != nil && !os.IsNotExist(verr):
		return nil, verr
	case verr == nil && ver != overlay.SchemaVersion():
		s.overlaySchemaSkew = fmt.Sprintf(
			"overlay schema v%d, this binary uses v%d — cross-member edges unavailable until the next refresh rebuilds it",
			ver, overlay.SchemaVersion())
	default:
		ov, err = overlay.Open(ovPath)
		if err != nil {
			return nil, err
		}
		sup, err = ov.Suppressions()
		if err != nil {
			ov.Close()
			return nil, err
		}
	}
	u := &unionCtx{
		s: s, ov: ov, present: present,
		idx:       make(map[string]int, len(s.ws.Members)),
		root:      make(map[string]string, len(s.ws.Members)),
		abs:       make(map[string]string, len(present)),
		stores:    make(map[string]*graph.Store, len(present)),
		sup:       sup,
		dropped:   make(map[string][]graph.TierOneEdge, len(present)),
		fileOwner: make(map[string]string),
	}
	for i, m := range s.ws.Members {
		u.idx[m.ID] = i
		u.root[m.ID] = m.Root
	}
	for _, rm := range present {
		u.abs[rm.Member.ID] = rm.AbsRoot
	}
	return u, nil
}

func (u *unionCtx) close() {
	for _, st := range u.stores {
		if st != nil {
			st.Close()
		}
	}
	if u.ov != nil {
		u.ov.Close()
	}
}

// store opens a member's index read-only, memoized.
//
// An index that will not open yields nil rather than an error, and the member
// simply contributes nothing. That is deliberate and matches wsfresh's
// availability predicate: a workspace that refuses to answer because one
// member's index is briefly unopenable is strictly worse than one that answers
// and discloses. The disclosure is the coverage clause's job (§4.2/§4.3), not
// this function's.
func (u *unionCtx) store(id string) *graph.Store {
	if st, ok := u.stores[id]; ok {
		return st
	}
	abs, ok := u.abs[id]
	if !ok {
		u.stores[id] = nil
		return nil
	}
	st, err := graph.OpenExisting(filepath.Join(abs, ".codeindex", "graph.db"))
	if err != nil {
		u.stores[id] = nil
		return nil
	}
	u.stores[id] = st
	return st
}

// wsPath rewrites a member-relative path onto the workspace (§5). A member root
// of "." leaves the path unchanged, which is what lets a single-member
// workspace meet D7's frozen "modulo the repo field" bar; a root that climbs
// out of the workspace ("../api") produces a path that climbs out too, which is
// still the workspace-relative form of that file.
func (u *unionCtx) wsPath(memberID, p string) string {
	if p == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join(u.root[memberID], p))
}

// memberRelPath is wsPath's inverse for a FILE ANCHOR: it maps a
// workspace-relative path back onto the member that contains it. It reports
// false when the path does not sit under that member's root.
func memberRelPath(memberRoot, wsPath string) (string, bool) {
	rr := filepath.ToSlash(filepath.Clean(memberRoot))
	p := filepath.ToSlash(filepath.Clean(wsPath))
	if rr == "." {
		return p, true
	}
	if p == rr {
		return ".", true
	}
	if strings.HasPrefix(p, rr+"/") {
		return p[len(rr)+1:], true
	}
	return "", false
}

// anchorMember is one member the anchor resolves into: the answer's OWN half is
// read from each of these, and their definitions supply the stable keys the
// cross half is looked up by.
type anchorMember struct {
	id      string
	absRoot string
	// defs are the anchor's definitions in this member, in the per-repo query's
	// own order.
	defs []graph.Symbol
	// file is the member-relative path when the anchor names a FILE rather than
	// a symbol (the `deps` file-anchor form). Empty otherwise.
	file string
}

// keys is the anchor's stable keys in this member — the dst key of an inbound
// cross-edge lookup and the src key of an outbound one.
func (am anchorMember) keys() []overlay.SymKey {
	out := make([]overlay.SymKey, 0, len(am.defs))
	for _, d := range am.defs {
		out = append(out, overlay.SymKey{Member: am.id, File: d.File, QName: d.QName()})
	}
	return out
}

// resolvedAnchor is one parsed anchor: the text the per-repo query sees, its
// (name, parent) split, and the members it resolves into.
type resolvedAnchor struct {
	// text is the anchor with any member prefix removed — what internal/query
	// is called with, so its own SplitAnchor sees exactly what it would in
	// repo mode.
	text string
	name string
	// parent is SplitAnchor's parent half.
	parent  string
	members []anchorMember
}

// resolveAnchor parses the anchor and finds the members it resolves into.
//
// A member prefix scopes the lookup to that member (§3.4). A BARE anchor
// matching in several members yields several anchorMembers, whose definitions
// concatenate into the same multi-candidate answer the single-repo path already
// returns for a duplicate name — no new answer shape.
//
// When NO member defines the anchor, every candidate member becomes an
// anchorMember with no definitions. That is not a guess: the per-repo readers
// are NAME-based (Callers matches `e.dst_name = ?`, Dependents likewise), so a
// name with no definition still has a well-defined per-repo answer, and
// scoping the union to definition owners would make the workspace answer for an
// undefined name empty where the single-repo answer is not — a non-regression
// break on D7's single-member bar.
func (u *unionCtx) resolveAnchor(anchor string, fileAnchor bool) (resolvedAnchor, error) {
	memberID, rest, err := u.s.splitMemberPrefix(anchor)
	if err != nil {
		return resolvedAnchor{}, err
	}
	ra := resolvedAnchor{text: rest}
	ra.name, ra.parent = query.SplitAnchor(rest)

	cands := u.present
	if memberID != "" {
		cands = nil
		for _, rm := range u.present {
			if rm.Member.ID == memberID {
				cands = append(cands, rm)
			}
		}
	}
	for _, rm := range cands {
		id := rm.Member.ID
		st := u.store(id)
		if st == nil {
			continue
		}
		if fileAnchor {
			if rel, ok := memberRelPath(rm.Member.Root, rest); ok {
				has, err := st.HasFile(rel)
				if err != nil {
					return resolvedAnchor{}, err
				}
				if has {
					ra.members = append(ra.members, anchorMember{id: id, absRoot: rm.AbsRoot, file: rel})
					continue
				}
			}
		}
		defs, err := st.Definitions(ra.name, ra.parent)
		if err != nil {
			return resolvedAnchor{}, err
		}
		if len(defs) > 0 {
			ra.members = append(ra.members, anchorMember{id: id, absRoot: rm.AbsRoot, defs: defs})
		}
	}
	if len(ra.members) == 0 {
		for _, rm := range cands {
			if u.store(rm.Member.ID) == nil {
				continue
			}
			ra.members = append(ra.members, anchorMember{id: rm.Member.ID, absRoot: rm.AbsRoot})
		}
	}
	return ra, nil
}

// keys is every anchor definition's stable key, gathered in manifest order of
// the owning member and then in definition order.
func (ra resolvedAnchor) keys() []overlay.SymKey {
	var out []overlay.SymKey
	for _, am := range ra.members {
		out = append(out, am.keys()...)
	}
	return out
}

// crossRef is one cross-member reference, resolved.
type crossRef struct {
	// member is the COUNTERPART member — the source for an inbound lookup, the
	// destination for an outbound one.
	member string
	// idx is member's manifest index: the union's sort key.
	idx  int
	edge overlay.CrossEdge
	// key is the counterpart's stable key. It speaks for the row ONLY when the
	// re-map produced no symbol to speak for it — see resolved.
	key   overlay.SymKey
	sym   graph.Symbol
	flags Flags
	// ambiguous marks a key that inverted to several symbols. It is rendered
	// with the answer surface's existing Ambiguous flag rather than resolved by
	// picking a row (§3.8, assumption 19).
	//
	// An ambiguous re-map is EXPANDED at the gather site: cross emits ONE
	// crossRef per candidate, each carrying that candidate in sym and
	// ambiguous set. That is §3.8's "rendered as ambiguous WITH ITS
	// CANDIDATES" — the reader gets every location the key could mean, each
	// flagged, and no single row is presented as the answer. Emitting one row
	// bearing key.File instead would print the path recorded at WRITE time,
	// which is precisely the datum the stable key exists because it may have
	// MOVED, at DefLine 0.
	ambiguous bool
	// resolved is true when sym holds a symbol read from the counterpart
	// member's own database — an exact re-map, or one candidate of an
	// ambiguous one. It is false only in the degenerate ambiguous-with-no-
	// candidates case, where the recorded key is all there is.
	resolved bool
}

func (c crossRef) qname() string {
	if !c.resolved {
		// A FILE-SCOPE key — empty QName — is a reference with no enclosing
		// symbol, and the file is its name. That is not this package's
		// invention: graph.Dependent.QName does exactly this for the own half's
		// module-scope import rows, so a dependents answer that unions the two
		// halves states one convention rather than two. The path is the
		// member-relative one the own half uses as well.
		if c.key.QName == "" {
			return c.key.File
		}
		return c.key.QName
	}
	return c.sym.QName()
}

func (c crossRef) file() string {
	if !c.resolved {
		return c.key.File
	}
	return c.sym.File
}

func (c crossRef) name() string {
	if !c.resolved {
		_, n := splitQName(c.key.QName)
		return n
	}
	return c.sym.Name
}

func (c crossRef) parent() string {
	if !c.resolved {
		p, _ := splitQName(c.key.QName)
		return p
	}
	return c.sym.Parent
}

func (c crossRef) defLine() int {
	if !c.resolved {
		return 0
	}
	return c.sym.StartLine
}

// cross reads the overlay's cross-edges at keys and resolves each counterpart
// key back to a symbol in its own member's database.
//
// inbound selects InEdges (the counterpart is the edge's source) over OutEdges
// (the counterpart is its destination). kinds, when non-nil, restricts the edge
// kinds to the ones the verb's own half reads.
//
// # Ordering
//
// The result is the cross rows in MANIFEST order of the counterpart member. The
// sort is STABLE over the gather order, which is itself total — keys in
// manifest-then-definition order, and In/OutEdges' own total order within each
// key — so rows that tie on the member index fall back to it rather than to
// SQLite. That tie is exercised: the fixture gives one member three rows.
func (u *unionCtx) cross(keys []overlay.SymKey, inbound bool, kinds map[string]bool) ([]crossRef, error) {
	var out []crossRef
	if u.ov == nil {
		// A schema-mismatched overlay, read as empty and disclosed by the
		// clause. See openUnion.
		return nil, nil
	}
	for _, k := range keys {
		var (
			edges []overlay.CrossEdge
			err   error
		)
		if inbound {
			edges, err = u.ov.InEdges(k)
		} else {
			edges, err = u.ov.OutEdges(k)
		}
		if err != nil {
			return nil, err
		}
		for _, e := range edges {
			if kinds != nil && !kinds[e.Kind] {
				continue
			}
			// An unrecognized confidence is PROPAGATED, never swallowed:
			// defaulting would present an unknown rung as the strongest one
			// there is, which is the lie D3 forbids arrived at by omission.
			flags, err := ReferenceFlags(e)
			if err != nil {
				return nil, err
			}
			ck := e.Dst
			if inbound {
				ck = e.Src
			}
			st := u.store(ck.Member)
			if st == nil {
				// The counterpart member's index will not open, so its key
				// cannot be inverted. NOT counted as keys_unmapped: that
				// counter means "the key inverted to no symbol", and an
				// unopened database inverted nothing at all.
				continue
			}
			u.s.consult(ck.Member)
			if ck.QName == "" {
				// A FILE-SCOPE counterpart: the reference sits at module scope
				// (an import), inside no symbol, and the ladder recorded it as
				// {member, file, ""}. There is nothing to invert — the key is
				// already complete — so it is NOT run through the re-map and
				// NOT counted as an unmapped key. Doing so would ask
				// ProjectDefs for a symbol named "", find none, and drop every
				// cross-repo import the ladder resolved into keys_unmapped.
				out = append(out, crossRef{
					member: ck.Member, idx: u.idx[ck.Member], edge: e, key: ck, flags: flags})
				continue
			}
			rm, err := u.s.remapKey(st, ck)
			if err != nil {
				return nil, err
			}
			ref := crossRef{member: ck.Member, idx: u.idx[ck.Member], edge: e, key: ck, flags: flags}
			switch rm.Status {
			case RemapUnmapped:
				// Dropped and counted by remapKey into keys_unmapped.
				continue
			case RemapExact:
				ref.sym, ref.resolved = rm.Symbol, true
				out = append(out, ref)
			default:
				ref.ambiguous = true
				ref.flags.Ambiguous = true
				if len(rm.Candidates) == 0 {
					// Cannot happen through RemapKey, which only reports
					// ambiguous over a non-empty set — kept so a future
					// producer cannot make the reference vanish silently.
					out = append(out, ref)
					break
				}
				for _, cand := range rm.Candidates {
					cr := ref
					cr.sym, cr.resolved = cand, true
					out = append(out, cr)
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].idx < out[j].idx })
	return out, nil
}

// truncate applies the caller's limit to an already-unioned list. A limit of
// zero or less yields an empty list, matching internal/query's `if i >= limit`
// loops exactly.
func truncate[T any](xs []T, limit int) []T {
	if limit < 0 {
		limit = 0
	}
	if len(xs) > limit {
		return xs[:limit]
	}
	return xs
}

// truncateOwned is truncate plus the disclosure D6's clause owes the reader: it
// records how many rows the cut discarded and WHICH MEMBERS those rows belonged
// to, so a manifest-order concatenation cannot silently answer for one member
// while omitting eight others (see Clause.RowsWithheld).
//
// owner maps a row to its member id. A row whose owner is unknown ("") still
// counts toward rows_withheld — the count must stay reconcilable with the list
// — but names nobody, because inventing an id is how a name and a denominator
// come apart.
//
// Every workspace list bounded by `limit` goes through THIS, not through
// truncate: a site that keeps calling the bare helper is a silent cut again,
// and this is the whole defect.
func truncateOwned[T any](s *session, xs []T, limit int, owner func(T) string) []T {
	kept := truncate(xs, limit)
	for _, x := range xs[len(kept):] {
		s.rowsWithheld++
		if id := owner(x); id != "" {
			if s.membersTruncated == nil {
				s.membersTruncated = map[string]bool{}
			}
			s.membersTruncated[id] = true
		}
	}
	return kept
}

// --- the suppression filter's own half (§3.6) --------------------------------
//
// The filter removes the OWN-member intra-repo edge that a cross-edge at the
// same call site already speaks for. It never touches a cross row — a cross row
// is the thing being deduplicated against.

// droppedEdges is memberID's tier-1 edges that the §3.6 filter removes.
//
// It short-circuits to nothing when no suppression names memberID as a
// consumer, which is the overwhelmingly common case and keeps two extra reads
// off every query.
func (u *unionCtx) droppedEdges(memberID string) ([]graph.TierOneEdge, error) {
	if got, ok := u.dropped[memberID]; ok {
		return got, nil
	}
	if u.ov == nil {
		// No readable overlay, so no suppressions and no cross-edges to
		// deduplicate an own-member edge against. Implied by u.sup being empty
		// as well; stated so the MemberEdges read below cannot be reached by a
		// later edit that fills u.sup from somewhere else.
		u.dropped[memberID] = nil
		return nil, nil
	}
	consumer := false
	for _, s := range u.sup {
		if s.ConsumerMember == memberID {
			consumer = true
			break
		}
	}
	if !consumer {
		u.dropped[memberID] = nil
		return nil, nil
	}
	st := u.store(memberID)
	if st == nil {
		u.dropped[memberID] = nil
		return nil, nil
	}
	tierOne, err := st.TierOneEdges()
	if err != nil {
		return nil, err
	}
	cross, err := u.ov.MemberEdges(memberID)
	if err != nil {
		return nil, err
	}
	_, dropped := FilterSuppressedEdges(memberID, u.sup, tierOne, cross)
	u.dropped[memberID] = dropped
	return dropped, nil
}

// inKey identifies an own-half row on the INCOMING side of the anchor: a
// callers or dependents row, keyed by the calling/importing end.
//
// Kind is empty for callers rows, because (*graph.Store).Callers matches on
// dst_name alone and its Caller struct carries no kind. Dependents rows do
// carry one, so their index includes it and is correspondingly narrower.
type inKey struct {
	Kind  string
	File  string
	QName string
	Line  int
}

// outKey identifies an own-half row on the OUTGOING side of the anchor: a
// callees or deps row, keyed by the target end.
type outKey struct {
	Kind    string
	DstName string
	Line    int
}

// dropSet is a MULTISET of rows to drop, not a set. Two edges can share a key
// (the same name called twice on one line, differing only in a column the
// answer row does not carry), and set semantics would drop both when the filter
// removed one — over-dropping a still-correct edge, which is the failure §3.6's
// narrowing exists to prevent in the other direction.
type dropSet[K comparable] map[K]int

// take reports whether a row keyed k should be dropped, consuming one drop.
func (d dropSet[K]) take(k K) bool {
	if d[k] > 0 {
		d[k]--
		return true
	}
	return false
}

// edgeQName is a tier-1 edge's source qualified name AS THE OWN-HALF ANSWER
// ROWS SPELL IT: internal/query renders a module-scope row (no enclosing
// symbol) with the file as its qualified name — graph.Dependent.QName — so a
// drop key built without that fallback would never match the row it is meant to
// remove, and the §3.6 filter would leave a double-counted import behind.
//
// It therefore does NOT match SourceCallSite, which keys the same edge for the
// join against the OVERLAY, where the stored src_qname of a module-scope source
// is the empty string. Two joins, two counterparties, two spellings — and the
// difference is exactly the module-scope case. Do not collapse them.
func edgeQName(e graph.UnresolvedEdge) string {
	if q := (graph.Symbol{Name: e.SrcName, Parent: e.SrcParent}).QName(); q != "" {
		return q
	}
	return e.SrcFile
}

// callerDrops indexes the dropped edges that target name, for filtering own
// callers rows. Kind is left empty: see inKey.
func callerDrops(dropped []graph.TierOneEdge, name string) dropSet[inKey] {
	d := dropSet[inKey]{}
	for _, e := range dropped {
		if e.DstName != name {
			continue
		}
		d[inKey{File: e.SrcFile, QName: edgeQName(e.UnresolvedEdge), Line: e.Line}]++
	}
	return d
}

// dependentDrops indexes the dropped import/extends/implements edges that
// target name, matching (*graph.Store).Dependents' own
// `dst_name = ? OR dst_name LIKE '%/' || ?` predicate.
func dependentDrops(dropped []graph.TierOneEdge, name string) dropSet[inKey] {
	d := dropSet[inKey]{}
	for _, e := range dropped {
		if !depKinds[e.Kind] {
			continue
		}
		if e.DstName != name && !strings.HasSuffix(e.DstName, "/"+name) {
			continue
		}
		d[inKey{Kind: e.Kind, File: e.SrcFile, QName: edgeQName(e.UnresolvedEdge), Line: e.Line}]++
	}
	return d
}

// outDrops indexes the dropped edges SOURCED at the anchor, for filtering own
// callees and deps rows. The source match mirrors the per-repo reader: name
// alone when the anchor carries no parent, name and parent when it does.
func outDrops(dropped []graph.TierOneEdge, name, parent string, kinds map[string]bool) dropSet[outKey] {
	d := dropSet[outKey]{}
	for _, e := range dropped {
		if !kinds[e.Kind] {
			continue
		}
		if e.SrcName != name || (parent != "" && e.SrcParent != parent) {
			continue
		}
		d[outKey{Kind: e.Kind, DstName: e.DstName, Line: e.Line}]++
	}
	return d
}

// --- the verbs ---------------------------------------------------------------

// callersUnion is the shared callers computation behind `callers`, `nav`'s
// callers component and `impact`'s callers block. There is ONE of it on
// purpose: three sites computing the same union three ways is the drift shape
// this slice is most exposed to.
//
// It returns the definitions, the complete unioned caller list and the complete
// unioned referencing-file list — all UNTRUNCATED. Applying limit is the
// caller's job, once, at the end.
func (u *unionCtx) callersUnion(ra resolvedAnchor) (defs []query.DefRef, callers []query.CallerRef, files []string, err error) {
	addFile := u.fileAdder(&files)
	for _, am := range ra.members {
		u.s.consult(am.id)
		inner, err := query.Callers(am.absRoot, ra.text, unlimited)
		if err != nil {
			return nil, nil, nil, err
		}
		dropped, err := u.droppedEdges(am.id)
		if err != nil {
			return nil, nil, nil, err
		}
		drops := callerDrops(dropped, ra.name)
		for _, d := range inner.Definitions {
			d.Repo = am.id
			d.File = u.wsPath(am.id, d.File)
			defs = append(defs, d)
		}
		for _, c := range inner.Callers {
			if drops.take(inKey{File: c.File, QName: c.QName, Line: c.Line}) {
				continue
			}
			c.Repo = am.id
			c.File = u.wsPath(am.id, c.File)
			callers = append(callers, c)
		}
		for _, f := range inner.ReferencedFiles {
			addFile(am.id, u.wsPath(am.id, f))
		}
	}
	cross, err := u.cross(ra.keys(), true, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, c := range cross {
		file := u.wsPath(c.member, c.file())
		callers = append(callers, query.CallerRef{
			Name: c.name(), Parent: c.parent(), QName: c.qname(),
			File: file, Line: c.edge.Line,
			Ambiguous: c.flags.Ambiguous, Inferred: c.flags.Inferred, Repo: c.member,
		})
		addFile(c.member, file)
	}
	return defs, callers, files, nil
}

// calleesUnion is the shared callees computation behind `callees` and
// `impact`'s callees block.
func (u *unionCtx) calleesUnion(ra resolvedAnchor) ([]query.CalleeRef, error) {
	var out []query.CalleeRef
	for _, am := range ra.members {
		u.s.consult(am.id)
		inner, err := query.Callees(am.absRoot, ra.text, unlimited)
		if err != nil {
			return nil, err
		}
		dropped, err := u.droppedEdges(am.id)
		if err != nil {
			return nil, err
		}
		drops := outDrops(dropped, ra.name, ra.parent, callKinds)
		for _, c := range inner.Callees {
			if drops.take(outKey{Kind: "calls", DstName: c.Name, Line: c.CallLine}) {
				continue
			}
			c.Repo = am.id
			c.DefFile = u.wsPath(am.id, c.DefFile)
			out = append(out, c)
		}
	}
	cross, err := u.cross(ra.keys(), false, callKinds)
	if err != nil {
		return nil, err
	}
	for _, c := range cross {
		out = append(out, query.CalleeRef{
			Name: c.name(), Parent: c.parent(), QName: c.qname(),
			CallLine: c.edge.Line,
			DefFile:  u.wsPath(c.member, c.file()), DefLine: c.defLine(),
			Ambiguous: c.flags.Ambiguous, Inferred: c.flags.Inferred, Repo: c.member,
		})
	}
	return out, nil
}

// dependentsUnion is the shared dependents computation behind `dependents` and
// `impact`'s dependents block. Both verbs call THIS, which is what makes the
// two sites incapable of disagreeing; the test asserts it anyway, because a
// later edit could split them again.
func (u *unionCtx) dependentsUnion(ra resolvedAnchor) ([]query.DependentRef, error) {
	var out []query.DependentRef
	for _, am := range ra.members {
		u.s.consult(am.id)
		inner, err := query.Dependents(am.absRoot, ra.text, unlimited)
		if err != nil {
			return nil, err
		}
		dropped, err := u.droppedEdges(am.id)
		if err != nil {
			return nil, err
		}
		drops := dependentDrops(dropped, ra.name)
		for _, d := range inner.Dependents {
			if drops.take(inKey{Kind: d.Kind, File: d.File, QName: d.QName, Line: d.Line}) {
				continue
			}
			d.Repo = am.id
			d.File = u.wsPath(am.id, d.File)
			out = append(out, d)
		}
	}
	cross, err := u.cross(ra.keys(), true, depKinds)
	if err != nil {
		return nil, err
	}
	for _, c := range cross {
		out = append(out, query.DependentRef{
			Kind: c.edge.Kind, QName: c.qname(),
			File: u.wsPath(c.member, c.file()), Line: c.edge.Line, Repo: c.member,
		})
	}
	return out, nil
}

// The owner accessors the truncation disclosure reads. Named functions rather
// than inline closures so every verb attributes a withheld row the same way.
func callerOwner(c query.CallerRef) string       { return c.Repo }
func calleeOwner(c query.CalleeRef) string       { return c.Repo }
func dependentOwner(d query.DependentRef) string { return d.Repo }
func depOwner(d query.DepRef) string             { return d.Repo }
func findOwner(f query.FindRef) string           { return f.Repo }
func grepOwner(g query.GrepRef) string           { return g.Repo }

// fileAdder returns the deduping appender every unioned FILE LIST is built
// with. There is ONE of these because there are two such lists (callers'
// referenced files and nav's) and each has to record its paths' OWNING MEMBER
// as it goes — a []string row carries no Repo field, so a path missed here is a
// row the truncation disclosure can count but never name. Two hand-rolled
// dedupe loops is exactly how one of them would forget.
//
// It reports whether the path was APPENDED; false means empty or already seen,
// which is what lets nav keep its FilesTotal from double-counting a path two
// nested members both claim.
func (u *unionCtx) fileAdder(dst *[]string) func(member, p string) bool {
	seen := map[string]bool{}
	return func(member, p string) bool {
		if p == "" || seen[p] {
			return false
		}
		seen[p] = true
		u.fileOwner[p] = member
		*dst = append(*dst, p)
		return true
	}
}

// fileOwnerOf attributes a path in a unioned FILE LIST to its member.
func (u *unionCtx) fileOwnerOf(p string) string { return u.fileOwner[p] }

func callersWorkspace(s *session, anchor string, limit int) (*query.CallersAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()
	ra, err := u.resolveAnchor(anchor, false)
	if err != nil {
		return nil, err
	}
	defs, callers, files, err := u.callersUnion(ra)
	if err != nil {
		return nil, err
	}
	a := &query.CallersAnswer{
		Anchor:               anchor,
		Definitions:          append(make([]query.DefRef, 0, len(defs)), defs...),
		CallersTotal:         len(callers),
		Callers:              append(make([]query.CallerRef, 0), truncateOwned(s, callers, limit, callerOwner)...),
		ReferencedFilesTotal: len(files),
		ReferencedFiles:      append(make([]string, 0), truncateOwned(s, files, limit, u.fileOwnerOf)...),
	}
	return a, nil
}

func calleesWorkspace(s *session, anchor string, limit int) (*query.CalleesAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()
	ra, err := u.resolveAnchor(anchor, false)
	if err != nil {
		return nil, err
	}
	callees, err := u.calleesUnion(ra)
	if err != nil {
		return nil, err
	}
	return &query.CalleesAnswer{
		Anchor:  anchor,
		Total:   len(callees),
		Callees: append(make([]query.CalleeRef, 0), truncateOwned(s, callees, limit, calleeOwner)...),
	}, nil
}

func dependentsWorkspace(s *session, anchor string, limit int) (*query.DependentsAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()
	ra, err := u.resolveAnchor(anchor, false)
	if err != nil {
		return nil, err
	}
	deps, err := u.dependentsUnion(ra)
	if err != nil {
		return nil, err
	}
	return &query.DependentsAnswer{
		Anchor:     anchor,
		Total:      len(deps),
		Dependents: append(make([]query.DependentRef, 0), truncateOwned(s, deps, limit, dependentOwner)...),
	}, nil
}

// depsWorkspace unions `deps`.
//
// The symbol section is one unioned section — own extends/implements rows from
// every anchor member, then the cross rows — because it answers one question
// about one anchor. The defining file's imports stay PER MEMBER, one section
// each, because each names a different file and merging them would produce a
// section whose label could only name one of them.
//
// The file-imports section has no cross half. That is a property of THIS VERB's
// anchor, not of the data: the overlay does carry file-scope source keys
// ({member, file, ""}, from module-scope imports the ladder resolved), but this
// section is reached from a SYMBOL anchor and reads the anchor member's own
// imports of the defining file. Reading a file-scope key's outbound cross-edges
// here would be a new query surface, not a fix; it is deliberately out of scope.
func depsWorkspace(s *session, anchor string, limit int) (*query.DepsAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()
	ra, err := u.resolveAnchor(anchor, true)
	if err != nil {
		return nil, err
	}

	var symDeps []query.DepRef
	var fileSections []query.DepSection
	symLabel := "deps of " + anchor
	for _, am := range ra.members {
		u.s.consult(am.id)
		target := ra.text
		if am.file != "" {
			target = am.file
		}
		inner, err := query.Deps(am.absRoot, target, unlimited)
		if err != nil {
			return nil, err
		}
		dropped, err := u.droppedEdges(am.id)
		if err != nil {
			return nil, err
		}
		drops := outDrops(dropped, ra.name, ra.parent, symbolDepKinds)
		rewrite := func(ds []query.DepRef, filter bool) []query.DepRef {
			out := make([]query.DepRef, 0, len(ds))
			for _, d := range ds {
				if filter && drops.take(outKey{Kind: d.Kind, DstName: d.Target, Line: d.Line}) {
					continue
				}
				d.Repo = am.id
				d.DefFile = u.wsPath(am.id, d.DefFile)
				out = append(out, d)
			}
			return out
		}
		// query.Deps' section shape, relied on by index rather than by parsing
		// its labels: a FILE anchor yields exactly one "imports of <file>"
		// section; a SYMBOL anchor yields "deps of <anchor>" and, when the
		// anchor has a definition, "file imports (<defining file>)".
		if am.file != "" {
			for _, sec := range inner.Sections {
				rows := rewrite(sec.Deps, false)
				fileSections = append(fileSections, query.DepSection{
					Label: "imports of " + u.wsPath(am.id, am.file),
					Total: len(rows), Deps: truncateOwned(s, rows, limit, depOwner),
				})
			}
			continue
		}
		for i, sec := range inner.Sections {
			if i == 0 {
				symDeps = append(symDeps, rewrite(sec.Deps, true)...)
				continue
			}
			rows := rewrite(sec.Deps, false)
			label := sec.Label
			if len(am.defs) > 0 {
				label = "file imports (" + u.wsPath(am.id, am.defs[0].File) + ")"
			}
			fileSections = append(fileSections, query.DepSection{
				Label: label, Total: len(rows), Deps: truncateOwned(s, rows, limit, depOwner),
			})
		}
	}

	sections := make([]query.DepSection, 0, len(fileSections)+1)
	if !onlyFileAnchors(ra) {
		cross, err := u.cross(ra.keys(), false, symbolDepKinds)
		if err != nil {
			return nil, err
		}
		for _, c := range cross {
			symDeps = append(symDeps, query.DepRef{
				Kind: c.edge.Kind, Target: c.qname(),
				DefFile: u.wsPath(c.member, c.file()), DefLine: c.defLine(),
				Line: c.edge.Line, Repo: c.member,
			})
		}
		sections = append(sections, query.DepSection{
			Label: symLabel, Total: len(symDeps),
			Deps: append(make([]query.DepRef, 0), truncateOwned(s, symDeps, limit, depOwner)...),
		})
	}
	sections = append(sections, fileSections...)
	return &query.DepsAnswer{Anchor: anchor, Sections: sections}, nil
}

// onlyFileAnchors reports whether every member resolved the anchor as a file.
// A file anchor has no symbol section at all in repo mode, and manufacturing an
// empty "deps of <file>" section here would be a workspace-only answer shape.
func onlyFileAnchors(ra resolvedAnchor) bool {
	if len(ra.members) == 0 {
		return false
	}
	for _, am := range ra.members {
		if am.file == "" {
			return false
		}
	}
	return true
}

// impactWorkspace composes the depth-1 neighbourhood from the SAME three union
// functions the standalone verbs use. That shared construction is what makes
// the dependents block and `codeindex dependents` structurally incapable of
// reporting different numbers for one anchor.
func impactWorkspace(s *session, anchor string, limit int) (*query.ImpactAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()
	ra, err := u.resolveAnchor(anchor, false)
	if err != nil {
		return nil, err
	}
	defs, callers, _, err := u.callersUnion(ra)
	if err != nil {
		return nil, err
	}
	dependents, err := u.dependentsUnion(ra)
	if err != nil {
		return nil, err
	}
	callees, err := u.calleesUnion(ra)
	if err != nil {
		return nil, err
	}
	return &query.ImpactAnswer{
		Anchor:          anchor,
		Coverage:        impactCoverage,
		Definitions:     append(make([]query.DefRef, 0, len(defs)), defs...),
		CallersTotal:    len(callers),
		Callers:         append(make([]query.CallerRef, 0), truncateOwned(s, callers, limit, callerOwner)...),
		DependentsTotal: len(dependents),
		Dependents:      append(make([]query.DependentRef, 0), truncateOwned(s, dependents, limit, dependentOwner)...),
		CalleesTotal:    len(callees),
		Callees:         append(make([]query.CalleeRef, 0), truncateOwned(s, callees, limit, calleeOwner)...),
	}, nil
}

// navWorkspace unions nav's CALLERS component exactly as `callers` does, and
// fans its retrieval components out per member (§3.2).
//
// # Why the retrieval half is called with the CALLER'S limit, not `unlimited`
//
// nav's limit is not a pure display bound on those components: query.Nav passes
// it straight into search.Find, which TRUNCATES the ranked result set before nav
// derives anything from it. Both `Files` (find hits first, then grep hits) and
// the definitions fallback are then computed over that bounded match set, and
// FilesTotal counts the result. So `unlimited` would give a ONE-MEMBER workspace
// MORE files and MORE fallback definitions than the single-repo answer has —
// the opposite regression, and just as much a break of D7's frozen bar. The
// graph half is different and does use `unlimited`: CallersTotal there is a true
// count of a complete set, so §3.5's union arithmetic applies to it directly.
//
// # But the per-member *Total fields must be ACCUMULATED, never recounted
//
// The per-member-limit convention is shared with findWorkspace, but the
// arithmetic underneath it is NOT. FindAnswer.Total is POST-truncation in repo
// mode (query.Find sets Total = len(results) after search.Find applied the
// limit), so there the total and the row count coincide. NavAnswer's totals are
// the opposite: FilesTotal and the grep-attribution CallersTotal are
// PRE-truncation counts reported alongside lists query.Nav has already capped
// at `limit`, so the two DIVERGE the moment a member overflows. Recounting
// `len(files)` here would pin FilesTotal to the limit, so the renderer's
// "referenced in %d file(s)" header would under-report and its "... (+%d more)"
// tail could never fire. The per-member totals are therefore summed, and §3.5's
// "*Total counts the unioned set" holds over the union of the per-member sets.
//
// Truncating per member and concatenating is safe for the LISTS here (unlike the
// anchor verbs, where §3.5 forbids it) because each member contributes a prefix
// of length min(n_i, limit): the first `limit` rows of the concatenation are
// exactly the first `limit` rows of the untruncated concatenation.
func navWorkspace(s *session, anchor string, limit int) (*query.NavAnswer, error) {
	u, err := openUnion(s)
	if err != nil {
		return nil, err
	}
	defer u.close()
	ra, err := u.resolveAnchor(anchor, false)
	if err != nil {
		return nil, err
	}
	defs, callers, _, err := u.callersUnion(ra)
	if err != nil {
		return nil, err
	}

	var (
		matches     []query.FindRef
		grepCallers []query.CallerRef
		fallbackDef []query.DefRef
		files       []string
		// filesTotal and grepCallersTotal accumulate the per-member
		// PRE-truncation counts. See the note above: recounting the rows we
		// iterated would recount an already-truncated list.
		filesTotal       int
		grepCallersTotal int
	)
	addFile := u.fileAdder(&files)
	for _, rm := range u.present {
		id := rm.Member.ID
		if u.store(id) == nil {
			continue
		}
		u.s.consult(id)
		inner, err := query.Nav(rm.AbsRoot, ra.text, limit)
		if err != nil {
			return nil, err
		}
		for _, m := range inner.Matches {
			m.Repo = id
			m.File = u.wsPath(id, m.File)
			matches = append(matches, m)
		}
		filesTotal += inner.FilesTotal
		for _, f := range inner.Files {
			p := u.wsPath(id, f)
			if !addFile(id, p) {
				// A path two members both claim, which can only happen when one
				// member's root nests inside another's. Decrementing keeps the
				// total from double-counting what we can SEE collide; a
				// collision hidden past a member's own truncation is not
				// observable here and is left in the total. Single-member
				// workspaces cannot hit either case — query.Nav already deduped
				// — so §7.4's bar stays exact.
				filesTotal--
			}
		}
		if inner.CallersFromGrep {
			grepCallersTotal += inner.CallersTotal
			for _, c := range inner.Callers {
				c.Repo = id
				c.File = u.wsPath(id, c.File)
				grepCallers = append(grepCallers, c)
			}
		}
		for _, d := range inner.Definitions {
			d.Repo = id
			d.File = u.wsPath(id, d.File)
			fallbackDef = append(fallbackDef, d)
		}
	}

	a := &query.NavAnswer{
		Anchor:      anchor,
		Definitions: defs,
		Matches:     append(make([]query.FindRef, 0), truncateOwned(s, matches, limit, findOwner)...),
		Callers:     make([]query.CallerRef, 0),
		Files:       make([]string, 0),
	}
	if len(a.Definitions) == 0 {
		// Repo-mode nav falls back to every exact-name owner from the find lane
		// when the graph has no definition. Preserved rather than reimplemented:
		// the per-member answers already ran that fallback.
		a.Definitions = fallbackDef
	}
	if a.Definitions == nil {
		a.Definitions = make([]query.DefRef, 0)
	}
	// Grep attribution stands in for callers only when the union found NO call
	// edges at all — the same condition repo-mode nav uses, lifted to the union.
	//
	// The two lanes' totals come from different places, and that asymmetry is
	// the point: the graph lane's `callers` is a COMPLETE union (callersUnion
	// reads every member with `unlimited`), so len() is its true count, while
	// the grep lane's rows arrive already capped per member and its count must
	// be the accumulated one.
	callersTotal := len(callers)
	if len(callers) == 0 && len(grepCallers) > 0 {
		a.CallersFromGrep = true
		callers = grepCallers
		callersTotal = grepCallersTotal
	}
	a.CallersTotal = callersTotal
	a.Callers = append(a.Callers, truncateOwned(s, callers, limit, callerOwner)...)
	a.FilesTotal = filesTotal
	a.Files = append(a.Files, truncateOwned(s, files, limit, u.fileOwnerOf)...)
	return a, nil
}
