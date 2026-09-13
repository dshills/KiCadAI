# Retained development history

This record distinguishes development experiments from the frozen final verification. Every native development invocation had an exact source snapshot recorded before execution. No generated schematic or PCB was hand-edited. Complete command output, including incidental compile/test-fixture failures, is in `development-tests.log`.

| Attempt | Retained outcome | Interpretation |
| --- | --- | --- |
| dev1 | Standalone two native passes; controller project-write failure on long PF2 label | Field-first placement stranded an electrical label |
| dev2 | Standalone two passes; controller ground-label placement failure | Moving labels first exposed competing label positions |
| dev3 | Standalone two passes; controller net005 lacked a clear candidate | Added bounded joint label search; single-segment reach was too narrow |
| dev4 | Standalone two passes; controller ground-label failure | Connected-wire-island candidates still encountered an overbroad symbol/pin rectangle |
| dev5 | Both examples, both generations pass native stages | Body/pin corridors separated; no reading guide or final rotated-field support guard yet |
| dev6 | Both project writes blocked by readback audit | Standard paper dimensions needed name resolution; notes were raw preserved items |
| dev7 | Compile failure; no raw project directory | Audit referred to nonexistent `Raw` instead of raw item's `Body`; exact source snapshot retained |
| dev8 | Both fail writer round-trip | Reading-guide save order and explicit native simulation flag differed |
| dev9 | Both fail writer round-trip | Native `exclude_from_sim` subnode precedes `at` |
| dev10 | Both strict workflows fail at validation warning | Notes were preservation-only; writer/ERC/DRC passed but strict stage acceptance did not |
| dev11 | Both examples, both generations pass native stages | Default-font KiCad 10 text structurally modeled; rotated fields still not certified |
| final | Standalone two native passes; controller blocked at rotated C1 reference | Final guard preserves the unsupported-geometry failure; cross-phase board audit additionally fails standalone PCB preservation |

Development rows total 28 workflow executions: 16 all-required-stage successes, six project-write failures, four writer failures and two validation warnings. These numbers are execution-level diagnostics, not 16 passing examples. The final run has three workflow rows and two native project outputs; its full phase gate is 0/2.

The initial serialization regression failed because `Inferred` was runtime-only. The label-crossing regression first had an invalid test UUID, then failed for the intended missing glyph collision check after the fixture was corrected. A later idempotence test initially assumed symbol reference/value fields were stored in `Properties`, then `Fields`; the minimal test symbol uses generated defaults, so the explicit guide note became the test's crossed annotation. These fixture errors remain in the log and are not described as product regressions.

The independent verifier initially compared versioned resolution provenance as if it were electrical intent, then separated only the explicitly named resolution-hash property. Its subsequent PCB comparison found a real geometry change; that failure is preserved as false rather than excluded. Controller skipped/absent stages are represented exactly, not reported as executed failures or successes. Verification-script corrections do not alter the frozen generated outputs, source, thresholds or phase result.

A pre-final full-suite run and race run began before the last reader/angle guards and overlapped development. They are supplementary only. The final suite/race invocations were restarted against the frozen final source. The pre-final full suite timed out at the original 12-minute package limit in `internal/opentopologysynthesis`; its transcript is retained separately. Concurrent test work and archive I/O may affect duration, so this is not evidence of a semantic regression or a clean pass. No timeout was increased and no failing native run was repeated after final freeze.

The archive includes ten development raw roots (dev7 never compiled), the final raw root, and twelve source snapshots. Its fresh extraction and an independent recheck matched all original inventories. Prior archives and raw trees remain unchanged; no source snapshot was retroactively substituted for a development run.
