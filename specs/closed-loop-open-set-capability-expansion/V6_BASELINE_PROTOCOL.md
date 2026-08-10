# Closed-Loop Open-Set V6 Baseline Protocol

Status: freeze candidate; execution prohibited until corpus freeze

## Admissible state

Execution begins from the V6 starting commit plus separately committed contract,
author packet, fresh corpus, and outcome-neutral selector infrastructure. Every
run binds the exact corpus manifest, author and validator commitments, freeze
commits, evaluator, impact registry, synthesis policy, gap-transition policy,
selection policy, primitive inventory, catalog, model registry, simulation
environment, and installed-KiCad toolchain identity.

Mismatch, uncommitted outcome-affecting code, missing evidence, or an existing
retirement/final artifact aborts before synthesis and writes nothing.

## Discovery baseline

1. Load exactly 18 discovery entries in immutable manifest order and strictly
   decode each behavior-only requirement.
2. Run every case twice from isolated roots under identical seeds, inputs,
   ceilings, inventory, catalog, models, and environment.
3. Require byte-identical complete exported synthesis evidence.
4. Promote every synthesis pass twice through clean installed-KiCad roots;
   require clean ERC, strict DRC, connectivity, route completion, writer
   correctness, workflow, simulation, and zero round-trip diff evidence.
5. Observe and aggregate with the frozen realizability-aware evaluator and
   impact registry.
6. Preserve each nonpassing case's complete normalized exact root-gap set and
   causal-suppression evidence.
7. Build capability atoms and bounded candidate bundles from discovery only.
8. Credit a candidate with a case only when the candidate covers the case's
   complete exact root-member set and all reuse, size, outcome, and uniqueness
   constraints pass.
9. Reject nonminimal supersets, ineligible candidates, ambiguous ties, and any
   nondeterministic ordering.
10. Require rank one to have one executable generic plan covering every atom
    and exact member; do not skip rank one.
11. Atomically publish case evidence, aggregate, all candidate decisions,
    ranking, selection, generic plan, checksums, and audit.

No held-out source, outcome, gap, diagnostic, baseline payload, or bundle
membership may be opened or consulted during discovery selection.

## Selection artifact

The selection freezes:

- the canonical length-prefixed bundle key;
- sorted capability atoms and exact members;
- sorted claimed-unlock discovery cases and reporting domains;
- atom support, complete blocker-coverage proof, minimality proof, size proof,
  eligibility proof, and complete ranking tuple;
- required-evidence union and executable generic-plan hash;
- corpus, baseline, evaluator, impact, synthesis, transition, selection,
  inventory, catalog, model, environment, and toolchain commitments; and
- starting, contract, corpus, infrastructure, and selection commits.

Duplicate atoms/members/cases, partial blocker coverage, nonminimality, partial
plans, ambiguous ties, nondeterministic order, no eligible bundle, or an
unexecutable rank one fail closed.

## Held-out baseline seal

Only after selection is committed may an isolated custodian evaluate all 18
held-out cases through the identical replay and physical-promotion path. Source
and baseline payloads use distinct fresh external 0600 keys and authenticated
length-delimited metadata. The repository receives only ciphertext, counts,
hashes, algorithm metadata, and policy/selection commitments.

Plaintext requirements, per-case outcomes, gaps, diagnostics, bundle membership,
and promotion details remain unavailable to implementation, Prism, logs, and
repository history.

## Implementation and public admission

Outcome-affecting changes are restricted to the selected bundle and plan-bound
generic prerequisites. Prism reviews the non-sensitive staged implementation;
the exact reviewed bytes are self-hashed and committed.

Public discovery final runs before any held-out reveal. It must prove strict
total and claimed-unlock pass uplift, at least one realized claimed unlock,
exact unique case sets, no pass regression or unsafe-to-pass transition,
selected-member-only removal, monotonic nonselected-gap preservation,
deterministic replay, and all simulation/physical/KiCad/writer/routing/
connectivity/round-trip gates. Failure atomically retires V6 without opening
held-out material.

## One-time blind final

After public admission, a clean-tree authorized custodian validates the reviewed
seal and three distinct external 0600 key paths, then opens the held-out source
and baseline exactly once. Success requires strict held-out pass uplift, the
complete-blocker selected-bundle causal boolean, no pass regression or
unsafe-to-pass transition, exact case-set preservation, selected-member-only
removal, monotonic nonselected-gap preservation, and complete replay and
physical evidence.

Success artifacts are atomic and no-replace. Any failure after reveal writes
only a non-revealing permanent audit. Either result consumes V6 and blocks all
update modes forever.
