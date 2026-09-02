# v1.0.1 Engineering-Efficiency Specification

## Objective

Shorten the required bounded verification loop without changing KiCadAI's
frozen v1 circuit-generation surface. The milestone is release hardening, not
capability expansion.

## Frozen behavior

- The v1 supported/refused boundary, schemas, CLI behavior, circuit selection,
  routing, writers, and generated KiCad artifacts do not change.
- Every top-level test selected by the former `go test -short ./...` coverage
  invocation remains selected exactly once.
- The generated-excluded coverage floor remains 75 percent.
- Historical corpus seals, replay identities, and two-run ordering remain
  unchanged.
- Installed-KiCad promotion remains a separate required release gate.

## Required implementation

1. Record the v1.0.0 quality-gate baseline and package/test costs.
2. Partition the bounded coverage suite with a deterministic, measured-cost
   scheduler. The same inputs must always produce the same shard membership.
3. Run shards under a bounded worker count locally and as independent CI jobs.
4. Authenticate each profile, exact test inventory, package inventory, source
   identity, environment identity, and resource report before merging.
5. Reject stale, missing, duplicated, malformed, or unexpected artifacts.
6. Preserve Go's existing package-coverage semantics and merge duplicate
   set-mode regions with logical OR.
7. Cache only an exact authenticated proof. Any repository byte/mode,
   toolchain, environment, selection, or shard change must miss or fail closed.
8. Preserve the existing release, promotion, and external-review semantics.
9. Upgrade official GitHub Actions from Node.js 20 runners to verified Node.js
   24-compatible revisions.

## Performance targets

- GitHub's required offline quality gate completes in less than 15 minutes.
- A cold local bounded coverage run is materially faster than the former
  monolithic 46-minute GitHub job.
- An exact proof-cache replay completes in seconds and still reauthenticates
  every shard before merge.

## Acceptance

- Exact bounded test and package selection is machine-checked.
- Coverage remains at or above the frozen 75 percent floor and reports the same
  generated-excluded percentage as the v1 baseline on the reference run.
- Shard manifests and payload hashes authenticate independently.
- A changed input cannot reuse an old proof.
- Local formatting, lint, bounded/race/replay, seal, workflow-contract,
  release, and installed-KiCad gates pass.
- Prism has no unresolved finding.
- No circuit capability, schema, routing, writer, or support-boundary change is
  included.

## Next capability milestone

After this milestone is complete, begin a separately specified capability
milestone for generic analysis/model/solver availability. That work should be
driven by the highest-impact V19 failure cluster and must not be mixed into
this release-hardening change.
