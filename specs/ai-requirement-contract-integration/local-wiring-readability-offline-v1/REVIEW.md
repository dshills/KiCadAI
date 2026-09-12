# Local source and evidence review

Reviewer: implementing agent. Scope: 17 changed/new Go files relative to `3c1572f7cd9a8120b45e16c42c67500a5fb1097b`, the bounded runner, evidence verifiers, native outputs and reports. This is local review only, not the previously authorized Gemini campaign or an independent engineering/fabrication approval.

## Checked properties

- `ownership-v5` is opt-in at composition lowering, IR validation, local notes and the layout adapter. Earlier profiles retain their routing/packing defaults. The native annotation profile remains `annotation-v2`.
- Local-tree eligibility is conservative: resolver-backed geometry and known pins are required. Explicit `use_label:true/false`, ports, buses and unsafe transforms are not overridden. Quarter-turn metadata, missing pins, legacy behavior and caller intent have focused coverage.
- Local route trees are deterministic under shuffled inputs, grouped by explicit ownership, and form an N−1 logical connection tree. Obstructed routes fall back to labeled islands without retaining failed conductor points. The existing bounded grid search is retained.
- Foreign future-label corridors are included in route scoring, including bounded grid edges. Same-net conductors are allowed; different nets cannot consume these reservations. Placement shortens only local canonical power-net reservations; ground, signal, bias and cross-group singleton endpoints keep full clearance. Rotation/mirror and non-supply isolation are tested.
- Balanced group packing and removal of duplicate padding do not change the declared gutter, body/pin-label envelope calculation, final paper selection checks, electrical connectivity, native writer checks or PCB coordinates.
- Whole-request equality against ownership-v4 excludes only the new functional-profile string. Component values, all electrical/net/pad data, seeds, simulation inputs and other layout fields are exact. Same-phase replay uses no normalization; cross-phase PCB comparison only resolves numeric net IDs and excludes the drawing-paper setting.
- Three development evaluations and one final are bound to exclusive roots, execution receipts and source snapshots. All three diagnostic candidate failures remain visible. Provider credentials are stripped from Go/KiCad/render execution. Archive and report QA locally check exact-secret absence without printing the key.

Final focused tests, the declared race partitions, vet and scoped lint passed. Full-suite coverage and timings are recorded in [qa.json](qa.json) and [full.execution.json](full.execution.json), rather than inferred from earlier phases. No implementation/native iteration is permitted after the final freeze.

## Open findings and limits

1. Complete readability remains **0/2**. Reducing page size and label-only islands does not resolve long anonymous rail names, redundant same-island labels, remote capacitors/support parts, inconsistent field positions and the fragmented ADC/MCU flow. See [VISUAL_REVIEW.md](VISUAL_REVIEW.md).
2. More visible connections increase drawn wire length. The controller PF2 path is especially long and rectangular; this is a recorded tradeoff, not a compact local-connection success.
3. The routing strategy is greedy and conservative, not a global placement/routing/annotation optimizer. It may retain avoidable labeled islands or fail a genuinely crowded layout. Only the two preserved examples have complete native evidence here; broader support is not certified.
4. The earlier full compositionlowering race timeout remains uncertified. Focused ownership/pin-aware/joint/local-wiring race coverage is not a replacement for that partition.
5. No physical PCB improvements or manufacturing sign-off are claimed. Both outputs remain `fabrication_ready: false`; return paths, signal integrity, assembly and mechanical suitability require separate work.

Disposition: retain the versioned improvement and all evidence, with the incomplete readability result explicitly reported. No additional repair, API campaign, external review or PR is authorized by this phase. The broader practical AI-generated board milestone remains unmet.
