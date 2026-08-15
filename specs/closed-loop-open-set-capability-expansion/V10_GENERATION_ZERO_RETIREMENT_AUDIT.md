# V10 Generation-Zero Retirement Audit

V10 retired fail-closed during its public generation-zero evaluation. The
frozen evaluator authenticated its corpus, environment, and evaluator root and
completed 21 of 24 case checkpoints. It did not complete the corpus, publish an
accepted report, or publish a public baseline. Cases 013, 020, and 022 have no
checkpoint.

Repeated execution of the unchanged frozen evaluator reached the same 21-case
frontier before the host terminated it under memory pressure. The evaluator's
four concurrent workers retained two complete synthesis-run object graphs and
a full marshaled copy for every active case. Resident memory exceeded 45 GiB.
This is an evaluator resource failure, not a circuit outcome.

The committed root marker and checkpoint checksum list authenticate the
interrupted boundary without accepting partial results or publishing outcome
content. Checkpoints are retirement evidence only. They are not a baseline,
must not be resumed by a successor version, and must not influence capability
selection.

The V10 held-out source key exists as an external 0600 file, but no public
evaluator opened it. No held-out baseline key was created, no held-out record
was evaluated, and no held-out content or outcome was published.

The frozen V10 addendum requires terminal retirement for incomplete corpus
execution or resource failure and forbids repairing or retrying a retired
version. V10 is therefore permanently retired. V11 may reuse the independently
authenticated immutable V10 corpus, but it must use a new evaluator commitment,
a fresh evaluation root, and a memory-bounded replay implementation.
