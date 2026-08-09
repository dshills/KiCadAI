# Closed-Loop Open-Set V3 Baseline Protocol

Status: freeze candidate; execution prohibited until corpus freeze

## Inputs

Baseline execution is valid only from starting commit
`859c8df068db8254b715042b691c441a0d135fab` plus a later corpus-freeze commit
that changes no outcome-affecting production code.

The run binds:

- `V3_SPEC_ADDENDUM.md`, `V3_CORPUS_RULES.md`, and `V3_PLAN.md`;
- the V3 manifest and its checksum;
- the exact primitive inventory, catalog, model registry, environment, policy,
  and policy hashes;
- capability-feedback policy
  `closed-loop-capability-policy-v2-realizability`; and
- normalized V3 impact-registry hash
  `64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377`;
  and
- synthesis-policy hash
  `4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4`.

## Discovery baseline

1. Load discovery entries in manifest order.
2. Strictly decode and normalize each requirement.
3. Synthesize each case twice from isolated roots with the frozen policy.
4. Require byte-identical normalized synthesis evidence for each pair.
5. Promote a passing synthesis twice through the full clean-root installed-
   KiCad lane before counting it as a baseline pass.
6. Observe each case with `ObserveRealizabilityAware`.
7. Aggregate discovery with `EvaluateRealizabilityAware` and the frozen impact
   registry.
8. Write per-case evidence, the aggregate baseline report, checksums, and the
   complete deterministic rank ordering atomically.
9. Build and seal the rank-one expansion plan.

No held-out requirement, classification, synthesis result, or diagnostic may
participate in discovery baseline, clustering, ranking, or planning.

## Held-out baseline seal

After discovery selection is committed:

1. Run held-out entries in manifest order through the same two-run synthesis
   and promotion path.
2. Observe with policy v2 and aggregate only as held-out evidence.
3. Suppress per-case logs and plaintext artifacts from the implementation
   agent.
4. Encrypt the complete normalized payload with an authenticated algorithm and
   a fresh key stored outside the repository.
5. Commit only case count, manifest/selection/policy/registry commitments,
   plaintext payload hash, ciphertext hash, algorithm metadata, and ciphertext.

The reveal key must not enter the repository, shell transcript, Prism payload,
or implementation context before the reviewed production diff is sealed.

## Baseline artifacts

The discovery freeze writes:

- `V3_DISCOVERY_BASELINE_REPORT.json` and `.sha256`;
- `V3_SELECTION.json` and `.sha256`;
- normalized per-discovery-case evidence; and
- `V3_BASELINE_AUDIT.md`.

The blind baseline writes:

- `V3_HELD_OUT_BASELINE_SEAL.json` and `.sha256`; and
- authenticated ciphertext in the versioned baseline testdata directory.

Update modes must refuse to overwrite an existing frozen artifact. All normal
tests reproduce committed bytes read-only.

## Final comparison

Discovery final synthesis runs first. If total or selected discovery pass count
does not strictly improve, validation stops and held-out final evidence remains
unrevealed. Only a fixed, reviewed production diff with passing discovery may
open the one-time held-out final run.

The final verifier atomically writes comparison, promotion matrix, and final
report artifacts only after every V3 success criterion passes. Otherwise it
fails closed without partial completion artifacts.
