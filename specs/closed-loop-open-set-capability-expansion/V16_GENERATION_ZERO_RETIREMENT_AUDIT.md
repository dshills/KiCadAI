# V16 Generation-Zero Retirement Audit

V16 retired fail-closed during its public generation-zero evaluation. The
committed evaluator authenticated the immutable V10 corpus and environment,
created a fresh V16 evaluation root, and completed 21 of 24 case checkpoints.
It did not complete the corpus, publish an accepted report, or publish a public
baseline. Cases 022 through 024 have no accepted checkpoint.

V16 proved the generic top-level streaming-finalization correction introduced
after V15. Public cases 003, 007, 013, and 016 completed both deterministic
replays, including canonical synthesis evidence streams as large as
approximately 12.1 GB. Case 013, the exact V15 blocker, completed at a bounded
footprint, and replay storage was reclaimed across completed cases.

Public case 022 exposed a distinct remaining generic defect. A diagnostic
resume from the authenticated 21-case boundary reproduced the growth without
publishing a result. At approximately 16.6 GB RSS, a heap profile attributed
about 42.6% of live heap to retained per-step device observations and 28.2% to
retained per-step node observations. The allocations originate in the trusted
transient simulator, whose uniform event-aligned grid can retain up to one
million complete analysis points. The formal run crossed the 24 GiB safety
cutoff while still growing, so it was interrupted before host stability was
compromised. This is a deterministic transient-evidence retention defect, not
a circuit outcome and not a recurrence of V15's JSON hashing defect.

The committed root marker and checkpoint checksum list authenticate the
interrupted boundary without accepting partial results or publishing outcome
content. Checkpoints are retirement evidence only. They are not a baseline,
must not be resumed by a successor version, and must not influence capability
selection.

No held-out key or encrypted held-out record was opened. No held-out baseline
key was created, no held-out record was evaluated, and no held-out content or
outcome was published.

The successor must preserve V16 search, repair, ranking, streaming-finalization,
single-worker, one-live-replay, and memory-release semantics. It must add a
generic deterministic bound on retained transient proof points, retain a
bounded uniformly spaced report witness while preserving full proof hashes and
assertion results, and use streaming report hashing. The legacy evaluator path
must remain behaviorally unchanged. The successor must bind the corrected
production source and use a fresh evaluation root without reusing V16
checkpoints.
