# Native readability and serialized replay: retained negative result

The approved offline follow-up is closed with **0/2 complete phase passes**. The standalone project now replays byte-for-byte from a serialized request, but its PCB geometry changed. The controller stops before project emission because its rotated reference/value fields are outside the new annotation audit's supported geometry. This is not a completed practical-board milestone or permission to enable the new profile by default.

## Final outcomes

| Unchanged development example | Final native workflow | Actual serialized project replay | Required PCB preservation | Phase outcome |
| --- | --- | --- | --- | --- |
| Standalone regulator | Both runs pass all nine required stages; zero ERC/DRC findings and writer skips | All 14 primary project files byte-identical; no normalization | Failed: all eight footprint poses differ; routes change from 37 segments/3 vias to 38 segments/5 vias | Failed |
| Controller ADC, separately recorded 100 mA case | Blocked at project write: `native annotation profile does not support rotated field C1.property.7` | Not reached | No final emitted board to compare | Failed |

The controller's transaction identifies property 7 as `Reference=C1`, rotated 270 degrees; its `Value=22n` has the same rotation. The final fail-closed check must not be removed merely to recover earlier development passes. Writer correctness, structural validation and native checks are skipped after the blocked write; the workflow simulation stage is absent. Closed-loop promotion simulations are separately retained and passed, not substituted for the missing final workflow stages.

The standalone native netlist and board retain all 16 physical pin/pad endpoints across three nets and eight physical components. Its second run really unmarshals a serialized workflow request. Exact replay does **not** establish preservation relative to the previous version.

## Implemented, but still experimental

- Opt-in `topology-v2` / `annotation-v2`; empty and `topology-v1` remain available. Unknown profile and rank-policy versions are rejected. No provider prompt/schema or frozen case was changed to select the new profile.
- Persisted `rank_policy: inferred-v1` keeps inferred layout hints from becoming fixed constraints after JSON decoding; explicit groups remain fixed.
- Native label and centered-field bounds, label/wire collision checks, a deterministic 50,000-candidate bounded label search, and nonmutating final drawing projection. Labels may move only within their original connected wire island.
- A source-ID, connector-pin and net-role reading guide derived from explicit IR. It neither renames electrical nets nor invents signal directions.
- KiCad 10 default-font note writing/reading and a separate emitted-geometry audit. Unknown text effects remain preservation-only content. Flat sheets and supported text angles are explicit limits; the project-write path fails closed.

## Why PCB preservation failed

`PlaceExplicitCircuit` uses `GenerationHash` (falling back to `ResolutionHash`) as the placement seed: `internal/designworkflow/explicit_pcb.go:36`. The drawing-profile change changes those hashes even though electrical/PCB input components, nets, regions, simulation, support, routing policy and catalog remain equal. The standalone generated PCB consequently has different placements and routes. The audit retains those differences and marks preservation false; it does not normalize them away.

The cross-phase schematic-circuit comparison excludes only the explicitly recorded `KiCadAI Resolution Hash` property, whose value changes with the versioned resolution. Every other schematic-circuit field is equal. Same-phase serialized replay excludes nothing from primary project files; `.kicadai` diagnostic copies remain retained separately in raw evidence.

An intermediate commentary statement that PCB routes were unchanged was premature. The final independent comparison disproved it. This report and `verification.json` are authoritative for the result.

## Evidence and review

Final code checks passed: 151 test packages plus 14 with no tests, six-package race checks, seven-package lint (zero issues), and repository-wide vet. The longest final test package took 715.006 seconds under the unchanged 720-second bound. A supplementary pre-final suite timed out and is retained, not omitted; it does not replace the final-source result. Passing code checks do not overturn the negative native/preservation gate.

- [Machine verification](verification.json), reproduced by `node specs/ai-requirement-contract-integration/native-readability-replay-offline-v1/verify.mjs`. Exit zero means the recorded **negative result is authenticated**, not that the phase passed.
- [Local review and visual assessment](REVIEW.md), [development history](DEVELOPMENT.md), [execution record](execution.json).
- [Historical preservation](preservation.json), [predecessor source/raw/archive authentication](previous-phase-verification.json), [archive authentication](archive-verification.json).
- [Final native log](native-tests.log), [focused regressions](focused-tests.log), [race checks](race-tests.log), [lint](lint.log), [vet](vet.log), and [full-suite log](full-tests.log).

Final raw root: `/tmp/kicadai-native-readability-replay-offline-v1-final`. It has 170 files and 1,520,345,919 bytes; inventory SHA-256 `03a8607120794aa86323a68970137aa085d1b727984d94140d61d49437e9c036`. Both generated originals were sealed before exports, only disposable copies were rendered, and originals were checked unchanged afterward.

Final source snapshot: `.cache/native-readability-replay-v1-sources/final.json`, binding all 23 changed Go files to base `ec2ececb4e52643f0767d0fcbb70213f1ec7209c`; snapshot SHA-256 `33a5702254063ea692be740661e7e99176bc8dc06cc6a0e62335105b4c4f0d9a`. The source was not changed after the final native failure.

All eleven development attempts (including compile-only dev7), ten development raw trees, the final raw tree, and twelve exact source snapshots are retained. Local archive: `.cache/native-readability-replay-offline-v1-all-runs.tar.gz`, 828,583,049 bytes, SHA-256 `7e0207678095c5fd7326456122f269e3de02ae23f4947692f2abcb8245d72704`. Fresh extraction was compared against every original tree and source snapshot, then independently rechecked. Raw/extracted evidence is intentionally not committed to Git or remotely published.

## Boundaries and next decision

This phase made zero provider calls, added zero frozen cases, and did not push, open a PR, merge, release, or fabricate a board. Existing credentials were used only in local exact-secret scans; no key value is included in evidence. The earlier authenticated practical campaign remains 0/8 complete primary positive boards; this pilot does not change that denominator. The original 150 mA thermal rejection and separately recorded 100 mA requirement remain distinct.

Further implementation needs a separately bounded follow-up: first decouple drawing-only metadata from PCB placement identity while preserving historical seeds; then model or deliberately normalize rotated fields with emitted-geometry/native-render tests. Retest the unchanged two examples before considering any new live campaign. Functional block grouping and compact presentation remain usability work, even where individual annotations are legible. The six-positive/two-new-paired-design milestone and reviewed PR remain unachieved.
