# Functional grouping and readability, offline v1

Approved 2026-09-12 as a bounded follow-up to `9ac563194a4a3b89d52d54b8ee38ce04a157ff58`. The practical-board milestone remains unchanged and unachieved. This plan precedes implementation and evaluation.

## Scope

Implement a reusable opt-in drawing policy that keeps functional support components near their owners and makes reference/value association and circuit-block reading clearer. Derive ownership from explicit lowering/synthesis provenance; do not guess from reference numbers or hard-code example identities, coordinates, or seeds. Ambiguous ownership must remain explicit or unassigned. Preserve existing profiles and their semantics.

Evaluate only the unchanged standalone regulator and separately recorded controller ADC 100 mA requirements. Preserve all circuit components, values, physical pin/pad bindings, net names, electrical constraints, simulation assertions, PCB seed and geometry. Keep the original 150 mA thermal rejection. Do not claim fabrication readiness or benchmark passes from these examples.

No API calls, external review, new live campaign, push, PR, merge, release, fabrication, or edits to historical evidence. Remove provider keys from test processes; use cached dependencies with dependency networking disabled. A scoped local implementation, tests, evidence and review commit is authorized by this workflow.

## Preregistered bounds

- At most three development native evaluations and one frozen-source final native evaluation. Each uses the same two examples, with a second request actually decoded from JSON, 20 minutes per example and 45 minutes per test process. Preserve each attempt and exact changed-source snapshot. No native run or source repair after the final evaluation.
- Focused offline unit tests may guide implementation before freeze. One final full short suite, unchanged 12-minute per-package limit, plus focused race, lint and vet. Retain failures; do not extend timeouts or weaken checks to convert failures to passes.
- Check free space before native execution and retain at least 10 GiB. Preserve new raw roots and a compressed archive, verifying it without a second full extraction. Never delete prior evidence to obtain space.
- Seal original generated project trees before rendering disposable copies. Inspect every final sheet, dense areas and each copper layer. Recheck originals after export. No hand-editing generated outputs.
- Independently authenticate all nine strict stages, native ERC/DRC with all severities and violation-sensitive exits, writer correctness without skips, physical connectivity, emitted annotation audit, exact serialized request/project replay, and unchanged historical PCB geometry.
- Record functional-group membership and ownership sources. Assess functional locality, reference/value association, readable net/connector intent, page utilization, and clutter separately from electrical and automated annotation gates. A spatial improvement alone is not a complete readability pass.
- Authenticate the immediately preceding source/raw/archive evidence and its preserved history chain. Report all attempts, source hashes, commands, exits, review findings and limitations.

If development allowance is exhausted or the frozen final reveals a blocker, publish the negative outcome without another repair/evaluation iteration. The six-positive/two-new-design milestone cannot be satisfied by this two-example phase.
