package wsquery

import (
	"fmt"
	"strings"
)

// Anchor prefixes (§3.4).
//
// An anchor may carry an optional "<member-id>:" prefix — api:HandleLogin —
// which is stripped and used to SCOPE THE DEFINITION LOOKUP to that member.
// Everything after the prefix is handed to query.SplitAnchor untouched, so
// api:Type::method and api:Type.method both work: the prefix is gone before
// SplitAnchor ever sees the remainder.
//
// # The rule is narrower than "split on the first colon", deliberately
//
// query.SplitAnchor parses "::" FIRST (strings.Index) and only then the LAST
// "." (strings.LastIndex). A first-colon split would collide with that:
//
//	Strip a leading "<id>:" ONLY WHEN the text before the first ":" is a
//	DECLARED member id AND the character immediately after that ":" is NOT
//	itself ":". Otherwise the anchor goes to SplitAnchor untouched.
//
// THE SECOND CLAUSE IS LOAD-BEARING. Without it, "api::HandleLogin" in a
// workspace with a member named api would be shortened to ":HandleLogin",
// which NEITHER SplitAnchor branch parses — Index("::") misses because the
// remaining colon is leading and not doubled, LastIndex(".") misses because
// there is no dot — producing a silently wrong lookup in exactly the PHP and
// TypeScript anchors the bench corpus is full of. It is pinned by
// TestDoubleColonAnchorIsNotTreatedAsAMemberPrefix.

// UnknownMemberError is the §3.4 error for an anchor prefix naming a member the
// manifest does not declare. It lists the known ids, because the whole failure
// mode is a user who guessed an id, and an error that does not say what the
// ids ARE sends them to the manifest to find out.
type UnknownMemberError struct {
	// ID is the prefix as typed.
	ID string
	// Known is every declared member id, in manifest order.
	Known []string
}

func (e *UnknownMemberError) Error() string {
	return fmt.Sprintf("wsquery: unknown member id %q in anchor prefix %q; known members: %s",
		e.ID, e.ID+":", strings.Join(e.Known, ", "))
}

// declares reports whether id is a declared member of this session's manifest.
func (s *session) declares(id string) bool {
	for _, m := range s.ws.Members {
		if m.ID == id {
			return true
		}
	}
	return false
}

// splitMemberPrefix applies the rule above. It returns the member id the anchor
// scopes to ("" when the anchor is bare) and the remainder to hand to
// query.SplitAnchor.
//
// An unrecognized prefix is an ERROR rather than a pass-through. That is
// reachable only for a single ":" not followed by another ":", which is not a
// symbol separator in any of the four supported languages — PHP and Rust use
// "::", Go, TypeScript and Python use "." — so the shape is a member prefix or
// it is a typo, and answering a typo from every member would be a quietly
// wrong answer rather than a loud one.
func (s *session) splitMemberPrefix(anchor string) (memberID, rest string, err error) {
	i := strings.Index(anchor, ":")
	switch {
	case i <= 0:
		// No colon, or a leading one: nothing that could be an id sits before it.
		return "", anchor, nil
	case i+1 >= len(anchor):
		// A trailing colon leaves no anchor behind it; pass it through rather
		// than manufacture an empty lookup.
		return "", anchor, nil
	case anchor[i+1] == ':':
		// "::" — the load-bearing clause. SplitAnchor owns this shape.
		return "", anchor, nil
	}
	id := anchor[:i]
	if !s.declares(id) {
		// DEVIATION from §3.4's pass-through sentence: §3.4 states both "an
		// unknown id is an error listing the known ids" AND, in the operative
		// blockquote, "Otherwise the anchor is passed to SplitAnchor
		// UNTOUCHED." Those two sentences conflict for exactly this shape
		// (an undeclared "<text>:" prefix), and this implementation
		// deliberately takes the error reading, not the pass-through one.
		// Reason: a lone ":" is not a symbol separator in any of the four
		// supported languages (Go/TS/Python use ".", PHP/Rust use "::"), so
		// an undeclared "<text>:" prefix is far more likely a typo'd member
		// id than a legitimate anchor. Passing it through would hand
		// SplitAnchor a bogus symbol and surface a confusing "symbol not
		// found" instead of the member list the user actually needs. Do not
		// "fix" this back to a pass-through without re-reading §3.4.
		return "", "", &UnknownMemberError{ID: id, Known: s.declaredIDs()}
	}
	return id, anchor[i+1:], nil
}
