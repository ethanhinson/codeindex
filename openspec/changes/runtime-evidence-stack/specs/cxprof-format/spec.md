# cxprof-format Specification (delta)

## ADDED Requirements

### Requirement: Open one-page wire format
The cxprof format SHALL be JSONL: one header record
(`{"cxprof":1,"lang":...,"unit":"samples","hz":...,"start":...,"end":...}`
with optional `commit` and `tag`) followed by stack records
(`{"st":[[file,line],...],"n":count}`, frames innermost-last). The format
SHALL be documented in `docs/cxprof-format.md` on a single page, versioned
by the `cxprof` header field, with additive-only evolution inside a major
version.

#### Scenario: Third-party emission
- **WHEN** any program writes conforming JSONL without using our SDKs
- **THEN** `codeindex ingest` accepts it identically to SDK output

#### Scenario: Unknown future fields
- **WHEN** a v1 record carries fields this binary does not know
- **THEN** ingestion ignores them and proceeds

### Requirement: Frames-only payloads
cxprof records SHALL contain only file paths, line numbers, and counts —
never argument values, environment data, or request payloads.

#### Scenario: Conformance suite rejects rich payloads
- **WHEN** the in-repo conformance checker sees a record with argument or
  environment fields
- **THEN** it reports the emitter non-conforming
