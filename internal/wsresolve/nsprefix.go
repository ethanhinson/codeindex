// Package wsresolve implements the workspace resolution ladder: resolving
// cross-member import hints against declared member namespaces.
//
// # Recorded obligation to the union-graph query layer (openspec §4.1)
//
// Member-over-dep precedence (see Suppress) does NOT mutate the consumer's
// graph.db: when member C vendors a namespace member O owns, C's intra-repo
// edge into its own tier-1 copy survives, and the overlay MAY ADDITIONALLY
// carry a cross-edge from the same call site into O. Where it does, a
// union-graph query over C that simply unions the two sources would count that
// one call TWICE.
//
// "May", not "does": a suppression is recorded on owner-uniqueness alone, but
// the re-pointed edge only becomes a cross-edge when the name also resolves in
// O. When it does not, the edge falls through rungs 2-4 (see Suppressed.Repoint)
// and the call site's ONLY edge is the surviving intra-repo one — a suppression
// record can therefore exist with no cross-edge behind it.
//
// §4.1 must read dep_suppressions and filter out an intra-repo edge whose
// resolved target is a tier-1 symbol in a suppressed namespace ONLY WHEN the
// overlay carries a cross-edge from the same call site — same source key
// (src_file, src_name, src_parent), same kind, same line. Absent such a
// cross-edge there is nothing to double-count, and filtering would delete the
// consumer's still-correct tier-1 edge with nothing replacing it. This package
// writes the record that makes that filter possible and does nothing else about
// it; nothing reads the overlay yet, so no double-count is observable before
// §4.1 exists. This is a stated obligation, not a side effect — deleting the
// filter requirement without replacing it reintroduces the double-count, and
// widening it past the same-call-site condition drops correct edges.
package wsresolve

import "strings"

// nsPrefix reports whether the import hint h falls inside the declared member namespace
// ns, matching only on a namespace boundary: exact equality, or h continuing after ns
// with one of the separators "/", "\", ".". Trailing separators on ns are trimmed first,
// because PSR-4 autoload keys are declared with one ("Symfony\Component\").
func nsPrefix(ns, h string) bool {
	ns = strings.TrimRight(ns, `/\.`)
	if ns == "" || h == "" {
		return false
	}
	if ns == h {
		return true
	}
	rest, ok := strings.CutPrefix(h, ns)
	if !ok {
		return false
	}
	switch rest[0] {
	case '/', '\\', '.':
		return true
	default:
		return false
	}
}
