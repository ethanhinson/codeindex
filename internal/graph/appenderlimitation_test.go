package graph

import (
	"path/filepath"
	"strings"
	"testing"
)

// putAppenderCandidates builds the `storage.Appender` shape: THREE tier-0
// symbols all named `Appender` inside the ONE namespace the hint narrows to,
// plus one same-named decoy in a different package. Modelled on prometheus's
// `storage` package, where the name is carried by an interface and by the
// methods that return it.
//
// The decoy is in cmd/prometheus (readyStorage's Appender method) and is there
// for one reason: "cmd/..." sorts before "storage/...", so the plain tier-0
// rung — which orders by file — picks the decoy when no hint is present. That
// makes the hinted half's namespace assertion below non-vacuous: it cannot pass
// unless the hint actually moved the answer into the right package.
func putAppenderCandidates(t *testing.T, st *Store) {
	t.Helper()

	putFile(t, st, &ParsedFile{
		Path: "cmd/prometheus/main.go",
		Symbols: []Symbol{
			{File: "cmd/prometheus/main.go", Name: "Appender", Parent: "readyStorage", Kind: KindMethod, StartLine: 10, EndLine: 12},
		},
	})

	// The three in-package same-name symbols. This is the whole limitation:
	// the hint narrows to a namespace, and there is more than one `Appender`
	// inside it.
	putFile(t, st, &ParsedFile{
		Path: "storage/fanout.go",
		Symbols: []Symbol{
			{File: "storage/fanout.go", Name: "Appender", Parent: "fanout", Kind: KindMethod, StartLine: 20, EndLine: 22},
		},
	})
	putFile(t, st, &ParsedFile{
		Path: "storage/generic.go",
		Symbols: []Symbol{
			{File: "storage/generic.go", Name: "Appender", Parent: "genericStorage", Kind: KindMethod, StartLine: 30, EndLine: 32},
		},
	})
	putFile(t, st, &ParsedFile{
		Path: "storage/storage.go",
		Symbols: []Symbol{
			{File: "storage/storage.go", Name: "Appender", Kind: KindType, StartLine: 40, EndLine: 45},
		},
	})
}

const storageImportPath = "github.com/prometheus/prometheus/storage"

// appenderEmbedder is scrape/target.go's `type limitAppender struct {
// storage.Appender }` shape: a qualified embed of a name that lives in an
// imported package. Source is the hint under test; passing "" gives the
// unhinted contrast.
func appenderEmbedder(path, source string) *ParsedFile {
	return &ParsedFile{
		Path: path,
		Symbols: []Symbol{
			{File: path, Name: "limitAppender", Kind: KindType, StartLine: 5, EndLine: 8},
		},
		Deps: []RawDep{
			{EnclosingIdx: -1, Kind: KindImports, Target: storageImportPath, Source: storageImportPath, Line: 1},
			{EnclosingIdx: 0, Kind: KindExtends, Target: "Appender", Source: source, Line: 6},
		},
	}
}

// TestKNOWNLIMITATIONHintedEmbedStaysAmbiguousAmongInPackageSameNameSymbols
// pins WHAT THE CODE DOES TODAY, not what it should do. Read the name
// literally: these assertions are a description, not an aspiration, and the
// `ambiguous` want below is not a bug being tolerated by accident.
//
// The recorded shape (change 0017's acceptance bar logs `storage.Appender` as
// PARTIAL and explicitly NEVER claimable as a win):
//
//	The namespace hint DOES work — the edge lands in the correct package,
//	`storage`, instead of the cmd/prometheus decoy the unhinted ladder picks.
//	It then stays `ambiguous`, because THREE symbols named `Appender` live in
//	that one package and a namespace hint cannot choose between them.
//
// Both halves are asserted deliberately. A test that only asserted `ambiguous`
// would still pass if the hint had failed entirely and the edge had stayed on
// the wrong package — it would pin the wrong half of the limitation and would
// stop nobody. "Right package, still ambiguous" is the record.
//
// # THE PREREQUISITE — read before changing the wants below
//
// Closing this gap requires IN-PACKAGE DISAMBIGUATION landing FIRST: the
// resolution ladder must be able to choose among same-name symbols inside one
// namespace, on evidence that actually distinguishes them (the embed's syntactic
// position selects a TYPE, so the interface `Appender` in storage/storage.go is
// the correct target and the two methods named `Appender` are not). That is a
// change of its own — it needs a real discriminator plumbed to the dep site and
// a decision about every language's version of the same question.
//
// Doing it WITHOUT that prerequisite causes a silent regression, and the naive
// close is attractive precisely because it looks like four lines. The tempting
// shortcuts, and what each actually does:
//
//   - Return ConfUnambiguous when boundIDs yields >1 id. This does not resolve
//     anything; it relabels a coin flip as certain, and every downstream
//     consumer that trusts `unambiguous` starts trusting a wrong answer with no
//     signal that it changed. Ambiguous-but-honest is strictly better than
//     confident-and-wrong.
//   - Break the tie by ordering — first file, or prefer KindType. Ordering is
//     not evidence. It happens to pick storage/storage.go in THIS fixture, and
//     it silently picks something arbitrary everywhere the shape differs, which
//     is the failure mode that produced this test.
//
// So: if you are here because you made in-package disambiguation work, change
// these wants and rename the test. If you are here because `ambiguous` looked
// like a bug, the gap is deliberate and the prerequisite above is the work.
func TestKNOWNLIMITATIONHintedEmbedStaysAmbiguousAmongInPackageSameNameSymbols(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	putAppenderCandidates(t, st)

	// The hinted embed: correct package, still ambiguous.
	putFile(t, st, appenderEmbedder("scrape/target.go", storageImportPath))
	dstFile, dstNS, conf, hint := resolvedDepEdge(t, st, "scrape/target.go", string(KindExtends))

	// Half one — the part that WORKS. The hint narrowed to the right package.
	if dstNS != "storage" {
		t.Errorf("hinted embed resolved into namespace %q, want %q — the hint does get the package right",
			dstNS, "storage")
	}
	if !strings.HasPrefix(dstFile, "storage/") {
		t.Errorf("hinted embed dst = %q, want a file in the storage package", dstFile)
	}
	if hint != storageImportPath {
		t.Errorf("persisted hint = %q, want the raw import path %q", hint, storageImportPath)
	}

	// Half two — the part that DOES NOT, and is not supposed to yet. Three
	// `Appender` symbols share the hinted namespace, so boundIDs returns three
	// ids and resolve reports its deterministic first pick as ambiguous.
	if conf != string(ConfAmbiguous) {
		t.Errorf("hinted embed confidence = %q, want %q — see the prerequisite in this test's doc comment "+
			"before treating this as a bug", conf, ConfAmbiguous)
	}

	// The contrast that makes half one mean something: with the Source removed
	// the plain tier-0 rung orders by file and lands on the cmd/prometheus
	// decoy — wrong package, and also ambiguous. So `ambiguous` alone never
	// distinguished a working hint from a broken one.
	putFile(t, st, appenderEmbedder("scrape/unhinted.go", ""))
	dstFile, dstNS, conf, hint = resolvedDepEdge(t, st, "scrape/unhinted.go", string(KindExtends))
	if hint != "" {
		t.Errorf("unhinted embed must carry no hint, got %q", hint)
	}
	if dstNS != "cmd/prometheus" || dstFile != "cmd/prometheus/main.go" {
		t.Errorf("unhinted embed dst = %q (ns %q), want the cmd/prometheus decoy — if this moved, the "+
			"hinted half above may be passing for a reason other than the hint", dstFile, dstNS)
	}
	if conf != string(ConfAmbiguous) {
		t.Errorf("unhinted embed confidence = %q, want %q", conf, ConfAmbiguous)
	}
}
