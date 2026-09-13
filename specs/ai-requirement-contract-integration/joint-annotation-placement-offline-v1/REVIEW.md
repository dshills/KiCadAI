# Local source and evidence review

## Disposition

Accept this as a local, opt-in repair of the native controller annotation conflict and placement-role handling. Do not describe `ownership-v4` as a compact default, a complete-readable schematic generator, a new benchmark pass, a reviewed PR or a fabrication-ready output.

This is the implementing agent's local source/evidence/visual review. It is not an independent human or external-model review. Credential-safety and evidence-validation skills kept test subprocesses key-free and separated diagnostic, native technical, visual and benchmark claims. No code or evidence was sent to another provider.

## Source checks

- New behavior is gated by `ownership-v4` through lowering, IR validation and the adapter. Ownership-v1/v2/v3 retain their existing branches and native annotation-v2 behavior. The prior controller conflict still reproduces under v3.
- Net-role normalization acts on a cloned layout slice, before pin-side priority and ground exclusion. `power_pos`/`power_neg` have supply priority; `return` cannot choose a side. The original names, roles and endpoint slices are not edited. Signal/bias priority, equal-priority ambiguity and transformed pin directions are preserved.
- Joint placement uses exact component geometry, endpoint metadata and actual label names. It contains no example-specific coordinates or component-ID naming heuristics. Deterministic radial bounds and legacy fallback on cycles/exhaustion remain; the native writer is still the final fail-closed constraint check.
- Corridor checks are conservative reservations, not a complete global component/wire/field optimizer. They cover already placed components in each group; cross-group routing and later annotation decisions remain separately validated by the native writer. This limitation is not hidden by the two successful examples.
- Diagnostic projection clones/restores drawing and label bookkeeping. Regular tests cover invalid/legacy input, success, repeated calls, caller mutation of returned candidates and an unsatisfiable panel; the frozen replay exposes actual candidate geometry without writing a project.
- The native runner retains strict stages and uses a real JSON-decoded second request. Independent verification compares full requests except versioned layout, exact primary replay bytes, physical net endpoints/pads, native raw reports and all PCB geometry except the documented drawing-paper field.
- No Go source changed after the final source snapshot. The full-suite and race receipts must be read with their exact scopes; the historical full compositionlowering race timeout remains unresolved.

## Resolved and unresolved findings

1. **Resolved for these examples:** v3's controller label collision. The diagnostic identifies a hard overlap between two single-candidate glyph boxes; v4 yields a legal arrangement and the completed controller passes strict native gates and exact replay.
2. **Resolved in v4 placement interpretation:** supported supply/return role variants no longer mis-prioritize or choose a support side. Tests cover canonical/variant roles, rotation/mirror, shuffled order and conflicting priorities. The examples themselves use canonical roles; variant coverage is from targeted tests, not additional native boards.
3. **Still blocking complete-readability promotion:** both layouts are too sparse and label-heavy. They grow one paper size each, every measured decoupler distance regresses, and connector/rail/signal intent remains difficult to trace. See the exact final images and findings in [VISUAL_REVIEW.md](VISUAL_REVIEW.md).
4. **Still outside certification:** J5 has shared MCU pins on different sides, so its attachment remains ambiguous; no artificial side-pass is assigned. Full global optimization, hierarchical-sheet certification, generalized live generation and manufacturing quality are not established here.

## Authority and handoff

Two development native attempts and the single final attempt are complete; the third development allowance was unused. The one bounded full short suite passed all 151 tested packages, with 14 no-test packages; all scoped races, focused tests, vet and lint also passed. The frozen-final boundary prohibits further repair or native evaluation in this phase. Preserve all evidence in the local evidence/review commit, then request new authority for any further readability phase, live campaign or PR work. No push, external review, merge, release or fabrication is part of this authorization.
