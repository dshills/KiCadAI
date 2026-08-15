# V17 Bounded-Transient-Evidence Single-Worker Evaluator Protocol

Authenticate the V16 retirement, V17 contract and evaluator manifests,
installed-KiCad environment, and immutable V10 corpus before evaluation. Run
from a clean committed repository into a fresh V17 evaluation root.

Production synthesis must preserve V16 topology, value-trial, repair,
physical-lowering, ranking, and budget semantics. V17 dynamic simulations use a
uniform grid of at most 100,000 steps. The existing duration, requested timing
resolution, and event-alignment calculation remains authoritative below that
ceiling. The complete report is canonically streamed into SHA-256, and the
assertion result is derived before the evaluator retains an endpoint-preserving
uniform witness of at most 256 points per analysis. Unsupported types, cycles,
writer errors, invalid grids, or any other encoding failure must terminate fail
closed rather than return an empty or partial hash.

V16's compact queued value trials, deterministic best-failure-per-topology
retention, bounded ordered proposal sizing, streamed repair/synthesis hashing,
and complete-proof report hashes remain mandatory. The legacy evaluator entry
point and its one-million-step ceiling remain unchanged for historical replay.

Use exactly one deterministic case worker. Within a case, execute exactly two
sequential clean-root replays. Stream each complete replay to a read-only,
no-replace canonical JSON spool while hashing it. Promotion, observation, and
gate evidence use the original typed replay. Return from the replay helper so
the typed graph is unreachable, then run full garbage collection and return
free heap pages to the operating system before synthesizing the next replay.
Authenticate matching replay and promotion evidence, atomically persist the
case checkpoint, and only then remove its spools.

V16 checkpoints are forbidden. Resume accepts only checkpoints bound to the
same V17 root, corpus, environment, requirement, and evaluator commitments.
Any replacement, malformed root, inconsistent replay, incomplete cohort,
resource termination, or held-out access fails closed and cannot publish an
accepted report.
