# Development attempt ledger

All roots below are retained under `/tmp/kicadai-schematic-readability-offline-v1-<suffix>` and included in the all-runs archive. They are local software-development iterations over unchanged synthetic electrical requirements, not extra live campaign attempts. No provider calls occurred.

| Suffix | Native workflow records | Outcome and reason |
|---|---:|---|
| dev1 | 0 | Both promotions blocked: initial zero schematic spacing violated graph validation. No board generated. |
| dev2 | 3 | Standard spacing enabled. Regulator passed twice; controller first run failed native ERC with an undriven pin, dangling wire and isolated net label. No second controller run. Native wires also rendered invisibly. |
| dev3 | 4 | Versioned opt-in policy, endpoint justification and auxiliary density. Both examples passed required native stages twice; rendered output still unreadable. Serialized-request layout diagnostics retained with their runtime-state caveat. |
| dev4 | 4 | Explicit schematic netclass drawing defaults made wires visible. Both examples passed native stages twice; annotation collisions remained. |
| dev5 | 2 | A vertical-label rotation disagreed with KiCad's canonical writer representation. Both first runs blocked at strict writer correctness; later native validation stages were skipped, not passed. No second runs. |
| dev6 | 4 | Canonical below-label orientation restored strict round-trip. Both examples passed native stages twice; broader label-geometry changes still required legacy compatibility correction. |
| dev7 | 4 | Native label geometry gated, legacy estimator restored, external pin-annotation envelope added. Both examples passed native stages twice; field collisions remained. |
| dev8 | 4 | Centered native field anchors and spacing. Both examples passed native stages twice; native XML connectivity exports added. |
| dev9 | 4 | Candidate label-stub symbol-body rejection. Both examples passed native stages twice; final visual issues remained. |
| final | 4 | Fresh final-source generation and repetition; all native/electrical checks passed, but both readability verdicts remain fail. Full pin/pad, source, preservation, sealed-render and archive audits performed. |

There are 33 recorded workflow executions across development and final roots: 30 reach `erc-drc`, one reaches only `connectivity`, and two reach only `structural`. Two additional dev1 promotions stop before workflow generation. These totals describe diagnostic iterations; the evaluated final-source denominator remains two designs and four runs, with zero complete readability passes.

The initial regression test failed for the three observed layout-policy defects; `initial-red-test.log` retains that output. The final version of the test also asserts legacy-policy preservation before invoking the opt-in helper. During development, applying topology/native geometry globally invalidated frozen capability hashes. The implementation was gated and legacy behavior restored; no frozen receipt was regenerated or relaxed. The final repository suite verifies the corrected source.

Development-console limitations are explicit: some large failed test transcripts were truncated by the tool and were not retained as complete logs. Native workflow, writer and ERC receipts in the raw roots preserve the corresponding engineering failures. Only the final source is bound by the 16-file verification receipt; intermediate uncommitted source snapshots were not separately sealed. Do not infer an exact source-to-artifact reconstruction for every development root.

Rendering diagnostic attempts included black-and-white and clean temporary-configuration exports for dev2. They did not repair the missing project drawing defaults. The user's KiCad settings were not changed. The first additional viewport-render invocation failed because negative offsets required `--left=-...` / `--top=-...` syntax; the corrected documented invocation succeeded. It did not change original project files. The final native invocation also contained the unused environment variable `KICADAI_KICADAI_OFFLINE=1`; that typo is not an offline safety control. Provider keys were removed and module downloads disabled by the actual test environment.

All final-source successful test stdout and command receipts are retained separately. The final render seal is created before any copies or native exports, and raw originals are rechecked after all review exports. The archive includes unsuccessful and successful roots alike; no negative attempt is discarded to improve a score.
