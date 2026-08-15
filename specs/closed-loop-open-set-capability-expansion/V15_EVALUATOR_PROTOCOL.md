# V15 Bounded-Failure Single-Worker Evaluator Protocol

Authenticate the V14 retirement, V15 contract and evaluator manifests,
installed-KiCad environment, and immutable V10 corpus before evaluation. Run
from a clean committed repository into a fresh V15 evaluation root.

Production synthesis must validate value trials in frozen deterministic order
without retaining a materialized graph per queued trial. During evaluation it
may retain at most one failed graph per topology: the graph selected by the
same penalty, candidate-index, and graph-hash ordering used by the frozen
repair phase. Replacing a retained failure must make the prior graph
unreachable. Complete candidate reports, evaluations, attempts, budgets,
repair order, and final output bytes remain unchanged.

Topology-repair sizing must visit whole-graph value variants in the same
deterministic order and stop immediately after the frozen per-proposal limit is
selected. It must not first materialize variants that cannot be evaluated.
Repair-result hashing must stream the exact `encoding/json.Marshal`-compatible
byte sequence into SHA-256 without retaining a complete multi-gigabyte encoded
copy. The resulting hash and returned repair evidence must remain byte-for-byte
equivalent to the frozen representation.

Use exactly one deterministic case worker. Within a case, execute exactly two
sequential clean-root replays. Stream each complete replay to a read-only,
no-replace canonical JSON spool while hashing it. Promotion, observation, and
gate evidence use the original typed replay. Return from the replay helper so
the typed graph is unreachable, then run full garbage collection and return
free heap pages to the operating system before synthesizing the next replay.
Authenticate matching replay and promotion evidence, atomically persist the
case checkpoint, and only then remove its spools.

V14 checkpoints are forbidden. Resume accepts only checkpoints bound to the
same V15 root, corpus, environment, requirement, and evaluator commitments.
Any replacement, malformed root, inconsistent replay, incomplete cohort,
resource termination, or held-out access fails closed and cannot publish an
accepted report.
