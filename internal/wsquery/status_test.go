package wsquery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/overlay"
	"codeindex/internal/query"
	"codeindex/internal/wsfresh"
)

// statusWorkspace builds the §6 report's exercise fixture: three declared
// members covering all three axes the report names —
//
//	api   present, INDEXED,   stamped
//	web   present, unindexed, unstamped
//	gone  MISSING,  unindexed, unstamped
//
// plus an overlay carrying one cross-edge, one ambiguity, and TWO suppressions:
// one with a recorded version and one with an EMPTY version. Both suppressions
// are required. A fixture with only the versioned one cannot tell a report that
// renders "version unknown" from one that silently drops the record, which is
// the exact failure the D3 obligation is written against.
func statusWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	members := []config.Member{
		{ID: "api", Root: "services/api", Namespaces: []string{}},
		{ID: "web", Root: "services/web", Namespaces: []string{}},
		{ID: "gone", Root: "services/gone", Namespaces: []string{}},
	}
	if err := config.SaveWorkspace(ws, &config.Workspace{Version: 1, Members: members}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"api", "web"} {
		dir := filepath.Join(ws, "services", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		src := "package " + id + "\n\nfunc Target() {}\n"
		if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// api is indexed for real — a hand-touched empty file would prove only
	// that os.Stat works.
	if _, err := query.Fresh(filepath.Join(ws, "services", "api")); err != nil {
		t.Fatalf("building api: %v", err)
	}

	ov, err := overlay.Open(overlay.Path(ws))
	if err != nil {
		t.Fatal(err)
	}
	defer ov.Close()
	if err := ov.PutCrossEdges([]overlay.CrossEdge{{
		Src:  overlay.SymKey{Member: "web", File: "a.go", QName: "WebOne"},
		Dst:  overlay.SymKey{Member: "api", File: "a.go", QName: "Target"},
		Kind: "calls", Provenance: "cross_repo_import", Confidence: "exact", Line: 4,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ov.PutAmbiguities([]overlay.Ambiguity{{
		Src:     overlay.SymKey{Member: "web", File: "b.go", QName: "WebTwo"},
		RefName: "Target", Kind: "calls", Line: 7,
		Candidates: []overlay.SymKey{
			{Member: "api", File: "a.go", QName: "Target"},
			{Member: "gone", File: "a.go", QName: "Target"},
		},
		Count: 2,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ov.PutSuppressions([]overlay.Suppression{
		{ConsumerMember: "drupal", Namespace: "symfony/http-foundation",
			OwnerMember: "symfony", SuppressedVersion: "v7.1.0"},
		{ConsumerMember: "drupal", Namespace: "symfony/console",
			OwnerMember: "symfony", SuppressedVersion: ""},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ov.PutStamp("api", "merkle-abc123"); err != nil {
		t.Fatal(err)
	}
	return ws
}

// The stamped merkle root is reported AS RECORDED, per member, alongside
// presence and indexed-ness — and no member line claims a dirty/clean verdict,
// which the report is deliberately unable to compute (assumption 12).
func TestWorkspaceStatusReportsPresenceIndexAndRecordedStamp(t *testing.T) {
	ws := statusWorkspace(t)
	st, err := WorkspaceStatus(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Members) != 3 {
		t.Fatalf("want 3 declared members, got %d", len(st.Members))
	}
	want := []MemberStatus{
		{ID: "api", Present: true, Indexed: true, Stamped: true, StampedMerkleRoot: "merkle-abc123"},
		{ID: "web", Present: true, Indexed: false, Stamped: false, StampedMerkleRoot: ""},
		{ID: "gone", Present: false, Indexed: false, Stamped: false, StampedMerkleRoot: ""},
	}
	for i, w := range want {
		got := st.Members[i]
		if got.ID != w.ID {
			t.Fatalf("member %d: manifest order broken: got %q want %q", i, got.ID, w.ID)
		}
		if got.Present != w.Present || got.Indexed != w.Indexed ||
			got.Stamped != w.Stamped || got.StampedMerkleRoot != w.StampedMerkleRoot {
			t.Errorf("member %s: got %+v, want %+v", w.ID, got, w)
		}
		if !filepath.IsAbs(got.Root) {
			t.Errorf("member %s: root %q is not absolute", w.ID, got.Root)
		}
	}
	text := st.Text()
	for _, want := range []string{
		"api: ", "present, indexed, stamped merkle merkle-abc123",
		"web: ", "present, unindexed, no stamp recorded",
		"gone: ", "missing, unindexed, no stamp recorded",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text report missing %q\n---\n%s", want, text)
		}
	}
	// The report must not pretend to a freshness verdict it cannot compute.
	for _, forbidden := range []string{"dirty", "clean"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Errorf("report claims a %q verdict it cannot compute:\n%s", forbidden, text)
		}
	}
}

// The overlay's schema version and its whole-overlay cross-edge / ambiguity
// counts are reported.
func TestWorkspaceStatusReportsSchemaVersionAndCounts(t *testing.T) {
	ws := statusWorkspace(t)
	st, err := WorkspaceStatus(ws)
	if err != nil {
		t.Fatal(err)
	}
	if !st.OverlayPresent || !st.OverlayReadable {
		t.Fatalf("overlay should be present and readable: %+v", st)
	}
	if st.OverlaySchemaVersion != overlay.SchemaVersion() {
		t.Errorf("schema version %d, want %d", st.OverlaySchemaVersion, overlay.SchemaVersion())
	}
	if st.CrossEdges != 1 {
		t.Errorf("cross-edges %d, want 1", st.CrossEdges)
	}
	if st.Ambiguities != 1 {
		t.Errorf("ambiguities %d, want 1", st.Ambiguities)
	}
	text := st.Text()
	for _, want := range []string{"cross-edges: 1", "ambiguities: 1", "schema v"} {
		if !strings.Contains(text, want) {
			t.Errorf("text report missing %q\n---\n%s", want, text)
		}
	}
}

// D3's reporting obligation: one line per suppression, in the §6 sentence
// shape, and an EMPTY SuppressedVersion rendered as "version unknown" rather
// than dropped or blank.
func TestWorkspaceStatusRendersEverySkewLineIncludingUnknownVersion(t *testing.T) {
	ws := statusWorkspace(t)
	st, err := WorkspaceStatus(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Skew) != 2 {
		t.Fatalf("want both suppressions reported, got %d: %+v", len(st.Skew), st.Skew)
	}
	text := st.Text()
	want := []string{
		"drupal vendors symfony/console at version unknown; member symfony wins",
		"drupal vendors symfony/http-foundation at v7.1.0; member symfony wins",
	}
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("skew line missing from the text report: %q\n---\n%s", w, text)
		}
	}
	// The unknown-version record must not degrade into a blank or an elided
	// clause — both read as "the versions agree", the opposite of the fact.
	if strings.Contains(text, "console at ;") || strings.Contains(text, "console at  ") {
		t.Errorf("empty version rendered as a blank rather than \"version unknown\":\n%s", text)
	}
}

// The --json shape carries every reported field, and suppressed_version is
// present even when empty: an omitempty there would make an unknown version
// indistinguishable from no skew at all.
func TestWorkspaceStatusJSONShape(t *testing.T) {
	ws := statusWorkspace(t)
	st, err := WorkspaceStatus(ws)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"workspace", "overlay_present", "overlay_schema_version",
		"overlay_schema_required", "overlay_readable",
		"cross_edges", "ambiguities", "members", "skew",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("json shape missing key %q: %s", key, b)
		}
	}
	members, _ := got["members"].([]any)
	if len(members) != 3 {
		t.Fatalf("json members: got %d, want 3: %s", len(members), b)
	}
	m0, _ := members[0].(map[string]any)
	for _, key := range []string{"id", "root", "present", "indexed", "stamped", "stamped_merkle_root"} {
		if _, ok := m0[key]; !ok {
			t.Errorf("json member shape missing key %q: %s", key, b)
		}
	}
	skew, _ := got["skew"].([]any)
	if len(skew) != 2 {
		t.Fatalf("json skew: got %d, want 2: %s", len(skew), b)
	}
	unknown := false
	for _, raw := range skew {
		s, _ := raw.(map[string]any)
		v, ok := s["suppressed_version"]
		if !ok {
			t.Fatalf("skew record omits suppressed_version entirely: %s", b)
		}
		if v == "" {
			unknown = true
			if txt, _ := s["text"].(string); !strings.Contains(txt, "version unknown") {
				t.Errorf("empty-version skew record's text does not say \"version unknown\": %q", txt)
			}
		}
	}
	if !unknown {
		t.Fatal("fixture lost its empty-version suppression; the omission case is untested")
	}
}

// workspace-status READS STATE AND DOES NOT FRESHEN. Freshening would make the
// verb unusable on exactly the workspace it exists to diagnose — one whose
// freshen is the problem.
//
// The assertion is structural rather than a stub check: against a workspace
// with NO overlay, a freshen would create (and populate) one. Status must
// leave the path untouched and simply report the overlay absent.
func TestWorkspaceStatusDoesNotFreshen(t *testing.T) {
	ws := t.TempDir()
	members := []config.Member{{ID: "api", Root: "services/api", Namespaces: []string{}}}
	if err := config.SaveWorkspace(ws, &config.Workspace{Version: 1, Members: members}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "services", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "services", "api", "a.go"),
		[]byte("package api\n\nfunc Target() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Guard the premise: if a freshen ran, THIS is what it would leave behind.
	if _, err := os.Stat(overlay.Path(ws)); !os.IsNotExist(err) {
		t.Fatalf("fixture already has an overlay; the assertion below proves nothing")
	}

	// Fail loudly if the freshen seam is reached at all.
	restore := stubFreshen(t, func(string) (wsfresh.Report, error) {
		t.Error("workspace-status ran the freshen pass; it must read state only")
		return wsfresh.Report{}, nil
	})
	defer restore()

	st, err := WorkspaceStatus(ws)
	if err != nil {
		t.Fatalf("status must work on an un-freshened workspace: %v", err)
	}
	if st.OverlayPresent {
		t.Errorf("status reported an overlay that does not exist: %+v", st)
	}
	if _, err := os.Stat(overlay.Path(ws)); !os.IsNotExist(err) {
		t.Errorf("workspace-status CREATED the overlay; it must not write")
	}
	if len(st.Members) != 1 || st.Members[0].ID != "api" || st.Members[0].Stamped {
		t.Errorf("member report wrong on an un-freshened workspace: %+v", st.Members)
	}
	if !strings.Contains(st.Text(), "overlay: absent") {
		t.Errorf("text report does not name the absent overlay:\n%s", st.Text())
	}
}

// The refusal a workspace-only verb returns on a repo root NAMES the repo-mode
// verb that answers the same question. A refusal with no forward pointer is a
// dead end.
func TestRefuseRepoRootNamesTheRepoModeVerb(t *testing.T) {
	repo := fixtureRepo(t)
	err := RefuseRepoRoot("workspace-status", repo, "status")
	if err == nil {
		t.Fatal("want a refusal")
	}
	msg := err.Error()
	for _, want := range []string{"codeindex workspace-status", repo, "is a repo, not a workspace",
		"codeindex status " + repo} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q:\n%s", want, msg)
		}
	}
}
