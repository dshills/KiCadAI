# Development and bounded attempt ledger

Approval: the user approved compacting both schematics and reducing label-only wiring, with at most three development native evaluations and one final evaluation, no API calls and no PR. [PLAN.md](PLAN.md) was written before implementation/evaluation on clean base `3c1572f7cd9a8120b45e16c42c67500a5fb1097b`. Initial free space was 57 GiB; it remained above the 12 GiB native-entry threshold and 10 GiB reserve. The existing credential was checked only for presence and local exact-secret scanning, never printed or used for a provider request.

## Read-only diagnosis and implementation

Structural discovery used Atlas, with targeted source inspection where its summaries did not answer the question. Two interacting causes were found: connector/high-fanout policy forced endpoint labels for most nets, and full net-name clearance was reserved around every support pin. A separately versioned `ownership-v5` drawing profile was added; no default or earlier profile changed.

The initial focused tests passed. Three separately recorded layout/candidate diagnostic pairs used the frozen ownership-v4 controller request and library inventory. They generated only diagnostic transaction JSON and replayed whitelisted non-writing operations in memory, matching the workflow's resolver hydration. None invoked KiCad or a provider, generated a project, modified the frozen input, or counted as a native evaluation.

| Diagnostic | Retained outcome | Resulting implementation change |
| --- | --- | --- |
| 1 | Layout generation passed; native candidate check failed at PF2 and ADC-net labels | Retain full signal/interface and cross-group singleton corridors; compact only supply/ground candidates |
| 2 | Candidate check failed at programming-header labels blocked by early vertical conductors | Reserve narrow future-label corridors during route scoring before any net is routed |
| 3 | Candidate check failed at a decoupler ground label | Retain full ground placement clearance; shorten only multi-endpoint canonical power-net placement corridors |

All diagnostic failures, source snapshots and exact log/transaction hashes are retained in [diagnosis.json](diagnosis.json). Later final transactions are not claimed to equal those earlier diagnostic projections.

The local routing policy checks resolver-backed geometry and preserves explicit `use_label` choices, buses and ports. Within each explicit functional group it tries deterministic clean branches; obstructed islands and different groups retain labeled connections. A failed route is never emitted as a dirty conductor. Existing grid-search bounds remain in force. The N−1 logical tree remains compatible with the transaction adapter. Signal/ground/foreign-label checks and the final native annotation writer remain strict.

## Native evaluations

Each invocation used both unchanged examples, a real installed KiCad library, strict workflow validation and a JSON-decoded second request. Per-example context: 20 minutes. Go process: 45 minutes. All outputs were retained in exclusive roots.

| Attempt | Change evaluated | Technical result | Paper sizes: regulator / controller |
| --- | --- | --- | --- |
| dev1 | Local trees, conservative ground/signal corridors, future-label reservations | 2/2, both runs each | A4 / A1 portrait |
| dev2 | Balanced explicit-group rows instead of the fixed 420 mm width guess | 2/2, both runs each | A4 / A1 portrait |
| dev3 | Remove duplicate outer padding while retaining declared inter-group gutter and actual body/pin-label envelopes | 2/2, both runs each | A4 / A2 landscape |
| final | Frozen dev3 source; no additional repair | 2/2, both runs each | A4 / A2 landscape |

Final started at **2026-09-12T14:02:36.714Z** and finished at **2026-09-12T14:03:03.609Z**. The exact 17-file source snapshot SHA-256 is `e901d838de6727e7c43ce7558204d4185d41da1426d1727a5e5ef7595812f9b2`. No Go edits or native reruns followed the final start. All three development native slots and the single final slot were used.

The final requests and all available primary project files exactly match dev3. Each attempt also passed byte-exact same-attempt replay. Earlier dev1/dev2 source snapshots differ and are authenticated against their own snapshots, not misrepresented as the final source. [attempts.json](attempts.json) accounts for every attempted, failed, absent or skipped stage; every native required stage passed in this phase. The three negative diagnostics remain negative.

Native originals were sealed before exports, disposable copies were rendered, and originals were rechecked. Whole-sheet and group views from development informed the bounded changes. Only the 14 final images support the final visual result. The new 85 mm group crop margin includes the full target controller panel while showing some neighboring content at crop edges.

## Evidence and completion boundary

The [historical authentication](history-authentication.json) checks ownership-v4 committed source, raw inventory and archive, then the earlier preservation chain and original campaign. No old phase artifact, source snapshot, generated project or archive was deleted or changed.

The [full execution log](full.log), scoped race receipts, vet/lint receipts, final verification, image hashes, [archive manifest](archive-manifest.json) and [QA receipt](qa.json) retain the exact outcomes and bounds. The full compositionlowering race partition's historical timeout is not erased by this phase's narrower focused race coverage.

Final suite outcome: 151 tested packages passed, 14 packages had no tests, and the longest package took 678.803 seconds, below 720 seconds. All 23 execution receipts and source snapshots are retained. The archive authenticates 1,571 raw/source members without full extraction and finds no exact current credential. No evaluation was restarted or given a longer timeout.

The bounded phase produces measurable drawing improvements but no complete-readability or new frozen benchmark passes. No provider call, external review, credential/firewall change, push, PR, merge, release or fabrication was performed. A further implementation/evaluation phase requires fresh approval.
