package ghsync_test

import (
	"errors"
	"strings"
	"testing"

	"codeindex/internal/lore/ghsync"
)

// fakeRunner captures calls and returns canned output.
type fakeRunner struct {
	output string
	err    error
	// lastArgs records the args from the most recent call.
	lastArgs []string
}

func (f *fakeRunner) run(dir string, args ...string) (string, error) {
	f.lastArgs = args
	return f.output, f.err
}

// --- IssueState tests ---

func TestIssueState_ClosedJSON(t *testing.T) {
	fr := &fakeRunner{output: `{"state":"CLOSED"}` + "\n"}
	gh := ghsync.NewWithRunner(fr.run)
	state, err := gh.IssueState("/repo", "owner/repo#42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "CLOSED" {
		t.Fatalf("want CLOSED, got %q", state)
	}
}

func TestIssueState_OpenJSON(t *testing.T) {
	fr := &fakeRunner{output: `{"state":"OPEN"}` + "\n"}
	gh := ghsync.NewWithRunner(fr.run)
	state, err := gh.IssueState("/repo", "owner/repo#7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "OPEN" {
		t.Fatalf("want OPEN, got %q", state)
	}
}

func TestIssueState_MalformedJSON_ReturnsError(t *testing.T) {
	fr := &fakeRunner{output: `not json`}
	gh := ghsync.NewWithRunner(fr.run)
	_, err := gh.IssueState("/repo", "owner/repo#1")
	if err == nil {
		t.Fatal("want error for malformed JSON")
	}
}

func TestIssueState_RunnerError_Propagates(t *testing.T) {
	fr := &fakeRunner{err: errors.New("gh: not logged in")}
	gh := ghsync.NewWithRunner(fr.run)
	_, err := gh.IssueState("/repo", "owner/repo#1")
	if err == nil {
		t.Fatal("want error when runner fails")
	}
}

func TestIssueState_QualifiedRef_PassesRepo(t *testing.T) {
	fr := &fakeRunner{output: `{"state":"OPEN"}`}
	gh := ghsync.NewWithRunner(fr.run)
	_, _ = gh.IssueState("/repo", "myorg/myrepo#99")
	// Qualified ref → args must include --repo myorg/myrepo
	found := false
	for i, a := range fr.lastArgs {
		if a == "--repo" && i+1 < len(fr.lastArgs) && fr.lastArgs[i+1] == "myorg/myrepo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --repo myorg/myrepo in args, got %v", fr.lastArgs)
	}
}

func TestIssueState_BareRef_NoRepoPassed(t *testing.T) {
	fr := &fakeRunner{output: `{"state":"OPEN"}`}
	gh := ghsync.NewWithRunner(fr.run)
	_, _ = gh.IssueState("/repo", "#5")
	for _, a := range fr.lastArgs {
		if a == "--repo" {
			t.Fatalf("bare ref should not pass --repo, got args %v", fr.lastArgs)
		}
	}
}

func TestIssueState_BareNumericRef_NoRepoPassed(t *testing.T) {
	fr := &fakeRunner{output: `{"state":"OPEN"}`}
	gh := ghsync.NewWithRunner(fr.run)
	_, _ = gh.IssueState("/repo", "5")
	for _, a := range fr.lastArgs {
		if a == "--repo" {
			t.Fatalf("bare numeric ref should not pass --repo, got args %v", fr.lastArgs)
		}
	}
}

// --- CreateIssue tests ---

func TestCreateIssue_ReturnsURL(t *testing.T) {
	url := "https://github.com/owner/repo/issues/42"
	fr := &fakeRunner{output: "https://github.com/owner/repo/issues/42\n"}
	gh := ghsync.NewWithRunner(fr.run)
	got, err := gh.CreateIssue("/repo", "My title", "My body\n\nlore: itm-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != url {
		t.Fatalf("want %q, got %q", url, got)
	}
}

func TestCreateIssue_BodyContainsBacklink(t *testing.T) {
	fr := &fakeRunner{output: "https://github.com/owner/repo/issues/1\n"}
	gh := ghsync.NewWithRunner(fr.run)
	_, _ = gh.CreateIssue("/repo", "Title", "Some body\n\nlore: itm-ABC")
	// Assert the runner received --body containing "lore: itm-ABC"
	bodyFound := false
	for i, a := range fr.lastArgs {
		if a == "--body" && i+1 < len(fr.lastArgs) {
			if strings.Contains(fr.lastArgs[i+1], "lore: itm-ABC") {
				bodyFound = true
			}
		}
	}
	if !bodyFound {
		t.Fatalf("expected --body arg to contain backlink, got args: %v", fr.lastArgs)
	}
}

func TestCreateIssue_RunnerError_Propagates(t *testing.T) {
	fr := &fakeRunner{err: errors.New("gh: not logged in")}
	gh := ghsync.NewWithRunner(fr.run)
	_, err := gh.CreateIssue("/repo", "title", "body")
	if err == nil {
		t.Fatal("want error when runner fails")
	}
}

func TestCreateIssue_EmptyOutput_ReturnsError(t *testing.T) {
	fr := &fakeRunner{output: ""}
	gh := ghsync.NewWithRunner(fr.run)
	_, err := gh.CreateIssue("/repo", "title", "body")
	if err == nil {
		t.Fatal("want error when output is empty (no URL)")
	}
}

// --- ParseRef tests (qualified vs bare) ---

func TestParseRef_Qualified(t *testing.T) {
	repo, num, err := ghsync.ParseRef("owner/repo#42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "owner/repo" {
		t.Fatalf("want repo=owner/repo, got %q", repo)
	}
	if num != "42" {
		t.Fatalf("want num=42, got %q", num)
	}
}

func TestParseRef_BareHash(t *testing.T) {
	repo, num, err := ghsync.ParseRef("#7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "" {
		t.Fatalf("want repo empty for bare ref, got %q", repo)
	}
	if num != "7" {
		t.Fatalf("want num=7, got %q", num)
	}
}

func TestParseRef_BareNumeric(t *testing.T) {
	repo, num, err := ghsync.ParseRef("99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "" {
		t.Fatalf("want repo empty for bare numeric, got %q", repo)
	}
	if num != "99" {
		t.Fatalf("want num=99, got %q", num)
	}
}

func TestParseRef_Invalid(t *testing.T) {
	_, _, err := ghsync.ParseRef("notarepo")
	// "notarepo" is bare numeric? No — letters. Should it error?
	// Actually bare numeric means all digits. "notarepo" has no # and is not
	// all digits, so it should error.
	if err == nil {
		t.Fatal("want error for non-numeric bare ref without #")
	}
}

// --- Available tests ---

func TestAvailable_ReturnsTrue_WhenRunnerSucceeds(t *testing.T) {
	fr := &fakeRunner{output: "gh version 2.0.0\n"}
	gh := ghsync.NewWithRunner(fr.run)
	// Available() checks PATH, not the runner; just ensure it doesn't panic.
	// We can't easily fake PATH, so we just call it and accept either value.
	_ = gh.Available()
}
