# V13 Single-Worker Memory-Release Evaluator Protocol

Authenticate the V12 retirement, V13 contract and evaluator manifests,
installed-KiCad environment, and immutable V10 corpus before evaluation. Run
from a clean committed repository into a fresh V13 evaluation root.

Use exactly one deterministic case worker. Within a case, execute exactly two
sequential clean-root replays. Stream each complete replay to a read-only,
no-replace canonical JSON spool while hashing it. Promotion, observation, and
gate evidence use the original typed replay. Return from the replay helper so
the typed graph is unreachable, then run full garbage collection and return
free heap pages to the operating system before synthesizing the next replay.
Authenticate matching replay and promotion evidence, atomically persist the
case checkpoint, and only then remove its spools.

The single-worker limit and post-replay memory release are global and
independent of circuit content. V12 checkpoints are forbidden. Resume accepts
only checkpoints bound to the same V13 root, corpus, environment, requirement,
and evaluator commitments.

Any replacement, malformed root, inconsistent replay, incomplete cohort,
resource termination, or held-out access fails closed and cannot publish an
accepted report.
