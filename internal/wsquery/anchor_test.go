package wsquery

import (
	"errors"
	"strings"
	"testing"

	"codeindex/internal/config"
	"codeindex/internal/query"
	"codeindex/internal/wsfresh"
)

// prefixSession builds a session over a manifest declaring api, shared and web
// without touching disk beyond the manifest: splitMemberPrefix reads nothing
// but the declared ids.
func prefixSession(t *testing.T) *session {
	t.Helper()
	ws := &config.Workspace{Version: 1, Members: []config.Member{
		{ID: "api", Root: "services/api"},
		{ID: "shared", Root: "services/shared"},
		{ID: "web", Root: "services/web"},
	}}
	return sessionFrom("/ws", ws, wsfresh.Report{Resolved: true}, nil)
}

// TestSplitMemberPrefixEveryShape is §3.4's rule, case by case. The
// SplitAnchor column records what the remainder parses to downstream, because
// the rule exists precisely to keep that parse correct — the prefix rule and
// query.SplitAnchor are one grammar split across two functions, and reading
// them apart is how the collision gets reintroduced.
func TestSplitMemberPrefixEveryShape(t *testing.T) {
	s := prefixSession(t)
	cases := []struct {
		anchor     string
		wantMember string
		wantRest   string
		wantName   string
		wantParent string
		why        string
	}{
		{"HandleLogin", "", "HandleLogin", "HandleLogin", "",
			"bare anchor: no colon, nothing to strip"},
		{"api:HandleLogin", "api", "HandleLogin", "HandleLogin", "",
			"declared id, next char is not a colon: stripped"},
		{"api::HandleLogin", "", "api::HandleLogin", "HandleLogin", "api",
			"THE LOAD-BEARING CASE: '::' is SplitAnchor's, not a member prefix. " +
				"Stripping would leave ':HandleLogin', which NEITHER SplitAnchor branch parses"},
		{"api:Type::method", "api", "Type::method", "method", "Type",
			"prefix stripped BEFORE SplitAnchor, so the '::' inside still parses"},
		{"api:Type.method", "api", "Type.method", "method", "Type",
			"same for the dotted form"},
		{"Type::method", "", "Type::method", "method", "Type",
			"no prefix at all: SplitAnchor's own '::' branch"},
		{"Type.method", "", "Type.method", "method", "Type",
			"no colon: SplitAnchor's last-dot branch"},
		{":HandleLogin", "", ":HandleLogin", ":HandleLogin", "",
			"a leading colon has no id before it"},
		{"api:", "", "api:", "api:", "",
			"a trailing colon leaves no anchor behind it; passed through untouched"},
		{"shared:A.B.C", "shared", "A.B.C", "C", "A.B",
			"the remainder's LAST dot splits, so a dotted parent survives"},
	}
	for _, tc := range cases {
		t.Run(tc.anchor, func(t *testing.T) {
			gotMember, gotRest, err := s.splitMemberPrefix(tc.anchor)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.why, err)
			}
			if gotMember != tc.wantMember || gotRest != tc.wantRest {
				t.Fatalf("splitMemberPrefix(%q) = (%q, %q), want (%q, %q)\nwhy: %s",
					tc.anchor, gotMember, gotRest, tc.wantMember, tc.wantRest, tc.why)
			}
			name, parent := query.SplitAnchor(gotRest)
			if name != tc.wantName || parent != tc.wantParent {
				t.Errorf("SplitAnchor(%q) = (%q, %q), want (%q, %q)\nwhy: %s",
					gotRest, name, parent, tc.wantName, tc.wantParent, tc.why)
			}
		})
	}
}

// TestSplitMemberPrefixUnknownIDListsTheKnownOnes: the whole failure mode is a
// user who guessed an id, so an error that does not say what the ids ARE just
// sends them to the manifest.
func TestSplitMemberPrefixUnknownIDListsTheKnownOnes(t *testing.T) {
	s := prefixSession(t)
	_, _, err := s.splitMemberPrefix("nope:HandleLogin")
	if err == nil {
		t.Fatal("unknown member prefix returned no error")
	}
	var unknown *UnknownMemberError
	if !errors.As(err, &unknown) {
		t.Fatalf("error is %T, want *UnknownMemberError", err)
	}
	if unknown.ID != "nope" {
		t.Errorf("UnknownMemberError.ID = %q, want %q", unknown.ID, "nope")
	}
	want := []string{"api", "shared", "web"}
	if len(unknown.Known) != len(want) {
		t.Fatalf("Known = %v, want %v (manifest order)", unknown.Known, want)
	}
	for i := range want {
		if unknown.Known[i] != want[i] {
			t.Errorf("Known[%d] = %q, want %q (manifest order)", i, unknown.Known[i], want[i])
		}
	}
	for _, id := range want {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("message does not list %q: %s", id, err.Error())
		}
	}
}

// TestMemberRelPathIsWsPathsInverse pins the file-anchor mapping, including the
// "." root that D7's single-member bar depends on.
func TestMemberRelPathIsWsPathsInverse(t *testing.T) {
	cases := []struct {
		root, wsPath string
		want         string
		ok           bool
	}{
		{".", "a/b.go", "a/b.go", true},
		{"services/api", "services/api/handlers/u.go", "handlers/u.go", true},
		{"services/api", "services/web/u.go", "", false},
		{"services/api", "services/apiary/u.go", "", false},
		{"../api", "../api/u.go", "u.go", true},
	}
	for _, tc := range cases {
		got, ok := memberRelPath(tc.root, tc.wsPath)
		if ok != tc.ok || got != tc.want {
			t.Errorf("memberRelPath(%q, %q) = (%q, %t), want (%q, %t)",
				tc.root, tc.wsPath, got, ok, tc.want, tc.ok)
		}
	}
}
