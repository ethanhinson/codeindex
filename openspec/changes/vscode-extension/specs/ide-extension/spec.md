## ADDED Requirements

### Requirement: Proactive workspace indexing with consent

The extension SHALL detect on workspace open whether an indexable workspace
lacks a current index (via the side-effect-free status verb), SHALL obtain
one-time per-workspace consent before first indexing (persisted; a manual
command overrides a decline), and SHALL run indexing through the codeindex
binary — never reimplementing index logic.

#### Scenario: First open of an unindexed repo

- **WHEN** a workspace with supported files and no index opens
- **THEN** the user is prompted once, and accepting starts a visible build

### Requirement: Visible progress

Indexing SHALL render live progress (phase and counts from the JSONL feed)
in a progress notification and a persistent status bar item that reflects
building, fresh, and failed states.

#### Scenario: Build progress

- **WHEN** the initial build runs
- **THEN** the status bar shows a spinner with done/total counts and flips
  to a checkmark on completion

### Requirement: Keep-warm on save

With keepFresh enabled (default), saving a supported file SHALL trigger a
debounced, serialized `codeindex refresh` so the index tracks the working
tree; refreshes SHALL NOT raise notifications.

#### Scenario: Save triggers incremental refresh

- **WHEN** a tracked file is saved twice within the debounce window
- **THEN** exactly one refresh runs after the window closes

### Requirement: Refresh verb

`codeindex refresh <root> [--progress]` SHALL build if the index is missing
and incrementally patch otherwise, with the standard progress surfaces.

#### Scenario: Refresh patches

- **WHEN** refresh runs on an indexed repo after one file changed
- **THEN** exactly the changed file is re-parsed (drift reported)
