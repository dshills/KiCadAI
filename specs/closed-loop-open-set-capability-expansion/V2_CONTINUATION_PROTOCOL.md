# Closed-Loop Open-Set Capability Expansion V2 Continuation Protocol

Status: independent corpus authoring authorized; candidate corpus not yet frozen

## Purpose

V1 produced two new discovery passes but no held-out pass. Because its
held-out result is now known, additional implementation work cannot be
validated against V1 without test-set contamination. V2 preserves the original
closed-loop objective while starting a fresh, auditable selection and blind
validation cycle.

This protocol does not relax the V1 specification. It establishes the evidence
required for a new version after an honest failed experiment.

## Preconditions

Before any additional production change capable of altering a corpus outcome:

1. Preserve `V1_VALIDATION_AUDIT.md` and the committed V1 corpus, baseline,
   selection, and hashes unchanged.
2. Review the current generic implementation and either checkpoint it as the
   explicit V2 starting state or remove it. The choice and commit hash become
   part of the V2 baseline contract.
3. Assign fresh corpus authorship to a context-isolated author that has not
   seen V1 held-out requirements, identities, outcomes, diagnostics, or the
   implementation diff.
4. Freeze the V2 specification addendum, corpus rules, authorship statement,
   manifest, source hashes, policies, catalogs, environment, and work budgets
   in a commit before V2 baseline execution.

## Corpus

V2 must contain 12 discovery and 12 held-out behavior-only requirements with
the same six-domain balance as V1. All V1 neutrality constraints continue to
apply. The V2 corpus must also prove:

- no exact or normalized-text overlap with V1;
- no project, topology, part, model, coordinate, repair, expected-outcome, or
  capability language;
- no access by the implementation agent to held-out source bytes before the
  capability choice and production diff are sealed;
- independent source hashes and authorship provenance; and
- a deterministic role split committed before baseline execution.

The author may use the public behavior schema and domain quotas. The author may
not inspect current search candidates, failure clusters, production code,
component choices, or V1 held-out material.

## Baseline and selection

Run every V2 case twice from the checkpointed starting commit through the
normal production path. Classify each case only as `pass`, `unsupported`,
`unsafe`, or `exhausted`. Promote baseline passes through the same physical and
installed-KiCad gates.

Cluster and rank discovery failures with the unchanged identity-neutral
policy. Seal the new rank-one cluster, affected discovery membership, complete
ranking tuple, required evidence, expansion-plan hash, and baseline hashes
before opening any V2 held-out evidence to the implementation agent.

## Implementation boundary

Implement only the V2 rank-one reusable capability or inseparable generic
prerequisites expressly listed in its required evidence. Production code and
tests must not contain V1 or V2 identities, file paths, hashes, coordinates,
expected outcomes, allowlists, or fixture-specific topology families.

The implementation is complete for validation only after focused generic,
boundary, adversarial, deterministic replay, and fail-closed tests pass and the
production diff is sealed.

## Blind validation and success

Run V2 discovery to completion before V2 held-out execution begins. The final
verifier must require, without policy or budget drift:

- at least one additional discovery pass;
- at least one additional held-out pass;
- increased pass count among rank-one-affected discovery cases;
- no baseline-pass regression;
- preserved unsafe evidence;
- stable remaining cluster identities;
- byte-identical two-run synthesis evidence;
- two clean-root promotions for every new pass; and
- local installed-KiCad ERC, strict DRC, connectivity, route completion,
  writer correctness, zero round-trip differences, and replay.

If held-out uplift is again zero, V2 fails and its held-out set is consumed.
No retry, corpus mutation, gate relaxation, hidden budget change, or
post-revelation tuning may convert that run into a pass.

## Release boundary

Only a passing blind verifier may generate final report, comparison, promotion
matrix, and completion-audit artifacts. Before release, run the complete local
regression suite with an explicit timeout suitable for the electrothermal
corpus, rerun both protected USB-C fixtures, scan for corpus-identity leakage,
review the staged diff with Prism, remediate findings, commit, and push. GitHub
Actions are not manually invoked or monitored.
