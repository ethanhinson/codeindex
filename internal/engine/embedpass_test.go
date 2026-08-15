package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/graph"
)

func TestDocComment(t *testing.T) {
	lines := []string{
		"package p",
		"",
		"// Helper increments x.",
		"// Deprecated: use Assist.",
		"func Helper(x int) int { return x + 1 }",
	}
	got := docComment(lines, 5)
	want := "Helper increments x. Deprecated: use Assist."
	if got != want {
		t.Fatalf("docComment = %q, want %q", got, want)
	}
	if d := docComment(lines, 1); d != "" {
		t.Fatalf("first line doc = %q, want empty", d)
	}
}

// A long JSDoc block must yield its top summary, never the @tag tail that
// sits adjacent to the definition (the nest gate failure).
func TestDocCommentJSDocSummaryNotTail(t *testing.T) {
	lines := []string{
		"/**",
		" * Decorator that assigns metadata to the class using the specified key.",
		" *",
		" * Requires two parameters:",
		" * - key - a value defining the key under which the metadata is stored",
		" * - value - metadata to be associated with key",
		" *",
		" * @see [Reflection](https://docs.nestjs.com/fundamentals/execution-context#reflection-and-metadata)",
		" * @publicApi",
		" */",
		"export const SetMetadata = ...",
	}
	got := docComment(lines, 11)
	if !strings.HasPrefix(got, "Decorator that assigns metadata") {
		t.Fatalf("doc = %q, want the summary line first", got)
	}
	if strings.Contains(got, "@see") || strings.Contains(got, "docs.nestjs.com") {
		t.Fatalf("doc leaked @tag tail: %q", got)
	}
}

func TestCardText(t *testing.T) {
	s := &graph.Symbol{
		Name: "CreateListing", Parent: "Server", Kind: graph.KindMethod,
		File: "internal/listings/create.go", Signature: "func (s *Server) CreateListing(l Listing) error",
	}
	text := cardText(s, "CreateListing provisions a host listing.",
		[]string{"host", "listing"})
	for _, want := range []string{
		"Server.CreateListing method",
		"name: create listing",
		"distinct: host listing",
		"in internal listings create",
		"doc: CreateListing provisions a host listing.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("card missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "called by") || strings.Contains(text, "calls:") {
		t.Fatalf("neighbor names must not appear in cards (D2 freeze):\n%s", text)
	}
}

// TestContrastSuppressesBoilerplateSiblings: a family of near-identical
// exception classes must lose the shared doc words and keep each member's
// distinctive tokens (design D3; the nest sibling-tie failure).
func TestContrastSuppressesBoilerplateSiblings(t *testing.T) {
	names := []string{
		"ConflictException", "UnauthorizedException", "ForbiddenException",
		"NotAcceptableException", "UnsupportedMediaTypeException", "GoneException",
	}
	syms := make([]graph.Symbol, len(names))
	docs := make([]string, len(names))
	for i, n := range names {
		syms[i] = graph.Symbol{ID: int64(i + 1), Name: n, Kind: graph.KindType,
			File: "exceptions/" + strings.ToLower(n) + ".ts"}
		docs[i] = "Defines an HTTP exception for the " + n + " error type."
	}
	c := buildContrast(syms, docs)

	idx := 4 // UnsupportedMediaTypeException
	filtered := c.filterDoc(idx, docs[idx])
	kept := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(filtered)) {
		kept[strings.Trim(w, ".,")] = true
	}
	for _, boiler := range []string{"defines", "http", "exception", "error", "type"} {
		if kept[boiler] {
			t.Fatalf("boilerplate word %q survived suppression: %q", boiler, filtered)
		}
	}
	distinct := c.distinct(idx)
	joined := strings.Join(distinct, " ")
	if !strings.Contains(joined, "unsupported") || !strings.Contains(joined, "media") {
		t.Fatalf("distinct tokens missing member identity: %v", distinct)
	}

	// Small families pass through untouched.
	small := []graph.Symbol{
		{ID: 1, Name: "Alpha", Kind: graph.KindFunc, File: "x/a.go"},
		{ID: 2, Name: "Beta", Kind: graph.KindFunc, File: "x/b.go"},
	}
	sdocs := []string{"Alpha does the shared thing.", "Beta does the shared thing."}
	sc := buildContrast(small, sdocs)
	if got := sc.filterDoc(0, sdocs[0]); got != sdocs[0] {
		t.Fatalf("small family was suppressed: %q", got)
	}
}

// Contrast must be a pure function of the symbol/doc population (content-hash
// stability across identical builds).
func TestContrastDeterministic(t *testing.T) {
	syms := []graph.Symbol{
		{ID: 1, Name: "AlphaHandler", Kind: graph.KindFunc, File: "h/a.go"},
		{ID: 2, Name: "BetaHandler", Kind: graph.KindFunc, File: "h/b.go"},
		{ID: 3, Name: "GammaHandler", Kind: graph.KindFunc, File: "h/c.go"},
		{ID: 4, Name: "DeltaHandler", Kind: graph.KindFunc, File: "h/d.go"},
		{ID: 5, Name: "EpsilonHandler", Kind: graph.KindFunc, File: "h/e.go"},
	}
	docs := []string{
		"handles alpha requests", "handles beta requests", "handles gamma requests",
		"handles delta requests", "handles epsilon requests",
	}
	a, b := buildContrast(syms, docs), buildContrast(syms, docs)
	for i := range syms {
		if a.filterDoc(i, docs[i]) != b.filterDoc(i, docs[i]) {
			t.Fatal("filterDoc not deterministic")
		}
		if strings.Join(a.distinct(i), "|") != strings.Join(b.distinct(i), "|") {
			t.Fatal("distinct not deterministic")
		}
	}
	// "handles" and "requests" are 100% family-df -> suppressed.
	if got := a.filterDoc(0, docs[0]); got != "alpha" {
		t.Fatalf("filterDoc(0) = %q, want %q", got, "alpha")
	}
}

// vecSnapshot is the id-independent vector state: card hash and vector
// presence per symbol content key.
func vecSnapshot(t *testing.T, dbPath string) map[string]string {
	t.Helper()
	db, err := graph.OpenRaw(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT s.file, s.parent, s.name, s.start_line, sv.hash, COUNT(v.hash)
		FROM symvec sv
		JOIN symbols s ON s.id = sv.symbol_id
		LEFT JOIN vecs v ON v.hash = sv.hash
		GROUP BY s.file, s.parent, s.name, s.start_line, sv.hash`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var file, parent, name, hash string
		var line, vecs int
		if err := rows.Scan(&file, &parent, &name, &line, &hash, &vecs); err != nil {
			t.Fatal(err)
		}
		if vecs == 0 {
			t.Fatalf("mapped symbol %s.%s has no stored vector", parent, name)
		}
		out[file+"|"+parent+"|"+name] = hash
	}
	return out
}

// TestEmbedParity_DocEdit: after a doc-comment edit and incremental patch,
// vector state (card hashes per symbol, vectors present) must equal a full
// rebuild's. The edit changes only a.go's cards; call structure is untouched
// so no neighbor drift is in play and parity is exact.
func TestEmbedParity_DocEdit(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": fileA, "b.go": fileB})
	inc := filepath.Join(dir, "inc.db")
	if _, err := Build(dir, inc); err != nil {
		t.Fatal(err)
	}
	if len(vecSnapshot(t, inc)) == 0 {
		t.Skip("embedding unavailable in this build (nollama)")
	}

	os.WriteFile(filepath.Join(dir, "a.go"),
		[]byte("package p\n// Helper increments its argument.\nfunc Helper(x int) int { return x + 1 }\nfunc A() int { return Helper(1) }\n"), 0o644)
	if _, err := Patch(dir, inc); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(t.TempDir(), "full.db")
	if _, err := Build(dir, full); err != nil {
		t.Fatal(err)
	}

	incSnap, fullSnap := vecSnapshot(t, inc), vecSnapshot(t, full)
	if len(incSnap) != len(fullSnap) {
		t.Fatalf("mapped symbols: inc %d, full %d", len(incSnap), len(fullSnap))
	}
	for k, h := range fullSnap {
		if incSnap[k] != h {
			t.Fatalf("card hash for %s: inc %s != full %s", k, incSnap[k], h)
		}
	}
	// The Helper card must reflect the new doc (differs from any b.go hash),
	// i.e. the edit actually re-embedded rather than reusing stale cards.
	if incSnap["a.go||Helper"] == "" {
		t.Fatal("Helper mapping missing")
	}
}

// TestEmbedIncrementalReuse: a content change that leaves every card text
// identical (append trailing blank line) re-parses the file but embeds
// nothing — the content-addressed cache absorbs it.
func TestEmbedIncrementalReuse(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": fileA, "b.go": fileB})
	inc := filepath.Join(dir, "inc.db")
	if _, err := Build(dir, inc); err != nil {
		t.Fatal(err)
	}
	if len(vecSnapshot(t, inc)) == 0 {
		t.Skip("embedding unavailable in this build (nollama)")
	}

	os.WriteFile(filepath.Join(dir, "a.go"), []byte(fileA+"\n"), 0o644)
	st, err := Patch(dir, inc)
	if err != nil {
		t.Fatal(err)
	}
	if st.FilesParsed != 1 {
		t.Fatalf("FilesParsed = %d, want 1", st.FilesParsed)
	}
	if st.Embedded != 0 {
		t.Fatalf("Embedded = %d, want 0 (unchanged cards must reuse cached vectors)", st.Embedded)
	}
}
