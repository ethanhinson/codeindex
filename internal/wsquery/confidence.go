package wsquery

import (
	"fmt"

	"codeindex/internal/overlay"
)

// Confidence vocabulary reconcile (§3.7).
//
// overlay.CrossEdge.Confidence is "exact" | "inferred" — the frozen workspace
// vocabulary, deliberately NOT graph.Confidence ("unambiguous" | "ambiguous" |
// "unresolved"). The reconcile is not overlay-vocabulary -> graph.Confidence:
// NO ANSWER TYPE CARRIES A graph.Confidence AT ALL. The only confidence signals
// on the answer surface are CallerRef/CalleeRef's Ambiguous and the Inferred
// field added alongside Repo.
//
// The mapping lives in ONE function that every reference-construction site
// calls. The alternative is the same two words translated at nine renderers,
// and the renderers are not taught the overlay's vocabulary at all: ambigTag
// keeps keying on Ambiguous, and the inferred tag keys on Inferred.

// Overlay confidence values, as the ladder writes them.
const (
	// ConfidenceExact is rung 1 — import-mediated.
	ConfidenceExact = "exact"
	// ConfidenceInferred is rung 2 — a bare-name match.
	ConfidenceInferred = "inferred"
)

// Flags is the pair of booleans the answer surface carries for one reference.
//
// Keeping inferred DISTINGUISHABLE is not cosmetic: D3's epistemics rule is
// that a bare-name (rung-2) answer must never be presented as an
// import-mediated one. Collapsing Inferred into "not ambiguous" would do
// exactly that.
type Flags struct {
	// Ambiguous is the existing signal on CallerRef and CalleeRef: the
	// reference could not be resolved to one symbol.
	Ambiguous bool
	// Inferred marks a rung-2 bare-name resolution.
	Inferred bool
}

// ReferenceFlags maps one overlay record onto the answer surface's booleans:
//
//	overlay record                              Ambiguous  Inferred
//	cross-edge, confidence == "exact"   (rung 1)   false     false
//	cross-edge, confidence == "inferred" (rung 2)  false     TRUE
//	ambiguity record (not an edge)                 TRUE      false
//
// It takes the record itself rather than a confidence string so a caller cannot
// mis-bind the two vocabularies: an ambiguity record has no confidence column,
// and a graph.Confidence value is not a record type this function knows.
//
// An UNRECOGNIZED confidence is an ERROR, never a silent fall-through to exact.
// Defaulting would present an unknown rung as the strongest one there is, which
// is the same lie D3 forbids, arrived at by omission. The zero Flags is
// returned alongside the error precisely because it reads as "exact" — a caller
// that ignores the error must not get a usable-looking answer.
func ReferenceFlags(record any) (Flags, error) {
	switch r := record.(type) {
	case overlay.CrossEdge:
		return crossEdgeFlags(r.Confidence)
	case *overlay.CrossEdge:
		if r == nil {
			return Flags{}, fmt.Errorf("wsquery: nil *overlay.CrossEdge has no confidence")
		}
		return crossEdgeFlags(r.Confidence)
	case overlay.Ambiguity:
		return Flags{Ambiguous: true}, nil
	case *overlay.Ambiguity:
		if r == nil {
			return Flags{}, fmt.Errorf("wsquery: nil *overlay.Ambiguity")
		}
		return Flags{Ambiguous: true}, nil
	default:
		return Flags{}, fmt.Errorf(
			"wsquery: %T is not an overlay record; the confidence reconcile knows "+
				"overlay.CrossEdge and overlay.Ambiguity only", record)
	}
}

func crossEdgeFlags(confidence string) (Flags, error) {
	switch confidence {
	case ConfidenceExact:
		return Flags{}, nil
	case ConfidenceInferred:
		return Flags{Inferred: true}, nil
	default:
		return Flags{}, fmt.Errorf(
			"wsquery: unrecognized cross-edge confidence %q; the frozen workspace "+
				"vocabulary is %q|%q", confidence, ConfidenceExact, ConfidenceInferred)
	}
}
