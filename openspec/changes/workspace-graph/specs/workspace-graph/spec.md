# workspace-graph Specification (delta)

## ADDED Requirements

### Requirement: Workspace manifest and member registry
A workspace SHALL be defined by `.codeindex/workspace.json` at a
workspace root, listing members with stable id, root path relative to
the workspace, exported namespaces, and optional dependency hints.
`init-workspace --scan` SHALL generate the manifest with auto-discovered
namespaces (go.mod, package.json, composer.json, Python top-level
modules) and SHALL discover monorepo members from workspace
declarations (go.work, pnpm-workspace.yaml, composer path
repositories); manifest values SHALL override discovery. A root without a
manifest SHALL behave exactly as a single repo today.

#### Scenario: Scan generates a reviewable manifest
- **WHEN** `init-workspace --scan` runs over a directory containing a Go
  service and a TS library
- **THEN** the manifest lists both members with their module/package
  namespaces, and the user can edit ids, namespaces, and deps before
  any index is built

#### Scenario: Single-repo behavior is untouched
- **WHEN** any verb runs against a repo root with no workspace manifest
- **THEN** output is byte-identical to the pre-workspace binary

### Requirement: Single-graph query semantics
Every graph verb SHALL accept a workspace root wherever it accepts a
repo root — `callers`, `callees`, `find`, `grep`, `nav`, `impact` —
and SHALL answer over the union of all members plus cross-repo edges,
using today's answer schema extended with a per-reference `repo` member
id and workspace-relative paths. Anchor arguments SHALL accept an
optional `<member-id>:` prefix; a bare anchor matching in multiple
members SHALL return the same disambiguation answer shape used for
duplicate names within one repo. `impact` SHALL cross member boundaries
by default, its coverage clause reporting the workspace boundary. No
new query dialect or tool SHALL be introduced for workspace querying.

#### Scenario: Callers cross the repo boundary
- **WHEN** `callers <workspace-root> shared:ParseToken` runs and two
  other members import the shared library and call it
- **THEN** the answer lists callers from all members, each with
  workspace-relative path, line, and its `repo` id

#### Scenario: MCP serves the union graph
- **WHEN** the MCP server is started with a workspace root
- **THEN** existing tools answer over all members with the `repo` field
  populated, with no new tools required

### Requirement: Overlay storage preserves member sources of truth
Workspace state SHALL live in an overlay store containing only the
member registry, cross-repo edges, and per-member freshness stamps.
Member indexes SHALL remain independently buildable, patchable, and
artifact-importable; no symbol SHALL be duplicated into the overlay.
Cross-repo edges SHALL reference symbols by stable key (member id, file
path, qualified name) so member rebuilds do not invalidate the overlay.

#### Scenario: Member artifact import composes with the workspace
- **WHEN** one member's index is replaced via artifact import
- **THEN** workspace queries reflect the imported index after stamp
  reconciliation, with no workspace-wide rebuild

### Requirement: Cross-repo resolution ladder
Cross-repo resolution SHALL apply only to edges unresolved within their
own member, in frozen order: (1) namespace-hint maps to exactly one
member namespace and the name resolves uniquely there → provenance
`cross_repo_import`, confidence exact; (2) bare name resolving in
exactly one other member → provenance `cross_repo_name`, confidence
inferred; (3) multiple candidate members → ambiguous with recorded
candidate count, manifest deps ordering candidates only; (4) otherwise
unresolved. Import mediation SHALL be the only path to exact-class
cross-repo edges. When a namespace is claimed by a workspace member and
also present as a tier-1 depmap attachment in a consumer, the member
SHALL win and the attachment SHALL be suppressed for that namespace,
with the suppression recorded for version-skew reporting.

#### Scenario: Import-mediated call resolves exact
- **WHEN** member `api` imports `github.com/acme/shared/auth` and calls
  `Verify`, which is unique in member `shared`
- **THEN** the edge resolves with provenance `cross_repo_import` and
  exact-class confidence

#### Scenario: Vendored member resolves to the live member
- **WHEN** member `shared` is also vendored inside member `api` and a
  call in `api` targets a `shared` namespace symbol
- **THEN** the edge resolves to the workspace member `shared`, not the
  vendored tier-1 copy, so blast radius points at editable code

#### Scenario: Same name in three members stays honest
- **WHEN** `Login` exists in three members and an unresolved call
  carries no namespace hint
- **THEN** the answer marks the edge ambiguous with candidate count 3
  and never presents one candidate as exact

### Requirement: Workspace freshness contract
The always-fresh contract SHALL extend across the workspace: queries
SHALL freshen consulted members via the existing per-repo mechanism and
SHALL re-resolve overlay edges incident to any member whose content
stamp changed, before answering. An answer SHALL never silently reflect
a stale member: any member not freshened SHALL be named in the coverage
clause.

#### Scenario: Mutation in one member is visible from another
- **WHEN** a new call into `shared` is added in member `api` and a
  workspace `callers` query runs with no explicit rebuild
- **THEN** the new caller appears in the answer, or the coverage clause
  names `api` as stale — silence is a defect

### Requirement: Workspace coverage and provenance fields (M3 reservation)
The M3 edge/answer schema SHALL reserve provenance mechanism values
`cross_repo_import` and `cross_repo_name`, and a coverage clause
`workspace: {members_consulted, members_stale, boundary}` stating that
symbols outside the workspace are unknown. Confidence classes SHALL
remain resolver-visibility claims. These reservations SHALL land with
M3's schema freeze regardless of when workspace-graph itself builds.

#### Scenario: Caller answer states its blind spots
- **WHEN** a workspace `callers` answer is produced while one member is
  unavailable
- **THEN** the coverage clause lists consulted members, names the
  missing member, and states the workspace boundary — the answer does
  not look authoritative beyond what was consulted

### Requirement: Pre-registered evidence gate
See design.md's dated 2026-08-18 "D7 merge-gate interpretation" amendment for how this gate is read.

Implementation SHALL NOT merge before the pre-registered gate passes on
a ≥30-task cross-repo corpus over a 3–5 member, ≥2-language workspace:
recall ≥ the grep-across-checkouts control and ≥0.9 absolute on
import-mediated edges; ≥40% fewer exploration tokens or tool calls;
single-repo goldens byte-identical; the freshness scenario holds; all
four discipline-rule leak classes checked. A control win SHALL be
published as a FINDINGS entry and close the change.

#### Scenario: The control wins
- **WHEN** the gate run shows the shell-grep arm matching the workspace
  arm on recall and cost
- **THEN** the result is recorded in FINDINGS and the change closes
  without shipping — the frontier hypothesis is answered either way
