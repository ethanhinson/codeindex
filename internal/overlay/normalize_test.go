package overlay

import (
	"reflect"
	"testing"

	"codeindex/internal/config"
)

// TestNormalizeMembers covers the contract wsfresh depends on: the result must
// equal what Registry() returns for the same manifest.
func TestNormalizeMembers(t *testing.T) {
	in := []config.Member{
		{
			ID:         "a",
			Root:       "pkg/a",
			Namespaces: []string{"A", "B", "A", "B", "C"},
			Deps:       []string{},
		},
		{
			ID:         "b",
			Root:       "pkg/b",
			Namespaces: []string{},
			Deps:       []string{"a", "a"},
		},
	}
	want := []config.Member{
		{ID: "a", Root: "pkg/a", Namespaces: []string{"A", "B", "C"}, Deps: nil},
		{ID: "b", Root: "pkg/b", Namespaces: nil, Deps: []string{"a"}},
	}
	got := NormalizeMembers(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeMembers = %#v, want %#v", got, want)
	}
}

func TestNormalizeMembersDoesNotMutateInput(t *testing.T) {
	in := []config.Member{
		{ID: "a", Root: "pkg/a", Namespaces: []string{"A", "A"}, Deps: []string{"b", "b"}},
	}
	NormalizeMembers(in)
	if !reflect.DeepEqual(in, []config.Member{
		{ID: "a", Root: "pkg/a", Namespaces: []string{"A", "A"}, Deps: []string{"b", "b"}},
	}) {
		t.Fatalf("input mutated: %#v", in)
	}
	// Result slices must not alias the input's backing arrays either.
	got := NormalizeMembers(in)
	got[0].Namespaces[0] = "mutated"
	if in[0].Namespaces[0] != "A" {
		t.Fatalf("result aliases input backing array: %#v", in)
	}
}

func TestNormalizeMembersEmpty(t *testing.T) {
	if got := NormalizeMembers(nil); len(got) != 0 {
		t.Fatalf("NormalizeMembers(nil) = %#v", got)
	}
}

// TestNormalizeMembersMatchesRegistry ties the export to its stated contract:
// what ReplaceRegistry stores is what NormalizeMembers predicts.
//
// The ids are declared OUT of sort order on purpose ("b" before "a"): wsfresh
// compares the stored registry to NormalizeMembers over WHOLE RECORDS,
// member order included, so this test must pin ordinal round-tripping too.
// With ids in sort order, Registry()'s ORDER BY ord and an accidental
// ORDER BY id are indistinguishable and the guard proves nothing about order.
func TestNormalizeMembersMatchesRegistry(t *testing.T) {
	s := newStore(t)
	members := []config.Member{
		{ID: "b", Root: "pkg/b", Namespaces: nil, Deps: []string{"a", "a"}},
		{ID: "a", Root: "pkg/a", Namespaces: []string{"A", "A", "B"}, Deps: []string{}},
	}
	if err := s.ReplaceRegistry(&config.Workspace{Members: members}); err != nil {
		t.Fatalf("ReplaceRegistry: %v", err)
	}
	got, err := s.Registry()
	if err != nil {
		t.Fatalf("Registry: %v", err)
	}
	if want := NormalizeMembers(members); !reflect.DeepEqual(got, want) {
		t.Fatalf("Registry() = %#v, want NormalizeMembers = %#v", got, want)
	}
}
