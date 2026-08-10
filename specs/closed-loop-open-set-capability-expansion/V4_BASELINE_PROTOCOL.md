# Closed-Loop Open-Set V4 Baseline Protocol

Status: freeze candidate; execution prohibited until corpus freeze

## Admissible inputs

Baseline execution is valid only from outcome-changing starting commit
`3d2d9bb0e8ff3e68ae6a160c136030b5a3b6d7db` plus later contract and corpus
commits that change no outcome-affecting production code.

Every run binds:

- the complete V4 contract manifest and corpus manifest;
- starting, contract-freeze, and corpus-freeze commit hashes;
- the exact primitive inventory, catalog, model registry, and environment
  hashes;
- capability-feedback policy
  `closed-loop-capability-policy-v2-realizability`;
- normalized impact-registry hash
  `64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377`;
- synthesis-policy hash
  `4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4`;
  and
- gap-transition policy `closed-loop-gap-transition-v1` and its file hash.

Any mismatch fails before synthesis and writes no baseline artifact.

## Discovery baseline

1. Load discovery entries in manifest order and strictly decode them.
2. Run each case twice from isolated roots under the frozen policy and budgets.
3. Require byte-identical normalized synthesis evidence for each pair.
4. Promote each passing result twice through the complete clean-root installed-
   KiCad lane before counting it as a baseline pass.
5. Observe with `ObserveRealizabilityAware` and aggregate with
   `EvaluateRealizabilityAware` plus the frozen impact registry.
6. Atomically write per-case evidence, aggregate report, checksums, complete
   deterministic ranking, and rank-one expansion plan.
7. Freeze the selected tuple `(stage, scope, capability, code)`, required
   evidence, affected discovery cases, and all policy/corpus commitments.

No held-out requirement, outcome, gap, diagnostic, or classification may
participate in discovery baseline, ranking, or planning.

Both replay runs use the same frozen toolchain/environment, deterministic
seeds, and canonical map/set ordering. Normalization excludes volatile root
paths, timestamps, process IDs, and other non-semantic execution metadata
before byte comparison; electrical values and all decision evidence remain.

## Held-out baseline seal

Only after discovery selection is committed:

1. run held-out entries in manifest order through the identical two-run and
   promotion path;
2. aggregate held-out evidence separately;
3. suppress plaintext content, per-case logs, outcomes, gaps, and diagnostics
   from the implementation context;
4. encrypt the normalized payload with authenticated encryption and a fresh
   key held outside the repository; and
5. commit only counts, manifest/selection/policy commitments, plaintext hash,
   ciphertext hash, algorithm metadata, and ciphertext.

The key must never enter the repository, command transcript, Prism payload, or
implementation context before the implementation seal.

## Frozen artifacts and overwrite behavior

Discovery freeze creates versioned discovery evidence, aggregate report,
selection, checksums, and baseline audit. Held-out freeze creates authenticated
ciphertext, its seal, and checksum. Creation is atomic; update modes refuse to
overwrite any existing artifact. Normal tests are read-only reproducers.

The starting-state implementation hashes protect classifier and evaluator
semantics during baseline. The selected implementation phase may change only
the sealed capability boundary; it must not change evaluator, impact, synthesis,
gap-transition, corpus, baseline, or selection semantics.

## Final comparison

Discovery final synthesis runs first. Validation stops without opening held-out
final evidence unless total discovery and rank-one-affected discovery pass
counts strictly improve and all discovery preservation gates pass.

For every still-nonpassing case, comparison applies
`V4_GAP_TRANSITION_PROTOCOL.md`: remove only exact selected-cluster gaps from
the baseline set, then require the remaining baseline identities to be a subset
of the final identities. Final supersets are diagnostic progress, not failure.
Missing, renamed, reclassified, or required-evidence-mutated nonselected
baseline identities fail closed.

Only the fixed Prism-reviewed implementation seal may enter the one-time blind
held-out run. Success artifacts are written atomically only after every V4
criterion passes. Failure writes no partial success artifact, consumes the
held-out set if revealed, and creates an immutable audit marker that permanently
blocks update modes.
