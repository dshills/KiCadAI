# Ownership and support locality, offline v2

Approved 2026-09-12 as a separately bounded follow-up to `c2ec8c88da11f8cf4aac498775e216ae2baaa34d`. Recorded before implementation or evaluation. The original practical-board milestone remains unachieved.

## Scope and authority

Reject malformed known synthesis-parent chains instead of treating their components as external boundaries. Introduce an opt-in, versioned schematic locality policy, preserving ownership-v1 layout behavior. Improve support-component association using explicit provenance and reusable geometry, never example identities or fixed coordinates. Retain all circuit components, values, physical pin/pad mappings, electrical net names, simulations, PCB seeds, poses and routes. The unchanged standalone regulator and separately recorded controller ADC 100 mA example are the only native examples; retain the original 150 mA thermal rejection.

No API calls, external review, new live campaign, push, PR, merge, release, fabrication, or modification/deletion of historical evidence. Remove provider credentials from test subprocesses and disable dependency networking. A scoped local code/tests/evidence/review commit is authorized.

## Frozen bounds and acceptance

- At most two development native evaluations and one frozen-source final native evaluation, each with actual JSON-decoded second-request replay, 20 minutes per example and 45 minutes per process. Preserve every attempt and source snapshot. No source repair or additional native run after final starts.
- Focused unit tests may guide development. Run one final full short suite with the unchanged 12-minute per-package limit, one final scoped race check, lint and vet. Race scope: all schematiclayout and schematicir short tests plus ownership/locality-specific compositionlowering tests, each partition bounded at 12 minutes. This closes changed-path race coverage only; it does not certify the previously timed-out full compositionlowering suite. Do not increase timeouts or alter unrelated simulation tests.
- Require at least 12 GiB free before each native run, preserving a 10 GiB reserve. Retain raw roots and a verified compressed archive without full duplicate extraction. Never delete evidence to make room.
- Seal generated primary projects before native rendering of disposable copies; inspect every final sheet, dense support areas and all copper layers. Recheck originals afterward. No hand-edited generated output.
- Independently verify all nine strict workflow stages, full-project all-severity ERC/DRC with violation-sensitive exits, no skipped writer checks, physical connectivity, emitted annotation audits, exact JSON/project replay and unchanged PCB geometry/non-layout request.
- Ownership hardening must reject missing, cyclic and conflicting explicit parent provenance, including connector/rail roles. Require deterministic layout under input permutation, caller-data immutability, safe treatment of explicit fixed placements, and preservation of ownership-v1 output.
- For each example, report decoupling-to-active-owner schematic-origin distances using the preceding phase's definitions. Seek lower mean and maximum without an individual regression; failures remain failures. These distances assess locality, not PCB quality or complete visual readability. Complete readability additionally requires clear reference/value association, connector/net intent, useful page utilization and uncluttered functional reading; do not infer it from automated gates.
- Authenticate preceding source/raw/archive evidence and the historical chain. Publish exact commands, source hashes, exits, all failures, review findings and limitations. Stop after final evaluation even if a blocker remains. No new practical benchmark passes can be earned by this two-example offline phase.
