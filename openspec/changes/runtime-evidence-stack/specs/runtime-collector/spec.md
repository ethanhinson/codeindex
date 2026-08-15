# runtime-collector Specification (delta)

## ADDED Requirements

### Requirement: Collector mode (stage 2)
`codeindex collector` SHALL run the same binary as a bounded HTTP receiver
that accepts cxprof payloads and writes them to a spool directory,
enforcing size and rate caps, requiring no privileges, and never applying
backpressure to emitters (over-limit payloads are dropped and counted).

#### Scenario: Production sampling without disruption
- **WHEN** SDKs in production post sampled profiles to the collector
- **THEN** application latency is unaffected (fire-and-forget), and the
  spool grows within its configured bound

#### Scenario: Same-binary deployment
- **WHEN** an operator deploys the collector
- **THEN** it is the codeindex binary with a flag — no separate artifact
