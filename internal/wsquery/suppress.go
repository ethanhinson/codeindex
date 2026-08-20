package wsquery

import (
	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// The dep_suppressions filter (§3.6) — internal/wsresolve's recorded
// obligation, discharged.
//
// Member-over-dep precedence does NOT mutate the consumer's graph.db: when
// member C vendors a namespace member O owns, C's intra-repo edge into its own
// tier-1 copy SURVIVES, and the overlay MAY ADDITIONALLY carry a cross-edge
// from the same call site into O. Where it does, a union that reads both
// sources counts that one call TWICE. This filter removes the intra-repo half.
//
// The obligation, verbatim from internal/wsresolve/wsresolve.go's package doc,
// implemented with NO WIDENING:
//
//	filter out an intra-repo edge whose resolved target is a tier-1 symbol in
//	a suppressed namespace ONLY WHEN the overlay carries a cross-edge from the
//	same call site — same source key (src_file, src_name, src_parent), same
//	kind, same line.
//
// THE CONDITION IS A CROSS-EDGE AT THE SAME CALL SITE, NEVER THE SUPPRESSION
// RECORD ALONE. A suppression is recorded on owner-uniqueness alone, so one can
// exist with no cross-edge behind it: the re-pointed edge fell through rungs
// 2-4 because the owner lacks the name. There is nothing to double-count there,
// and filtering would delete the consumer's still-correct tier-1 edge with
// NOTHING replacing it. That case is pinned by
// TestBYDESIGNSuppressionWithoutACrossEdgeKeepsTheIntraRepoEdge, whose premise
// internal/wsresolve's TestRepointedEdgeFallsThroughWhenOwnerLacksTheName
// proves is real.
//
// No new internal/graph reader is needed. (*graph.Store).TierOneEdges returns
// TierOneEdge{UnresolvedEdge, DstNamespace} — the five-part call-site tuple
// plus the resolved target's namespace — and it lines up with the overlay's
// src_* columns because internal/wsresolve/ladder.go builds the cross-edge
// source key from the same three columns and overlay's insertCrossEdge writes
// that key verbatim.

// CallSite is the source end of one edge: the four-part key both sides of the
// join are reduced to. src_name and src_parent collapse into one QName here
// because that is the form the overlay stores (SymKey.QName) and the form
// graph.Symbol.QName produces from the pair — see the two builders below, which
// exist so the collapse happens in one place per shape rather than at the join.
type CallSite struct {
	File  string
	QName string
	Kind  string
	Line  int
}

// SourceCallSite keys the source end of an intra-repo edge, from the
// (src_file, src_name, src_parent, kind, line) tuple TierOneEdges returns.
func SourceCallSite(e graph.UnresolvedEdge) CallSite {
	return CallSite{
		File:  e.SrcFile,
		QName: graph.Symbol{Name: e.SrcName, Parent: e.SrcParent}.QName(),
		Kind:  e.Kind,
		Line:  e.Line,
	}
}

// CrossEdgeCallSite keys the source end of a cross-edge, from the overlay's
// (src_file, src_qname, kind, line) columns. Src.Member is NOT part of the key
// — it is the scope, checked by the caller — so the two builders produce
// comparable values.
func CrossEdgeCallSite(e overlay.CrossEdge) CallSite {
	return CallSite{File: e.Src.File, QName: e.Src.QName, Kind: e.Kind, Line: e.Line}
}

// FilterSuppressedEdges partitions memberID's tier-1 edges into the ones a
// union answer keeps and the ones it drops as double-counted.
//
// sup is overlay.Suppressions() (every consumer's; this scopes to memberID's
// own — reading them unscoped would drop another member's correct edges).
// tierOne is memberID's own (*graph.Store).TierOneEdges(). cross is the
// overlay's cross-edges; only those whose Src.Member is memberID can name one
// of memberID's call sites.
//
// Both slices keep tierOne's order, which is total by TierOneEdges'
// construction, so the partition is deterministic.
//
// An edge is dropped only when BOTH halves hold:
//
//  1. its DstNamespace is a namespace suppressed FOR THIS CONSUMER, and
//  2. a cross-edge from this member exists at the same call site.
//
// The namespace test is per EDGE, not per call site: one call site can carry a
// suppressed and an unsuppressed tier-1 edge, and dropping by call site alone
// would take the innocent one with it.
func FilterSuppressedEdges(memberID string, sup []overlay.Suppression, tierOne []graph.TierOneEdge, cross []overlay.CrossEdge) (kept, dropped []graph.TierOneEdge) {
	suppressed := make(map[string]bool, len(sup))
	for _, s := range sup {
		if s.ConsumerMember == memberID {
			suppressed[s.Namespace] = true
		}
	}
	sites := make(map[CallSite]bool, len(cross))
	for _, e := range cross {
		if e.Src.Member == memberID {
			sites[CrossEdgeCallSite(e)] = true
		}
	}

	for _, e := range tierOne {
		if suppressed[e.DstNamespace] && sites[SourceCallSite(e.UnresolvedEdge)] {
			dropped = append(dropped, e)
			continue
		}
		kept = append(kept, e)
	}
	return kept, dropped
}
