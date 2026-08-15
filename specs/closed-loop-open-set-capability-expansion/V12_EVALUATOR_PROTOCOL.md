# V12 Aggregate-Memory-Bounded Evaluator Protocol

Authenticate the V11 retirement, V12 contract and evaluator manifests,
installed-KiCad environment, and immutable V10 corpus before evaluation. Run
from a clean committed repository into a fresh V12 evaluation root.

Use exactly two deterministic case workers. Within a case, execute exactly two
sequential clean-root replays. Stream each complete replay to a read-only,
no-replace canonical JSON spool while hashing it. Promotion, observation, and
gate evidence use the original typed replay. Make replay one unreachable before
synthesizing replay two. Authenticate matching replay and promotion evidence,
atomically persist the case checkpoint, and only then remove its spools.

The two-worker limit is global and independent of circuit content. V11
checkpoints are forbidden. Resume accepts only checkpoints bound to the same
V12 root, corpus, environment, requirement, and evaluator commitments.

Any replacement, malformed root, inconsistent replay, incomplete cohort,
resource termination, or held-out access fails closed and cannot publish an
accepted report.
