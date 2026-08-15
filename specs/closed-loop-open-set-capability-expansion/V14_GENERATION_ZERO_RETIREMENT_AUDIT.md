# V14 Generation-Zero Retirement Audit

V14 retired fail-closed during its public generation-zero evaluation. The
committed evaluator authenticated the immutable V10 corpus, environment, and
fresh V14 evaluation root and completed 2 of 24 case checkpoints. It did not
complete the corpus, publish an accepted report, or publish a public baseline.
Cases 003 through 024 have no accepted checkpoint.

V14 proved that compact value-trial descriptors remove eager queued graph
retention. It also isolated the next generic resource defect. During case 003
replay 1, the single live evaluator retained a failed materialized candidate
graph for every evaluated value trial even though the repair phase later keeps
only the best failure per topology. Resident memory rose from approximately
5.6 GiB to approximately 24.6 GiB while the process remained inside that one
replay. The evaluator was interrupted with status 130 before host stability was
compromised. This is a deterministic per-trial retention defect, not a circuit
outcome.

The committed root marker and checkpoint checksum list authenticate the
interrupted boundary without accepting partial results or publishing outcome
content. Checkpoints are retirement evidence only. They are not a baseline,
must not be resumed by a successor version, and must not influence capability
selection.

No held-out key or encrypted held-out record was opened. No held-out baseline
key was created, no held-out record was evaluated, and no held-out content or
outcome was published.

The successor must preserve V14's single-worker, one-live-replay, explicit
post-replay memory release, and lazy value-trial materialization rules. It must
retain only the deterministic best failed graph per topology during initial
evaluation, preserve complete public output semantics and repair order, bind
the corrected production synthesis source, and use a fresh evaluation root.
