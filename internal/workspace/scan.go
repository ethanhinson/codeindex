package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"codeindex/internal/config"
)

// Scan discovers what `init-workspace --scan` should write for wsRoot, in two
// independent passes.
//
// Pass 1 is member discovery: Members(wsRoot) reads the five monorepo
// declaration sources at the workspace root. On the dedicated
// workspace-directory shape — an empty directory whose members all live at
// "../" roots — this legitimately returns zero, which is not an error.
// Discovery emits only members at or below wsRoot (the stated limitation on
// Members), so pass 1 also reports the declarations it dropped for escaping the
// root; they are used solely to keep the empty-result diagnostic honest.
//
// Pass 2 is the namespace pass, and it runs over every authored member root as
// well as every member pass 1 discovered. The authored roots are precisely the
// ones holding the build manifests in that shape, so without pass 2 a scan of a
// hand-authored skeleton would return it unchanged.
//
// authored is passed in solely to enumerate the roots pass 2 must probe. The
// return is scan results only: for each member touched, the id/root that
// identifies it plus the namespaces discovered under it. Authored deps are
// never echoed and no field is invented — Merge is the single place authored
// and scanned values meet.
//
// A member root that is not on disk is not an error (Assumption 16); it
// contributes no namespaces. Erroring there would re-introduce the member-root
// stat that config.LoadWorkspace deliberately keeps out of the load path;
// absence is config.Resolve's to report.
func Scan(wsRoot string, authored []config.Member) ([]config.Member, error) {
	discovered, escaped, err := membersAndEscaped(wsRoot)
	if err != nil {
		return nil, err
	}

	// The empty-result error fires only when the run would write a manifest
	// with no members at all. Pass 1 finding zero is the normal, sanctioned
	// outcome for a dedicated workspace directory, so it is not sufficient on
	// its own.
	if len(authored) == 0 && len(discovered) == 0 {
		// Two very different situations reach zero, and conflating them makes
		// the diagnostic false: nothing was declared at all, or declarations
		// were found and every one of them resolved outside the workspace root
		// (the stated limitation on Members). Name an escaping path in the
		// second case, since that is what the user has to act on.
		if len(escaped) > 0 {
			return nil, fmt.Errorf("%s: found no workspace members: every declared member resolves outside "+
				"the workspace root (for example %q), and discovery only emits members at or below it; "+
				"author such members explicitly in the workspace manifest, which does accept \"../\" roots",
				wsRoot, escaped[0])
		}
		return nil, fmt.Errorf("%s: found no workspace members: no members are declared by go.work, "+
			"pnpm-workspace.yaml, composer.json path repositories, lerna.json, or package.json workspaces, "+
			"and the manifest authors none", wsRoot)
	}

	out := make([]config.Member, 0, len(authored)+len(discovered))
	byID := map[string]bool{}
	byRoot := map[string]bool{}

	// record records one member's scan result. ns is the namespace pass's
	// output for root, already computed by the caller.
	record := func(id, root string, ns []string) {
		byID[id] = true
		byRoot[cleanRoot(root)] = true
		out = append(out, config.Member{ID: id, Root: root, Namespaces: ns})
	}

	for _, m := range authored {
		ns, err := namespacesAt(wsRoot, m.Root)
		if err != nil {
			return nil, err
		}
		record(m.ID, m.Root, ns)
	}
	for _, m := range discovered {
		// A discovered member that is already authored is the same member;
		// pass 2 has probed it once already.
		if byID[m.ID] || byRoot[cleanRoot(m.Root)] {
			continue
		}
		// Pass 1 already ran the namespace pass over this root to apply guard
		// 3, and it ran it over exactly the directory namespacesAt would probe
		// — a discovered root exists and is a directory by guard 1, so the
		// absent-root branch cannot apply. Reuse it rather than probing twice.
		record(m.ID, m.Root, m.Namespaces)
	}
	return out, nil
}

// namespacesAt runs the namespace pass for one member root, which is relative
// to wsRoot and may climb out of it. A root that is not a directory on disk
// contributes nothing.
func namespacesAt(wsRoot, root string) ([]string, error) {
	dir := filepath.Join(wsRoot, filepath.FromSlash(root))
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		// Non-nil and empty, not nil: a nil slice marshals as JSON `null` and
		// would overwrite an authored "namespaces": [] via Merge's
		// empty-list-is-a-gap rule.
		return []string{}, nil
	}
	return namespacesProbe(dir)
}

// cleanRoot normalizes a member root for matching, so "../a/./" and "../a"
// name the same member.
func cleanRoot(root string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(root)))
}

// Merge folds scan results into an existing manifest's members.
//
// Members are matched by id first, then by cleaned root. For a matched member a
// non-empty authored field is an override and wins, while an empty namespaces
// list is a gap the scan fills. That asymmetry is the mechanism the
// dedicated-workspace-directory bootstrap rides on, not an edge case: the
// skeleton is authored with `namespaces: []` precisely so the scan can fill it.
// root and deps are never overwritten by the scan.
//
// Ordering lives here rather than in config.SaveWorkspace: existing members
// keep their manifest order and newly discovered members are appended sorted by
// id. SaveWorkspace receives only the final slice and cannot tell authored from
// appended, so the rule is unimplementable there — and a scan that reshuffled a
// reviewed manifest would silently change answer ordering.
func Merge(existing, scanned []config.Member) []config.Member {
	matched := make([]bool, len(scanned))
	out := make([]config.Member, 0, len(existing)+len(scanned))

	for _, e := range existing {
		merged := e
		if i := matchIndex(e, scanned, matched); i >= 0 {
			matched[i] = true
			if len(merged.Namespaces) == 0 {
				merged.Namespaces = scanned[i].Namespaces
			}
		}
		out = append(out, merged)
	}

	var added []config.Member
	for i, s := range scanned {
		if !matched[i] {
			added = append(added, s)
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].ID < added[j].ID })
	return append(out, added...)
}

// matchIndex finds e's counterpart among the unmatched scan results: by id
// first, then by cleaned root.
func matchIndex(e config.Member, scanned []config.Member, matched []bool) int {
	for i, s := range scanned {
		if !matched[i] && e.ID != "" && s.ID == e.ID {
			return i
		}
	}
	for i, s := range scanned {
		if !matched[i] && e.Root != "" && cleanRoot(s.Root) == cleanRoot(e.Root) {
			return i
		}
	}
	return -1
}
