# Local source and evidence review

Reviewer: implementing agent. Scope: 16 changed/new Go files against `e6f648bbbb05d1d8bc883e61356511d8708aec2e`, evidence runners, preserved attempts and emitted final KiCad projects. This is local review, not independent Gemini review or engineering/fabrication sign-off.

## Checked properties

- `ownership-v6` is explicitly accepted through lowering, layout validation, annotation blocks and the IR adapter. Older profiles retain their routing/placement flags and default behavior. No stable/default profile is replaced.
- The implementation uses functional roles and actual group membership, not fixture IDs, expected outputs or case-specific coordinates. Placement/routing changes are limited to pure single-active-plus-capacitor blocks. Mixed MCU/support/interface groups retain V5 behavior; shuffled ordering and group isolation have regressions.
- The bounded radial candidate set is unchanged. V6 ranks capacitor candidates by the actual shared supply-pin axis, leaving unknown/multiple-pin or non-power associations unchanged. Rotation and mirror tests cover all quarter turns. No electrical names, pin mappings, values, required corners or PCB coordinates change.
- Same-net local power/return corridor pairs may overlap, while body checks, foreign-net corridors and signal/isolated label corridors remain enforced. The pure-block minimum remains at least the declared spacing and 15.24 mm; only the previous extra 17.78 mm radial floor is removed.
- The root-first power tree retains N−1 logical connections and the existing bounded search. Pin access now escapes padded body envelopes along actual pin directions before turning. The complete candidate, including endpoint segments, is checked by the unchanged scorer; failed routes retain labeled islands, not unchecked conductors.
- Both final examples preserve complete requests except the versioned functional-profile string, all physical pin/pad connectivity, exact PCB geometry, electrical/simulation assertions and placement/generation seeds. Both actual JSON-decoded second requests reproduce all primary project bytes without normalization.
- All three diagnostic pairs, two negative unit-test records and the failed development-1 controller ERC outcome remain retained. Development 2 and the final pass both examples. No repair or native retry follows the final freeze; the third development slot is unused.
- Source snapshots, logs, exported images, native raw inventory and archive member hashes provide local integrity evidence. Provider keys are removed from execution children and checked only in memory for exact evidence leakage. These hashes are not an external attestation.

Final focused regressions, scoped races, vet and scoped lint passed. The single full short-suite result and exact commands are reported in [qa.json](qa.json); no earlier result substitutes for this run. The historical full compositionlowering race timeout remains uncertified and is not erased by focused race coverage.

## Remaining findings

1. Complete readability remains 0/2: anonymous/redundant labels, inconsistent reference/value positions, whole-sheet interface ordering and fragmented analog/controller signal flow remain. [Visual review](VISUAL_REVIEW.md).
2. The improved regulator spacing and visible branches do not generalize to arbitrary multi-active or MCU-support groups. Development 1 demonstrated why broader use is not certified; its controller ERC failure remains in [diagnosis.json](diagnosis.json).
3. Capacitor-origin distances are only drawing measures. PCB copper and placement did not improve; no return-path, thermal-layout, signal-integrity, assembly, mechanical or manufacturing approval is claimed.
4. The original practical-board milestone still has no new complete benchmark passes from this reused offline pair. A fresh live campaign, external review or PR requires separately authorized scope.

Disposition: preserve the versioned improvement, exact evidence and negative complete-readability outcome in a local commit. No push, PR, merge, release or fabrication is included.
