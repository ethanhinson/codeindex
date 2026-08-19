package wsresolve

import (
	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// Member is one AVAILABLE workspace member as the ladder sees it: its manifest
// identity (id, declared namespaces, declared deps) plus an already-open index.
//
// The store is opened by the caller with graph.OpenExisting — never the
// creating build path, which would create an absent index and rebuild a
// version-mismatched one. The ladder only reads from it.
type Member struct {
	ID         string
	Namespaces []string
	Deps       []string
	Store      *graph.Store
}

// Records is the ladder's in-memory output for one source member. The ladder
// never touches the overlay: write ordering across members is load-bearing
// (a per-member ReplaceMemberEdges deletes on either end), so the orchestrator
// accumulates every member's Records and writes them in one clear-then-put
// pass.
type Records struct {
	CrossEdges  []overlay.CrossEdge
	Ambiguities []overlay.Ambiguity
}

// Provenance and confidence vocabulary. These four strings are the only values
// the ladder writes; "exact" is produced by rung 1 alone.
const (
	provImport = "cross_repo_import"
	provName   = "cross_repo_name"
	confExact  = "exact"
	confInfer  = "inferred"
)

// Pass is the state one whole-workspace resolution pass shares across its
// per-source Ladder calls. Today that is only the definition cache, and that is
// the point: the cache MUST outlive a single source, because every source in a
// pass asks the same targets the same defs(X) questions and a per-source cache
// re-issues each ProjectDefs up to M-1 times in an M-member workspace.
//
// A Pass is not safe for concurrent use and must not outlive the *graph.Store
// handles it cached answers from.
type Pass struct {
	defs map[defsKey][]graph.Symbol
}

// NewPass returns a Pass for one resolution pass. Callers create exactly one
// and run every source member's ladder through it.
func NewPass() *Pass {
	return &Pass{defs: map[defsKey][]graph.Symbol{}}
}

// Ladder runs the four-rung cross-repo resolution ladder over edges sourced in
// member src, against members (the available members in MANIFEST ORDER; src may
// appear in the slice and is skipped).
//
// Per edge, with N = DstName, Q = DstQualifier, H = DstNS and others = every
// member but src:
//
//  1. import-mediated: H != "" and exactly one other member declares a
//     namespace that nsPrefix-matches H and defines N exactly once →
//     cross_repo_import / exact. The only rung producing exact.
//  2. unique bare name: otherwise, if exactly one other member defines N
//     exactly once → cross_repo_name / inferred.
//  3. ambiguous: otherwise, if the candidate definitions across all other
//     members number two or more → one Ambiguity carrying all of them.
//  4. otherwise nothing is recorded at all.
//
// src is never its own candidate: in-repo resolution already ran and already
// lost, which is what made the edge a candidate.
//
// Rung 2 fires on "no rung-1 HIT", not on "H is literally empty" (spec
// assumption 6). The frozen rung order is untouched — rung 1 still runs first
// and still wins — this only settles the rung-1 miss case when H is non-empty.
// The literal reading is non-monotonic: rung 3's guard carries no H condition,
// so a rung-1 miss with non-empty H would still record an AMBIGUOUS answer when
// N resolves in ≥2 members, while the better-evidenced unique-hit case was
// dropped to unresolved — recording the weaker answer and discarding the
// stronger. A consequence is that a rung-3 record MAY carry a non-empty RefNS.
//
// The returned records are in edge order; within an Ambiguity, candidates are
// in the order described on orderedCandidates.
func (p *Pass) Ladder(src Member, members []Member, edges []graph.UnresolvedEdge) (Records, error) {
	var out Records

	others := make([]Member, 0, len(members))
	for _, m := range members {
		if m.ID != src.ID {
			others = append(others, m)
		}
	}
	if len(others) == 0 {
		// Degenerate single-member workspace: every rung requires a member
		// other than src, so there is nothing any edge could resolve to.
		return out, nil
	}

	for _, e := range edges {
		sk := overlay.SymKey{
			Member: src.ID,
			File:   e.SrcFile,
			QName:  graph.Symbol{Name: e.SrcName, Parent: e.SrcParent}.QName(),
		}

		// Materialise each other member's definitions of N once per edge; all
		// three rungs read the same slices.
		perMember := make([][]graph.Symbol, len(others))
		for i, m := range others {
			d, err := p.lookupDefs(m, e.DstName, e.DstQualifier)
			if err != nil {
				return Records{}, err
			}
			perMember[i] = d
		}

		// Rung 1 — import-mediated, the only exact rung.
		if e.DstNS != "" {
			hinted := -1
			multi := false
			for i, m := range others {
				if !memberClaims(m, e.DstNS) {
					continue
				}
				if hinted >= 0 {
					multi = true
					break
				}
				hinted = i
			}
			if !multi && hinted >= 0 && len(perMember[hinted]) == 1 {
				out.CrossEdges = append(out.CrossEdges, crossEdge(
					sk, others[hinted].ID, perMember[hinted][0], e,
					provImport, confExact))
				continue
			}
		}

		// Rung 2 — unique bare name.
		hit := -1
		hits := 0
		for i := range others {
			if len(perMember[i]) == 1 {
				hits++
				hit = i
			}
		}
		if hits == 1 {
			out.CrossEdges = append(out.CrossEdges, crossEdge(
				sk, others[hit].ID, perMember[hit][0], e, provName, confInfer))
			continue
		}

		// Rung 3 — ambiguous. Count is ALWAYS len(Candidates): this slice never
		// truncates and ambiguity records are never thinned.
		cands := orderedCandidates(src, others, perMember)
		if len(cands) >= 2 {
			out.Ambiguities = append(out.Ambiguities, overlay.Ambiguity{
				Src:        sk,
				RefName:    e.DstName,
				RefNS:      e.DstNS,
				Kind:       e.Kind,
				Line:       e.Line,
				Candidates: cands,
				Count:      len(cands),
			})
			continue
		}

		// Rung 4 — unresolved: no record of any kind.
	}

	return out, nil
}

// memberClaims reports whether any namespace m declares contains the import
// hint h under the namespace-boundary rule. It CALLS nsPrefix rather than
// restating the rule: nsPrefix is one invariant with two call sites (here and
// suppression detection) and the two must not be able to drift.
func memberClaims(m Member, h string) bool {
	for _, ns := range m.Namespaces {
		if nsPrefix(ns, h) {
			return true
		}
	}
	return false
}

type defsKey struct{ member, name, qualifier string }

// lookupDefs is defs(X): X's tier-0 definitions of name, RETRIED bare when the
// qualified form finds nothing. The qualifier is a lexical narrowing hint, and
// a cross-repo lookup must not be stricter than the in-repo one it extends —
// requiring it to match would drop every method call whose receiver type is
// defined in a third member.
//
// The cache key carries m.ID as well as (name, qualifier). That is a
// CORRECTNESS requirement, not a nicety: the cache is pass-scoped, so a key
// without the member would hand one member's definitions back for another
// member's lookup and silently fabricate candidates.
func (p *Pass) lookupDefs(m Member, name, qualifier string) ([]graph.Symbol, error) {
	k := defsKey{m.ID, name, qualifier}
	if d, ok := p.defs[k]; ok {
		return d, nil
	}
	d, err := m.Store.ProjectDefs(name, qualifier)
	if err != nil {
		return nil, err
	}
	if len(d) == 0 && qualifier != "" {
		if d, err = m.Store.ProjectDefs(name, ""); err != nil {
			return nil, err
		}
	}
	p.defs[k] = d
	return d, nil
}

func crossEdge(src overlay.SymKey, dstMember string, dst graph.Symbol, e graph.UnresolvedEdge, prov, conf string) overlay.CrossEdge {
	return overlay.CrossEdge{
		Src:        src,
		Dst:        overlay.SymKey{Member: dstMember, File: dst.File, QName: dst.QName()},
		Kind:       e.Kind,
		Provenance: prov,
		Confidence: conf,
		Line:       e.Line,
	}
}

// orderedCandidates flattens every (member, definition) pair into recorded
// order: the candidates of the single member named in src's manifest deps
// first — ONLY when deps names exactly one CANDIDATE member, D3's literal
// tiebreaker, so naming two or more means no reorder at all — then the
// remaining members in manifest order, and within a member in ProjectDefs
// order, which is (file, start_line, name).
func orderedCandidates(src Member, others []Member, perMember [][]graph.Symbol) []overlay.SymKey {
	first := -1
	for i, m := range others {
		if len(perMember[i]) == 0 {
			continue
		}
		named := false
		for _, d := range src.Deps {
			if d == m.ID {
				named = true
				break
			}
		}
		if !named {
			continue
		}
		if first >= 0 {
			// deps names two or more candidate members: no reorder.
			first = -1
			break
		}
		first = i
	}

	var out []overlay.SymKey
	appendMember := func(i int) {
		for _, d := range perMember[i] {
			out = append(out, overlay.SymKey{Member: others[i].ID, File: d.File, QName: d.QName()})
		}
	}
	if first >= 0 {
		appendMember(first)
	}
	for i := range others {
		if i != first {
			appendMember(i)
		}
	}
	return out
}
