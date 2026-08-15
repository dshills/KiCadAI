# V13 Generation-Zero Retirement Audit

V13 retired fail-closed during its public generation-zero evaluation. The
committed evaluator authenticated the immutable V10 corpus, environment, and
fresh V13 evaluation root and completed 21 of 24 case checkpoints. It did not
complete the corpus, publish an accepted report, or publish a public baseline.
Cases 022, 023, and 024 have no accepted checkpoint.

V13 proved that exactly one case worker, one live replay, and explicit
post-replay garbage collection avoid cumulative and cross-replay graph
retention. It completed all earlier large-graph boundaries, including cases
007, 016, 020, and 021, with byte-identical replays and removed spools after
each durable checkpoint.

V13 also isolated the remaining generic resource defect. Case 022 replay 1
alone retained a live synthesis graph large enough to drive the host into
memory compression before the operating system terminated the process with
status 137. Production synthesis eagerly materialized and retained a cloned
candidate graph for every enumerated value trial before evaluating the first
trial. This is a per-graph representation defect, not a worker-count defect or
a circuit outcome.

The committed root marker and checkpoint checksum list authenticate the
interrupted boundary without accepting partial results or publishing outcome
content. Checkpoints are retirement evidence only. They are not a baseline,
must not be resumed by a successor version, and must not influence capability
selection.

No held-out key or encrypted held-out record was opened. No held-out baseline
key was created, no held-out record was evaluated, and no held-out content or
outcome was published.

V14 must preserve V13's single-worker and one-live-replay rules while removing
eager per-value-trial graph retention generically. It must preserve trial
validity and evaluation order, bind the corrected production synthesis source,
and use a new evaluator commitment and fresh evaluation root.
