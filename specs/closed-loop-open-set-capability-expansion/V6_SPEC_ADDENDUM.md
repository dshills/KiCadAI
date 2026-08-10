# Closed-Loop Open-Set Capability Expansion V6 Addendum

Status: freeze candidate; corpus authoring and synthesis prohibited until the
contract freeze commit exists

## 1. Purpose and inherited contract

V6 continues `SPEC.md` and inherits every unchanged fail-closed, deterministic,
physical-promotion, installed-KiCad, writer, routing, connectivity, and
round-trip requirement.

V5 retired before held-out final evaluation. Its public result established that
ranking a capability by the number of failed cases containing that gap does not
prove that implementing the capability can make any case pass. The selected V5
member was removed from all three affected discovery cases, but each retained
other independent baseline gaps and pass counts did not improve.

V6 corrects the selection method. It ranks minimal causal-unlock bundles by the
number and diversity of discovery cases whose complete known root-blocker sets
the bundle covers. It does not weaken any admission gate and does not inspect,
reuse, decrypt, or infer from V5 held-out material.

V6 starts from commit:

`9b6f8be61006f7de179099feb0b38080ff18ecb3`

## 2. Fresh corpus and isolation

V6 contains 36 new behavior-only requirements: 18 discovery and 18 held-out.
Three independent authors each receive only the committed V6 author packet,
public requirement schema and vocabulary, and disjoint assignments. Authors
receive no repository implementation, prior corpus source, synthesis result,
selection result, or other author's work.

All V1 through V5 source and semantic commitments are historical exclusions.
V5 held-out plaintext remains permanently unavailable. Historical comparisons
use commitments only. Corpus authoring, validation, encryption, publication,
and implementation occur in separate contexts with the same non-collusion and
quarantine rules as V5.

## 3. Baseline evidence

Every discovery requirement runs twice from isolated roots with frozen inputs,
seeds, ceilings, inventory, catalog, model registry, and simulation environment.
The complete exported synthesis evidence must be byte-identical. Every
synthesis pass must independently pass two clean-root installed-KiCad physical
promotions before it is observed as pass.

Each nonpassing case contributes its complete normalized typed root-gap set
after causal suppression. A root member identity is exactly:

`(stage, scope, capability, code)`

Required-evidence values are retained in the full gap identity used by
preservation checks. Duplicate members, duplicate cases, missing identities,
unknown vocabulary, empty root sets for nonpassing cases, and outcome or
ordering ambiguity fail closed.

## 4. Causal-unlock bundle model

A capability atom is identified by `(scope, capability)`. Its exact members are
the sorted union of observed discovery root members carrying that atom.

A candidate bundle is a canonical sorted set of capability atoms and their
exact member union. Candidate generation uses discovery evidence only and is
the deduplicated closure of unions of eligible case blocker signatures. It is
bounded by `V6_SELECTION_POLICY.json`; exceeding the candidate ceiling fails
closed rather than truncating the ranking. A candidate receives unlock credit
for a case only when all of the following are true:

- the baseline outcome is `unsupported` or `exhausted`;
- every exact root member of the case is contained in the candidate member set;
- every candidate atom meets the frozen discovery reuse floor;
- the case and member identities are unique and canonical; and
- the candidate stays within the frozen atom and exact-member ceilings.

Merely sharing one member with a case grants no unlock credit. Unsafe cases are
never counted as unlockable and may never become pass.

A candidate is minimal only when no proper subset of its atoms unlocks the same
case set. Nonminimal supersets are rejected before ranking. This prevents a
large bundle from winning by absorbing unrelated capabilities.

## 5. Eligibility, ranking, and planning

An eligible bundle must unlock at least two discovery cases across at least two
reporting domains. Every atom must independently occur in at least two failed
discovery cases. Eligibility and ranking use only discovery cases.

Eligible bundles are ranked deterministically by:

1. unlocked discovery case count, descending;
2. unlocked reporting-domain count, descending;
3. unlocked safety weight, descending;
4. capability-atom count, ascending;
5. exact-member count, ascending.

The canonical length-prefixed bundle key orders candidates only for stable
publication after the five semantic ranking fields. If two rank-one candidates
tie on all five semantic fields, selection fails closed; identity order may not
break the admission tie. V6 does not choose by filesystem, author, case, map,
or concurrency order.

Rank one is admissible only if one executable generic expansion plan covers
every exact member and every capability atom. An unplanned member, fixture-
specific action, or missing generic evidence retires V6; the selector may not
skip to an easier bundle.

## 6. Implementation boundary

The frozen selection records the canonical bundle key, atoms, exact members,
claimed unlock cases and domains, ranking tuple, required-evidence union,
generic plan, corpus and policy commitments, and freeze commits.

Only the selected bundle and inseparable generic prerequisites explicitly
bound by its plan may change outcome-affecting code. Production code and its
generic tests must not contain V6 case identities, corpus paths or hashes,
expected outcomes, fixture coordinates, allowlists, named circuit families, or
block-family dispatch.

The exact staged implementation must pass representative, adversarial,
boundary, deterministic-replay, and fail-closed tests; preserve the full local
regression suite and protected KiCad fixtures; receive Prism review; and be
self-hashed and committed before public admission.

## 7. Public admission

Discovery final evaluation must prove:

- strict total discovery pass-count improvement;
- strict pass-count improvement among the selected claimed-unlock cases;
- at least one claimed-unlock case becoming pass;
- exact case-set preservation;
- no baseline pass regression or unsafe-to-pass transition;
- removal only of exact selected members from still-nonpassing cases;
- monotonic preservation of every nonselected baseline gap identity; and
- complete replay, simulation, physical-promotion, KiCad, writer, routing,
  connectivity, and round-trip evidence for every pass.

Failure retires V6 before held-out final evidence is opened. Corpus mutation,
selection change, budget increase, retry, tuning, or gate relaxation after a
public result is prohibited.

## 8. One-time blind final

Only after public admission may an isolated authorized custodian open the V6
held-out source and baseline exactly once using three distinct external 0600
keys. Success requires strict total held-out pass improvement, at least one
newly passing held-out case whose complete baseline root-member set was covered
by the selected bundle, exact case-set preservation, no pass regression or
unsafe-to-pass transition, exact selected-member removal only, and preservation
of all nonselected gaps for still-nonpassing cases.

The held-out causal result is disclosed only as a boolean. Success publishes
encrypted evidence and permitted aggregates atomically. Failure publishes only
a non-revealing permanent audit. Either result consumes V6 and permanently
retires all update modes.

GitHub Actions are not manually started or inspected. Local evidence is the
authoritative development gate.
