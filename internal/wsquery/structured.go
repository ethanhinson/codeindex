package wsquery

import (
	"codeindex/internal/engine"
	"codeindex/internal/query"
)

// The structured surface: §4.5's JSON sibling, reachable from a caller that
// emits an answer rather than a pre-rendered string.
//
// # Why this family exists at all
//
// wsquery's plain entry points (Callers, Find, …) return internal/query's
// CONCRETE answer types unchanged — that verbatim tail-call is the mechanical
// basis of the repo-mode non-regression bar, so their signatures do not move.
// But a concrete *query.CallersAnswer has nowhere to hang a workspace-only
// field, so a caller holding one has already lost the clause. The Text family
// solved that for the text surface by rendering inside wsquery; this family
// solves it for the structured surface by returning the Answer pairing instead
// of a string, so `--json` consumers get the reserved
// `workspace: {members_consulted, members_stale, boundary}` object that §4.5
// requires and that the Text family can only render as a sentence.
//
// # Repo mode returns the INNER ANSWER ITSELF
//
// Not an Answer with an empty clause: an empty clause would still marshal a
// `"workspace"` key and move every repo-mode JSON golden, which is the one
// thing this slice may not do. In repo mode these functions are the same
// verbatim tail-call the plain entry points make, boxed in an interface — the
// bytes json.Marshal produces are the bytes it produced before. If a repo-mode
// golden ever moves, the wrapper is firing in repo mode; that is the bug, and
// the golden is not.

// Emittable is the render-either-way surface a CLI holds: text via Text(),
// structured via the standard json marshallers. Both internal/query's concrete
// answers and wsquery.Answer satisfy it, which is what lets one call site
// serve repo and workspace mode without branching on root kind itself.
type Emittable interface{ Text() string }

// structured is the shared body of the family: detect once, tail-call in repo
// mode, and in workspace mode compose the answer and pair it with the clause.
// One body rather than nine copies, because "repo mode must not gain a
// workspace key" is a single invariant and nine hand-written copies of it is
// nine places for it to drift.
func structured[T textAnswer](root, verb string, repoFn func() (T, error), wsFn func(*session) (T, error)) (Emittable, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		a, err := repoFn()
		if err != nil {
			return nil, err
		}
		return a, nil
	}
	s, err := workspaceSession(root)
	if err != nil {
		return nil, err
	}
	a, err := wsFn(s)
	if err != nil {
		return nil, err
	}
	return WithClause(a, s.clause(verb)), nil
}

// CallersStructured is Callers for a structured/emitting caller.
func CallersStructured(root, anchor string, limit int) (Emittable, error) {
	return structured(root, "callers",
		func() (*query.CallersAnswer, error) { return query.Callers(root, anchor, limit) },
		func(s *session) (*query.CallersAnswer, error) { return callersWorkspace(s, anchor, limit) })
}

// CalleesStructured is Callees for a structured/emitting caller.
func CalleesStructured(root, anchor string, limit int) (Emittable, error) {
	return structured(root, "callees",
		func() (*query.CalleesAnswer, error) { return query.Callees(root, anchor, limit) },
		func(s *session) (*query.CalleesAnswer, error) { return calleesWorkspace(s, anchor, limit) })
}

// NavStructured is Nav for a structured/emitting caller.
func NavStructured(root, anchor string, limit int) (Emittable, error) {
	return structured(root, "nav",
		func() (*query.NavAnswer, error) { return query.Nav(root, anchor, limit) },
		func(s *session) (*query.NavAnswer, error) { return navWorkspace(s, anchor, limit) })
}

// DependentsStructured is Dependents for a structured/emitting caller.
func DependentsStructured(root, anchor string, limit int) (Emittable, error) {
	return structured(root, "dependents",
		func() (*query.DependentsAnswer, error) { return query.Dependents(root, anchor, limit) },
		func(s *session) (*query.DependentsAnswer, error) { return dependentsWorkspace(s, anchor, limit) })
}

// DepsStructured is Deps for a structured/emitting caller.
func DepsStructured(root, anchor string, limit int) (Emittable, error) {
	return structured(root, "deps",
		func() (*query.DepsAnswer, error) { return query.Deps(root, anchor, limit) },
		func(s *session) (*query.DepsAnswer, error) { return depsWorkspace(s, anchor, limit) })
}

// FindStructured is Find for a structured/emitting caller.
func FindStructured(root, q, kind, path string, limit int) (Emittable, error) {
	return structured(root, "find",
		func() (*query.FindAnswer, error) { return query.Find(root, q, kind, path, limit) },
		func(s *session) (*query.FindAnswer, error) { return findWorkspace(s, q, kind, path, limit) })
}

// GrepStructured is Grep for a structured/emitting caller.
func GrepStructured(root, pattern string, limit int, word bool) (Emittable, error) {
	return structured(root, "grep",
		func() (*query.GrepAnswer, error) { return query.Grep(root, pattern, limit, word) },
		func(s *session) (*query.GrepAnswer, error) { return grepWorkspace(s, pattern, limit, word) })
}

// EnclosingStructured is Enclosing for a structured/emitting caller.
func EnclosingStructured(root, file string, start, end int) (Emittable, error) {
	return structured(root, "enclosing",
		func() (*query.EnclosingAnswer, error) { return query.Enclosing(root, file, start, end) },
		func(s *session) (*query.EnclosingAnswer, error) { return enclosingWorkspace(s, file, start, end) })
}

// ImpactStructured is Impact for a structured/emitting caller.
//
// It is the one member of the family that is not a call to structured: impact's
// clause rides INSIDE ImpactAnswer.Coverage, so it is folded with
// WithImpactClause and the pairing carries no trailing text line. Routing it
// through the shared body would put the same sentence on the answer twice.
func ImpactStructured(root, anchor string, limit int) (Emittable, error) {
	kind, err := RootKind(root)
	if err != nil {
		return nil, err
	}
	if kind == engine.RootRepo {
		a, err := query.Impact(root, anchor, limit)
		if err != nil {
			return nil, err
		}
		return a, nil
	}
	s, err := workspaceSession(root)
	if err != nil {
		return nil, err
	}
	a, err := impactWorkspace(s, anchor, limit)
	if err != nil {
		return nil, err
	}
	return WithImpactClause(a, s.clause("impact")), nil
}
