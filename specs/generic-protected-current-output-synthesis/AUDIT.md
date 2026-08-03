# Generic Protected Current-Output Synthesis Audit

Date: 2026-08-03

## Result

The frozen protected programmable current output, independently authored
fault-protected low-side sink, and independently authored startup-safe
high-side source all pass deterministic behavior-only synthesis and two-run
local installed-KiCad promotion. The implementation derives direction,
transconductance, control composition, compliance, value, thermal, SOA,
lowering, placement, and routing decisions from normalized behavior, graph
relationships, and reviewed evidence. It adds no corpus identity, project
name, fixture coordinate, catalog-ID exception, allowlist, schema, or named
current-driver block family to production code.

## Requirement Audit

| Requirement | Authoritative evidence | Result |
| --- | --- | --- |
| Failure plus two independent variants frozen before implementation | Checksum-locked corpus manifest, freeze tests, `BASELINE_REPORT.json`, and `BASELINE_REPORT.sha256` | Pass |
| Baseline failure diagnosed by synthesis stage | `DIAGNOSIS.md` and frozen baseline counts identify incomplete orientation/control architecture, shape-specific value scaling, trusted command-transfer failure, bounded repair exhaustion, and unreached physical lowering | Pass |
| Source and sink architectures derived generically | Relationship, topology search, value-domain, and simulation tests cover low-side sink and high-side source polarity without corpus identity | Pass |
| Current accuracy and compliance | Selected candidates pass every declared command, supply, load, tolerance, temperature, and headroom assertion | Pass |
| Startup permission and fault-dominant shutdown | Directed transient evidence covers default-safe startup, independent permission, asserted fault, and off-state current | Pass |
| Dissipation, thermal, overload, and SOA | Critical assertions are measured from reviewed model/rating evidence; insufficient or unsafe evidence remains blocking | Pass |
| Deterministic repair and fail-closed behavior | Bounded repair traces, consumption accounting, unsafe perturbations, and two-run byte equality are covered by the package suite | Pass |
| Conventional readable lowering | Topology-derived ranks, left-to-right boundary flow, explicit local control/output trees, power/return lanes, auxiliary spreading, and collision-safe labels are present in the rendered KiCad schematics | Pass |
| Physical realization | Every valid case has complete placement, route completion, connectivity, writer correctness, clean ERC, strict DRC, and zero normalized schematic/PCB round-trip differences | Pass |
| Deterministic installed-KiCad replay | Each case passes twice in an isolated retained root with identical synthesis, topology, physical, project, and normalized evidence hashes | Pass |
| Preservation | Frozen 8/8 benchmark, both neutral synthesis and physical promotions, four amplifier fixtures, protected USB-C LED, and protected USB-C I2C pass locally | Pass |

## Promotion Matrix

| Case | Synthesis | Topology | Physical | Project |
| --- | --- | --- | --- | --- |
| `fault_protected_low_side_current_sink` | `5c99f07e0437f363ca532dc90b56a2d690744b475a85a9b78477b63af4d15e2e` | `0e086b5b7b9859f65d01aa6562481a65ab542eeb687a7d8ebac2449527a0a60a` | `23412a867f376f8fd1388acee1f195c3eb2c469e0c4cdc61c9aadc142aeea4de` | `08b48cdb0d4ce33b84f0829920611b2236f4cd49317a708e3b621a7738b9247b` |
| `protected_programmable_current_output` | `4b97cc1ebafb6ed9682d314add22b8bcc9486e606d99c81d33543ab610af348c` | `f1d8e2d71aee8c2d6de3795109c1956750af1958cb9dd9cc2b2514734e1fac71` | `0fecaee12e0161877351c5f9dad0a6cc13f0bac110b84ffaecdae8f391ec5345` | `286480e8e69f4fe72904036377bdd5cc91b470c0c6bbf8895797097357ca2d50` |
| `startup_safe_high_side_current_source` | `73997954ddbba5508c944d1b62ba4ee256891da68843ceda146ab56da8472a1c` | `2b768a7f431c59d8373f931b93afd8a4c90255419cc9557be314af976517eef1` | `e339b08d2848b2d787aba21d32d5a0712f5420e2094999a29bffafedb8005121` | `ff669a64e609ef801eee845abeae136a8773100a94e3b1b8154d6507d9525d0b` |

Retained final artifacts are under
`/tmp/kicadai-protected-current-final-019f6b30-v14/`. They are local
verification products and are intentionally not repository inputs.

## Local Verification

The following lanes passed on 2026-08-03:

```text
env GOCACHE=/tmp/kicadai-gocache KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT=/tmp/kicadai-protected-current-final-019f6b30-v14 go test ./internal/opentopologysynthesis -run '^TestProtectedCurrentOutputCorpusOptionalKiCadPromotion$' -count=1 -timeout=20m -v
env GOCACHE=/tmp/kicadai-gocache KICADAI_PROTECTED_CURRENT_OUTPUT_PROMOTION=1 KICADAI_OPEN_TOPOLOGY_PROMOTION=1 go test ./internal/opentopologysynthesis -run '^(TestProtectedCurrentOutputCorpusPromotion|TestFrozenHeldOutCorpusSimulationPromotion|TestMultiBranchAnalogNeutralCorpusPromotion)$' -count=1 -timeout=20m -v
env GOCACHE=/tmp/kicadai-gocache KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT=/tmp/kicadai-neutral-preservation-019f6b30-v4 go test ./internal/opentopologysynthesis -run '^TestMultiBranchAnalogNeutralCorpusOptionalKiCadPromotion$' -count=1 -timeout=20m -v
env GOCACHE=/tmp/kicadai-gocache KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli go test ./internal/designworkflow -run '^TestDesignExamplesOptionalKiCadBackedTier$/(class_a_bjt_line_preamplifier|class_ab_headphone_driver|class_ab_headphone_protected|class_ab_speaker_10w_protected|usb_c_led_indicator_protected|usb_c_i2c_sensor_3v3_protected)$' -count=1 -timeout=30m -v
env GOCACHE=/tmp/kicadai-gocache go test ./internal/kicadfiles/designapi ./internal/schematiclayout ./internal/schematicir -count=1
env GOCACHE=/tmp/kicadai-gocache go test ./internal/compositionlowering -run '^TestFrozenOpenSetCorpusPassesOfflineWorkflow$' -count=1 -timeout=15m
env GOCACHE=/tmp/kicadai-gocache go test ./internal/circuitgraph -run '^TestFunctionLevelCorpusCapabilityReportMatchesAuthoritativeEvidence$' -count=1
```

Installed-KiCad lanes used KiCad 10.0.3 and fresh local artifact roots. No
GitHub-hosted test run was used as completion evidence.

A repository-wide `go test ./... -count=1 -timeout=20m` attempt passed every
completed package except `internal/compositionlowering`, where the known
`class_ab_dynamic_output_stage.json` case exceeded the aggregate timeout. The
only separate actionable failure from that attempt, the translated-sensor
cross-net label contact, is covered by the focused five-case composition lane
above and now passes. No broad complete-suite claim depends on the timed-out
dynamic case.

## Remaining Boundary

This proves protected programmable current sources and sinks only inside the
reviewed primitive, component, model, rating, simulation, and two-layer
physical envelope exercised by the corpus. It does not establish arbitrary
current-controller ICs, switching current regulators, RF behavior,
mains/high-energy safety, unreviewed substitutions, heatsink qualification,
or unrestricted dense-board generation. Those requests must continue to fail
closed or enter evidence-backed capability expansion.
