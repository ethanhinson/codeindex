package wsquery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"codeindex/internal/query"
)

// Boundary is the fixed sentence the coverage clause carries: what the answer
// cannot speak for at all. It is not a degradation notice and does not vary
// with the workspace's state — a symbol defined outside every declared member
// is simply not in the union, on a clean pass and a degraded one alike.
const Boundary = "symbols outside this workspace are unknown to it"

// Layer names which layer of the system a verb's coverage clause describes.
// A graph-layer clause bounds the COMPLETENESS OF AN EDGE SET; a
// retrieval-layer clause bounds the FRESHNESS OF A SEARCHED INDEX. They are
// different claims and a reader who conflates them will over- or under-trust
// the answer.
type Layer string

const (
	// LayerGraph: the clause states which members' edge sets were unioned. A
	// stale member means edges may be missing or superseded.
	LayerGraph Layer = "graph"
	// LayerRetrieval: the clause states which members' indexes were searched.
	// A stale member means its rows may be from an older tree.
	LayerRetrieval Layer = "retrieval"
	// LayerNone is the answer for a verb that carries no clause at all.
	LayerNone Layer = ""
)

// clauseLayers is design §4.6's per-verb layer policy, in force at the clause
// construction site rather than in prose alone:
//
//	callers, callees, impact, nav, dependents, deps  graph
//	find, grep                                       retrieval
//	enclosing                                        graph, single-member
//	search                                           n/a — refuses (§2.2)
//
// enclosing is graph-layer over exactly one member: the clause names the one
// owning member and is stale iff that member is.
//
// nav is GRAPH-layer despite composing retrieval components, because its
// headline claim ("who calls this") is an edge claim. Its find/grep components
// inherit nav's single clause rather than carrying a second one of their own —
// two clauses on one answer would make the reader arbitrate between them.
//
// search maps to LayerNone because it never produces an answer on a workspace
// root; SearchText refuses there (§2.2).
var clauseLayers = map[string]Layer{
	"callers":    LayerGraph,
	"callees":    LayerGraph,
	"impact":     LayerGraph,
	"nav":        LayerGraph,
	"dependents": LayerGraph,
	"deps":       LayerGraph,
	"find":       LayerRetrieval,
	"grep":       LayerRetrieval,
	"enclosing":  LayerGraph,
	"search":     LayerNone,
}

// ClauseLayer reports which layer verb's coverage clause describes. An
// unrecognized verb returns LayerNone: a verb with no entry in the table has
// no policy, and inventing one for it would be the second enforcement site the
// table exists to prevent.
func ClauseLayer(verb string) Layer { return clauseLayers[verb] }

// Clause is the workspace coverage clause D6 reserves: what this answer looked
// at, what of it could not be verified, and what it cannot speak for.
//
// The three reserved fields are members_consulted, members_stale and boundary.
// freshen_failed is the §4.2 degrade disclosure and is present only on that
// path. Layer is deliberately NOT serialized: §4.6's policy is a statement
// about how to READ the clause, discharged by documentation and by the
// clauseLayers table above, and D6 reserved a three-field shape that this
// slice does not widen for a constant-per-verb annotation.
type Clause struct {
	// MembersConsulted is the set of member ids whose graph.db was read TO
	// COMPOSE THIS ANSWER, EXCLUDING the freshen pass, in manifest order.
	MembersConsulted []string `json:"members_consulted"`

	// MembersStale is the four-way union of §4.3, in manifest order.
	MembersStale []string `json:"members_stale"`

	// Boundary is the fixed sentence; see the Boundary constant.
	Boundary string `json:"boundary"`

	// FreshenFailed carries wsfresh.Freshen's error verbatim when the freshen
	// pass failed and the query degraded past it (§4.2). Empty otherwise.
	FreshenFailed string `json:"freshen_failed,omitempty"`

	// Layer is §4.6's per-verb reading policy. Not serialized; see the type
	// comment.
	Layer Layer `json:"-"`
}

// String renders the clause as ONE line. One line is required, not preferred:
// the impact clause rides inside ImpactAnswer.Coverage, which the renderer
// prints as "(coverage: %s)" on a single line, and the other eight carry the
// same string as a trailing line. One renderer for both keeps the two surfaces
// from drifting.
func (c Clause) String() string {
	var b strings.Builder
	b.WriteString("workspace: members_consulted: ")
	b.WriteString(idList(c.MembersConsulted))
	b.WriteString("; members_stale: ")
	b.WriteString(idList(c.MembersStale))
	if c.FreshenFailed != "" {
		b.WriteString("; freshen_failed: ")
		b.WriteString(singleLine(c.FreshenFailed))
	}
	b.WriteString("; boundary: ")
	b.WriteString(c.Boundary)
	return b.String()
}

// idList renders a member-id set for the text surface. An empty set is
// "(none)", never blank: a blank list reads as a rendering bug and invites the
// reader to assume the field was not computed.
func idList(ids []string) string {
	if len(ids) == 0 {
		return "(none)"
	}
	return strings.Join(ids, ", ")
}

// singleLine flattens embedded newlines so a multi-line freshen error cannot
// break the one-line invariant String promises.
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// AppendClauseLine appends the clause to an answer's text as a TRAILING LINE.
// This is the surface for the eight answer types that have no coverage field
// (§4.5). It runs at the wsquery layer; internal/query's renderers are not
// taught about workspaces.
func AppendClauseLine(text string, c Clause) string {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + c.String() + "\n"
}

// AppendClauseToCoverage is impact's surface: ImpactAnswer.Coverage stays a
// string (assumption 8 — re-typing it to a struct would break repo-mode JSON,
// which is a non-regression bar), and in workspace mode it carries the
// existing sentence plus this clause. The renderer needs no change, because it
// already prints Coverage verbatim.
func AppendClauseToCoverage(coverage string, c Clause) string {
	if coverage == "" {
		return c.String()
	}
	return coverage + "; " + c.String()
}

// textAnswer is what every internal/query answer type already satisfies.
type textAnswer interface{ Text() string }

// Answer is a per-repo answer carried together with its workspace coverage
// clause. It exists because wsquery's entry points return internal/query's
// concrete answer types unchanged, so there is nowhere on those types to hang
// a workspace-only field — the pairing has to live at this layer.
//
// It satisfies the CLI's `interface{ Text() string }` and marshals as the
// inner answer's own JSON with a `workspace` sibling spliced in, so a caller
// that emits an Answer gets both surfaces of §4.5 with no edit to
// internal/query and no edit to the emit path's shape.
//
// Repo mode never constructs one: repo mode emits neither surface.
type Answer struct {
	// Inner is the per-repo answer being carried.
	Inner textAnswer
	// Clause is this answer's coverage clause.
	Clause Clause
	// TrailingLine is true for the eight answer types with no coverage field,
	// whose clause is a trailing text line. It is FALSE for impact, whose
	// clause is already inside Coverage — emitting it twice would put two
	// copies of the same sentence on one answer.
	TrailingLine bool
}

// WithClause pairs one of the eight clause-less answer types with its clause.
func WithClause(inner textAnswer, c Clause) Answer {
	return Answer{Inner: inner, Clause: c, TrailingLine: true}
}

// WithImpactClause folds the clause into an ImpactAnswer's Coverage string and
// pairs the answer with it for the JSON sibling. The text surface carries no
// trailing line, because Coverage already carries the clause — which is also
// why the impact answer needs no renderer change at all (§4.5).
func WithImpactClause(a *query.ImpactAnswer, c Clause) Answer {
	a.Coverage = AppendClauseToCoverage(a.Coverage, c)
	return Answer{Inner: a, Clause: c, TrailingLine: false}
}

// Text renders the inner answer, plus the trailing clause line when this
// answer type carries one.
func (a Answer) Text() string {
	if a.Inner == nil {
		return ""
	}
	if !a.TrailingLine {
		return a.Inner.Text()
	}
	return AppendClauseLine(a.Inner.Text(), a.Clause)
}

// MarshalJSON emits the inner answer's own JSON with a "workspace" sibling
// appended. The clause is SPLICED rather than merged through a map so the
// inner answer's key order — which repo-mode goldens pin — is preserved
// byte-for-byte and the workspace key lands last.
func (a Answer) MarshalJSON() ([]byte, error) {
	if a.Inner == nil {
		return nil, fmt.Errorf("wsquery: Answer has no inner answer")
	}
	inner, err := json.Marshal(a.Inner)
	if err != nil {
		return nil, err
	}
	clause, err := json.Marshal(a.Clause)
	if err != nil {
		return nil, err
	}
	body := bytes.TrimSpace(inner)
	if len(body) < 2 || body[0] != '{' || body[len(body)-1] != '}' {
		return nil, fmt.Errorf("wsquery: inner answer is not a JSON object: %s", body)
	}
	inside := bytes.TrimSpace(body[1 : len(body)-1])

	var out bytes.Buffer
	out.WriteByte('{')
	if len(inside) > 0 {
		out.Write(inside)
		out.WriteByte(',')
	}
	out.WriteString(`"workspace":`)
	out.Write(clause)
	out.WriteByte('}')
	return out.Bytes(), nil
}
