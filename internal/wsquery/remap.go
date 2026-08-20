package wsquery

import (
	"strings"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// Stable-key re-map (§3.8).
//
// Cross-edges store overlay.SymKey{Member, File, QName} rather than symbol ids,
// because a member rebuild renumbers ids: a stored id would silently address a
// different symbol after the next build. The price is that every key must be
// INVERTED to a symbol at query time, against the member's OWN database.

// DefsReader is the one member-DB read the re-map is permitted to make.
//
// The interface has exactly ONE method, and that is the point: it makes
// graph.Store's Definitions UNREACHABLE from this code path by construction
// rather than by discipline. ProjectDefs' own doc comment says why that matters
// — Definitions neither filters tier nor returns Tier/Namespace, so inverting a
// key through it RE-ADMITS A VENDORED TIER-1 SNAPSHOT as a cross-repo target,
// the exact failure member-over-dep precedence exists to prevent.
//
// ProjectDefs is also the correct inverse in the narrow sense: the keys being
// inverted here were WRITTEN by ProjectDefs, in internal/wsresolve/ladder.go's
// lookupDefs.
type DefsReader interface {
	ProjectDefs(name, parent string) ([]graph.Symbol, error)
}

// A *graph.Store is the production DefsReader. Asserted here so a signature
// change on either side is a compile error at the seam.
var _ DefsReader = (*graph.Store)(nil)

// RemapStatus is the outcome of inverting one stable key. All three states are
// defined, because ProjectDefs is not a clean inverse: it treats parent == ""
// as NO PARENT RESTRICTION, and the write side has a deliberate parent-less
// fallback, so one key can come back with none, one, or many symbols.
type RemapStatus int

const (
	// RemapUnmapped: the key inverted to no symbol at all — the member was
	// rebuilt and the symbol is gone. The reference is DROPPED FROM THE ANSWER
	// AND COUNTED (see session.remapKey and Clause.KeysUnmapped); it is never
	// silently swallowed.
	RemapUnmapped RemapStatus = iota
	// RemapExact: the key inverted to exactly one symbol, which is used.
	RemapExact
	// RemapAmbiguous: several symbols survived, and the reference is rendered
	// AMBIGUOUS with its candidates. Picking a row would manufacture an
	// exact-looking answer out of a genuinely ambiguous key, which is the one
	// thing D3 forbids end to end (assumption 19).
	RemapAmbiguous
)

// String makes a failed assertion in a test read as a state, not an integer.
func (s RemapStatus) String() string {
	switch s {
	case RemapUnmapped:
		return "unmapped"
	case RemapExact:
		return "exact"
	case RemapAmbiguous:
		return "ambiguous"
	}
	return "unknown"
}

// Remap is one inverted stable key.
//
// Exactly one of Symbol and Candidates is populated, by status: Symbol on
// RemapExact, Candidates on RemapAmbiguous, neither on RemapUnmapped. An
// ambiguous re-map deliberately carries NO Symbol — a caller that reached for
// one anyway would be picking a row.
type Remap struct {
	Status     RemapStatus
	Symbol     graph.Symbol
	Candidates []graph.Symbol
}

// splitQName inverts graph.Symbol.QName, which is Parent + "." + Name.
//
// THE SPLIT IS ON THE LAST DOT. Dotted parents are ordinary in TypeScript and
// Python, so a first-dot split silently mis-parents every nested symbol in two
// of the four supported languages — and mis-parenting does not fail loudly, it
// just inverts to nothing and drops the reference.
func splitQName(qname string) (parent, name string) {
	i := strings.LastIndex(qname, ".")
	if i < 0 {
		return "", qname
	}
	return qname[:i], qname[i+1:]
}

// RemapKey inverts key to a symbol in the member's own database. r must be the
// store of key.Member; RemapKey cannot check that, because a DefsReader carries
// no identity.
//
// key.File is deliberately NOT used as a filter. §3.8 defines the cardinality
// rules on the parent alone, and a member rebuild that moved a symbol between
// files is exactly the case the stable key exists to survive — narrowing by
// file would turn those into silent drops.
func RemapKey(r DefsReader, key overlay.SymKey) (Remap, error) {
	parent, name := splitQName(key.QName)

	defs, err := r.ProjectDefs(name, parent)
	if err != nil {
		return Remap{}, err
	}
	switch len(defs) {
	case 0:
		return Remap{Status: RemapUnmapped}, nil
	case 1:
		return Remap{Status: RemapExact, Symbol: defs[0]}, nil
	}

	// Many. Narrow to the symbols whose Parent is EXACTLY the key's parent —
	// for a dotless QName that means Parent == "". With a non-empty parent
	// ProjectDefs already restricted the query, so this only ever narrows the
	// dotless case, which is the one the parent-less fallback creates.
	exact := make([]graph.Symbol, 0, len(defs))
	for _, d := range defs {
		if d.Parent == parent {
			exact = append(exact, d)
		}
	}
	if len(exact) == 1 {
		return Remap{Status: RemapExact, Symbol: exact[0]}, nil
	}
	if len(exact) == 0 {
		// The narrowing removed everything. It is a NARROWING of a many-set,
		// not a fourth drop condition: report the full candidate set as
		// ambiguous rather than dropping rows the database really returned.
		// The exactly-one rule above accepts its row without re-checking the
		// parent, so this narrowing must not be more destructive than either
		// neighbouring rule.
		return Remap{Status: RemapAmbiguous, Candidates: defs}, nil
	}
	return Remap{Status: RemapAmbiguous, Candidates: exact}, nil
}

// remapKey is RemapKey with the drop COUNTED. Every re-map on an answer path
// goes through here, so a reference cannot be dropped without the coverage
// clause learning about it — the counting site is the re-map site, which is the
// only place the two can be kept from disagreeing.
func (s *session) remapKey(r DefsReader, key overlay.SymKey) (Remap, error) {
	got, err := RemapKey(r, key)
	if err != nil {
		return Remap{}, err
	}
	if got.Status == RemapUnmapped {
		s.keysUnmapped++
	}
	return got, nil
}
