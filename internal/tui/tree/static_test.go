package tree

import "testing"

func TestStaticRendersFullTree(t *testing.T) {
	got := Static(BuildTree(fixtureSymbols()))
	want := `internal/
  graph/
    store.go
      Store  type  :5
        Close  method  :40
      open  func  :100
  query/
    query.go
      Fresh  func  :10
      Ghost.Orphan  method  :30
main.go
  main  func  :1
`
	if got != want {
		t.Errorf("static output mismatch:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestStaticEmpty(t *testing.T) {
	if got := Static(BuildTree(nil)); got != "" {
		t.Errorf("empty tree should render empty string, got %q", got)
	}
}
