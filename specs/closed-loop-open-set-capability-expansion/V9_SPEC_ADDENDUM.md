# Closed-Loop Open-Set Capability Expansion V9 Specification Addendum

Status: contract candidate. V9 becomes frozen only when these four V9 protocol
documents, `V9_CONTRACT.sha256`, and the executable V9 contract verifier are
committed together.

## 1. Purpose and predecessor boundary

V9 continues the closed-loop open-set capability-expansion objective with a
fresh corpus. It is not a retry or mutation of V8. V8 permanently retired at
generation one with `causal_lineage_invalid`, after all 18 public discovery
cases completed deterministic replay and before any held-out final access.

V9 may consume only committed public V8 evidence. It may not inspect, decrypt,
reuse, or infer from V8 held-out source or baseline plaintext; reuse a V8 key;
change a V8 artifact; or treat the V8 implementation as passing evidence.

The V8 lesson is normative: a candidate's fully covered cohort is not its full
effect surface. A case can contain a selected member beside nonselected sibling
paths. V9 therefore makes the complete effect-exposure cohort a first-class
selection, implementation, and round-gate commitment.

## 2. Milestone objective

Using a frozen independently authored behavior-only corpus, KiCadAI shall:

1. classify every case as `pass`, `unsupported`, `unsafe`, or `exhausted`;
2. expose complete normalized root frontiers in `topology`, `component`,
   `model`, `simulation`, `physical_design`, or `verification`;
3. cluster and rank reusable generic gaps using public discovery evidence only;
4. select capabilities by unlock impact and bounded collateral exposure;
5. implement only the selected generic capability and its proven closure;
6. rerun the complete frozen public corpus exactly once per committed round;
7. require measurable public coverage improvement before blind final access;
   and
8. publish enough public aggregate drift evidence to improve the next protocol
   without exposing held-out data or enabling fixture-specific tuning.

All existing local regressions, deterministic replay, fail-closed behavior,
installed-KiCad promotion, clean ERC, strict DRC, connectivity, route
completion, writer correctness, required simulation, safety evidence, and zero
round-trip diffs remain mandatory.

## 3. Fresh independent corpus

Six isolated authors each create eight requirements: four discovery and four
held-out. V9 therefore contains exactly 48 unique cases split 24/24. Authors
receive only a frozen public contract, assignment, authorship template, and
authenticated packet. They may not inspect implementation, any prior corpus
source, outcomes, synthesis evidence, frontiers, rankings, or another author's
work.

Requirements specify externally observable behavior and bounded environmental
limits only. Parts, topology, circuit families as implementation directives,
block names, footprints, packages, coordinates, routes, templates, and expected
outcomes are prohibited. The assignment balances reporting domains, circuit
roles, safety impact, static/dynamic primary analysis, off-nominal behavior,
and single/multiple outputs. Semantic duplication retires the candidate corpus.

Discovery is public after authenticated publication. Held-out requirements are
encrypted record by record and plaintext quarantine is removed only after
independent publication verification. Source, baseline, and final keys are
distinct external 0600 regular files and never enter repository bytes, command
arguments, environment variables, logs, or implementation contexts.

## 4. Stable obligations and complete frontiers

Every assertion has stable case-local operating-case, assertion, observation,
and output IDs. Its obligation anchor is the frozen length-prefixed SHA-256 of
corpus manifest hash, role, case ID, operating-case ID, assertion ID,
observation kind, observation ID, and output ID. Outcome, selected capability,
implementation detail, and failure classification are excluded.

Every nonpassing eligible obligation exposes a nonempty frontier. Each active
gap has one anchor, one current typed member, a sorted unique evidence set,
normalized diagnostics, and an append-only causal path. Generation zero has a
one-leaf path. A successor preserves the exact prior path and appends exactly
one different same-or-higher-stage member. Historical leaves cannot be
reselected.

## 5. Effect-exposure cohort

For a candidate, the evaluator derives two distinct public sets:

- `fully_covered_case_ids`: cases whose complete active frontier is covered by
  the candidate and declared closure; and
- `effect_exposure_case_ids`: every case containing any direct or closure atom
  or member anywhere in its active frontier, including partially covered cases.

The exposure set is derived mechanically from the committed frontier and must
be a superset of the fully covered set. For every exposed case the selection
publishes selected path hashes and nonselected sibling path hashes. Missing,
extra, duplicated, or ambiguous exposure membership invalidates selection.

The implementation seal maps every outcome-affecting production and
verification path to a direct or closure member and binds the exposure cohort.
Unbounded dynamic lookup, an unmapped consumer, a hidden prerequisite effect,
or an implementation path that can alter a nonselected sibling rejects the
seal before implementation.

## 6. Ranking and selection

Eligible candidates completely cover at least two active cases across at least
two reporting domains and two circuit roles. Unsafe cases remain regression
sentinels but receive no unlock credit. Prior selected atoms are ineligible.

Candidates rank by the following semantic tuple:

1. fully covered active cases, descending;
2. covered reporting domains, descending;
3. covered circuit roles, descending;
4. covered safety weight, descending;
5. exposed noncovered cases, ascending;
6. nonselected sibling paths in the exposure cohort, ascending;
7. direct plus closure atoms, ascending; and
8. direct plus closure exact members, ascending.

Canonical identity is only a labeled deterministic fallback after equality of
the complete semantic tuple. The complete co-rank-one set, exposure cohort,
and sibling commitments are public.

## 7. Bounded public rounds

V9 permits at most two implementation rounds, six total atoms, and 18 total
exact members, including closure. Each round permits at most three atoms and
nine exact members. Selection, effect plan, exposure cohort, sibling
commitments, implementation plan, and runner are committed before code changes.

A round advances only when:

- every direct/closure leaf in every exposed case disappears or advances by an
  admissible same-anchor successor;
- every committed nonselected sibling remains byte-identical;
- every non-exposed case remains byte-identical unless it was already a pass;
- at least two active cases across two domains and two roles advance;
- no pass regresses and no unsafe case becomes pass;
- replay, physical promotion, effect closure, and environment seals pass; and
- the round stays inside all budgets.

Strict total discovery pass uplift plus at least one newly passing active-cohort
case is final public admission. A no-op, sibling drift, exposure omission,
ambiguous successor, regression, budget excess, environment drift, or round
ceiling retires V9. Gates are never relaxed after observation.

## 8. Implementation boundary

Production behavior may dispatch only on generic typed contracts, normalized
electrical semantics, and admitted capability registries. Corpus IDs, author
IDs, requirement hashes, fixture identifiers, family-specific dispatch,
coordinates, allowlists, expected outcomes, and hidden templates are prohibited.

Representative, adversarial, boundary, deterministic, and fail-closed tests
are required. Outcome-neutral exposure tests must prove generic dispatch and
sibling preservation without synthesizing the V9 corpus or inspecting V9
outcomes. Full local tests, lint, historical public regressions, installed-KiCad
protected fixtures, and Prism review of exact staged bytes precede each seal.

## 9. Public retirement diagnosis

A public retirement artifact contains only frozen-enum reason, generation,
input commitments, implementation commitment, exposure cohort size, and
aggregate drift counts by `selected_path`, `nonselected_sibling`,
`non_exposed_case`, `regression`, `environment`, or `budget`. Public case IDs or
paths may be included only when the V9 contract explicitly marks them public;
held-out identity, outcome, path, diagnostic, timing, and membership never are.

Retirement evidence is diagnostic input for a later fresh protocol, not
permission to retry, tune, or mutate V9.

## 10. Blind final and terminal states

Only public admission permits one isolated blind held-out final. Success
requires strict held-out pass uplift, at least one new pass covered by the
public selected chain, exact case preservation, no pass regression, no
unsafe-to-pass transition, deterministic replay, and complete installed-KiCad
promotion. Only encrypted per-case evidence and non-revealing aggregate counts
are published.

V9 ends in exactly one committed state: successful encrypted admission, public
retirement before final access, or consumed blind-final retirement. There is no
corpus mutation, selection override, post-result retry, fixture-specific fix,
gate weakening, or manual GitHub Actions run.
