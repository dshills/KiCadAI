# v1.0.1 Engineering-Efficiency Plan

## Phase 1: baseline and cost model

- Preserve the v1.0.0 GitHub quality-gate duration and coverage totals.
- Extract package timings and profile the dominant open-topology top-level
  tests.
- Check in only the deterministic scheduling weights, not transient profiles.

## Phase 2: authenticated sharding

- Split ordinary packages and the dominant open-topology package into separate
  deterministic shard groups.
- Assign work with stable longest-processing-time scheduling: descending cost,
  lexical identity, then lowest-index least-loaded shard.
- Emit canonical test/package inventories and set-mode profiles.
- Merge only after complete-source and environment authentication.

## Phase 3: exact proof reuse

- Derive a proof key from repository bytes and tracked modes, Go identity,
  relevant environment, timeout, skip expression, and shard identity.
- Publish cache entries atomically.
- Verify cached artifacts before copying them and verify them again at merge.

## Phase 4: CI and action hardening

- Run four ordinary-package shards and six open-topology shards independently.
- Merge them in one required `Offline quality gates` job.
- Keep promotion and external-review jobs semantically unchanged.
- Pin verified Node.js 24 revisions of official GitHub Actions.

## Phase 5: preservation and closeout

- Prove exact test/package inventory and the unchanged coverage floor.
- Run a cold profile and an exact cache replay; record wall, CPU, memory when
  the platform exposes it, and cache effectiveness.
- Run all bounded local and installed-KiCad release gates.
- Review the staged change with Prism, remediate, and publish a completion
  audit with the natural pull-request CI measurement.
