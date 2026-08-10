# Closed-Loop Open-Set V5 Baseline Protocol

Status: freeze candidate; execution prohibited until corpus freeze

## Admissible state

Baseline execution starts from
`d8e98b4dee3212823525c5955e8e025bd0039d03` plus contract, authoring-packet,
corpus, and outcome-neutral package-ranking infrastructure commits. Every run
must prove that outcome-affecting production bytes still equal the starting
state.

The run binds the V5 contract checksum, authoring packet, corpus manifest,
author and validator commitments, freeze commits, primitive inventory, catalog,
model registry, simulation environment, evaluator policy, inherited public
impact/synthesis/gap-transition policies, and `V5_SELECTION_POLICY.json`.
Mismatch or missing evidence aborts before synthesis and writes nothing.

## Discovery baseline

1. Load 18 discovery entries in immutable manifest order and strictly decode.
2. Run every case twice from isolated roots with identical frozen inputs,
   deterministic seeds, and policy ceilings.
3. Require byte-identical normalized synthesis evidence. Normalization may
   remove only volatile paths, timestamps, and process metadata.
4. Promote each synthesis pass twice through clean installed-KiCad roots before
   it can be observed as pass.
5. Observe with `ObserveRealizabilityAware` and aggregate with
   `EvaluateRealizabilityAware` under the frozen impact registry.
6. Preserve exact typed root-gap clusters and causal suppression evidence.
7. Group clusters by `(scope, capability)`, compute package evidence, reject
   ineligible packages, and rank eligible packages exactly as frozen.
8. Require the highest-ranked package to have one executable generic plan that
   covers every member; do not skip to another package.
9. Atomically publish discovery evidence, aggregate, full cluster/package
   rankings, selection, plan, checksums, and audit.

No held-out source, outcome, gap, diagnostic, baseline payload, or membership
may be opened or consulted during these steps.

## Package selection artifact

The selection freezes:

- package identity `(scope, capability)` and canonical package key;
- sorted exact member identities `(stage, scope, capability, code)`;
- sorted affected discovery case IDs and reporting domains;
- complete ranking tuple and eligibility proof;
- sorted required-evidence union;
- executable generic expansion-plan hash;
- corpus, baseline, evaluator, impact, synthesis, transition, selection-policy,
  inventory, catalog, model, and environment hashes; and
- starting, contract, corpus, and selection commits.

Duplicate cases/members, partial plans, ambiguous ties, nondeterministic order,
or no eligible package fail closed.

## Held-out baseline seal

Only after selection is committed, evaluate all 18 held-out cases through the
identical two-run and promotion path. Encrypt held-out source and normalized
baseline evidence with distinct fresh external keys and authenticated,
length-delimited metadata. Commit only ciphertext, counts, hashes, algorithm
metadata, and policy/selection commitments.

Plaintext requirements, per-case evidence, outcomes, gaps, diagnostics,
package membership, and promotion details remain unavailable to implementation,
Prism, logs, and repository history.

## Implementation and public admission

Outcome-affecting changes are restricted to the selected package and plan-bound
generic prerequisites. Prism reviews the non-sensitive staged implementation;
the exact reviewed bytes are sealed and committed.

Final discovery runs before any held-out reveal. It must prove strict total and
selected-package-affected uplift, exact unique case sets, no pass/unsafe
regression, monotonic nonselected-gap preservation, deterministic replay, and
all simulation, physical, installed-KiCad, writer, routing, connectivity, and
round-trip gates. Failure retires V5 without opening held-out material.

## One-time blind comparison

After public admission, a clean-tree updater validates the reviewed seal and
three distinct external key paths, then opens the held-out baseline and source
exactly once. Success requires:

- strict held-out pass-count improvement;
- at least one newly passing held-out case whose baseline root-gap package
  exactly matches the selected `(scope, capability)`;
- no baseline pass regression or unsafe-to-pass transition;
- exact case-set preservation;
- removal only of exact selected member identities from still-nonpassing cases;
- monotonic preservation of all nonselected baseline gap identities; and
- all replay, simulation, physical-promotion, KiCad, writer, and round-trip
  evidence.

The selected-package causal result is published only as a boolean. Success
artifacts are atomic and no-replace. Any failure after reveal writes only a
non-revealing permanent audit. Either result consumes V5 and blocks all update
modes forever.
