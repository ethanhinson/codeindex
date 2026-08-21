package wsquery

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"codeindex/internal/query"
	"codeindex/internal/wsfresh"
)

// The D7 freshness property (§7.3) — the merge gate's hard fail.
//
//	Mutate one member, query from another, with NO EXPLICIT REBUILD.
//	The answer must EITHER reflect the mutation OR its coverage clause must
//	NAME THAT MEMBER STALE. Silent staleness is a HARD FAIL.
//
// It is written as a PROPERTY OVER THE UNION VERBS rather than one scripted
// scenario: eight verbs x four workspace states, each classified by the same
// rule. A scenario proves one path; the property is what the gate reads.
//
// # How "query from another" is realised
//
// Every probe below is issued at the WORKSPACE ROOT — never at the mutated
// member's own root, and never after an explicit build of it — and every anchor
// is a BARE name that BOTH members define. So each answer is genuinely composed
// from both members' databases (members_consulted names both), and the mutated
// member's contribution is one half of an answer the other half of which is
// already correct. That is the shape the bar is about: a reader trusting the
// whole answer because most of it is right.
//
// # Enumerated outcomes, never a skip
//
// Per the repo learning capability-skips-assert-the-reason, each arm declares
// the outcomes it accepts BY NAME and any other outcome fails. There is no
// fall-through and no t.Skip: an arm that produced neither reflection nor the
// disclosure it expected is a finding, not an environment quirk.

// d7Outcome is how one (verb, workspace-state) pair discharged the property.
type d7Outcome string

const (
	// d7Reflected: the answer carries the mutation. The freshness contract
	// held and no disclosure was owed.
	d7Reflected d7Outcome = "reflected the mutation"
	// d7NamedStale: the answer does not carry the mutation, and the coverage
	// clause names the mutated member in members_stale. This is the honest
	// degrade §4.2/§4.3 promise.
	d7NamedStale d7Outcome = "disclosed: members_stale names the member"
	// d7SilentlyStale: neither. THIS IS THE HARD FAIL — an answer that is
	// wrong and says nothing about it is the single failure D7 exists to
	// prevent, and it is worse than an answer that refuses.
	d7SilentlyStale d7Outcome = "SILENTLY STALE — neither reflected nor disclosed"
)

// d7Classify is the property itself, as a total function over the two
// observations that decide it. It is separated from the plumbing so it can be
// exercised directly (TestD7ClassifierCallsSilentStalenessAFailure) — a
// detector that has never been shown to fire is not a detector.
func d7Classify(reflected bool, c Clause, member string) d7Outcome {
	if reflected {
		return d7Reflected
	}
	for _, id := range c.MembersStale {
		if id == member {
			return d7NamedStale
		}
	}
	return d7SilentlyStale
}

// d7Require asserts that outcome is one of the outcomes this arm ENUMERATED.
// A silent-stale outcome fails loudly and by name; any other unlisted outcome
// fails too, so an arm cannot quietly start passing for a different reason than
// the one it was written for.
func d7Require(t *testing.T, arm, verb string, got d7Outcome, allowed ...d7Outcome) {
	t.Helper()
	if got == d7SilentlyStale {
		t.Errorf("[%s] %s: HARD FAIL — %s. D7's bar is that a workspace answer either "+
			"reflects a member's mutation or names that member stale in its coverage clause.",
			arm, verb, got)
		return
	}
	for _, a := range allowed {
		if got == a {
			return
		}
	}
	t.Errorf("[%s] %s: outcome %q is not one this arm accepts %v — the arm's premise no longer "+
		"holds, so it is no longer testing what it claims", arm, verb, got, allowed)
}

const (
	d7MemberAlpha = "alpha"
	d7MemberBeta  = "beta"
)

// d7AlphaBefore is alpha's source before the mutation. beta carries the same
// shape under its own names, so every anchor the probes use is defined in BOTH
// members and every answer is composed from both.
const d7AlphaBefore = `package alpha

func Shared() {
	alphaHelper()
}

func alphaHelper() {}

type SharedBase struct{}

type AlphaChild struct {
	SharedBase
}
`

// d7AlphaAfter is the mutation. It is deliberately one edit that is visible to
// every union verb at once — a new caller, a new callee, a new extender, a new
// embedded type — so the property runs over one workspace state rather than
// eight subtly different ones.
const d7AlphaAfter = `package alpha

func Shared() {
	alphaHelper()
	d7probeCallee()
}

func alphaHelper() {}

func d7probeCallee() {}

func D7probeCaller() {
	Shared()
}

type SharedBase struct{}

type D7probeParent struct{}

type D7probeChild struct {
	SharedBase
}

type AlphaChild struct {
	SharedBase
	D7probeParent
}
`

const d7Beta = `package beta

func Shared() {
	betaHelper()
}

func betaHelper() {}

type SharedBase struct{}

type BetaChild struct {
	SharedBase
}
`

// d7Fixture writes a two-member workspace, indexes both members, and runs ONE
// real wsfresh.Freshen so the overlay carries a stamp for each member.
//
// The initial freshen is part of the fixture, not part of the test: without it
// the workspace has never been resolved, so "no explicit rebuild since the
// mutation" would be indistinguishable from "never built at all", and the
// available -> unavailable transition arm below would have no stamp to trip on.
func d7Fixture(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".codeindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "members": [
    {"id": "alpha", "root": "services/alpha", "namespaces": ["example.com/alpha"]},
    {"id": "beta", "root": "services/beta", "namespaces": ["example.com/beta"]}
  ]
}
`
	if err := os.WriteFile(filepath.Join(ws, ".codeindex", "workspace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"services/alpha/shared.go": d7AlphaBefore,
		"services/beta/shared.go":  d7Beta,
	} {
		p := filepath.Join(ws, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Each member root is built for real. The fan-out SKIPS a member whose
	// store will not open, so a fixture that left one unbuilt would make every
	// assertion below vacuously true about a member nobody read.
	for _, id := range []string{d7MemberAlpha, d7MemberBeta} {
		if _, err := query.Fresh(filepath.Join(ws, "services", id)); err != nil {
			t.Fatalf("building member %s: %v", id, err)
		}
	}
	rep, err := wsfresh.Freshen(ws)
	if err != nil {
		t.Fatalf("fixture freshen: %v", err)
	}
	if !rep.Resolved {
		t.Fatalf("fixture freshen did not resolve, so no member is stamped: %+v", rep)
	}
	return ws
}

// d7Mutate rewrites alpha's source. NOTHING rebuilds it: that is the point.
func d7Mutate(t *testing.T, ws string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(ws, "services", d7MemberAlpha, "shared.go"),
		[]byte(d7AlphaAfter), 0o644); err != nil {
		t.Fatal(err)
	}
}

// d7DropIndex removes alpha's index while leaving its (mutated) source in
// place: the member is present but unopenable.
func d7DropIndex(t *testing.T, ws string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(ws, "services", d7MemberAlpha, ".codeindex")); err != nil {
		t.Fatal(err)
	}
}

// d7Probe is one union verb, its query at the workspace root, and the
// structural test for whether the answer carries the mutation.
//
// reflected reads the CONCRETE answer, never the rendered text. Substring
// matching would be vacuous for find and grep, whose Text() echoes the query
// string back — "reflected" would then be true even for an empty result set.
type d7Probe struct {
	verb      string
	run       func(ws string) (Emittable, error)
	reflected func(inner any) bool
}

func d7Probes() []d7Probe {
	return []d7Probe{
		{"callers", func(ws string) (Emittable, error) { return CallersStructured(ws, "Shared", 50) },
			func(inner any) bool {
				for _, c := range inner.(*query.CallersAnswer).Callers {
					if c.QName == "D7probeCaller" {
						return true
					}
				}
				return false
			}},
		{"callees", func(ws string) (Emittable, error) { return CalleesStructured(ws, "Shared", 50) },
			func(inner any) bool {
				for _, c := range inner.(*query.CalleesAnswer).Callees {
					if c.QName == "d7probeCallee" {
						return true
					}
				}
				return false
			}},
		{"impact", func(ws string) (Emittable, error) { return ImpactStructured(ws, "Shared", 50) },
			func(inner any) bool {
				for _, c := range inner.(*query.ImpactAnswer).Callers {
					if c.QName == "D7probeCaller" {
						return true
					}
				}
				return false
			}},
		{"nav", func(ws string) (Emittable, error) { return NavStructured(ws, "Shared", 50) },
			func(inner any) bool {
				for _, c := range inner.(*query.NavAnswer).Callers {
					if c.QName == "D7probeCaller" {
						return true
					}
				}
				return false
			}},
		{"dependents", func(ws string) (Emittable, error) { return DependentsStructured(ws, "SharedBase", 50) },
			func(inner any) bool {
				for _, d := range inner.(*query.DependentsAnswer).Dependents {
					if d.QName == "D7probeChild" {
						return true
					}
				}
				return false
			}},
		{"deps", func(ws string) (Emittable, error) { return DepsStructured(ws, "AlphaChild", 50) },
			func(inner any) bool {
				for _, s := range inner.(*query.DepsAnswer).Sections {
					for _, d := range s.Deps {
						if d.Target == "D7probeParent" {
							return true
						}
					}
				}
				return false
			}},
		{"find", func(ws string) (Emittable, error) { return FindStructured(ws, "D7probeCaller", "", "", 50) },
			func(inner any) bool {
				for _, r := range inner.(*query.FindAnswer).Results {
					if r.QName == "D7probeCaller" {
						return true
					}
				}
				return false
			}},
		{"grep", func(ws string) (Emittable, error) { return GrepStructured(ws, "D7probeCaller", 50, false) },
			func(inner any) bool {
				for _, g := range inner.(*query.GrepAnswer).Groups {
					if g.QName == "D7probeCaller" {
						return true
					}
				}
				return false
			}},
	}
}

// d7Run executes one probe and returns its outcome plus the clause it came
// with, so an arm can assert BOTH the property and the specific disclosure it
// expects to see behind it.
func d7Run(t *testing.T, ws string, p d7Probe) (d7Outcome, Clause) {
	t.Helper()
	e, err := p.run(ws)
	if err != nil {
		t.Fatalf("%s: %v", p.verb, err)
	}
	a, ok := e.(Answer)
	if !ok {
		// A workspace root must produce the clause-carrying pairing. Getting a
		// bare inner answer here would mean the clause was never computed, and
		// the property would be unfalsifiable rather than satisfied.
		t.Fatalf("%s on a workspace root returned %T, not the clause-carrying Answer", p.verb, e)
	}
	return d7Classify(p.reflected(a.Inner), a.Clause, d7MemberAlpha), a.Clause
}

// TestD7FreshnessPropertyWholeWorkspaceFreshenReflectsTheMutation is the
// POSITIVE CONTROL of the whole property, and it is not optional decoration:
// arms that accept a disclosure would pass just as happily against a probe that
// can never fire. This arm accepts NOTHING BUT reflection, for every verb, so
// each probe is proven able to observe the mutation before any other arm is
// allowed to conclude that it could not.
func TestD7FreshnessPropertyWholeWorkspaceFreshenReflectsTheMutation(t *testing.T) {
	ws := d7Fixture(t)
	d7Mutate(t, ws)

	for _, p := range d7Probes() {
		got, c := d7Run(t, ws, p)
		d7Require(t, "whole-workspace freshen", p.verb, got, d7Reflected)
		if len(c.MembersStale) != 0 {
			t.Errorf("%s: members_stale = %v on a freshened workspace; a permanently stale-looking "+
				"clause teaches the reader to ignore the field", p.verb, c.MembersStale)
		}
	}
}

// TestD7FreshnessPropertyFreshenFailureNamesEveryDeclaredMember is §4.2's
// degrade path, which §7.3 names explicitly as part of the property.
//
// The freshen failure is injected through the freshenWorkspace package-var seam
// — the degrade path cannot be provoked reliably from a fixture, and the
// alternative (trusting a partial Report) is exactly the quietly-wrong verdict
// wsfresh's own step 5d comment warns about.
//
// alpha's INDEX is dropped alongside, and that is what makes the disclosure
// load-bearing rather than incidental: the per-repo query.Fresh that every
// member read runs through would otherwise pick the mutation up on its own, and
// the arm would pass by reflecting rather than by disclosing.
func TestD7FreshnessPropertyFreshenFailureNamesEveryDeclaredMember(t *testing.T) {
	ws := d7Fixture(t)
	d7Mutate(t, ws)
	d7DropIndex(t, ws)

	injected := fmt.Errorf("injected: member %q would not fold mid-flight", d7MemberAlpha)
	defer stubFreshen(t, func(string) (wsfresh.Report, error) {
		// A PARTIAL report, as Freshen really returns on its error paths. The
		// clause must not read it as authoritative.
		return wsfresh.Report{Dirty: []string{d7MemberAlpha}}, injected
	})()

	for _, p := range d7Probes() {
		got, c := d7Run(t, ws, p)
		d7Require(t, "freshen failed", p.verb, got, d7NamedStale)

		if c.FreshenFailed != injected.Error() {
			t.Errorf("%s: freshen_failed = %q, want the error verbatim %q",
				p.verb, c.FreshenFailed, injected.Error())
		}
		// EVERY declared member, not just the ones the partial report listed.
		// Naming only alpha here would be the partial report being trusted.
		want := []string{d7MemberAlpha, d7MemberBeta}
		if !reflect.DeepEqual(c.MembersStale, want) {
			t.Errorf("%s: members_stale = %v, want every declared member %v — a failed freshen "+
				"cannot vouch for any member, including the ones its partial report omits",
				p.verb, c.MembersStale, want)
		}
	}
}

// TestD7FreshnessPropertyMissingMemberIsNamedStale is the declared-but-absent
// state: alpha's whole directory is gone, so no per-member freshen can rescue
// the answer and the disclosure is the only thing standing between the reader
// and a confidently incomplete union.
//
// The real wsfresh.Freshen runs here — no stub — and the arm queries TWICE,
// because MembersMissing is recomputed from the manifest on every pass and the
// disclosure must therefore be durable rather than one-shot. (Contrast
// TestBYDESIGNAnUnindexedMemberIsNamedStaleOnlyOnTheTransitionPass below, where
// it is not.)
func TestD7FreshnessPropertyMissingMemberIsNamedStale(t *testing.T) {
	ws := d7Fixture(t)
	if err := os.RemoveAll(filepath.Join(ws, "services", d7MemberAlpha)); err != nil {
		t.Fatal(err)
	}

	for _, pass := range []string{"first pass", "second pass"} {
		for _, p := range d7Probes() {
			got, c := d7Run(t, ws, p)
			d7Require(t, "member missing / "+pass, p.verb, got, d7NamedStale)
			if got := c.MembersConsulted; !reflect.DeepEqual(got, []string{d7MemberBeta}) {
				t.Errorf("[%s] %s: members_consulted = %v, want only the surviving member",
					pass, p.verb, got)
			}
		}
	}
}

// TestD7StaleStampedMemberIsNamedOnTheTransitionPass is the
// available -> unavailable transition: alpha is present and mutated, but its
// index is gone, and an earlier pass stamped it. wsfresh reports it
// StaleStamped, which §4.3 puts in the four-way union, so the clause names it.
//
// Only ONE probe runs here, and deliberately so: the very pass that reports
// StaleStamped also triggers the whole-pass resolution that RETIRES the stamp,
// so the disclosure exists exactly once. That is not a defect of this test —
// it is behaviour, and the next test pins it.
func TestD7StaleStampedMemberIsNamedOnTheTransitionPass(t *testing.T) {
	ws := d7Fixture(t)
	d7Mutate(t, ws)
	d7DropIndex(t, ws)

	p := d7Probes()[0] // callers
	got, c := d7Run(t, ws, p)
	d7Require(t, "stale-stamped transition", p.verb, got, d7NamedStale)
	if len(c.MembersStale) == 0 {
		t.Fatalf("clause named nothing stale on the transition pass: %+v", c)
	}
}

// TestBYDESIGNAnUnindexedMemberIsNamedStaleOnlyOnTheTransitionPass pins
// behaviour that this task DISCOVERED and deliberately did not change. A reader
// who finds this test must not read it as an aspiration, and must not "fix" it
// here — see the prerequisite at the bottom of this comment.
//
// # What the code does
//
// A member that is present but whose index will not open is counted in
// wsfresh.Report.MembersUnindexed, which §4.3 excludes from members_stale by
// design, on the argument that such a member is "covered by StaleStamped when
// it previously contributed rows, and by boundary when it never did".
//
// The StaleStamped half of that cover is ONE-SHOT. The pass that reports
// StaleStamped also trips wsfresh's gate, and the whole-pass wsresolve.Resolve
// it triggers PRUNES the stamp it fired on. From the next query onward the
// member is unindexed and unstamped: not Dirty (never re-folded), not
// StaleStamped (no stamp left), not MembersMissing (still on disk). So
// members_stale is empty while the answer keeps omitting that member's rows.
//
// # Why it is pinned rather than fixed
//
// The exclusion is an explicit, argued design decision (§4.3 and assumption 6),
// MembersUnindexed is a COUNT with no id slice to draw from, and this task is
// tests-only. Widening the union would contradict the spec, not implement it.
//
// # The surviving disclosure, asserted here so it cannot rot away
//
// members_consulted still omits the member while the manifest still declares
// it, so the difference between the two sets is the signal a reader has. This
// test asserts that difference exists; if a later change makes members_consulted
// list unreadable members, this test reddens and the last disclosure channel is
// gone.
//
// # Prerequisite for tightening it
//
// An additive MembersUnindexedIDs slice on wsfresh.Report — written at the same
// site as the count, exactly as MembersFreshenFailedIDs already is — plus an
// amendment to §4.3 admitting it as a fifth set in the union. Both are outside
// this slice.
func TestBYDESIGNAnUnindexedMemberIsNamedStaleOnlyOnTheTransitionPass(t *testing.T) {
	ws := d7Fixture(t)
	d7Mutate(t, ws)
	d7DropIndex(t, ws)

	p := d7Probes()[0] // callers
	// Pass 1: the transition. The stamp is still there, so the clause discloses
	// — and this pass is what retires the stamp.
	if got, _ := d7Run(t, ws, p); got != d7NamedStale {
		t.Fatalf("premise broken: the transition pass did not disclose (%s); the characterization "+
			"below is about what happens AFTER that disclosure", got)
	}

	// Pass 2: the stamp is gone. This is what the code DOES today.
	got, c := d7Run(t, ws, p)
	if got != d7SilentlyStale {
		t.Fatalf("this characterization no longer holds: pass 2 gave %q. If members_stale now "+
			"names the unindexed member, the §4.3 exclusion was tightened — DELETE this test and "+
			"fold the case into the property arms above.", got)
	}
	if len(c.MembersStale) != 0 {
		t.Errorf("members_stale = %v, want empty — the documented consequence of §4.3 excluding "+
			"MembersUnindexed once the stamp has been retired", c.MembersStale)
	}
	// The surviving disclosure channel: consulted omits alpha, the manifest
	// does not. A reader can still tell a member went unread.
	if !reflect.DeepEqual(c.MembersConsulted, []string{d7MemberBeta}) {
		t.Errorf("members_consulted = %v, want only %q — this is the LAST signal that alpha went "+
			"unread, and losing it would make the omission undetectable",
			c.MembersConsulted, d7MemberBeta)
	}
}

// TestD7DetectorRedensWhenTheFreshenLiesAboutAnUnreadableMember is the mutation
// evidence for the property above: the guard is defeated and observed to fire.
//
// The thing being guarded is the pairing of the whole-workspace freshen with the
// coverage clause. It is defeated by replacing the freshen with one that reports
// a CLEAN, RESOLVED workspace while alpha is in fact mutated and unreadable —
// the exact "quietly WRONG clean verdict" wsfresh's step 5d comment refuses to
// produce. Every index-backed verb then answers from beta alone and discloses
// nothing, and d7Classify must call that out.
//
// If this test ever passes with d7Reflected or d7NamedStale, the property arms
// above are not testing anything: their acceptance of a disclosure would be
// unfalsifiable because silent staleness could not be produced at all.
func TestD7DetectorRedensWhenTheFreshenLiesAboutAnUnreadableMember(t *testing.T) {
	ws := d7Fixture(t)
	d7Mutate(t, ws)
	d7DropIndex(t, ws)
	defer cleanFreshen(t)()

	for _, p := range d7Probes() {
		e, err := p.run(ws)
		if err != nil {
			t.Fatalf("%s: %v", p.verb, err)
		}
		a := e.(Answer)
		got := d7Classify(p.reflected(a.Inner), a.Clause, d7MemberAlpha)
		if got != d7SilentlyStale {
			t.Errorf("%s: a lying-clean freshen over an unreadable, mutated member classified as "+
				"%q — the property arms cannot fail, so they prove nothing", p.verb, got)
		}
	}
}

// TestD7ClassifierCallsSilentStalenessAFailure exercises the rule itself over
// all four of its input combinations, so the classifier cannot be quietly
// weakened into one that returns d7Reflected for everything.
func TestD7ClassifierCallsSilentStalenessAFailure(t *testing.T) {
	clean := Clause{MembersConsulted: []string{"beta"}, Boundary: Boundary}
	names := Clause{MembersConsulted: []string{"beta"}, MembersStale: []string{"alpha"}, Boundary: Boundary}

	cases := []struct {
		name      string
		reflected bool
		clause    Clause
		want      d7Outcome
	}{
		{"reflected, nothing stale", true, clean, d7Reflected},
		{"reflected and disclosed", true, names, d7Reflected},
		{"not reflected but disclosed", false, names, d7NamedStale},
		{"not reflected, not disclosed", false, clean, d7SilentlyStale},
	}
	for _, c := range cases {
		if got := d7Classify(c.reflected, c.clause, "alpha"); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	// A clause that names some OTHER member stale is not a disclosure about
	// alpha. Without this the classifier could pass by checking len() != 0.
	other := Clause{MembersStale: []string{"gamma"}, Boundary: Boundary}
	if got := d7Classify(false, other, "alpha"); got != d7SilentlyStale {
		t.Errorf("a clause naming another member stale counted as a disclosure about alpha: %q", got)
	}
}
