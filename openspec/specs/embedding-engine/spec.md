# embedding-engine Specification

## Purpose
TBD - created by archiving change semantic-code-search. Update Purpose after archive.
## Requirements
### Requirement: Bundled local embedder
The system SHALL provide a local text-embedding provider whose inference
engine (ggml/llama.cpp) is statically linked into the `codeindex` binary and
whose default model weights are compiled in via `go:embed`, such that
embedding works with no network access, no runtime installs, and CPU-only
hardware.

#### Scenario: Air-gapped first use
- **WHEN** `codeindex build` runs on a machine with no network access and no
  model cache
- **THEN** symbol embeddings are produced using the bundled model, and the
  build completes without attempting any download

#### Scenario: CPU-only inference
- **WHEN** embeddings are computed on a machine without a GPU
- **THEN** inference runs on CPU using all available cores and completes the
  embedding pass within the registered build-time budget

### Requirement: Pluggable embedding providers
The system SHALL expose embedding through a provider interface with at least
two implementations — `local` (default) and `api` (hosted embedder, opt-in via
per-repo config plus environment credential) — selected by configuration
without code changes.

#### Scenario: Default provider
- **WHEN** no embedding configuration is present in the repo
- **THEN** the `local` provider with the bundled model is used

#### Scenario: API provider selected
- **WHEN** config selects the `api` provider and a credential is present in
  the environment
- **THEN** embeddings are requested from the configured hosted embedder and
  stored under that provider's model ID

#### Scenario: API provider missing credential
- **WHEN** config selects the `api` provider but no credential is available
- **THEN** the system falls back to the `local` provider and reports the
  fallback in command output

### Requirement: Model management for larger local models
The system SHALL provide `codeindex model` subcommands to pull a larger GGUF
model into the user cache, select it for a repo, and report the active model;
a selected pulled model SHALL take precedence over the bundled model.

#### Scenario: Pull and select a larger model
- **WHEN** the user runs `codeindex model pull <name>` followed by
  `codeindex model use <name>` in a repo
- **THEN** subsequent embedding passes for that repo use the pulled model and
  `codeindex model status` reports it as active

### Requirement: Vector storage in the symbol graph
The system SHALL store symbol embeddings int8-quantized in the repo's
`.codeindex/graph.db` via the sqlite-vec extension, keyed to symbol rows and
stamped with a model ID (model name plus weights hash).

#### Scenario: Embeddings die with their symbols
- **WHEN** a source file is deleted and the index is patched
- **THEN** embeddings for symbols defined in that file are removed in the same
  transaction as the symbol rows

#### Scenario: Model swap invalidates vectors
- **WHEN** the active embedding model changes (bundled → pulled, or version
  bump)
- **THEN** stored vectors with a mismatched model ID are treated as absent and
  are re-embedded on the next build

### Requirement: Incremental re-embedding on the patch path
The system SHALL re-embed only symbols whose own card text changed during the
fresh-on-query incremental patch, and SHALL NOT trigger repo-wide embedding
work at query time.

#### Scenario: Single-file edit
- **WHEN** one source file changes and any query runs
- **THEN** only symbols from the changed file are re-embedded as part of the
  standard patch, and the query answers from the updated index

#### Scenario: Vectors absent at query time
- **WHEN** a query runs on a repo whose vector table is missing or empty for
  the active model
- **THEN** the query does not block on a full embedding pass; embedding
  backfill is deferred to the next explicit or implicit `build`

