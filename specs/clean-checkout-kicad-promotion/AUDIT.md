# Clean-Checkout KiCad Promotion Acceptance Audit

Audit date: 2026-07-23

## Result

The milestone implements the accepted contract in `SPEC.md`: one
manifest-driven command resolves the locked KiCad toolchain, runs every
positive promotion-matrix scenario twice in isolated roots, requires every
internal and KiCad gate, compares normalized evidence, and emits an
independently verifiable content-addressed bundle.

The result proves reproducibility for the checked-in promotion matrix and
locked platform. It does not claim cross-platform byte identity, arbitrary
circuit coverage, fabrication readiness, or analog-performance verification.

## Phase History

| Phase | Commit | Verified GitHub Actions run |
| --- | --- | --- |
| Contract and baseline | `95ae21a3` | `30002664761` |
| Toolchain lock and resolver | `aa9dff2c` | `30004148705` |
| Manifest-driven two-run orchestration | `39654ac4` | `30006582174` |
| Normalized deterministic replay | `41d04dc6`, `2aaee160` | `30012901815` |
| Content-addressed bundle and verifier | `1789c6f0` | `30016644281` |
| Clean-checkout command and publication | `304451e9` | `30018787240` |

Each phase was staged, reviewed with Prism, committed, pushed, and allowed to
reach a green Actions result before the dependent phase proceeded.

## Reproduction Contract

From a clean checkout, the sole promotion command is:

```sh
make promotion-bundle
```

The command:

- reads `toolchain/kicad-promotion.lock.json`;
- discovers the exact locked installation or verifies and installs the locked
  bootstrap distribution;
- derives scenario commands from the generic lane registry and
  `testdata/external-review-mitigation/matrix.json`;
- runs each positive scenario twice with isolated `HOME`, KiCad configuration,
  and output roots;
- requires clean ERC, strict DRC, connectivity, route completion, writer
  correctness, and zero KiCad round-trip differences;
- compares normalized project and evidence inventories;
- writes `manifest.json` plus `manifest.sha256`; and
- verifies the directory name and every inventoried byte before returning
  success.

An existing bundle can be verified offline, without KiCad, with:

```sh
go run ./cmd/kicadai-promotion verify --bundle /path/to/sha256-<digest>
```

## Published Evidence

The installed-KiCad publication workflow for source commit
`304451e9bc3fd022cc75b14cd8246c6b7cec81bf` completed successfully as Actions
run `30018805756`.

- Artifact ID: `8568671680`
- Artifact name:
  `kicadai-promotion-304451e9bc3fd022cc75b14cd8246c6b7cec81bf`
- Uploaded ZIP SHA-256:
  `906c8762d5933aa44ab0f49c14c64e482c5c84ff84fa25a9318a338fb65abc9`
- Verified content-addressed directory digest:
  `eceb4fe9a5e3ec9adbe2afb0bbf9f105093b5ed63d189f2e1aa77ed5b7944cc7`

The corresponding local clean-checkout run used KiCad `10.0.3`, executed 12
scenario runs and six deterministic comparisons, inventoried 286 files, and
produced verified bundle digest
`a16e359ed7a4a4090945134725855e911a29832243630d3873ccf397ac13d414`.
The locally bootstrapped path independently passed with bundle digest
`6cdcfe1a6abcc57a33268613832547046a89c37f3c1676a8031be45a2f80239e`.
Bundle addresses differ when their recorded source/toolchain provenance
differs; each receipt verified against its own manifest and inventory.

## Regression Preservation

The final phase exposed a placement regression in the protected 10 W Class AB
speaker fixture. The atomic rigid-group planner had committed an interior
decoupling group before reserving space for the rest of the board. The generic
correction now:

1. previews the ordinary deterministic placement for exactly one movable
   interior rigid group;
2. treats successful ungrouped placements as reservations;
3. searches and commits the rigid transform atomically against those
   reservations; and
4. falls back to the existing bounded atomic search if the reservation-backed
   plan is infeasible.

Edge-constrained and joint multi-group designs continue through the established
edge-first/joint planner. No circuit ID, fixture path, net name, coordinate,
component family, allowlist, or schema exception was added.

Installed KiCad `10.0.3` passed the combined required design set:

- `class_a_bjt_line_preamplifier`;
- `class_ab_headphone_driver`;
- `class_ab_headphone_protected`;
- `class_ab_speaker_10w_protected`;
- `esp32_wroom_32e_minimal_pass`;
- `usb_c_i2c_sensor_3v3_protected`; and
- `usb_c_led_indicator_protected`.

The combined amplifier/USB rerun completed with clean routing, writer
correctness, ERC, strict DRC, connectivity, route completion, and zero
round-trip differences. The external-review matrix also passed twice across
the CLI, placement, circuit-graph, design-workflow, and creation-evidence
packages.

## Verification Commands

The final worktree was checked with:

```sh
make lint
make GO_TEST_FLAGS=-count=1 test
make review-matrix
go test -count=1 ./internal/writercorrectness ./internal/kicadfiles/roundtrip
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  go test -count=1 ./internal/designworkflow \
  -run '^TestDesignExamplesOptionalKiCadBackedTier$/(class_a_bjt_line_preamplifier|class_ab_headphone_driver|class_ab_headphone_protected|class_ab_speaker_10w_protected|esp32_wroom_32e_minimal_pass|usb_c_i2c_sensor_3v3_protected|usb_c_led_indicator_protected)$'
git diff --check
```

The exact installed promotion command is rerun from the committed final-phase
source before publication, because the clean-checkout contract intentionally
rejects a dirty worktree.

## Prism Review

Repeated staged Prism review identified and drove fixes for:

- an ambiguous dynamic-versus-static placement-state invariant;
- unnecessary keepout recomputation;
- loss of a preview anchor in the per-group fallback;
- an idiomatic test-context issue;
- explicit preview completeness validation; and
- duplicated anchor-triggered keepout refresh logic.

The remaining performance observation was adjudicated as an intentional,
bounded tradeoff rather than an unresolved material finding. Preview runs only
for exactly one interior translated group, every successful ungrouped placement
is reused by the final pass, incomplete previews fail open to the existing
bounded planner, and edge or joint multi-group searches never pay the cost.
Candidate evidence is recomputed from final-state-valid dimensions rather than
copying provisional scores. The complete placement package remained below one
second (`0.689s` in the final focused run), while the installed 10 W Class AB
fixture completed in `32.35s` with all gates green.

The final review's remaining state/metric observations were inspected and
rejected as false positives: rigid candidates are added to occupancy and
`placedByRef` during group planning but appended to `Result.Placements` exactly
once when their outer-loop component is visited; `UnplacedCount` starts at zero
and increments only on a failed placement, matching the ordinary placement
path. The placement suite plus the combined installed-KiCad amplifier/USB run
exercise both paths.

## Remaining Boundary

The orchestrator makes supported evidence reproducible; it does not broaden the
registered architecture providers, component catalog, solver families, or
placement/routing capability envelope. The next milestone toward arbitrary
AI-directed circuit generation should measure and expand held-out functional
coverage while keeping this promotion bundle as the non-regression gate.
