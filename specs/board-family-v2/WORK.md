# Two-family generation milestone

Goal: deliver two distinct reviewed sensor/controller board families through one natural-language-to-KiCad command, with deterministic construction, complete-board validation, no manual evaluation-output repair and consistent native/BOM/preview/Gerber/drill/placement bundles.

Base: merged `main` at `101a96fd1bf095a4b82727508f0a1e72177bc3f7`. Work branch: `codex/board-family-v2`. Preserve the entire earlier board-family and practical-board evidence history; do not reinterpret earlier failures.

Execution order:

1. Restore CI with explicit, hash-bound handling of historical lint-policy conflicts; verify static contracts and coverage merge/floor, not only lint.
2. Inspect reusable blocks and qualify a second genuinely distinct integrated board, engineering references openly before evaluation. Preserve fixed family-specific placement/routing in production.
3. Extend the shared command with explicit family capabilities, configuration validation, family selection and targeted clarification/refusal. Never weaken unsupported requests into supported ones.
4. Export and inspect consistent manufacturing-data bundles for both families; revalidate complete native boards and deterministic replay.
5. Run focused offline tests during development, full bounded integration tests and one compact final live evaluation after a new explicit request/dollar budget is approved.

The previous goal consumed all 41 authorized physical calls. Its ledger stays unchanged. Existing-key preference is retained, but **no live calls are authorized for this new goal yet**. No physical fabrication or bench-performance claim. Do not mark complete before every goal requirement is verified.

## Progress

- Confirmed clean merged base and failed main CI: the same two frozen-source cleanup findings cause the static failure and skip the dependent quality gate.
- Added exact historical lint exceptions following existing repository policy, with pre-lint immutable-source and negative-control tests. This is a disclosed exception, not a repair of historical source. See [CI.md](CI.md).
- Local full-repository lint passes with zero issues. The hash/negative-control and workflow-contract tests pass. Full bounded repository regression passed in 60.967 s (`.cache/board-family-v2/regression-ci-01`), with provider keys removed. The original 677-file evidence audit still passes. Remote CI, including the previously skipped quality gate, remains to be verified.
- Selected SHT31-DIS-B as the second reference candidate, reusing the existing sensor block and installed KiCad symbol/footprint. It adds temperature/humidity measurement with a different sensor, pad/net map and electrical contract. Sensirion's December 2022 v7 datasheet has been inspected; integration and native qualification are not yet complete.
