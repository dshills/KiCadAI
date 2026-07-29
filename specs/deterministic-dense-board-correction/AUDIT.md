# Deterministic Dense-Board Correction Audit

Date: 2026-07-29

## Result

The milestone is implemented and passes its bounded local acceptance suite.
Correction is derived from normalized issues and the current generated route
operation slice. Routing-only recovery does not invoke placement, and
placement recovery expands the reroute set only from pads on components that
actually moved.

No production path contains fixture identities, fixture coordinates,
allowlists, block-family dispatch, or topology-specific repair logic.

## Requirement Evidence

| Requirement | Current evidence |
| --- | --- |
| Diagnose crossings and correlate both tracks | `ROUTE_COPPER_CONFLICT` carries both sorted net identities; `BuildAutonomousCorrectionDiagnosticsForRouting` maps the issue to every current generated route operation on both affected nets. |
| Reject stale or ambiguous mutation scope | Canonical operation IDs, indexes, route-state hashes, and exact current-slice checks stop with `route_operation_scope_mismatch`. Unit coverage includes identity, path, net, missing, ambiguous, and stale cases. |
| Reroute only affected nets | `ApplyAutonomousRoutingCorrectionPlan` partitions operations by affected-net membership, presents preserved copper as fixed net-aware obstacles, routes only the affected request subset, and splices replacements at the first affected position. |
| Preserve unrelated copper | Canonical bytes and relative order for non-affected operations are compared before accepting a candidate. The three-net corpus and direct integration test both preserve the decoy operation byte-for-byte. |
| Correct branch order deterministically | Route-tree execution evaluates the access-ranked baseline, failed-last, failed-first, and canonical endpoint-key orders; duplicate orders are suppressed and selected-order evidence is recorded. Tests cover identity-neutral canonical order, stable failure partitions, recovered-result ranking, and immutable inputs. |
| Correct legal layer transitions | The smallest correction inserts a via only at a proven same-net cross-layer junction under the resolved layer/via policy. Illegal transitions remain unchanged and stop; otherwise affected-net-only layer-aware routing is attempted. |
| Preserve endpoint and placement contracts | Routing-only actions bypass the placer. Placement actions retain the existing mobility and hard-constraint checks, and moved-pad connectivity mechanically expands the affected-net set. |
| Preserve topology and rules | The existing circuit invariant fingerprint is checked before application. Selective routing reuses the unchanged normalized board, obstacles, net endpoints, widths, clearances, allowed layers, and via limits. |
| Identity-neutral held-out behavior | The dense-board corpus covers a renamed three-net crossing, a renamed legal missing junction, an illegal transition, unrelated bystanders, deterministic replay, and fail-closed behavior. The existing generic stress fixture covers bounded placement/routing recovery under renamed identities. |
| Deterministic replay | Unit and corpus runs compare diagnostics, plans, applications, operations, and preserved canonical bytes across repeated runs. The routing performance suite also retains deterministic route signatures and metrics. |

## Local Verification

All commands used `GOCACHE=/tmp/kicadai-go-build`.

- `go test ./... -short -count=1 -timeout 300s`: pass. This covers every
  package while intentionally skipping the explicitly long promotion and
  optional-KiCad lanes.
- `go test -v ./internal/compositionlowering -run
  '^TestClassABDynamicPerformancePlansPass$' -count=1 -timeout 600s`: pass in
  158.13 seconds, including output power, THD, phase margin, and gain margin.
- Focused offline promotion corpus for neutral MCU synthesis, power interfaces,
  standalone and held-out clocks, clock programming, and MCU power integrity:
  pass in 322.59 seconds. The Class-AB power-interface case passed.
- `go test ./internal/intentplanner ./internal/designworkflow -count=1`: pass
  after confirming ordinary strict-DRC workflows continue to delegate
  conservative clearance findings while correction-enabled workflows retain
  structured cross-net conflicts.
- `TestAutonomousCorrectionStressOptionalKiCad`: pass with installed KiCad.
- Installed-KiCad promotion set: pass for
  `class_a_bjt_line_preamplifier`, `class_ab_headphone_driver`,
  `class_ab_headphone_protected`, `class_ab_speaker_10w_protected`,
  `esp32_wroom_32e_minimal_pass`,
  `usb_c_i2c_sensor_3v3_protected`, and
  `usb_c_led_indicator_protected`.

The installed-KiCad fixture harness requires clean ERC, strict DRC,
required-net connectivity, route completion, writer correctness, and zero
round-trip differences. The final seven-fixture run passed in 45.36 seconds;
the generic correction stress fixture passed in 6.52 seconds.

## Scope Review

Production changes use only structured issue codes, issue net identities,
canonical transaction content, current board geometry, resolved routing rules,
component mobility, and pad/net membership. Fixture names and coordinates
occur only in tests and corpus data.

Prism review was not run because this audit does not carry fresh authorization
to send the staged diff to the configured external provider.
