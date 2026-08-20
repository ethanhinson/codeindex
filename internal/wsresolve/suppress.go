package wsresolve

import (
	"sort"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// Suppressed is one whole-workspace member-over-dep precedence pass, held in
// memory. Nothing here is written to the overlay: the orchestrator (§3.3's
// Resolve) owns write ordering for the entire pass, because a per-member
// ReplaceMemberEdges deletes on either end.
type Suppressed struct {
	// Records are the consumer-scoped suppressions, ordered by consumer in the
	// order members were supplied (manifest order) and, within a consumer, by
	// namespace.
	Records []overlay.Suppression

	// Repoint maps a CONSUMER member id to the ladder candidate edges its
	// suppressions created — the member's tier-1 edges into a suppressed
	// namespace, restated as unresolved edges whose DstNS is that namespace.
	// Membership is EXACT EQUALITY between the edge's DstNamespace (from
	// TierOneEdges, sourced from symbols.namespace) and the suppressed
	// namespace (from depfiles.namespace) — NOT the nsPrefix boundary rule
	// rung 1 uses. Both sides come from the same attachment, so today they
	// always agree; the equality check, not a prefix match, is what the code
	// actually does. Per consumer they are in TierOneEdges order. A consumer
	// with no re-pointed edge has no entry. The caller runs these through Ladder
	// alongside the member's own UnresolvedEdges; since exactly one member owns
	// the namespace, each enters at rung 1 and selects that owner directly,
	// yielding cross_repo_import/exact when the name resolves uniquely there and
	// falling through rungs 2-4 unchanged when it does not.
	Repoint map[string][]graph.UnresolvedEdge
}

// Suppress derives member-over-dep precedence across the available members
// (in manifest order; every member in the slice is both a possible consumer
// and a possible owner).
//
// For each member C and each namespace A that C has a depmap attached for, the
// owner candidates are the members O != C that claim A under the SAME
// namespace-boundary rule rung 1 uses — memberClaims, hence nsPrefix, called
// and never restated.
//
//   - Exactly one such O: the member wins. One Suppression is recorded, and
//     C's tier-1 edges into A become ladder candidates re-pointed at O.
//   - Two or more: NOTHING is suppressed and nothing is recorded. "The member
//     wins" presupposes an unambiguous member; picking one would be a silent
//     lie about which code an agent would edit (spec assumption 11).
//   - None: A is a genuine third-party dependency; it is left entirely alone.
//
// Suppression never mutates C's graph.db: the tier-1 symbol rows stay, the
// intra-repo edge stays, and the overlay MAY additionally carry a cross-edge
// into O — only where the re-pointed edge actually resolves in O (see
// Suppressed.Repoint). A record with no cross-edge behind it is a normal
// outcome, which is why the §4.1 obligation in the package doc is conditioned
// on a cross-edge from the same call site rather than on the record alone.
//
// A known non-match is deliberately left alone: a member that declares only a
// composer package name while its vendored copy is recorded under a PSR-4 root
// will not match, so it is not suppressed. That is a member-DISCOVERY gap, not
// a resolution one — record nothing, resolve nothing, the safe direction.
func Suppress(members []Member) (Suppressed, error) {
	var out Suppressed

	for _, c := range members {
		deps, err := c.Store.DepFiles()
		if err != nil {
			return Suppressed{}, err
		}
		if len(deps) == 0 {
			continue
		}

		// Group C's depfiles rows by attached namespace. DepFiles has no ORDER
		// BY, so the namespaces are sorted here: two passes over an unchanged
		// workspace must produce byte-identical overlay content.
		versions := map[string][]string{}
		for _, d := range deps {
			versions[d.Namespace] = append(versions[d.Namespace], d.Version)
		}
		namespaces := make([]string, 0, len(versions))
		for ns := range versions {
			namespaces = append(namespaces, ns)
		}
		sort.Strings(namespaces)

		suppressed := map[string]bool{}
		for _, ns := range namespaces {
			owner, ok := soleOwner(members, c.ID, ns)
			if !ok {
				continue
			}
			suppressed[ns] = true
			out.Records = append(out.Records, overlay.Suppression{
				ConsumerMember:    c.ID,
				Namespace:         ns,
				OwnerMember:       owner,
				SuppressedVersion: smallestVersion(versions[ns]),
			})
		}
		if len(suppressed) == 0 {
			continue
		}

		edges, err := c.Store.TierOneEdges()
		if err != nil {
			return Suppressed{}, err
		}
		var cands []graph.UnresolvedEdge
		for _, e := range edges {
			if !suppressed[e.DstNamespace] {
				continue
			}
			// The embedded UnresolvedEdge carries the source key and the call
			// site's own hint columns verbatim; only the hint is replaced, with
			// the suppressed namespace, which is what makes this enter at rung 1.
			u := e.UnresolvedEdge
			u.DstNS = e.DstNamespace
			cands = append(cands, u)
		}
		if len(cands) > 0 {
			if out.Repoint == nil {
				out.Repoint = map[string][]graph.UnresolvedEdge{}
			}
			out.Repoint[c.ID] = cands
		}
	}

	return out, nil
}

// soleOwner returns the id of the ONLY member other than consumerID claiming
// namespace ns, and whether there was exactly one. Two claimants report none:
// the ambiguity is the answer, not an obstacle to be broken.
func soleOwner(members []Member, consumerID, ns string) (string, bool) {
	owner := ""
	found := false
	for _, o := range members {
		if o.ID == consumerID || !memberClaims(o, ns) {
			continue
		}
		if found {
			return "", false
		}
		owner, found = o.ID, true
	}
	return owner, found
}

// smallestVersion is the lexicographically smallest NON-EMPTY version among C's
// depfiles rows for one namespace, or "" when every row is unset. Rows can
// disagree across re-attachments, and the recorded version must not depend on
// row order.
func smallestVersion(vs []string) string {
	best := ""
	for _, v := range vs {
		if v == "" {
			continue
		}
		if best == "" || v < best {
			best = v
		}
	}
	return best
}
