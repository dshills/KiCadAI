# V14 Lazy-Graph Single-Worker Evaluator Protocol

Authenticate the V13 retirement, V14 contract and evaluator manifests,
installed-KiCad environment, and immutable V10 corpus before evaluation. Run
from a clean committed repository into a fresh V14 evaluation root.

Production synthesis must validate value trials in its frozen deterministic
order while retaining no materialized candidate graph per queued value trial.
Materialize exactly the graph needed for the current evaluation, preserve the
validated trial order and all count budgets, and fail closed if a previously
validated trial cannot be materialized identically.

Use exactly one deterministic case worker. Within a case, execute exactly two
sequential clean-root replays. Stream each complete replay to a read-only,
no-replace canonical JSON spool while hashing it. Promotion, observation, and
gate evidence use the original typed replay. Return from the replay helper so
the typed graph is unreachable, then run full garbage collection and return
free heap pages to the operating system before synthesizing the next replay.
Authenticate matching replay and promotion evidence, atomically persist the
case checkpoint, and only then remove its spools.

V13 checkpoints are forbidden. Resume accepts only checkpoints bound to the
same V14 root, corpus, environment, requirement, and evaluator commitments.
Any replacement, malformed root, inconsistent replay, incomplete cohort,
resource termination, or held-out access fails closed and cannot publish an
accepted report.
