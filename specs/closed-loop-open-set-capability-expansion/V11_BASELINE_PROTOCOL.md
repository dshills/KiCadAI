# V11 Memory-Bounded Public Baseline Protocol

## Preconditions

Run from a clean committed repository after authenticating the V11 contract,
evaluator, environment, immutable V10 corpus, and V10 retirement. The V11
working root and report destination must not exist. Discovery evaluation may
not read any held-out key or encrypted record.

## Evaluation

Evaluate all 24 public discovery cases with the frozen V10 synthesis policy and
installed-KiCad environment. Use four case workers. Within each case, execute
exactly two sequential replays from clean roots.

Each replay is streamed as canonical JSON to a case-local no-replace spool file
while its SHA-256 is computed. The file is made read-only before use. Promotion
is run against the original typed replay. Observation and gate evidence are
derived provisionally from replay one, after which its complete synthesis value
becomes unreachable before replay two begins. Replay two is then synthesized,
streamed, and promoted. At no point may a worker retain two complete replay
values.

Replay hashes and promotion results must match before any evidence is accepted.
Write the case checkpoint atomically, authenticate it, then remove only that
case's spool files.

The streaming serializer must produce byte-for-byte the prior `json.Marshal`
encoding. Symlinks, replacement, truncation, write failure, or hash mismatch
fail closed.

## Publication

Only a complete deterministic 24-case report with two matching replays per
case, complete route/evidence gates, and required installed-KiCad promotions is
eligible for atomic baseline publication. Partial checkpoints are never an
accepted report. Any incomplete or invalid state publishes only the
protocol-defined retirement artifact.
