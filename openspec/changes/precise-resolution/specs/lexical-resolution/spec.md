## ADDED Requirements

### Requirement: Parent attribution on method symbols

Every adapter SHALL attribute method symbols to their parent type, derived
lexically: Go from the receiver declaration, TS/JS and PHP from the enclosing
class/interface/trait, Python from the enclosing class (including
`@classmethod`/`@staticmethod` members).

#### Scenario: Go receiver parent

- **WHEN** the Go adapter parses `func (w *Widget) Grow() int`
- **THEN** symbol `Grow` has kind method and parent `Widget` (pointer stripped)

#### Scenario: Enclosing-class parent in TS, Python, PHP

- **WHEN** a method is defined inside `class Widget` (TS), `class Widget:`
  (Python), or `class Widget { }` (PHP)
- **THEN** the method symbol's parent is `Widget`
- **AND** top-level functions have an empty parent

### Requirement: Lexical call qualifiers

Adapters SHALL attach a qualifier to call sites only where lexically
meaningful: `this`/`self`/`cls`/`$this` receivers map to the enclosing class;
Go calls through the enclosing method's receiver variable map to the receiver
type; PHP scoped calls carry the scope name (`self::`/`static::` map to the
enclosing class, `parent::` carries none); TS/Python bare uppercase-identifier
receivers carry the identifier as a candidate hint.

#### Scenario: this/self/$this qualifies to the enclosing class

- **WHEN** `$this->scale()` appears inside PHP `class Widget` (likewise
  `self.scale()` in Python, `this.scale()` in TS)
- **THEN** the raw call carries name `scale` and qualifier `Widget`

#### Scenario: Go receiver-variable call

- **WHEN** `w.scale(n)` appears inside `func (w Widget) Grow(n int)`
- **THEN** the raw call carries qualifier `Widget`

#### Scenario: No qualifier where lexically unknown

- **WHEN** a call goes through an arbitrary expression or a lowercase variable
  (`obj.save()` in TS where `obj` is a parameter)
- **THEN** the raw call carries no qualifier and resolution behaves exactly as
  before this change

### Requirement: Qualifier-validated resolution with total fallback

The resolver SHALL, for a call carrying qualifier `Q` and name `n`, resolve to
the symbol named `n` with parent `Q` when exactly one exists (confidence
unambiguous); when multiple exist, pick deterministically and flag ambiguous;
when none exist, fall back to plain name-based resolution unchanged. Edges
SHALL persist the qualifier so incremental re-resolution reproduces the same
result.

#### Scenario: Collision collapsed by qualifier

- **WHEN** `scale` is defined on both `Widget` and `Gauge`, and a call inside
  `Widget` uses `$this->scale()`
- **THEN** the edge resolves to `Widget.scale` with confidence unambiguous
  (previously ambiguous)

#### Scenario: Wrong hint degrades harmlessly

- **WHEN** a call carries qualifier `Foo` but no symbol `n` has parent `Foo`
- **THEN** resolution falls back to plain name-based behavior (same result and
  confidence as before this change)

#### Scenario: Incremental equivalence preserved

- **WHEN** files are edited and patched incrementally versus fully rebuilt
- **THEN** the normalized snapshots — now including symbol parents and edge
  qualifiers — are equal (the equivalence check passes on all four languages'
  pinned repositories)

### Requirement: Qualified names in output and as anchors

Query and MCP outputs SHALL display qualified names (`Parent.name`) for symbols
with parents, and `callers`/`callees`/`impact` SHALL accept qualified anchors
(`Type.method` or `Type::method`) that filter to the matching parent.

#### Scenario: Qualified anchor disambiguates

- **WHEN** the user queries `callers Builder.firstOrCreate` in laravel
- **THEN** only definitions with parent `Builder` and the callers resolving to
  them are returned, without the other 3 same-named definitions

#### Scenario: Unqualified queries unchanged

- **WHEN** the user queries bare `callers firstOrCreate`
- **THEN** all definitions and callers are returned as today, with qualified
  display names and ambiguity flags intact

### Requirement: Versioned schema with automatic rebuild

The index SHALL carry a schema version (`PRAGMA user_version`); opening an
index with a mismatched version SHALL rebuild it automatically (logged), since
the index is a derived artifact.

#### Scenario: Old index encountered

- **WHEN** a v2 binary opens a v1 `.codeindex/graph.db`
- **THEN** the index is deleted, rebuilt from source on the next fresh-on-query
  pass, and a one-line notice is emitted to stderr

### Requirement: Measured precision improvement and agent re-test

The change SHALL record, per pinned repository (kubernetes, nest, flask,
laravel), the count of ambiguous call edges before and after, and SHALL re-run
the agent A/B (v5) with caller-attribution tasks on laravel using the plugin
arm.

#### Scenario: Precision metric recorded

- **WHEN** the before/after measurement runs
- **THEN** a findings note reports ambiguous-edge counts and percentage
  reduction per repository

#### Scenario: v5 boundary validation off-Go

- **WHEN** the v5 run completes on laravel caller-attribution tasks
- **THEN** the report records median paired cost reduction, success parity,
  and adoption — with the pre-registered expectation (branch-out savings ≥30%,
  success delta ≥ −5pp) evaluated PASS/FAIL
