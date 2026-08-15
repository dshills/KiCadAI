# V16 Streaming-Finalization Single-Worker Evaluator Protocol

Authenticate the V15 retirement, V16 contract and evaluator manifests,
installed-KiCad environment, and immutable V10 corpus before evaluation. Run
from a clean committed repository into a fresh V16 evaluation root.

Production synthesis must preserve V15 topology, value-trial, simulation,
repair, physical-lowering, ranking, budget, and output semantics. Top-level
synthesis finalization must stream the exact `encoding/json.Marshal`-compatible
byte sequence into SHA-256 without retaining a complete encoded copy. The
resulting synthesis hash and returned evidence must remain byte-for-byte
equivalent to the frozen representation. Unsupported types, cycles, writer
errors, or any other encoding failure must terminate fail closed rather than
returning an empty or partial hash.

V15's compact queued value trials, deterministic best-failure-per-topology
retention, bounded ordered proposal sizing, and streamed repair-result hashing
remain mandatory and unchanged.

Use exactly one deterministic case worker. Within a case, execute exactly two
sequential clean-root replays. Stream each complete replay to a read-only,
no-replace canonical JSON spool while hashing it. Promotion, observation, and
gate evidence use the original typed replay. Return from the replay helper so
the typed graph is unreachable, then run full garbage collection and return
free heap pages to the operating system before synthesizing the next replay.
Authenticate matching replay and promotion evidence, atomically persist the
case checkpoint, and only then remove its spools.

V15 checkpoints are forbidden. Resume accepts only checkpoints bound to the
same V16 root, corpus, environment, requirement, and evaluator commitments.
Any replacement, malformed root, inconsistent replay, incomplete cohort,
resource termination, or held-out access fails closed and cannot publish an
accepted report.
