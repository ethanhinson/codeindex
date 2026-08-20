package wsquery

import (
	"strings"
	"testing"

	"codeindex/internal/overlay"
)

// §3.7's table, row by row. The mapping exists ONCE so the same two words are
// not translated at nine renderers; these three rows are the whole vocabulary.
func TestReferenceFlagsTableRowByRow(t *testing.T) {
	cases := []struct {
		name   string
		record any
		want   Flags
	}{
		{
			name:   "cross-edge, exact (rung 1)",
			record: overlay.CrossEdge{Confidence: "exact"},
			want:   Flags{Ambiguous: false, Inferred: false},
		},
		{
			name:   "cross-edge, inferred (rung 2)",
			record: overlay.CrossEdge{Confidence: "inferred"},
			want:   Flags{Ambiguous: false, Inferred: true},
		},
		{
			name:   "ambiguity record (not an edge)",
			record: overlay.Ambiguity{RefName: "Zeta"},
			want:   Flags{Ambiguous: true, Inferred: false},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ReferenceFlags(c.record)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("flags = %+v, want %+v", got, c.want)
			}
		})
	}
}

// A rung-2 bare-name answer must never be presented as an import-mediated one:
// D3's epistemics rule. This is the same assertion as the table's second row,
// stated as the property it defends so a future edit that collapses Inferred
// into "not ambiguous" reads as the regression it is.
func TestInferredStaysDistinguishableFromExact(t *testing.T) {
	exact, err := ReferenceFlags(overlay.CrossEdge{Confidence: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	inferred, err := ReferenceFlags(overlay.CrossEdge{Confidence: "inferred"})
	if err != nil {
		t.Fatal(err)
	}
	if exact == inferred {
		t.Fatal("exact and inferred cross-edges produce identical flags: " +
			"a bare-name answer would be indistinguishable from an import-mediated one")
	}
}

// An unrecognized confidence string must NOT silently map to exact. Silently
// defaulting would present an unknown rung as the strongest one there is.
func TestUnrecognizedConfidenceIsAnErrorNotExact(t *testing.T) {
	for _, conf := range []string{"", "unambiguous", "ambiguous", "unresolved", "EXACT", "probably"} {
		got, err := ReferenceFlags(overlay.CrossEdge{Confidence: conf})
		if err == nil {
			t.Fatalf("confidence %q returned flags %+v with no error", conf, got)
		}
		if !strings.Contains(err.Error(), conf) && conf != "" {
			t.Fatalf("error %q does not name the offending confidence %q", err, conf)
		}
		if got != (Flags{}) {
			t.Fatalf("confidence %q returned flags %+v alongside its error", conf, got)
		}
	}
}

// graph.Confidence's vocabulary is "unambiguous"|"ambiguous"|"unresolved" and
// is deliberately NOT this one. Feeding a record type the mapping does not know
// is an error rather than a zero-valued Flags, which would read as "exact".
func TestReferenceFlagsRejectsAnUnknownRecordType(t *testing.T) {
	if got, err := ReferenceFlags("exact"); err == nil {
		t.Fatalf("a bare string returned %+v with no error", got)
	}
	if got, err := ReferenceFlags(nil); err == nil {
		t.Fatalf("nil returned %+v with no error", got)
	}
}

// The records arrive from overlay readers as values and from loop variables as
// pointers; both must map the same way.
func TestReferenceFlagsAcceptsPointersToo(t *testing.T) {
	got, err := ReferenceFlags(&overlay.CrossEdge{Confidence: "inferred"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Flags{Inferred: true}); got != want {
		t.Fatalf("flags = %+v, want %+v", got, want)
	}
	got, err = ReferenceFlags(&overlay.Ambiguity{RefName: "Zeta"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Flags{Ambiguous: true}); got != want {
		t.Fatalf("flags = %+v, want %+v", got, want)
	}
}
