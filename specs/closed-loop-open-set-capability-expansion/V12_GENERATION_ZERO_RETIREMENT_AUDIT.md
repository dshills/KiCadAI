# V12 Generation-Zero Retirement Audit

V12 retired fail-closed during its public generation-zero evaluation. The
committed evaluator authenticated the immutable V10 corpus, environment, and
fresh V12 evaluation root and completed 21 of 24 case checkpoints. It did not
complete the corpus, publish an accepted report, or publish a public baseline.
Cases 022, 023, and 024 have no accepted checkpoint.

V12 proved that one-live-replay storage and two-worker scheduling release large
completed replay spools and avoid cumulative graph retention. It passed the
V11 failure point and deterministically completed cases 013, 020, and 021.
However, two individually large synthesis graphs could still coexist. While
cases 022 and 023 were active, resident memory again approached the host limit
and the operating system terminated the process with status 137. This is an
evaluator resource failure, not a circuit outcome.

The committed root marker and checkpoint checksum list authenticate the
interrupted boundary without accepting partial results or publishing outcome
content. Checkpoints are retirement evidence only. They are not a baseline,
must not be resumed by a successor version, and must not influence capability
selection.

No held-out key or encrypted held-out record was opened. No held-out baseline
key was created, no held-out record was evaluated, and no held-out content or
outcome was published.

V13 must retain the one-live-replay rule, use exactly one case worker, and
explicitly return unreachable replay memory to the operating system before the
next replay begins. It must use a new evaluator commitment and fresh evaluation
root.
