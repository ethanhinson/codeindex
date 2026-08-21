package wsquery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/query"
	"codeindex/internal/wsfresh"
)

// Cross-member truncation disclosure (rows_withheld / members_truncated).
//
// The defect these tests pin: at the default limit a workspace fan-out could
// print one member's rows and DROP eight members' worth, while the clause said
// members_consulted: <all ten>; members_stale: (none). Nothing on either
// surface distinguished "only werkzeug has hits" from "the limit ate flask
// whole". D4's manifest-order concatenation is frozen and is NOT what these
// tests are about — they assert only that the cut is DISCLOSED.
//
// # The fixture must OVERFLOW, and that is the whole point
//
// A fixture that fits inside `limit` never enters the truncation branch, which
// is precisely how this shipped undetected. skewFixture's FIRST member alone
// exceeds the limit used below, so the second member is cut out ENTIRELY — the
// member-skew case, not a generic "list was long" case.

const (
	skewMemberBig   = "big"
	skewMemberSmall = "small"
	// skewLimit is smaller than skewMemberBig's own match count, so big alone
	// fills the budget and small contributes nothing to the printed answer.
	skewLimit = 3
	// skewBigMatches must exceed skewLimit; skewSmallMatches must be > 0 so
	// there is a member to name as truncated OUT.
	skewBigMatches   = 5
	skewSmallMatches = 2
)

// skewFixture writes a two-member workspace whose FIRST manifest member has
// more matches than `limit` on its own. Both members are indexed for real; no
// cross-edges are needed, because truncation is a fan-out property.
func skewFixture(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "members": [
    {"id": "big", "root": "services/big"},
    {"id": "small", "root": "services/small"}
  ]
}
`
	if err := os.WriteFile(filepath.Join(ws, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(member string, n int) {
		var b strings.Builder
		b.WriteString("package " + member + "\n\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "func WidgetThing%d() {}\n\n", i)
		}
		p := filepath.Join(ws, "services", member, "widgets.go")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := query.Fresh(filepath.Dir(p)); err != nil {
			t.Fatalf("building member %s: %v", member, err)
		}
	}
	write(skewMemberBig, skewBigMatches)
	write(skewMemberSmall, skewSmallMatches)
	return ws
}

func skewSession(t *testing.T, ws string) *session {
	t.Helper()
	w, err := config.LoadWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	return sessionFrom(ws, w, wsfresh.Report{Resolved: true}, nil)
}

// TestFindDisclosesWhichMembersTheLimitTruncatedOut is the regression test for
// the defect: member 1 alone overflows `limit`, member 2's rows are discarded
// entirely, and the clause has to SAY SO — on both the text surface and the
// JSON sibling, the way keys_unmapped does.
func TestFindDisclosesWhichMembersTheLimitTruncatedOut(t *testing.T) {
	defer cleanFreshen(t)()
	ws := skewFixture(t)
	s := skewSession(t, ws)

	a, err := findWorkspace(s, "WidgetThing", "", "", skewLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Results) != skewLimit {
		t.Fatalf("find returned %d rows at limit %d; the fixture must OVERFLOW or this test is vacuous",
			len(a.Results), skewLimit)
	}
	for _, r := range a.Results {
		if r.Repo != skewMemberBig {
			t.Fatalf("printed row from %q; the first member was expected to fill the budget", r.Repo)
		}
	}

	c := s.clause("find")
	if c.RowsWithheld != skewSmallMatches {
		t.Errorf("rows_withheld = %d, want %d", c.RowsWithheld, skewSmallMatches)
	}
	if got, want := strings.Join(c.MembersTruncated, ","), skewMemberSmall; got != want {
		t.Errorf("members_truncated = %q, want %q — the member cut out ENTIRELY must be named", got, want)
	}
	if want := "; rows_withheld: 2; members_truncated: small"; !strings.Contains(c.String(), want) {
		t.Errorf("clause text %q lacks %q", c.String(), want)
	}
	if want := `"rows_withheld":2,"members_truncated":["small"]`; !strings.Contains(marshalClause(t, c), want) {
		t.Errorf("clause JSON %s lacks %q", marshalClause(t, c), want)
	}
}

// TestGrepDisclosesTruncationOfTheLaterMembers pins grep — the verb the owner
// smoke ran.
//
// It also pins the SCOPE of the count. `big` is not named here even at limit 2,
// because fanout.go caps each member at `limit` BEFORE concatenating: big
// contributed exactly 2 rows and both survived. Rows big never handed over were
// dropped by its own per-member cap, which is the pre-existing repo-mode-shaped
// behaviour `... (+N more; raise limit)` speaks for — not by the cross-member
// cut this field discloses. Counting them here would make rows_withheld a
// number nothing in the answer can be reconciled against.
func TestGrepDisclosesTruncationOfTheLaterMembers(t *testing.T) {
	defer cleanFreshen(t)()
	ws := skewFixture(t)
	s := skewSession(t, ws)

	const limit = 2 // < skewBigMatches, so big alone fills the budget
	a, err := grepWorkspace(s, "WidgetThing", limit, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Groups) != limit {
		t.Fatalf("grep returned %d groups at limit %d; fixture must overflow", len(a.Groups), limit)
	}
	c := s.clause("grep")
	if got, want := strings.Join(c.MembersTruncated, ","), skewMemberSmall; got != want {
		t.Errorf("members_truncated = %q, want %q", got, want)
	}
	if c.RowsWithheld != skewSmallMatches {
		t.Errorf("rows_withheld = %d, want %d", c.RowsWithheld, skewSmallMatches)
	}
}

// TestAnchorVerbTruncationIsDisclosed keeps the disclosure from being
// find/grep trivia: any workspace list `limit` cuts discloses the same way.
func TestAnchorVerbTruncationIsDisclosed(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)
	s := fanoutSession(t, ws)

	full, err := callersWorkspace(s, "Target", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Callers) < 2 {
		t.Fatalf("fixture has %d callers, need >= 2", len(full.Callers))
	}
	s2 := fanoutSession(t, ws)
	if _, err := callersWorkspace(s2, "Target", 1); err != nil {
		t.Fatal(err)
	}
	c := s2.clause("callers")
	if c.RowsWithheld == 0 {
		t.Errorf("callers truncated to 1 of %d but rows_withheld = 0", len(full.Callers))
	}
	if len(c.MembersTruncated) == 0 {
		t.Errorf("callers truncated but members_truncated is empty; clause = %q", c.String())
	}

	// nav builds its Files list in its OWN loop rather than through
	// callersUnion's addFile, so it is a second place a []string list can be
	// cut with no Repo field to attribute it. A withheld file that names nobody
	// is half a disclosure.
	sn := fanoutSession(t, ws)
	if _, err := navWorkspace(sn, "Target", 1); err != nil {
		t.Fatal(err)
	}
	cn := sn.clause("nav")
	if cn.RowsWithheld == 0 || len(cn.MembersTruncated) == 0 {
		t.Errorf("nav at limit 1 disclosed %d rows / %v members; clause = %q",
			cn.RowsWithheld, cn.MembersTruncated, cn.String())
	}
}

// TestWithheldFileRowsAreAttributedToAMember guards the []string lists
// specifically.
//
// It is a direct test of the seam rather than an assertion on a whole answer,
// and deliberately so: in every real answer that truncates a FILE list, the
// CALLER list truncates too, so members_truncated is already non-empty and an
// end-to-end assertion cannot fail when the file rows go unattributed. Dropping
// the fileOwner record leaves the count right and the names silently short —
// which is the exact failure mode this whole change exists to prevent — so it
// has to be caught here.
func TestWithheldFileRowsAreAttributedToAMember(t *testing.T) {
	defer cleanFreshen(t)()
	ws := unionFixture(t)
	s := fanoutSession(t, ws)
	u, err := openUnion(s)
	if err != nil {
		t.Fatal(err)
	}
	defer u.close()

	var files []string
	add := u.fileAdder(&files)
	if !add(wsMemberWeb, "services/web/a.go") {
		t.Fatal("first add reported not appended")
	}
	if add(wsMemberAPI, "services/web/a.go") {
		t.Error("duplicate path reported as appended")
	}
	if add(wsMemberAPI, "") {
		t.Error("empty path reported as appended")
	}
	if !add(wsMemberAPI, "services/api/a.go") {
		t.Fatal("second member's path reported not appended")
	}
	if got := u.fileOwnerOf("services/api/a.go"); got != wsMemberAPI {
		t.Errorf("fileOwnerOf = %q, want %q", got, wsMemberAPI)
	}

	// The cut keeps web's path and withholds api's; api must be NAMED, not
	// merely counted.
	kept := truncateOwned(s, files, 1, u.fileOwnerOf)
	if len(kept) != 1 {
		t.Fatalf("kept %d paths, want 1", len(kept))
	}
	if s.rowsWithheld != 1 {
		t.Errorf("rowsWithheld = %d, want 1", s.rowsWithheld)
	}
	if got := s.clause("callers").MembersTruncated; len(got) != 1 || got[0] != wsMemberAPI {
		t.Errorf("members_truncated = %v, want [%s] — a withheld file row must name its member",
			got, wsMemberAPI)
	}
}

// TestUntruncatedWorkspaceAnswerDisclosesNothing is the no-false-alarm bar. A
// field that warns on every answer is a field the reader learns to skip — the
// same reasoning that keeps Dirty out of members_stale when Resolved is true.
func TestUntruncatedWorkspaceAnswerDisclosesNothing(t *testing.T) {
	defer cleanFreshen(t)()
	ws := skewFixture(t)
	s := skewSession(t, ws)

	a, err := findWorkspace(s, "WidgetThing", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Results) != skewBigMatches+skewSmallMatches {
		t.Fatalf("got %d rows, want the whole union %d", len(a.Results), skewBigMatches+skewSmallMatches)
	}
	c := s.clause("find")
	if c.RowsWithheld != 0 || len(c.MembersTruncated) != 0 {
		t.Fatalf("untruncated answer disclosed truncation: %+v", c)
	}
	for _, bad := range []string{"rows_withheld", "members_truncated"} {
		if strings.Contains(c.String(), bad) {
			t.Errorf("clause text %q mentions %s with nothing withheld", c.String(), bad)
		}
		if strings.Contains(marshalClause(t, c), bad) {
			t.Errorf("clause JSON %s mentions %s with nothing withheld", marshalClause(t, c), bad)
		}
	}
}

// TestRepoModeEmitsNoTruncationDisclosure is the repo-mode non-regression bar
// at this layer: repo mode builds no clause and no wsquery.Answer, so a
// truncated repo-mode answer's text and JSON are byte-identical to what they
// were before this field existed. internal/query's goldens pin the nine
// renderers; this pins that the workspace layer adds nothing on that path.
func TestRepoModeEmitsNoTruncationDisclosure(t *testing.T) {
	defer cleanFreshen(t)()
	ws := skewFixture(t)
	repo := filepath.Join(ws, "services", skewMemberBig)

	a, err := query.Find(repo, "WidgetThing", "", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Results) != 2 {
		t.Fatalf("repo-mode find returned %d rows, want a TRUNCATED 2", len(a.Results))
	}
	js, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"rows_withheld", "members_truncated", "workspace"} {
		if strings.Contains(a.Text(), bad) {
			t.Errorf("repo-mode text mentions %s:\n%s", bad, a.Text())
		}
		if strings.Contains(string(js), bad) {
			t.Errorf("repo-mode JSON mentions %s: %s", bad, js)
		}
	}
}
