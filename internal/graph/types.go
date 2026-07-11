// Package graph defines the symbol-graph data model shared across the engine:
// files, symbols, edges, and the parser output that produces them.
package graph

// SymbolKind is the category of a defined symbol.
type SymbolKind string

const (
	KindFunc   SymbolKind = "func"
	KindMethod SymbolKind = "method"
	KindType   SymbolKind = "type"
)

// EdgeKind is the relationship an edge represents. The walking skeleton only
// produces call edges; the full engine adds imports/extends/implements/references.
type EdgeKind string

const (
	KindCalls EdgeKind = "calls"
)

// Confidence records how certain the name-based resolver is about an edge target.
type Confidence string

const (
	ConfUnambiguous Confidence = "unambiguous"
	ConfAmbiguous   Confidence = "ambiguous"
	ConfUnresolved  Confidence = "unresolved"
)

// Symbol is a defined function, method, or type. Parent is the lexically-known
// owner type for methods (Go receiver type, enclosing class elsewhere), empty
// for top-level symbols. Stored as TEXT (content key), not a rowid — same
// rationale as dst_name: survives per-file replacement and keeps the
// incremental==full snapshot id-independent.
type Symbol struct {
	ID        int64
	File      string
	Name      string
	Parent    string
	Kind      SymbolKind
	Signature string
	StartLine int
	EndLine   int
}

// QName is the display name: Parent.Name for methods, Name otherwise.
func (s Symbol) QName() string {
	if s.Parent != "" {
		return s.Parent + "." + s.Name
	}
	return s.Name
}

// RawCall is a call site discovered by an adapter, before name resolution.
// EnclosingIdx indexes into ParsedFile.Symbols (-1 for a top-level/unknown
// scope). Qualifier is a lexical hint about the callee's owner type
// (this/self/$this → enclosing class, Go receiver var → receiver type,
// PHP Foo:: → Foo); the resolver validates it and falls back to plain
// name-based resolution when it doesn't match anything.
type RawCall struct {
	EnclosingIdx int
	Callee       string
	Qualifier    string
	Line         int
}

// ParsedFile is an adapter's output for one source file.
type ParsedFile struct {
	Path    string
	Symbols []Symbol
	Calls   []RawCall
}

// Edge is a resolved relationship between symbols. DstSymbolID is 0 when the
// call could not be resolved to a known definition (DstName is always retained;
// DstQualifier preserves the lexical hint so re-resolution reproduces the same
// result).
type Edge struct {
	SrcSymbolID  int64
	DstSymbolID  int64
	DstName      string
	DstQualifier string
	Kind         EdgeKind
	Confidence   Confidence
	Line         int
}
