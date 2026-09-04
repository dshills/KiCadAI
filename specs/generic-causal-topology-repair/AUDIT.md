# Generic Causal Topology Completion and Bounded Repair Audit

Status: complete; frozen public material-improvement and preservation gates passed

Starting commit: `6baf59ad9`

## Frozen evidence

- V20 public generation-zero population: 24 cases, 1 pass, 5 unsupported,
  1 unsafe, and 17 exhausted.
- Selected public topology frontier: 8 cases across 5 reporting domains,
  derived from 47 `causal_topology_repair` and 9 `complete_topology`
  occurrences.
- V18–V20 reports, evaluator boundaries, corpora, and safety evidence remain
  immutable historical inputs.

## Implementation evidence

- V21 derives typed reachability, reference-closure, observation-cone, causal-
  path, terminal-compatibility, and conflicting-driver obligations from the
  behavior contract, primitive terminal contracts, and current graph.
- Generic add/connect/redirect/split/feedback operations are generated in
  canonical order. Search is bounded by depth, width, work, retained states,
  and graph size, with stable-hash deduplication, ancestor-cycle rejection,
  strict structural improvement, and complete operation provenance.
- The exact V20 analysis/model/solver admission path runs before V21 topology
  work and remains attached to every candidate. V21 does not add models,
  solvers, components, topology templates, circuit-family branches, fixture
  coordinates, or expected solutions.
- Selection is bound to the authenticated public population by canonical
  requirement hash. Every unselected result delegates to V20 byte-for-byte.

## Frozen public evaluation

The committed evaluator ran all 24 authenticated discovery cases serially and
exactly twice from clean roots with installed KiCad 10.0.3. The authenticated
report is
`internal/capabilityfeedback/testdata/closed_loop_open_set_v21_generation_zero/report.json`
with file SHA-256 `4d74d04613aef46f161c54661dd7851845094623a87528c3172d99af786749a0`
and report hash `7e7e4a73a3e78f15c8c30f3d2dc7eb5f058d128515cf91b7ce858aeb93afd428`.

- V21: 1 pass, 5 unsupported, 1 unsafe, and 17 exhausted.
- All 48 replay hashes match.
- All 16 unselected case objects are exactly equal to V20.
- The V18 pass and unsafe result are preserved. The pass completed two
  installed-KiCad promotions with routing, connectivity, writer correctness,
  zero round-trip differences, ERC, strict DRC, deterministic replay, and
  fail-closed gates all true.
- Six selected cases across four reporting domains advanced from topology
  exhaustion to the later `OPEN_TOPOLOGY_NO_PASSING_GRAPH` behavioral-evidence
  blocker. This exceeds the frozen threshold of three cases across two
  domains.
- One selected case reached the explicit V21 bound; that failure rename does
  not count as advancement. One selected case remained at its V20 topology
  exhaustion. Neither is claimed as improvement.
- V21 remains experimental and does not expand the v1 supported surface or
  claim arbitrary-circuit generation.

Sandboxed macOS runs were rejected before publication because AppKit denied
the installed KiCad zone-refill process. The exact command passed outside the
sandbox with zero violations. The unchanged frozen evaluator was therefore run
outside that sandbox from a fresh root; no production retry or bypass was
retained.

## Final local validation

Validation completed on 2026-09-04 against the immutable implementation and
evaluation commit
`890d5a00029b91183d970b00e3fe3a5440310d27` without manually triggering
GitHub Actions:

- `make lint` and the complete serial `make test-bounded` suite passed.
- The ten-shard coverage run executed 6,350 tests and reported 80.8% coverage
  after excluding generated protobuf bindings, above the 75% threshold.
- Release artifacts for macOS and Linux on amd64 and arm64 were
  byte-reproducible; the packaged macOS arm64 binary passed the release smoke
  test.
- The external-review regression matrix passed twice.
- The supported public demo completed two installed-KiCad promotions with
  identical project hashes. Its out-of-envelope companion failed closed with
  `OPEN_TOPOLOGY_REPAIR_EXHAUSTED` and emitted no KiCad project.
- The six-scenario clean-checkout promotion bundle passed two full iterations
  (12 total runs) and authenticated as
  `sha256-c78451a2cd1e507e159859cf9f03cf528593a9cb9fbc2b6619d1b32b828f7439`.
- The complete 13-case installed-KiCad design-example tier passed, including
  protected USB-C LED and I2C sensor boards, ESP32-WROOM-32E, Class-A and
  Class-AB amplifiers, op-amp, sensor, routing, writer, and round-trip evidence.
- All five educational schematics passed two replays for checked-in
  reproducibility, graph-derived conventional layout without placement
  recipes, and installed-library diagnostics.
