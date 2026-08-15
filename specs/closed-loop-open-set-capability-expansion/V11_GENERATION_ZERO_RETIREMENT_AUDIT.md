# V11 Generation-Zero Retirement Audit

V11 retired fail-closed during its public generation-zero evaluation. The
committed evaluator authenticated the immutable V10 corpus, environment, and
fresh V11 evaluation root and completed 19 of 24 case checkpoints. It did not
complete the corpus, publish an accepted report, or publish a public baseline.
Cases 013, 020, 022, 023, and 024 have no accepted checkpoint.

V11 successfully removed the prior two-replay-per-worker retention defect. Its
resident memory repeatedly fell after large cases completed, and authenticated
spools were removed only after checkpoint publication. However, four case
workers could still hold four individually large synthesis graphs at once.
Resident memory again approached 50 GiB and the host terminated the process
with status 137. This is an evaluator resource failure, not a circuit outcome.

The committed root marker and checkpoint checksum list authenticate the
interrupted boundary without accepting partial results or publishing outcome
content. Checkpoints are retirement evidence only. They are not a baseline,
must not be resumed by a successor version, and must not influence capability
selection.

No held-out key or encrypted held-out record was opened. No held-out baseline
key was created, no held-out record was evaluated, and no held-out content or
outcome was published.

V12 must retain the one-live-replay rule while also bounding aggregate
case-level concurrency. It must use a new evaluator commitment and a fresh
evaluation root.
