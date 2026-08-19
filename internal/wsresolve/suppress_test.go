package wsresolve

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"codeindex/internal/graph"
	"codeindex/internal/overlay"
)

// --- fixtures --------------------------------------------------------------

// depRow is one file inside a hand-built depmap. version is per ROW, because
// depfiles carries the version per row and rows can disagree.
type depRow struct {
	file    string
	version string
	syms    []string
}

// vend is one dependency map attached into a consumer index under prefix.
type vend struct {
	namespace string
	prefix    string
	rows      []depRow
}

// buildConsumer builds a member index holding one project file whose function
// Caller calls each name in callees, then attaches each vendored depmap and
// re-resolves. It returns the member and the path of its graph.db (the
// byte-snapshot test needs the path). graph.Open is the BUILD path and is
// legitimate in test code; it is banned only from this package's non-test code.
func buildConsumer(t *testing.T, id string, namespaces, callees []string, vends ...vend) (Member, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "graph.db")

	st, err := graph.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	pf := &graph.ParsedFile{
		Path: "c.go",
		Symbols: []graph.Symbol{
			{File: "c.go", Name: "Caller", Kind: graph.KindFunc, StartLine: 1, EndLine: 2},
		},
	}
	for i, c := range callees {
		pf.Calls = append(pf.Calls, graph.RawCall{EnclosingIdx: 0, Callee: c, Line: 10 + i})
	}
	tx, err := st.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PutFile(tx, pf, graph.FileMeta{Path: "c.go", Hash: "h", Size: 1, Mtime: 1}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	for i, v := range vends {
		mapPath := filepath.Join(dir, fmt.Sprintf("dep%d.map.db", i))
		m, err := graph.Open(mapPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.WriteDepMeta(v.namespace, v.rows[0].version); err != nil {
			t.Fatal(err)
		}
		for _, r := range v.rows {
			var syms []graph.Symbol
			for _, n := range r.syms {
				syms = append(syms, graph.Symbol{Name: n, Kind: graph.KindFunc, StartLine: 1, EndLine: 2})
			}
			if err := m.PutDepSymbols(v.namespace, r.version, r.file, "maphash-"+r.file, 1, 1, syms); err != nil {
				t.Fatal(err)
			}
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		ns, _, names, err := st.AttachMap(mapPath, v.prefix)
		if err != nil {
			t.Fatal(err)
		}
		if ns != v.namespace {
			t.Fatalf("attached namespace = %q, want %q", ns, v.namespace)
		}
		if err := st.ReResolve(names); err != nil {
			t.Fatal(err)
		}
	}
	return Member{ID: id, Namespaces: namespaces, Store: st}, dbPath
}

// vendorLib is the common shape: a consumer calling Zeta, which resolves to
// its own vendored tier-1 copy of namespace "acme/lib" at version v1.2.3.
func vendorLib(t *testing.T, id string) (Member, string) {
	t.Helper()
	return buildConsumer(t, id, []string{"example.com/app"}, []string{"Zeta"},
		vend{
			namespace: "acme/lib",
			prefix:    "vendor/acme/lib",
			rows:      []depRow{{file: "z.go", version: "v1.2.3", syms: []string{"Zeta"}}},
		})
}

func mustSuppress(t *testing.T, members []Member) Suppressed {
	t.Helper()
	got, err := Suppress(members)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// assertTierOneEdgeExists guards the fixture: every re-point assertion below is
// vacuous unless the consumer really holds a tier-1 edge into the namespace.
func assertTierOneEdge(t *testing.T, m Member, ns string) {
	t.Helper()
	edges, err := m.Store.TierOneEdges()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edges {
		if e.DstNamespace == ns {
			return
		}
	}
	t.Fatalf("fixture %s has no tier-1 edge into %q: %+v", m.ID, ns, edges)
}

// --- the member wins -------------------------------------------------------

func TestSuppressionRecordsConsumerOwnerAndVersion(t *testing.T) {
	app, _ := vendorLib(t, "app")
	lib := newMember(t, "lib", []string{"acme/lib"}, nil, def{"lib/z.go", "Zeta", "", 1})

	got := mustSuppress(t, []Member{app, lib})
	want := []overlay.Suppression{{
		ConsumerMember:    "app",
		Namespace:         "acme/lib",
		OwnerMember:       "lib",
		SuppressedVersion: "v1.2.3",
	}}
	if len(got.Records) != 1 || got.Records[0] != want[0] {
		t.Fatalf("records = %+v, want %+v", got.Records, want)
	}
}

// The point of the whole slice: the consumer's tier-1 edge is re-pointed at the
// owning MEMBER, entering the ladder at rung 1 (the suppressed namespace is the
// hint) and landing as cross_repo_import/exact.
func TestSuppressedTierOneEdgeIsRepointedAtOwner(t *testing.T) {
	app, _ := vendorLib(t, "app")
	lib := newMember(t, "lib", []string{"acme/lib"}, nil, def{"lib/z.go", "Zeta", "", 4})
	assertTierOneEdge(t, app, "acme/lib")

	got := mustSuppress(t, []Member{app, lib})
	cands := got.Repoint["app"]
	if len(cands) != 1 {
		t.Fatalf("repoint candidates = %+v, want exactly 1", cands)
	}
	if cands[0].DstName != "Zeta" || cands[0].DstNS != "acme/lib" {
		t.Fatalf("candidate = %+v, want DstName Zeta and DstNS acme/lib", cands[0])
	}
	if cands[0].SrcFile != "c.go" || cands[0].SrcName != "Caller" {
		t.Fatalf("candidate source = %+v, want the calling symbol", cands[0])
	}

	recs, err := Ladder(app, []Member{app, lib}, cands)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs.Ambiguities) != 0 {
		t.Fatalf("ambiguities = %+v, want none", recs.Ambiguities)
	}
	if len(recs.CrossEdges) != 1 {
		t.Fatalf("cross edges = %+v, want exactly 1", recs.CrossEdges)
	}
	ce := recs.CrossEdges[0]
	if ce.Dst.Member != "lib" || ce.Dst.File != "lib/z.go" || ce.Dst.QName != "Zeta" {
		t.Fatalf("dst = %+v, want the owning member's own definition", ce.Dst)
	}
	if ce.Src != (overlay.SymKey{Member: "app", File: "c.go", QName: "Caller"}) {
		t.Fatalf("src = %+v", ce.Src)
	}
	if ce.Provenance != "cross_repo_import" || ce.Confidence != "exact" {
		t.Fatalf("provenance/confidence = %q/%q, want cross_repo_import/exact",
			ce.Provenance, ce.Confidence)
	}
}

// A re-pointed edge is only a ladder CANDIDATE: when the owner does not define
// the name uniquely, rungs 2-4 apply unchanged. Here nobody defines Zeta, so
// rung 4 records nothing — while the suppression record still stands.
func TestRepointedEdgeFallsThroughWhenOwnerLacksTheName(t *testing.T) {
	app, _ := vendorLib(t, "app")
	lib := newMember(t, "lib", []string{"acme/lib"}, nil, def{"lib/o.go", "Other", "", 1})

	got := mustSuppress(t, []Member{app, lib})
	if len(got.Records) != 1 {
		t.Fatalf("records = %+v, want the suppression to stand", got.Records)
	}
	recs, err := Ladder(app, []Member{app, lib}, got.Repoint["app"])
	if err != nil {
		t.Fatal(err)
	}
	if len(recs.CrossEdges) != 0 || len(recs.Ambiguities) != 0 {
		t.Fatalf("want nothing recorded, got %+v", recs)
	}
}

// --- the member does NOT win ----------------------------------------------

// Two members could own the namespace, so there is no unambiguous member for
// "the member wins" to name. Nothing is suppressed and nothing is re-pointed:
// inventing a winner would be a silent lie about which code an agent would edit.
func TestTwoPossibleOwnersSuppressNothing(t *testing.T) {
	app, _ := vendorLib(t, "app")
	// Both claim "acme/lib" under the boundary rule: one exactly, one as a
	// parent namespace.
	exact := newMember(t, "exact", []string{"acme/lib"}, nil, def{"e/z.go", "Zeta", "", 1})
	parent := newMember(t, "parent", []string{"acme"}, nil, def{"p/z.go", "Zeta", "", 1})
	assertTierOneEdge(t, app, "acme/lib")

	got := mustSuppress(t, []Member{app, exact, parent})
	if len(got.Records) != 0 {
		t.Fatalf("records = %+v, want none: two members could own acme/lib", got.Records)
	}
	if len(got.Repoint) != 0 {
		t.Fatalf("repoint = %+v, want none", got.Repoint)
	}

	// Guard against a vacuous pass: with the second claimant removed, the same
	// fixture DOES suppress. Only the ambiguity suppressed the suppression.
	solo := mustSuppress(t, []Member{app, exact})
	if len(solo.Records) != 1 || len(solo.Repoint["app"]) != 1 {
		t.Fatalf("control arm did not suppress: %+v", solo)
	}
}

// A namespace no member owns is a genuine third-party dependency: untouched.
func TestUnownedNamespaceIsLeftAlone(t *testing.T) {
	app, _ := vendorLib(t, "app")
	other := newMember(t, "other", []string{"example.com/other"}, nil, def{"o/z.go", "Zeta", "", 1})

	got := mustSuppress(t, []Member{app, other})
	if len(got.Records) != 0 || len(got.Repoint) != 0 {
		t.Fatalf("want nothing for an unowned namespace, got %+v", got)
	}
}

// The known non-match the spec deliberately leaves alone: the member declares
// only its composer package name while the vendored copy is recorded under the
// PSR-4 root. That is a member-DISCOVERY gap, so nothing is suppressed.
func TestComposerNameVersusPsr4RootIsNotSuppressed(t *testing.T) {
	app, _ := buildConsumer(t, "app", []string{"App"}, []string{"Client"},
		vend{
			namespace: `Acme\Lib`,
			prefix:    "vendor/acme/lib",
			rows:      []depRow{{file: "Client.php", version: "1.0.0", syms: []string{"Client"}}},
		})
	lib := newMember(t, "lib", []string{"acme/lib"}, nil, def{"src/Client.php", "Client", "", 1})

	got := mustSuppress(t, []Member{app, lib})
	if len(got.Records) != 0 || len(got.Repoint) != 0 {
		t.Fatalf("want nothing recorded, got %+v", got)
	}
}

// A member never suppresses its own attachment: the owner search excludes the
// consumer, so a member that both declares and vendors a namespace is untouched.
func TestConsumerIsNeverItsOwnOwner(t *testing.T) {
	app, _ := buildConsumer(t, "app", []string{"acme/lib"}, []string{"Zeta"},
		vend{
			namespace: "acme/lib",
			prefix:    "vendor/acme/lib",
			rows:      []depRow{{file: "z.go", version: "v1.2.3", syms: []string{"Zeta"}}},
		})

	got := mustSuppress(t, []Member{app})
	if len(got.Records) != 0 || len(got.Repoint) != 0 {
		t.Fatalf("want nothing recorded, got %+v", got)
	}
}

// --- version selection -----------------------------------------------------

// C's depfiles rows for one namespace can disagree across re-attachments. The
// recorded version is the lexicographically smallest NON-EMPTY one, never the
// first row the reader happened to return.
func TestVersionDisagreementPicksSmallestNonEmpty(t *testing.T) {
	app, _ := buildConsumer(t, "app", []string{"example.com/app"}, []string{"Zeta"},
		vend{
			namespace: "acme/lib",
			prefix:    "vendor/acme/lib",
			rows: []depRow{
				{file: "a.go", version: "v2.0.0", syms: []string{"Zeta"}},
				{file: "b.go", version: "", syms: []string{"Yankee"}},
				{file: "c.go", version: "v1.5.0", syms: []string{"Xray"}},
			},
		})
	lib := newMember(t, "lib", []string{"acme/lib"}, nil, def{"lib/z.go", "Zeta", "", 1})

	got := mustSuppress(t, []Member{app, lib})
	if len(got.Records) != 1 {
		t.Fatalf("records = %+v, want 1", got.Records)
	}
	if got.Records[0].SuppressedVersion != "v1.5.0" {
		t.Fatalf("SuppressedVersion = %q, want v1.5.0", got.Records[0].SuppressedVersion)
	}
}

func TestVersionUnsetOnEveryRowRecordsEmpty(t *testing.T) {
	app, _ := buildConsumer(t, "app", []string{"example.com/app"}, []string{"Zeta"},
		vend{
			namespace: "acme/lib",
			prefix:    "vendor/acme/lib",
			rows:      []depRow{{file: "z.go", version: "", syms: []string{"Zeta"}}},
		})
	lib := newMember(t, "lib", []string{"acme/lib"}, nil, def{"lib/z.go", "Zeta", "", 1})

	got := mustSuppress(t, []Member{app, lib})
	if len(got.Records) != 1 || got.Records[0].SuppressedVersion != "" {
		t.Fatalf("records = %+v, want one with an empty version", got.Records)
	}
}

// --- the consumer's index is never written --------------------------------

// The shape of the bar, not a "we didn't call Open" grep: snapshot the bytes.
func snapshot(t *testing.T, dbPath string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		p := dbPath + suffix
		b, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		out[p] = b
	}
	if _, ok := out[dbPath]; !ok {
		t.Fatalf("%s does not exist", dbPath)
	}
	return out
}

func TestConsumerGraphDBIsByteUnchanged(t *testing.T) {
	app, dbPath := vendorLib(t, "app")
	lib := newMember(t, "lib", []string{"acme/lib"}, nil, def{"lib/z.go", "Zeta", "", 1})
	assertTierOneEdge(t, app, "acme/lib")

	before := snapshot(t, dbPath)

	got := mustSuppress(t, []Member{app, lib})
	if len(got.Records) != 1 {
		t.Fatalf("fixture did not suppress, so the snapshot proves nothing: %+v", got)
	}
	// Include the ladder run over the re-pointed edges: it reads the owner's
	// index but must not write the consumer's either.
	if _, err := Ladder(app, []Member{app, lib}, got.Repoint["app"]); err != nil {
		t.Fatal(err)
	}

	after := snapshot(t, dbPath)
	if len(after) != len(before) {
		t.Fatalf("sidecar files changed: %v vs %v", keys(before), keys(after))
	}
	for p, b := range before {
		if string(after[p]) != string(b) {
			t.Fatalf("%s changed across the pass (%d -> %d bytes)", p, len(b), len(after[p]))
		}
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- determinism -----------------------------------------------------------

// Two passes over an unchanged workspace must produce identical records: the
// overlay content has to be byte-identical, and DepFiles has no ORDER BY.
func TestSuppressIsDeterministicAcrossNamespaces(t *testing.T) {
	app, _ := buildConsumer(t, "app", []string{"example.com/app"}, []string{"Zeta", "Alpha"},
		vend{namespace: "zeta/lib", prefix: "vendor/zeta", rows: []depRow{{file: "z.go", version: "v9", syms: []string{"Zeta"}}}},
		vend{namespace: "alpha/lib", prefix: "vendor/alpha", rows: []depRow{{file: "a.go", version: "v1", syms: []string{"Alpha"}}}},
	)
	z := newMember(t, "z", []string{"zeta/lib"}, nil, def{"z/z.go", "Zeta", "", 1})
	a := newMember(t, "a", []string{"alpha/lib"}, nil, def{"a/a.go", "Alpha", "", 1})
	members := []Member{app, z, a}

	first := mustSuppress(t, members)
	if len(first.Records) != 2 {
		t.Fatalf("records = %+v, want 2", first.Records)
	}
	// Namespaces are sorted, so alpha/lib precedes zeta/lib regardless of the
	// order DepFiles returned the rows in.
	if first.Records[0].Namespace != "alpha/lib" || first.Records[1].Namespace != "zeta/lib" {
		t.Fatalf("records not in namespace order: %+v", first.Records)
	}
	second := mustSuppress(t, members)
	if len(second.Records) != len(first.Records) {
		t.Fatalf("second pass = %+v, want %+v", second.Records, first.Records)
	}
	for i := range first.Records {
		if first.Records[i] != second.Records[i] {
			t.Fatalf("record %d differs across passes: %+v vs %+v", i, first.Records[i], second.Records[i])
		}
	}
}

// --- the shared boundary rule ---------------------------------------------

// Suppression detection is the SECOND call site of nsPrefix (rung 1 is the
// first), and per the one-invariant-many-sites-drifts learning both are pinned
// against the same table so they cannot drift apart.
func TestSuppressUsesSharedNsPrefixTable(t *testing.T) {
	for _, c := range nsPrefixCases() {
		if c.h == "" {
			// An empty attached namespace is unrepresentable: AttachMap rejects
			// a depmap with no namespace metadata. Covered by TestNsPrefix.
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			app, _ := buildConsumer(t, "app", []string{"consumer.only"}, []string{"Zeta"},
				vend{
					namespace: c.h,
					prefix:    "vendor/dep",
					rows:      []depRow{{file: "z.go", version: "v1", syms: []string{"Zeta"}}},
				})
			owner := newMember(t, "owner", []string{c.ns}, nil, def{"o/z.go", "Zeta", "", 1})

			got := mustSuppress(t, []Member{app, owner})
			if c.want {
				if len(got.Records) != 1 || got.Records[0].OwnerMember != "owner" ||
					got.Records[0].Namespace != c.h {
					t.Fatalf("nsPrefix(%q,%q) is true but records = %+v", c.ns, c.h, got.Records)
				}
				if len(got.Repoint["app"]) != 1 {
					t.Fatalf("nsPrefix(%q,%q) is true but nothing was re-pointed: %+v", c.ns, c.h, got.Repoint)
				}
				return
			}
			if len(got.Records) != 0 || len(got.Repoint) != 0 {
				t.Fatalf("nsPrefix(%q,%q) is false but suppression fired: %+v", c.ns, c.h, got)
			}
		})
	}
}
