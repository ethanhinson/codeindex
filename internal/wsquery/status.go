package wsquery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeindex/internal/config"
	"codeindex/internal/overlay"
)

// workspace-status: the read-only diagnostic (§6).
//
// # It reads state; it does not freshen
//
// Nothing in this file calls wsfresh.Freshen or newSession, and that is the
// whole point of the verb. workspace-status is what an operator runs when the
// freshen itself is the thing being diagnosed, so a version of it that
// freshens first would be unusable exactly when it is needed. The tests hold
// this directly: a status call against a workspace with no overlay leaves the
// overlay uncreated, which a freshen would not.
//
// # Why the stamp is REPORTED and no dirty verdict is
//
// Per member this reports stamp presence plus the stamped merkle root AS
// RECORDED — explicitly not a "currently dirty" verdict. Dirtiness is by
// definition a fresh merkle re-fold compared against the stamp;
// wsfresh.foldMember is unexported and no exported fold-and-compare exists, so
// reporting a verdict would mean either doing the expensive fold half (which
// falsifies "reads state, does not freshen") or writing a SECOND
// implementation of the dirty predicate — the same second-enforcement-site
// objection §4.1 uses to reject a subset freshen. The honest cheap field is
// the recorded stamp; `codeindex refresh` is what turns it into a verdict
// (assumption 12). Do not add a fold-and-compare export to satisfy this
// surface.
//
// # Why the overlay is never OPENED here
//
// overlay.Open MUTATES: on a schema-version mismatch it deletes the file and
// recreates it empty, and on an absent file it creates one. Both are writes,
// and a diagnostic that rebuilds the artifact it was asked to describe destroys
// the evidence. So this reads the version with overlay.FileSchemaVersion first
// and opens the store only when the file exists AND its version matches what
// this binary writes. Every other case is reported as the state it is.

// MemberStatus is one member's line of the report.
type MemberStatus struct {
	// ID is the manifest id.
	ID string `json:"id"`
	// Root is the member's resolved absolute root. For a MISSING member it is
	// the absolute path the manifest declares — where the member would be —
	// because "missing" is only actionable next to the path that was checked.
	Root string `json:"root"`
	// Present is whether the member root is a directory on disk.
	Present bool `json:"present"`
	// Indexed is whether the member carries a built graph.db.
	Indexed bool `json:"indexed"`
	// Stamped is whether the overlay records a freshness stamp for this
	// member. It is NOT a cleanliness claim; see the file comment.
	Stamped bool `json:"stamped"`
	// StampedMerkleRoot is the merkle root AS RECORDED by the last resolution,
	// empty when Stamped is false. Never compared against a fresh fold here.
	StampedMerkleRoot string `json:"stamped_merkle_root"`
}

// SkewLine is one member/vendor version-skew record, as recorded, plus the
// sentence the text surface prints. This is the D3 reporting obligation the
// suppression record was written for.
type SkewLine struct {
	// ConsumerMember vendored the namespace and lost.
	ConsumerMember string `json:"consumer_member"`
	// Namespace is the contested namespace.
	Namespace string `json:"namespace"`
	// OwnerMember claims the namespace as a member and wins.
	OwnerMember string `json:"owner_member"`
	// SuppressedVersion is the vendored snapshot's version as recorded, "" if
	// unknown. It carries NO omitempty and is NEVER dropped: a skew line whose
	// version silently vanishes reads as "no skew here", which is the opposite
	// of what it records. The text surface renders "" as "version unknown".
	SuppressedVersion string `json:"suppressed_version"`
	// Text is the rendered sentence, so a --json consumer and the text surface
	// cannot disagree about how a record reads.
	Text string `json:"text"`
}

// Status is the whole workspace-status report.
type Status struct {
	// Workspace is the workspace root as given.
	Workspace string `json:"workspace"`
	// OverlayPresent is whether the overlay database file exists at all.
	OverlayPresent bool `json:"overlay_present"`
	// OverlaySchemaVersion is the version embedded in the overlay FILE, 0 when
	// absent. It may differ from OverlaySchemaRequired; see OverlayReadable.
	OverlaySchemaVersion int `json:"overlay_schema_version"`
	// OverlaySchemaRequired is the version this binary writes and requires.
	OverlaySchemaRequired int `json:"overlay_schema_required"`
	// OverlayReadable is whether the overlay's contents were read. False when
	// the file is absent or its schema version does not match — the mismatched
	// file is deliberately left alone rather than rebuilt (see the file
	// comment), so its counts, stamps and suppressions are unavailable, not
	// zero.
	OverlayReadable bool `json:"overlay_readable"`
	// CrossEdges and Ambiguities are the whole-overlay totals, 0 when the
	// overlay is not readable.
	CrossEdges  int `json:"cross_edges"`
	Ambiguities int `json:"ambiguities"`
	// Members is one entry per DECLARED member, in manifest order.
	Members []MemberStatus `json:"members"`
	// Skew is every recorded suppression, in overlay order.
	Skew []SkewLine `json:"skew"`
}

// WorkspaceStatus reads wsRoot's declared state and returns the §6 report. It
// performs no freshen, no resolution, and no write of any kind.
//
// A manifest fault is an ERROR, not a degraded report: with no declared member
// set there is nothing to report on. Everything after the manifest — an absent
// overlay, a schema-mismatched overlay, a missing or unindexed member — is a
// reportable state, because reporting exactly those states is the verb's job.
func WorkspaceStatus(wsRoot string) (Status, error) {
	ws, err := config.LoadWorkspace(wsRoot)
	if err != nil {
		return Status{}, err
	}
	present, _, err := ws.Resolve(wsRoot)
	if err != nil {
		return Status{}, err
	}
	byID := make(map[string]config.ResolvedMember, len(present))
	for _, m := range present {
		byID[m.Member.ID] = m
	}

	st := Status{
		Workspace:             wsRoot,
		OverlaySchemaRequired: overlay.SchemaVersion(),
		Members:               make([]MemberStatus, 0, len(ws.Members)),
		Skew:                  []SkewLine{},
	}

	stamps := map[string]overlay.Stamp{}
	ovPath := overlay.Path(wsRoot)
	if ver, verr := overlay.FileSchemaVersion(ovPath); verr == nil {
		st.OverlayPresent = true
		st.OverlaySchemaVersion = ver
		if ver == overlay.SchemaVersion() {
			if err := readOverlayState(ovPath, &st, stamps); err != nil {
				return Status{}, err
			}
		}
	} else if !os.IsNotExist(verr) {
		return Status{}, verr
	}

	for _, m := range ws.Members {
		ms := MemberStatus{ID: m.ID}
		if rm, ok := byID[m.ID]; ok {
			ms.Present, ms.Root = true, rm.AbsRoot
		} else {
			ms.Root = declaredRoot(wsRoot, m.Root)
		}
		if ms.Present {
			if _, serr := os.Stat(memberIndexPath(ms.Root)); serr == nil {
				ms.Indexed = true
			}
		}
		if s, ok := stamps[m.ID]; ok {
			ms.Stamped, ms.StampedMerkleRoot = true, s.MerkleRoot
		}
		st.Members = append(st.Members, ms)
	}
	return st, nil
}

// readOverlayState fills the overlay-derived half of the report. It is only
// reached for a file that exists at the version this binary writes, so
// overlay.Open here cannot trigger the delete-and-recreate path.
func readOverlayState(ovPath string, st *Status, stamps map[string]overlay.Stamp) error {
	ov, err := overlay.Open(ovPath)
	if err != nil {
		return err
	}
	defer ov.Close()

	edges, ambig, err := ov.Counts()
	if err != nil {
		return err
	}
	st.CrossEdges, st.Ambiguities, st.OverlayReadable = edges, ambig, true

	all, err := ov.Stamps()
	if err != nil {
		return err
	}
	for _, s := range all {
		stamps[s.MemberID] = s
	}

	sup, err := ov.Suppressions()
	if err != nil {
		return err
	}
	for _, x := range sup {
		st.Skew = append(st.Skew, SkewLine{
			ConsumerMember:    x.ConsumerMember,
			Namespace:         x.Namespace,
			OwnerMember:       x.OwnerMember,
			SuppressedVersion: x.SuppressedVersion,
			Text:              SkewSentence(x),
		})
	}
	return nil
}

// SkewSentence renders one suppression as §6's sentence:
//
//	drupal vendors symfony/http-foundation at v7.1.0; member symfony wins
//
// An EMPTY SuppressedVersion renders as "version unknown" and the clause is
// never dropped. An unknown version is still skew — the consumer is carrying
// its own copy of a namespace a member owns — and eliding the phrase would
// leave the sentence reading as though the versions agreed.
func SkewSentence(x overlay.Suppression) string {
	version := x.SuppressedVersion
	if version == "" {
		version = "version unknown"
	}
	return fmt.Sprintf("%s vendors %s at %s; member %s wins",
		x.ConsumerMember, x.Namespace, version, x.OwnerMember)
}

// Text renders the report as the human surface.
func (s Status) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "workspace: %s\n", s.Workspace)

	switch {
	case !s.OverlayPresent:
		fmt.Fprintf(&b, "overlay: absent (run: codeindex refresh %s)\n", s.Workspace)
	case !s.OverlayReadable:
		fmt.Fprintf(&b, "overlay: schema v%d, this binary uses v%d — counts, stamps and skew unavailable until the next refresh rebuilds it\n",
			s.OverlaySchemaVersion, s.OverlaySchemaRequired)
	default:
		fmt.Fprintf(&b, "overlay: schema v%d; cross-edges: %d; ambiguities: %d\n",
			s.OverlaySchemaVersion, s.CrossEdges, s.Ambiguities)
	}

	b.WriteString("members:\n")
	if len(s.Members) == 0 {
		b.WriteString("  (none declared)\n")
	}
	for _, m := range s.Members {
		fmt.Fprintf(&b, "  %s: %s — %s, %s, %s\n",
			m.ID, m.Root, presence(m.Present), indexedWord(m.Indexed), stampPhrase(m))
	}

	b.WriteString("version skew:\n")
	if len(s.Skew) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, k := range s.Skew {
		fmt.Fprintf(&b, "  %s\n", k.Text)
	}
	return b.String()
}

func presence(present bool) string {
	if present {
		return "present"
	}
	return "missing"
}

func indexedWord(indexed bool) string {
	if indexed {
		return "indexed"
	}
	return "unindexed"
}

// stampPhrase renders the recorded stamp. It says "stamped merkle <root>",
// never "clean" or "dirty": the recorded root is a fact, a verdict would be an
// unbacked inference. See the file comment.
func stampPhrase(m MemberStatus) string {
	if !m.Stamped {
		return "no stamp recorded"
	}
	return "stamped merkle " + m.StampedMerkleRoot
}

// declaredRoot is where the manifest says a member lives, for a member that is
// not there. config.Resolve only reports AbsRoot for members it FOUND, so the
// absent half is derived here from the same base and the same relative root.
func declaredRoot(wsRoot, memberRoot string) string {
	p := filepath.Join(wsRoot, memberRoot)
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// memberIndexPath is the per-repo index location, mirroring the engine's own
// layout. A member with no graph.db is UNINDEXED, which is a state the report
// exists to name rather than an error.
func memberIndexPath(memberRoot string) string {
	return filepath.Join(memberRoot, ".codeindex", "graph.db")
}

// RefuseRepoRoot is the mirror of RefuseWorkspaceRoot: the message a
// workspace-only verb returns when handed a repo root, NAMING the repo-mode
// verb that does the same job. A refusal that does not name the alternative
// leaves the user with a dead end.
func RefuseRepoRoot(verb, root, insteadVerb string) error {
	return fmt.Errorf("codeindex %s: %s is a repo, not a workspace. Use the repo-mode %s verb, e.g.\n  codeindex %s %s",
		verb, root, insteadVerb, insteadVerb, root)
}
