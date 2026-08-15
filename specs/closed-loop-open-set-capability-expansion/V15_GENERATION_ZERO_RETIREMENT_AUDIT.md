# V15 Generation-Zero Retirement Audit

V15 retired fail-closed during its public generation-zero evaluation. The
committed evaluator authenticated the immutable V10 corpus and environment,
created a fresh V15 evaluation root, and completed 12 of 24 case checkpoints.
It did not complete the corpus, publish an accepted report, or publish a public
baseline. Cases 013 through 024 have no accepted checkpoint.

V15 proved the generic bounded-repair changes introduced after V14. Public case
003 completed both deterministic replays, and public case 007 completed two
approximately 8.6 GB canonical evidence streams without retaining the encoded
buffers in memory. Memory was reclaimed across the first 12 completed cases.

Public case 013 exposed a distinct remaining generic defect. Search stayed below
approximately 8.5 GiB RSS, but top-level synthesis finalization still used the
legacy whole-value `json.Marshal` hash path. A diagnostic heap profile attributed
approximately 6 GiB of live heap to `finalizeSynthesisRun` through `hashJSON` and
`encoding/json.Marshal`. RSS then crossed the 24 GiB safety cutoff and continued
to approximately 26.4 GiB, so the evaluator was interrupted before host
stability was compromised. This is a deterministic evidence-hashing resource
defect, not a circuit outcome.

The committed root marker and checkpoint checksum list authenticate the
interrupted boundary without accepting partial results or publishing outcome
content. Checkpoints are retirement evidence only. They are not a baseline,
must not be resumed by a successor version, and must not influence capability
selection.

No held-out key or encrypted held-out record was opened. No held-out baseline
key was created, no held-out record was evaluated, and no held-out content or
outcome was published.

The successor must preserve V15 search, repair, ranking, output, single-worker,
one-live-replay, and memory-release semantics. It must replace only the
top-level synthesis-run whole-buffer hash with byte-identical streaming
canonical hashing, fail closed on encoding error, bind the corrected production
source, and use a fresh evaluation root without reusing V15 checkpoints.
