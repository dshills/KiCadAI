# Pin-aware schematic readability, offline v1

Approved as a separately bounded follow-up to `f8b29a9904dd02c38583cb4c15d72f55b5ee6b75`. Recorded before implementation and evaluation. The original practical-board goal remains unachieved; these two offline examples cannot satisfy its six-positive/two-new-design criteria.

## Scope

Implement a new opt-in, versioned drawing policy for pin-aware support placement and local explanatory annotations. Use actual connected pin geometry, explicit ownership and source pin/net metadata, not component-name heuristics or example-specific coordinates. Preserve ownership-v1 and ownership-v2 drawing behavior. Keep every electrical net name, component value, physical symbol/pad binding, simulation requirement, generation/placement seed and PCB pose/route unchanged. Do not invent voltage levels or signal direction when the recorded contract does not prove them.

Evaluate the unchanged standalone regulator and separately recorded controller ADC 100 mA requirements, with the original 150 mA thermal rejection retained. Native electrical checks are mandatory but do not establish readable circuit intent. A local implementation/tests/evidence/review commit is authorized. No API calls, external review, new live campaign, push, PR, merge, release, fabrication, or modifications/deletions of historical evidence.

## Preregistered evaluation bounds

- At most three development native evaluations and one frozen-source final native evaluation. Each uses both unchanged examples, actual JSON-decoded second-request replay, 20 minutes per example and 45 minutes per test process. Preserve every attempted root and exact changed-source snapshot. No Go source repair or additional native run after final starts.
- Focused offline unit tests may guide development. After freeze, one full short suite with the unchanged 12-minute per-package limit, final focused tests, scoped race checks, lint and vet. Race scope is all short schematiclayout/schematicir/designapi tests plus focused ownership and pin-aware compositionlowering tests, partitioned into separate 12-minute package-bounded invocations. The historical full compositionlowering race timeout remains uncertified; do not disguise narrower coverage as a full-package pass.
- Check free space before each native evaluation, require at least 12 GiB free and retain a 10 GiB reserve. Preserve all raw evidence and a compressed archive; verify members by streaming without a second full extraction. Do not delete prior evidence or change its paths to make room.
- Seal originals before exporting disposable render copies; inspect every final sheet, dense support/annotation areas and every copper layer. Recheck originals after exports. Never hand-edit generated projects.
- Independently authenticate all nine strict stages, all-severity full-project native ERC/DRC with violation-sensitive exits, no skipped writer checks, emitted annotation audit, physical pin/pad connectivity, exact JSON/project replay, non-layout request equality and unchanged historical PCB geometry.
- Authenticate the preceding source/raw/archive and preserved history chain. Keep all negative outcomes, command receipts, source hashes, logs and review findings. No threshold relaxation or hidden retries after a failed frozen final.

## Readability claims to evaluate

1. Pin-aware side association: when a support part and its owner share one unambiguous non-ground attachment side, place the support on that side of the transformed owner pin envelope. Test left/right/top/bottom and rotations, shuffled input, ambiguous attachments and explicit fixed placement. For the native regulator blocks, input and output bypasses must appear on their respective VIN/VOUT sides.
2. Local annotation association: explanatory group headings and concise interface/pin intent must remain spatially associated with the corresponding circuit group. Avoid a detached multi-column global netlist guide or a wrapped entry split between distant panels. Keep exact electrical names and make any interpretation auditable from recorded metadata.
3. Complete visual readability: independently inspect reference/value association, input-to-output reading, local connector/power intent, page utilization, dense-label/wire clutter and annotation placement. A geometry-only improvement, clean ERC/DRC, or passing text bounds is not sufficient. Prefer a smaller usable controller page, but do not shrink text or suppress information to claim success.
4. Report all support/decoupler distances against ownership-v2, including regressions; these are descriptive tradeoffs, not substitutes for pin-aware or complete-readability acceptance. Report technical, pin-aware, local-annotation and complete-readability outcomes separately, with exact denominators and caveats.

If the allowance is exhausted or final evaluation fails any gate, publish the negative outcome without another repair/evaluation iteration. A further phase requires new approval.
