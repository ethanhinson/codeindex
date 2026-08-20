// Package wsquery is the root-kind routing layer in front of internal/query.
//
// The package is forced, not chosen: internal/wsfresh imports internal/query
// and calls query.Fresh, so a layer that must call both wsfresh.Freshen and
// the per-repo query.* answers cannot live inside internal/query without an
// import cycle. wsquery importing engine + config + graph + overlay + wsfresh
// + query is acyclic.
//
// Its exported surface mirrors internal/query's one-for-one, with identical
// signatures and identical return types. Every entry point begins with a
// root-kind detection, and the RootRepo branch is a verbatim tail-call into
// internal/query — same arguments, same return, no wrapping, no re-formatting,
// no error decoration. That tail-call is the mechanical basis of the
// non-regression bar: repo-mode answers are the same bytes because they are
// literally the same call.
//
// # One detection site
//
// engine.DetectRootKind has exactly ONE non-test caller outside
// internal/wsfresh and internal/wsresolve, namely wsquery.RootKind. Every
// root-kind branch in the binary — the query verbs here, and the eight
// non-query verbs in cmd/codeindex/main.go — reaches it through
// wsquery.RootKind and wsquery.RefuseWorkspaceRoot. Do not add a second
// detection site: two branches on one rule are two enforcement sites for one
// invariant, and they drift.
package wsquery

import (
	"errors"
	"fmt"
	"strings"

	"codeindex/internal/config"
	"codeindex/internal/engine"
	"codeindex/internal/query"
)

// ErrWorkspaceNotWired marks a workspace-root answer path that this slice has
// not implemented yet. Tasks 5-8 replace each of its return sites with real
// union logic; it exists so a premature workspace query fails loudly and
// identifiably rather than silently answering from one member.
var ErrWorkspaceNotWired = errors.New("wsquery: workspace-root queries are not wired yet")

// RootKind classifies root as a repo or a workspace. It is the single
// detection call in the binary (see the package doc): callers that need to
// branch on root kind — including the non-query verbs in main.go — call this
// rather than engine.DetectRootKind.
func RootKind(root string) (engine.RootKind, error) {
	return engine.DetectRootKind(root)
}

// refusalExampleArgs gives the trailing arguments each refusing verb's example
// line shows after the member root, so the printed example is a command the
// user could actually run. A verb with no trailing arguments maps to "".
var refusalExampleArgs = map[string]string{
	"export": "out.db",
	"import": "artifact.db",
	"ingest": "",
	"depmap": "--namespace <ns> --version <v> -o out.db",
	"serve":  "",
	"search": `"<query>"`,
}

// searchRefusalReason is why search refuses, and it is deliberately not the
// reason the other five refuse. Cross-workspace semantic search is a frozen
// non-goal — vectors stay per-repo — not a mechanical limitation that a later
// slice might lift.
const searchRefusalReason = "Cross-workspace semantic search is a frozen non-goal — vectors stay per-repo."

// RefuseWorkspaceRoot builds the single per-repo refusal message for a verb
// that does not accept a workspace root: export, import, ingest, depmap,
// serve, and search. It loads the manifest so the message can list the member
// ids and show an example against the first member's root.
//
// The shape is fixed:
//
//	codeindex export: /ws is a workspace, not a repo. Run it per member, e.g.
//	  codeindex export /ws/services/api out.db
//	members: api, shared, web
//
// search carries the same shape with its own reason sentence inserted.
func RefuseWorkspaceRoot(verb, root string) error {
	ws, err := config.LoadWorkspace(root)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(ws.Members))
	for _, m := range ws.Members {
		ids = append(ids, m.ID)
	}
	exampleRoot := root
	if len(ws.Members) > 0 {
		exampleRoot = strings.TrimSuffix(root, "/") + "/" + strings.TrimPrefix(ws.Members[0].Root, "./")
	}
	example := "codeindex " + verb + " " + exampleRoot
	if args := refusalExampleArgs[verb]; args != "" {
		example += " " + args
	}
	reason := "Run it per member, e.g."
	if verb == "search" {
		reason = searchRefusalReason + " Run it per member, e.g."
	}
	return fmt.Errorf("codeindex %s: %s is a workspace, not a repo. %s\n  %s\nmembers: %s",
		verb, root, reason, example, strings.Join(ids, ", "))
}

// Callers routes to query.Callers on a repo root.
func Callers(root, anchor string, limit int) (*query.CallersAnswer, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		return query.Callers(root, anchor, limit)
	}
	return nil, ErrWorkspaceNotWired
}

// CallersText routes to query.CallersText on a repo root.
func CallersText(root, anchor string, limit int) (string, error) {
	kind, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if kind == engine.RootRepo {
		return query.CallersText(root, anchor, limit)
	}
	return "", ErrWorkspaceNotWired
}

// Callees routes to query.Callees on a repo root.
func Callees(root, anchor string, limit int) (*query.CalleesAnswer, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		return query.Callees(root, anchor, limit)
	}
	return nil, ErrWorkspaceNotWired
}

// CalleesText routes to query.CalleesText on a repo root.
func CalleesText(root, anchor string, limit int) (string, error) {
	kind, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if kind == engine.RootRepo {
		return query.CalleesText(root, anchor, limit)
	}
	return "", ErrWorkspaceNotWired
}

// Impact routes to query.Impact on a repo root.
func Impact(root, anchor string, limit int) (*query.ImpactAnswer, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		return query.Impact(root, anchor, limit)
	}
	return nil, ErrWorkspaceNotWired
}

// ImpactText routes to query.ImpactText on a repo root.
func ImpactText(root, anchor string, limit int) (string, error) {
	kind, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if kind == engine.RootRepo {
		return query.ImpactText(root, anchor, limit)
	}
	return "", ErrWorkspaceNotWired
}

// Nav routes to query.Nav on a repo root.
func Nav(root, anchor string, limit int) (*query.NavAnswer, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		return query.Nav(root, anchor, limit)
	}
	return nil, ErrWorkspaceNotWired
}

// NavText routes to query.NavText on a repo root.
func NavText(root, anchor string, limit int) (string, error) {
	kind, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if kind == engine.RootRepo {
		return query.NavText(root, anchor, limit)
	}
	return "", ErrWorkspaceNotWired
}

// Find routes to query.Find on a repo root.
func Find(root, q, kind, path string, limit int) (*query.FindAnswer, error) {
	rk, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if rk == engine.RootRepo {
		return query.Find(root, q, kind, path, limit)
	}
	return nil, ErrWorkspaceNotWired
}

// FindText routes to query.FindText on a repo root.
func FindText(root, q string, kind, path string, limit int) (string, error) {
	rk, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if rk == engine.RootRepo {
		return query.FindText(root, q, kind, path, limit)
	}
	return "", ErrWorkspaceNotWired
}

// Grep routes to query.Grep on a repo root.
func Grep(root, pattern string, limit int, word bool) (*query.GrepAnswer, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		return query.Grep(root, pattern, limit, word)
	}
	return nil, ErrWorkspaceNotWired
}

// GrepText routes to query.GrepText on a repo root.
func GrepText(root, pattern string, limit int, word bool) (string, error) {
	kind, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if kind == engine.RootRepo {
		return query.GrepText(root, pattern, limit, word)
	}
	return "", ErrWorkspaceNotWired
}

// Dependents routes to query.Dependents on a repo root.
func Dependents(root, anchor string, limit int) (*query.DependentsAnswer, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		return query.Dependents(root, anchor, limit)
	}
	return nil, ErrWorkspaceNotWired
}

// DependentsText routes to query.DependentsText on a repo root.
func DependentsText(root, anchor string, limit int) (string, error) {
	kind, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if kind == engine.RootRepo {
		return query.DependentsText(root, anchor, limit)
	}
	return "", ErrWorkspaceNotWired
}

// Deps routes to query.Deps on a repo root.
func Deps(root, anchor string, limit int) (*query.DepsAnswer, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		return query.Deps(root, anchor, limit)
	}
	return nil, ErrWorkspaceNotWired
}

// DepsText routes to query.DepsText on a repo root.
func DepsText(root, anchor string, limit int) (string, error) {
	kind, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if kind == engine.RootRepo {
		return query.DepsText(root, anchor, limit)
	}
	return "", ErrWorkspaceNotWired
}

// Enclosing routes to query.Enclosing on a repo root. There is no
// query.EnclosingText, so wsquery has no EnclosingText either.
func Enclosing(root, file string, start, end int) (*query.EnclosingAnswer, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		return query.Enclosing(root, file, start, end)
	}
	return nil, ErrWorkspaceNotWired
}

// SearchText routes to query.SearchText on a repo root. On a workspace root it
// returns the search refusal — which is why neither the CLI nor the MCP search
// handler tests the root kind itself.
func SearchText(root, q string, hints []string, errorText string, limit int, flat bool) (string, error) {
	kind, err := RootKind(root)
	if err != nil {
		return "", err
	}
	if kind == engine.RootRepo {
		return query.SearchText(root, q, hints, errorText, limit, flat)
	}
	return "", RefuseWorkspaceRoot("search", root)
}

// Fresh routes to query.Fresh on a repo root.
func Fresh(root string) (query.FreshInfo, error) {
	kind, err := RootKind(root)
	if err != nil {
		return query.FreshInfo{}, err
	}
	if kind == engine.RootRepo {
		return query.Fresh(root)
	}
	return query.FreshInfo{}, ErrWorkspaceNotWired
}
