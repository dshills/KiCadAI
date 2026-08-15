# Closed-Loop Open-Set Capability Expansion V11 Addendum

Status: corrective evaluator candidate; no V11 evaluation, baseline, selection,
held-out access, or capability implementation has begun.

## 1. Corrective boundary

V11 succeeds the permanently retired V10 experiment. V10 stopped during public
generation-zero evaluation because its frozen evaluator exhausted host memory
after 21 of 24 authenticated checkpoints. No accepted V10 report or baseline
exists, and V10 held-out material was not opened.

V11 binds `V10_GENERATION_ZERO_RETIREMENT.json`, its audit, the exact V10
evaluation-root commitment, and the partial-checkpoint checksum commitment.
Those partial checkpoints are diagnostic retirement evidence only and are not
inputs to V11 evaluation, ranking, or selection.

## 2. Immutable corpus reuse

V11 reuses the authenticated V10 corpus byte-for-byte. No requirement,
authorship record, manifest, checksum, encrypted held-out record, assignment,
or corpus key may change. Reuse is permitted because V10 produced no accepted
outcome and the V11 correction changes only generic evaluator storage.

V11 starts from a fresh absent working root and evaluates all 24 discovery
cases. It does not resume or copy V10 checkpoints. Held-out records remain
encrypted and unopened until the normal post-selection blind protocol permits
access.

## 3. Outcome-equivalent evaluator correction

V11 preserves the V10 synthesis policy, installed-KiCad gates, evidence schema,
case ordering, exactly two sequential replays per case, and four-case worker
limit. The only permitted evaluator change is memory-bounded replay storage:

- canonical replay JSON is streamed to a fresh read-only no-replace spool file;
- the replay SHA-256 is computed over exactly the bytes produced by the prior
  `json.Marshal` representation;
- provisional promotion, observation, and gate evidence is derived from the
  original typed first replay before that replay is released;
- a complete synthesis-run object is released before the next replay begins;
- no worker may retain two complete replay values or a complete marshaled
  replay byte slice;
- authenticated spool files are removed only after the durable case checkpoint
  is written.

Provisional evidence is never accepted unless both replay hashes and promotion
results match. Synthetic equivalence tests must prove that the streaming bytes,
replay hashes, observations, gates, and promotions are identical to the frozen
V10 semantics. V11 root/version commitments and hashes derived from them are
intentionally version-separated. Limits and fail-closed gates may not be
weakened.

## 4. Freeze and execution

The V11 contract and evaluator manifests bind the immutable corpus commitments,
V10 retirement, generic implementation, tests, and protocol documents. V11 may
start only after those manifests are reviewed, committed, and verified from a
clean repository.

Any corpus mutation, checkpoint reuse, held-out access before authorization,
byte-equivalence failure, incomplete 24-case run, nondeterministic replay,
installed-KiCad failure, or publication failure retires V11 without accepting
partial evidence.
