# Uncertainty-Aware Behavioral Intent Compilation Plan

## Phase 1 — Deterministic Trust Boundary (Implemented)

- Add compiler-owned prompt hashing and stable statement segmentation.
- Add strict proposal decoding and a normalized compilation result.
- Represent source coverage, uncertainty, clarification, and capability gaps.
- Release executable requirements only for valid `ready` results.
- Enforce complete downstream acceptance gates in compiler-owned code.
- Add ready, clarification, unsupported, invalid, determinism, and strict-schema
  unit coverage.

## Phase 2 — Installed Semantic Capability Snapshot (Implemented)

- Export one deterministic architecture capability document derived from the
  registered providers, behavioral metric registry, operating-axis registry,
  canonical units, supported interfaces, and trusted analysis/model registry.
- Hash the snapshot and persist it with every compilation.
- Fail before provider execution when the snapshot is missing, malformed,
  oversized, stale, or inconsistent with the executing registries.
- Keep component identities and topology payloads out of the provider context.

## Phase 3 — AI Orchestration And CLI (Implemented)

- Add a behavioral intent command/profile that prepares source statements,
  invokes OpenAI or recorded providers, strict-decodes the proposal, compiles
  it, and persists source, capability, proposal, result, diagnostics, and replay
  artifacts.
- Map `needs_clarification`, `unsupported`, provider failure, and invalid output
  to stable CLI/API statuses without writing KiCad project files.
- Permit bounded retries only for repairable schema/coverage diagnostics.
- Feed only a `ready` strict v3 requirement into architecture search.

## Phase 4 — Minimal Clarification And Gap Closure (Implemented)

- Add deterministic checks that each unresolved ambiguity is owned once and
  related uncertainties are coalesced by requirement path.
- Add a follow-up input contract that applies answers only to named
  uncertainties and recompiles the complete original source.
- Add mutation tests for duplicate, circular, unrelated, missing, and invented
  clarification/gap references.

## Phase 5 — Frozen Held-Out Paraphrase Corpus (Implemented)

- Freeze and SHA-256 pin independently authored prompt/paraphrase groups across
  amplifiers, filters, power, protection, sensors, and MCU interfaces.
- Include ready, ambiguous, unsupported, unsafe, out-of-model, and unavailable
  capability cases.
- Reject implementation-detail language and production identity matching.
- Prove paraphrases normalize to semantically equivalent contracts or the same
  minimal blocker outcome.

## Phase 6 — Full KiCad-Backed Promotion (Implemented)

- Run supported compiled requirements through deterministic search and trusted
  closed-loop simulation over all declared corners.
- Require route completion, connectivity, writer correctness, zero round-trip
  differences, clean KiCad ERC, and strict DRC.
- Prove byte-identical recorded replay and preserve all existing promotion
  suites.

## Phase 7 — Completion Audit And Delivery (Implemented)

- Audit every specification clause against authoritative source and fresh test
  evidence.
- Review the complete staged diff with Prism and resolve actionable findings.
- Commit, push, and verify GitHub Actions for the exact commit.
