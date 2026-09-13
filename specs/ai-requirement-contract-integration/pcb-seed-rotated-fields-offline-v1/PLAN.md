# PCB seed and rotated-field follow-up

Approved 2026-09-12 after the recorded negative result at `446e19c9b934ad8026b4e9ec2996b47d9ef413f9`. This is a new bounded offline phase, not a retry of the frozen prior evaluation. The full practical-board milestone remains unchanged and unachieved.

## Scope and invariants

Implement reusable corrections for drawing-profile coupling to PCB placement seeds and unsupported cardinally rotated reference/value fields. Retest only the unchanged standalone regulator and separately recorded controller ADC 100 mA requirements. Preserve electrical components, nets, physical pin/pad mappings, constraints, and the original 150 mA thermal rejection. Compare PCB placements/routes to the last emitted topology-v1 boards (`schematic-readability-offline-v1`), not to the drifted topology-v2 board. Preserve legacy seeds and receipts; do not hard-code example seeds, copy old placements into generation, hand-edit generated files, weaken native gates, or normalize away physical differences.

No API/provider calls, external review, new live campaign, push, PR, merge, release, fabrication or changes to prior evidence. Provider credentials must be removed from test environments. Current branch may receive the scoped local implementation, tests and evidence commit.

## Preregistered bounds and evidence

- At most two development native evaluations and one final frozen-source native evaluation, each with the existing 20-minute per-example context and 45-minute test-process bound. Each evaluates both unchanged examples with two runs, the second from an actual JSON round trip. Retain every attempted run and exact changed-source snapshot. No new native run after the final evaluation, even on failure.
- Targeted offline unit tests may guide the two fixes before final source freeze. One final full short suite, with the existing 12-minute per-package timeout; focused race checks, lint and vet. Use cached dependencies only. Retain failures, do not extend timeouts to convert failures into passes.
- Keep new evidence under a new phase directory and new raw roots. Check free space before native evaluation; stop before consuming the last 10 GiB. Retain raw outputs plus a compressed archive; authenticate its contents without creating another full disk copy.
- Before final rendering, seal generated project trees. Render disposable copies only; inspect all final sheets and dense areas, plus both copper layers. Recheck seals after export. Independently compare serialized request/project replay, strict nine-stage outcomes, exact physical connectivity, emitted annotation audit, and historical PCB geometry.
- Authenticate the previous final raw inventory/archive and frozen source snapshot, retaining its negative result. Record source, commands, exit codes, assertions, review findings, limitations and all final gate outcomes in inspectable repository artifacts.

If either bounded development work exhausts its allowance or final verification exposes any blocker, retain and publish the negative outcome without another repair/evaluation iteration. Neither passing unit tests nor two development examples establishes the six-positive/two-new-design milestone.
