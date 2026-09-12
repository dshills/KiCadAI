# Ownership and support locality — offline v2

**Two technical examples pass and both meet the decoupler-locality target; complete readability remains 0/2. No practical benchmark pass was added.** The malformed support-parent fallback is hardened and a versioned, opt-in drawing policy now puts support parts near their owner. The original practical campaign remains [0/8 complete primary-board passes](../../practical-sensor-controller-boards/PROTOCOL-V2-RESULTS.md).

This phase follows `c2ec8c88da11f8cf4aac498775e216ae2baaa34d`. It is an offline engineering improvement, not a new AI evaluation campaign, fabrication approval or release. See the preregistered [plan](PLAN.md), complete [development record](DEVELOPMENT.md), and [source/visual review](REVIEW.md).

## What improved

Lowering rejects invalid known support parents instead of misclassifying their components as external interfaces. Both profiles reject unknown children/parents, cycles, duplicate parents and fragment/support conflicts without mutating the caller's request. The new `ownership-v2` profile also requires consistent provenance-source categories. The prior `ownership-v1` geometry path remains unchanged.

`ownership-v2` uses explicit synthesis parents, or the unique active component in an explicitly owned fragment for otherwise unparented decouplers. A deterministic bounded search uses transformed symbol/pin-label envelopes and clearance. No example IDs, coordinates, seeds, net aliases or component values are hard-coded. Fixed placements are preserved. Ambiguous multi-active groups are not assigned guessed parents.

| Final example | Mean decoupler distance, before → after | Maximum, before → after | Individual decoupler regressions |
| --- | --- | --- | --- |
| Standalone regulator | 67.21 → 33.02 mm (50.9% lower) | 79.56 → 35.56 mm | 0/4 |
| Controller ADC, 100 mA | 89.03 → 35.92 mm (59.6% lower) | 139.14 → 43.18 mm | 0/7 |

Distances are Euclidean schematic symbol-origin measurements, not PCB distances or wire lengths. Both examples meet the preregistered lower-mean/lower-maximum/no-decoupler-regression target. Controller reset C10 and R3 moved slightly farther from U3; all measurements, including those regressions, are retained in [readability-metrics.json](readability-metrics.json).

Complete readability remains blocked by opaque net names, a detached and split reading guide, large unused page areas, weak field association in the analog block, and placement that does not yet express conventional input/output flow. Regulator input/output bypass parts can appear on the opposite schematic side from their functional role. Electrical connectivity remains exact; geometric proximity is not a readability certificate.

## Native and preservation evidence

Both unchanged requirements pass all nine strict stages on their first run and actual JSON-decoded replay. All-severity, full-project KiCad ERC/DRC reports contain zero findings, writer checks have zero skips/failures, and emitted annotation audits have zero issues. All 14 regulator and 23 controller primary project files replay byte-for-byte, with no normalization. Development and final requests and primary projects also match exactly.

The regulator retains 8 physical components, 3 nets, 16 connected physical endpoints, 37 routed segments and 3 vias on two layers. The controller retains 21 physical components, 10 nets, 52 connected endpoints, 203 segments and 29 vias on four layers. The non-layout request, component values, pin/pad bindings, generation and placement seeds, PCB poses and all routes are unchanged. Cross-phase board comparison resolves numeric net IDs to names and excludes only drawing-paper settings; same-phase replay is exact. See [verification.json](verification.json).

Both rails remain 3.3 V. Model-based junction temperatures remain 103.05°C and 114.05°C; controller cutoff remains 923.36 Hz. The 100 mA controller is the separately recorded variant, not a replacement for the original 150 mA thermal rejection. Acceptance remains `erc-drc`, with `fabrication_ready: false`. No bench or manufacturing validation is claimed.

## Test results

All planned checks passed. The full short suite finished with 151 passing test packages and 14 packages without tests; the longest package, opentopologysynthesis, took 690.673 seconds under the unchanged 720-second bound. Compositionlowering passed in 89.145 seconds, retaining the existing 150 mA rejection test. No failure was retried or timeout extended.

| Check | Result | Receipt |
| --- | --- | --- |
| Final focused tests | All three packages pass | [focused-final](focused-final.execution.json) |
| Final native evaluation | 2 examples × 2 runs, 9 strict stages each | [native-final](native-final.execution.json) |
| Full short suite | 151 pass, 14 without tests | [full](full.execution.json) |
| Ownership-path race tests | Pass, 2.764 s | [race-ownership](race-ownership.execution.json) |
| Full schematiclayout/schematicir short race tests | Pass, 3.505 s / 208.727 s | [race-layout](race-layout.execution.json) |
| Vet / lint | Pass / zero issues | [vet](vet.execution.json), [lint](lint-final.execution.json) |

This closes race coverage for the changed ownership/layout paths. The previously timed-out full compositionlowering race suite was not rerun and is still not certified; the scoped passes do not replace that historical failure. Full-suite wall time was 771 seconds because the timeout is per package, not per invocation.

## Scope and authenticated artifacts

One development native run and one frozen final native run were used; the second permitted development run was unused. No Go source changed after the final evaluation began. Tests ran without provider credentials and with dependency networking disabled. No API calls, external review, push, PR, merge, release or fabrication occurred.

- [Final source/native verification](verification.json): 11 changed Go files; 387 raw final files, 1,531,697,222 bytes; inventory SHA-256 `cd3dd9467ede0ff7efb1ffe590f95a3fd1a463a88335b5c50427d4d7171813cc`.
- Final source snapshot: [.cache/ownership-locality-v2-sources/native-final.json](../../../.cache/ownership-locality-v2-sources/native-final.json), SHA-256 `60ee989ea6de4ff6815f0df2a2ac0c62183bc9667f12231334df0c14b913d471`.
- [Archive receipt](archive.json) and [complete member manifest](archive-manifest.json): 785 files, 155,401,900 compressed bytes, SHA-256 `10bb71602b9707f902b1ad956106706708eea1205875d3ca75d0c1408da0fab4`. Every original and archived member was verified, without a second full extraction. Exact local key scan: absent.
- [Historical authentication](history-authentication.json): preceding source/raw/archive evidence and original campaign preservation chain remain intact.
- [Final QA](qa.json): source/log/receipt integrity, execution outcomes, locality target checks, development/final equality, links and local secret scanning.

The final raw root is `/tmp/kicadai-ownership-locality-offline-v2-final`; the archive is [ownership-locality-offline-v2-all-runs.tar.gz](../../../.cache/ownership-locality-offline-v2-all-runs.tar.gz). Both full sheets, six dense-group crops and every copper layer were visually inspected from disposable render copies. Originals were sealed before export and rechecked afterward.

Evidence-quality and validation skills kept electrical/native passes, locality improvement, visual readability and benchmark readiness separate. The overall practical-board goal remains unachieved. This bounded phase does not authorize another source iteration or live campaign.
