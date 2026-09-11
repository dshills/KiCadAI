# Explicit reference routing and offline native promotion

Approved continuation on 2026-09-11. Starting commit: `44223a065e645f7abccf67d66988edea925ec9a9`; branch: `codex/explicit-reference-promotion`.

## Scope

Add an explicit supply-to-reference-domain contract and carry its identity through architecture selection, physical lowering, loads/stimuli and behavioral measurements. Preserve legacy single-reference behavior; reject ambiguous new forms and invalid explicit references instead of guessing from names. Keep isolation boundaries explicit.

Then exercise complete schematic/PCB creation with the installed KiCad CLI and real libraries using the preceding phase's standalone regulator and separately declared 100 mA controller examples. These are synthetic integration examples, not new frozen benchmark passes. Keep the original 150 mA thermal failure unchanged.

## Verification and limits

- Add adversarial identity, validation, physical-net and simulation tests; verify strict provider schema and the existing request-byte cap offline.
- Run native workflows with ERC, strict DRC, writer round-trip and requirement enforcement enabled. Retain stage results and artifacts even on failure. No synthetic library substitutions and no edits of generated designs.
- Bound each native example to 20 minutes and the repository short suite to 12 minutes. Repeat successful native generation once for full generated-project comparison, normalizing only documented volatile fields.
- Inspect any produced schematic/board render before a readability claim. A blocked stage or absent artifact is not a pass.
- Preserve all frozen inputs, historical raw runs, archives, seals and earlier phase reports. Authenticate them separately; new source hashes belong only to this phase.
- No provider calls, model changes, evaluation retry/campaign, remote publication, PR, merge, release or fabrication. Tests run with provider keys removed. Existing credentials may be used only by the local exact-secret scan.
- Retain downstream failures without expanding into an unapproved repair phase. The overall six-board/two-paired-design milestone remains unmet unless its own complete evidence is actually obtained.

## Deliverables

Source and regressions, bounded execution logs, source-bound verification receipts, native stage/replay/readability results where available, historical-preservation result, and a local review recording remaining gaps.
