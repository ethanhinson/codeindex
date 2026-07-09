package golang

import "testing"

const sample = `package p

func Helper(x int) int {
	return x + 1
}

func Caller() int {
	y := Helper(2)
	return y
}

type Widget struct{ n int }

func (w Widget) Grow() int {
	return Helper(w.n)
}
`

func TestParseSymbols(t *testing.T) {
	pf, err := (Adapter{}).Parse("p.go", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Helper": false, "Caller": false, "Widget": false, "Grow": false}
	for _, s := range pf.Symbols {
		if _, ok := want[s.Name]; ok {
			want[s.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing symbol %q; got %+v", name, pf.Symbols)
		}
	}
}

func TestCallsAttributedToEnclosing(t *testing.T) {
	pf, err := (Adapter{}).Parse("p.go", []byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	// Both Caller and Grow call Helper; attribute each call to its enclosing symbol.
	callsFrom := map[string][]string{}
	for _, c := range pf.Calls {
		from := "<top>"
		if c.EnclosingIdx >= 0 {
			from = pf.Symbols[c.EnclosingIdx].Name
		}
		callsFrom[from] = append(callsFrom[from], c.Callee)
	}
	if !contains(callsFrom["Caller"], "Helper") {
		t.Errorf("Caller should call Helper; got %v", callsFrom["Caller"])
	}
	if !contains(callsFrom["Grow"], "Helper") {
		t.Errorf("Grow should call Helper; got %v", callsFrom["Grow"])
	}
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
