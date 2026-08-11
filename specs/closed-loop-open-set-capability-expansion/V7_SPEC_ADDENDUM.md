# Closed-Loop Open-Set Capability Expansion V7 Addendum

Status: freeze candidate; corpus authoring and synthesis prohibited until the
V7 contract freeze commit exists

## 1. Purpose and inherited contract

V7 continues `SPEC.md` and inherits every unchanged fail-closed,
deterministic-replay, physical-promotion, installed-KiCad, writer, routing,
connectivity, and round-trip requirement.

V6 retired at public discovery admission before any held-out source, baseline,
or final key was opened. Its public aggregate result was zero discovery passes
before and after the selected bundle and zero claimed-unlock passes before and
after. The selected exact root blockers were therefore not a sufficient model
of the complete blocker chain: removing or changing a visible root can expose a
new downstream blocker without making the case pass.

V7 replaces one-shot root-set selection with a bounded adaptive causal-chain
protocol. Every adaptation uses the unchanged public discovery corpus, follows
a frozen deterministic policy, receives one implementation and one discovery
rerun, and is committed before the next frontier is known. Held-out evidence
remains isolated and cannot influence any round.

The sole authoritative starting commit is the full object ID in
`V7_SELECTION_POLICY.json.starting_commit`. Human-readable artifacts reference
that field and do not repeat its value. The freeze validator proves that the
object exists and matches the V6 retirement commitment; a symbolic branch
reference is not admissible.

## 2. Fresh corpus and isolation

V7 contains 36 new behavior-only requirements: 18 discovery and 18 held-out.
Exactly three context-isolated authors each receive only the committed V7
public packet and one disjoint assignment. Authors receive no repository
implementation, prior requirement source, synthesis result, gap, selection,
round transcript, or other author's work.

V1 through V6 raw and available semantic commitments are historical
exclusions. Retired held-out plaintext remains permanently unavailable.
Validation is outcome-neutral and publication encrypts held-out source with a
fresh external 0600 key.

## 3. Initial baseline and causal frontier

Every discovery case runs twice from isolated roots under frozen limits,
seeds, policies, catalog, inventory, model registry, simulation environment,
and installed-KiCad promotion environment. Complete exported replay evidence
must be byte-identical. Every synthesis pass must pass two clean-root physical
promotions.

Each nonpassing observation publishes:

- its complete normalized typed root-member set;
- the complete normalized suppressed and candidate failure inventory allowed
  by the public evaluator;
- deterministic causal-suppression edges;
- the frozen gap-transition classification for every edge; and
- an explicit frontier generation number.

A member identity remains `(stage, scope, capability, code)`. Full gap identity
also binds required evidence. Empty, duplicate, unknown, or noncanonical
frontiers fail closed.

`Unsafe` is the frozen evaluator outcome, not a reviewer judgment. It is emitted
when a critical safety assertion fails, required safety evidence is absent, or
physical promotion reports a safety-critical electrical, thermal, clearance,
connectivity, or protection violation. The unsafe-to-pass gate mechanically
rejects any case whose prior frozen outcome is `unsafe` and whose later outcome
is `pass`; V7 never grants unlock or advancement credit to an unsafe case.

## 4. Adaptive causal-chain rounds

V7 permits at most three discovery implementation rounds. The ceiling, total
atom and member budgets, reuse floors, cohort rules, ranking, tie behavior, and
round gates are frozen before corpus authoring.

At generation zero, every discovery case with outcome `unsupported` or
`exhausted` is chain-eligible. A round advances a case only when every member of
its current complete root frontier is covered by that round's selected bundle.
An uncovered case carries its current frontier forward and remains eligible for
a later round; it receives no advancement credit in the current round. Every
case remains in regression and aggregate evidence.

For each round:

1. deterministically build minimal bundles from the active cohort's complete
   current frontiers;
2. select the highest-impact eligible bundle without identity-order
   tie-breaking;
3. freeze one complete generic implementation plan for every selected atom and
   exact member;
4. implement only that bundle and prerequisites admitted by the frozen
   compile/execution/shared-invariant checklist;
5. Prism-review, remediate, self-hash, and commit the exact implementation;
6. rerun all 18 discovery cases exactly once, with two synthesis replays and
   required physical promotions; and
7. publish either a passing final public admission, an admitted next frontier,
   or a permanent retirement.

An intermediate round advances only when at least two active cases across at
least two reporting domains remove all selected current-frontier members and
each still-nonpassing case exposes a nonempty next frontier. A nonselected gap
may not disappear silently: it must remain byte-identical or have an explicit
successor edge admitted by the frozen transition policy. No-op rounds,
ambiguous lineage, selected-member persistence, regressions, unsafe-to-pass
transitions, or environment drift retire V7.

The protocol stops at the first round that proves strict total discovery pass
uplift and at least one newly passing active-cohort case. It may not continue to
collect optional improvements. Reaching the round ceiling without those gates
retires V7.

## 5. Selection and ranking

Every round ranks minimal current-frontier bundles by:

1. active causal-cohort cases whose complete frontier is covered, descending;
2. reporting-domain count of those cases, descending;
3. safety weight of those cases, descending;
4. capability-atom count, ascending; and
5. exact-member count, ascending.

The canonical bundle key controls publication order only after the five
semantic fields. When multiple bundles tie for semantic rank one, the selector
publishes the complete co-rank-one set and chooses the canonical bundle key in
ascending byte order. The artifact states that this is a deterministic fallback
among equally supported interventions, not evidence that the chosen bundle has
greater impact. Every atom must occur in at least two eligible cases, and every
bundle must cover at least two cases across two domains. Prior-round atoms
cannot be selected again.

The final selection artifact is an ordered chain. It binds every round's input
frontier hash, bundle, members, cohort, plan, implementation seal, discovery
result, lineage proof, environment transition, and output-frontier hash.

## 6. Environment transition boundary

Toolchain, synthesis policy, evaluator, gap-transition policy, physical gates,
budgets, seeds, and promotion environment are immutable across rounds.

Catalog, inventory, or model-registry bytes may change only when the selected
round explicitly contains a component or model capability whose frozen plan
requires that change. The exact before/after byte set and hashes must be
recorded in the implementation seal, every changed artifact must be mapped to
the selected capability, and all other environment bytes must remain
unchanged. Such a reviewed selected delta is an admitted environment
transition, not unqualified environment preservation.

## 7. Implementation boundary

Production code and generic tests must not contain V7 case identities, corpus
paths or hashes, expected outcomes, fixture coordinates, allowlists, named
circuit families, or block-family dispatch. Tests must be representative,
adversarial, boundary-focused, deterministic, and fail closed.

Each round preserves the full local suite and protected installed-KiCad
fixtures. No round may alter corpus source, selection policy, evaluator,
budgets, acceptance gates, or prior evidence. A round implementation is final
once its reviewed seal is committed.

## 8. Held-out boundary and final

After generation-zero selection is committed, an isolated custodian evaluates
the 18 held-out cases against the untouched starting implementation and
publishes only authenticated ciphertext, counts, commitments, and
non-revealing audit evidence under a distinct external key. No plaintext
outcome, gap, diagnostic, lineage, or bundle membership is disclosed.

Only a successful public discovery admission may authorize one blind held-out
final. The custodian replays the final ordered capability chain as a single
sealed implementation transition and requires strict held-out pass uplift, at
least one newly passing held-out case whose hidden baseline blocker lineage is
covered by the ordered selected chain, no regression, no unsafe-to-pass
transition, exact case-set preservation, deterministic replay, and complete
physical evidence. The causal result is disclosed only as a boolean.

The blind final is one logical evaluation session. A post-reveal infrastructure
interruption may resume only from the encrypted authenticated case-boundary
checkpoint under the frozen protocol; completed cases are never reevaluated and
no human may inspect plaintext or results. Success publishes encrypted evidence
and only the exact aggregate fields enumerated by
`V7_SELECTION_POLICY.json.held_out_disclosure` atomically. A terminal failure
publishes only a non-revealing permanent audit and no outcome-derived aggregate.
Either terminal outcome consumes V7.
