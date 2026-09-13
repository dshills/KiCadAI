# Development record

Baseline: `e6f648bbbb05d1d8bc883e61356511d8708aec2e`. All production changes are opt-in under `ownership-v6`; electrical requirements and physical boards are unchanged. The implementing agent supplied the code, tests and review; no external reviewer or provider was used.

## Recorded diagnosis and corrections

1. The initial candidate implementation aligned capacitor supply pins with the owner axis, allowed same-net power/return corridor sharing and rooted local trees at the active component. The first focused test failed because it assumed final sorted connection order identified the routing root. Its preserved tree contained the intended active-root branch; the assertion was corrected to check membership instead of list position.
2. Diagnostic pair 1 passed its non-writing candidate check but showed excessive MCU support distances under broad pin-axis alignment. Placement was restricted to pure blocks containing one active device and capacitors. Root priority was restricted to supply/return nets. Pair 2 passed, but the regulator's VIN/VOUT branches remained label-only.
3. Actual layout metadata showed pins inside the padded body envelope. One-grid access points left no clean turning point. V6 now escapes that envelope along the actual resolver pin direction before the unchanged route scorer checks the whole conductor. Diagnostic pair 3 passed and produced visible regulator supply branches. The regression fixture initially supplied off-grid anchors; its negative result was retained and the fixture corrected to use canonical grid-aligned pin anchors. Both early focused failures remain in the execution ledger.
4. Native development 1 passed the regulator and failed the controller's first strict ERC check with `label_multiple_wires` and `unconnected_wire_endpoint` warnings in interface wiring. Controller second replay was not attempted. This failed native run, its three primary projects, exports, diagnostics and exact source remain retained. All nine stages were not passed, so it is not a controller success.
5. Routing's new behavior was restricted to the same pure active-plus-capacitor blocks as placement. Mixed MCU/support and interface blocks retain V5 routing behavior. Pure-block spacing uses the declared minimum (at least 15.24 mm) without V5's extra 17.78 mm radial floor; no declared spacing or native check is reduced. Native development 2 passed both examples and both actual serialized replays, with A4/A2 paper retained. The independent emitted audit finds all four regulator-to-capacitor paths in each example, previously zero. All 13 unambiguous support-side associations and six panels are retained.

## Final selection

Development 2 is the selected source. A third development native evaluation is unnecessary and will not be used. The single final will run with this exact source and unchanged requests, then source is immutable for the rest of this phase. No post-final repair or native retry is permitted. Final results, tests, image review and archive authentication are recorded separately; this decision is not a final success claim.

## Reproduction and limits

`run.mjs` records create-exclusive execution logs, source snapshots, UTC start/end times, exact commands and source-unchanged checks. Go/KiCad children have provider keys removed and dependency downloads disabled. `verify.mjs`, `power-rail-audit.mjs`, `wiring-metrics.mjs` and `pin-aware-audit.mjs` are independently inspectable local verification paths. All diagnostic projections are transactions/layout metadata only, not generated boards or benchmark passes.

Complete readability remains a separate final visual decision. Logical/electrical connectivity, visible conductor paths, support distance and full readability must not be conflated. The original frozen benchmark and 150 mA thermal rejection remain unchanged.
