# Development and negative-outcome record

Base: `f8b29a9904dd02c38583cb4c15d72f55b5ee6b75`. See [preregistered plan](PLAN.md), [all attempt receipts](attempts.json) and [local review](REVIEW.md).

## Native sequence

| Attempt | Regulator | Controller | Subsequent action |
| --- | --- | --- | --- |
| Development 1 | Both native runs pass | First run emits a project but ERC reports one dangling wire and one isolated label on `composition_net_006`; second run not attempted | Preserve root; investigate exact transaction and emitted wires |
| Development 2 | Both native runs pass | Writer cannot find a clear candidate for the long PA13 label; no project emitted | Preserve root; reserve owner pin-label corridors |
| Development 3 | Both native runs pass | Writer's joint annotation placement fails after 3 candidate visits; no project emitted | Development allowance exhausted; freeze without another repair |
| Frozen final | Both native runs pass | Same joint annotation-placement failure; second run not attempted | Publish negative outcome; no more native runs |

Every invocation exits 1 because its controller subcase fails. The positive regulator subcases do not convert these processes into passing runs. Development 3 and final have identical workflow inputs for both examples and identical available regulator primary project files. The [attempt inventory](attempts.json) retains every stage status, absent/skipped stage and failure message.

## Implementation sequence

Initial implementation introduced `ownership-v3`, support placement from exact shared owner pins and indivisible local native annotation panels. Initial focused tests and lint passed. Development-1 native ERC exposed a pre-existing writer shortcut: when an explicit label position conflicted and a pin already had a conductor, the fallback stub helper returned without ensuring that conductor's island had a label. The new profile adds that missing island label without adding a stub or changing the net. A focused disconnected-same-name-island regression covers the behavior.

The first visual inspection also showed that center-vector classification could treat a left-facing pin near the top of a tall MCU as top-facing. The implementation now uses the resolver's transformed pin direction when available, retaining positional fallback for missing direction. This put the PF2-connected support on the left, but moved the multi-side programming connector toward the MCU's long PA13/PA14 labels. Development 2 was rejected by the writer.

The last development change reserved outward owner label corridors from actual pin direction and exact net-name glyph width. Its unit tests and lint passed, but the controller still had no jointly valid label arrangement. The final run intentionally used this unchanged source and reproduced the failure. No attempt was made to relax collision/ERC gates, change names, alter electrical requirements, or move generated output by hand.

Post-freeze local source review additionally identified unhandled noncanonical supply/return net-role variants; see review finding 5. No Go change or new test/native attempt was made after this finding.

## Evidence-tool corrections

These affected evidence/report tooling only, never the frozen Go source or native artifacts:

- The success-only verifier inherited from ownership-v2 was adapted to authenticate failed cases explicitly. Missing stages/projects/replay are reported as uncertified; the phase result remains false. An initial patch with a partial long-line context was rejected atomically, then applied with exact context.
- The independent panel audit initially assumed native file order matched panel order. Its assertion failed before creating a receipt. The native writer sorts by UUID; the corrected audit matches exact content on spatially contiguous 2.54 mm rows, detects missing/ambiguous panels and accounts for every text item. It does not change any annotation.
- An early image inspection used a guessed regulator basename and failed after the controller image was displayed. The actual retained basename `offline_regulated_output` was then listed and inspected. No image was created, edited or replaced by that correction.
- Two navigation commands used nonexistent shell globs and returned errors without accessing or changing files; source navigation then used Atlas and exact observed paths.

## Preservation and resource discipline

The existing API key was checked without exposure and was never used for a provider request. Test, native KiCad and rendering subprocesses remove provider-key variables; dependency downloads are disabled. Archive/phase checks use the existing secret in memory solely to ensure its exact bytes are absent from retained data. No firewall setting or credential was changed.

Approximately 68 GiB was free at entry and 63 GiB before the frozen final. Each native runner enforces the preregistered 12 GiB minimum; previous raw roots and archives were retained, not deleted to make space. All native runs have exclusive roots and source snapshots. Originals were sealed before export; visual work used disposable copies; original seals were rechecked. Archive authentication streams members instead of extracting a second full tree.

The historical full compositionlowering race timeout is still uncertified. This phase runs only the preregistered focused ownership/pin-aware composition race partition, plus full short schematiclayout/schematicir/designapi race partitions. Do not describe this as a full repository race pass.
