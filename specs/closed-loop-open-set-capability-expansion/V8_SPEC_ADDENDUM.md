# Closed-Loop Open-Set Capability Expansion V8 Specification Addendum

Status: contract candidate. It becomes frozen only when the V8 contract hash
manifest and its executable verifier are committed together.

## 1. Purpose and predecessor boundary

V8 continues the closed-loop open-set capability-expansion objective with a
fresh corpus and a new protocol. It is not a retry, mutation, or continuation
of V7. V7 permanently retired at its first public round because a nonselected
topology gap disappeared without an admissible successor. Its held-out source,
baseline plaintext, outcomes, gaps, and keys were not opened.

V8 may consume the public V7 retirement reason as protocol-design evidence. It
may not inspect or reuse V7 held-out material, change any V7 artifact, reuse a
V7 source or baseline key, or treat the V7 implementation as evidence that a
V8 case passes.

## 2. Milestone objective

KiCadAI shall use a frozen, independently authored behavior-only corpus to
determine which missing generic capabilities unlock the greatest number and
diversity of circuits. It shall:

1. baseline every case as `pass`, `unsupported`, `unsafe`, or `exhausted`;
2. expose complete normalized root gaps in `topology`, `component`, `model`,
   `simulation`, `physical_design`, or `verification`;
3. cluster and rank reusable gaps using only public discovery evidence;
4. implement only the selected highest-impact generic capability bundle and
   its predeclared bounded effect closure;
5. rerun the frozen public corpus exactly once per admitted round; and
6. prove measurable public coverage improvement before one blind held-out
   final is permitted.

Existing local regressions, installed-KiCad promotion, deterministic replay,
writer correctness, strict ERC/DRC, connectivity, route completion, zero
round-trip diffs, and fail-closed behavior remain mandatory.

## 3. Fresh independent corpus

Six isolated authors each create six requirements: three discovery and three
held-out. The resulting corpus has exactly 36 unique cases, split 18/18. Every
author receives only a frozen public behavior contract, a frozen assignment,
an authorship template, and a packet checksum. Authors may not inspect KiCadAI
implementation, prior corpus source, prior outcomes, synthesis artifacts,
frontiers, rankings, or another author's work.

Requirements describe externally observable behavior and environmental limits
only. They do not prescribe parts, topology, block families, footprints,
packages, coordinates, routes, templates, or implementation hints. The frozen
assignment balances six reporting domains, six circuit roles, four safety
impact levels, analysis kinds, single/multiple outputs, and static/dynamic
behavior. Semantic-signature duplication retires the corpus candidate.

The held-out half is encrypted record-by-record and its plaintext quarantine is
removed only after authenticated publication and verification. Discovery and
held-out use distinct external 0600 source keys. Neither key may be stored in
the repository, passed in command arguments or environment variables, or
opened by the implementation process.

## 4. Immutable behavior obligations

Every assertion is authored with stable case-local semantic identifiers for
its operating case, assertion, observation, and output. The publisher derives
an immutable obligation anchor with the frozen length-prefixed encoding:

`SHA-256(corpus_manifest_hash, role, case_id, operating_case_id, assertion_id,
observation_kind, observation_id, output_id)`.

The anchor contains no selected capability, outcome, implementation detail, or
failure classification. It therefore remains stable when the blocker advances
from topology to model, simulation, physical design, or verification.
Duplicate anchors, missing referenced IDs, or an anchor mismatch retire the
corpus before synthesis.

An assertion may emit multiple concurrent root gaps. Each gap has exactly one
obligation anchor, one current typed leaf, a sorted unique required-evidence
set, and an append-only causal path. The generation-zero path contains its root
leaf. A successor path must preserve the exact prior path as a prefix and append
one new leaf. Historical leaves are evidence of causal progression, not active
members eligible for reselection.

## 5. Typed gap and effect-closure model

An active gap leaf contains:

- stage and one of the six gap categories;
- scope, generic capability, stable diagnostic code, and severity;
- obligation anchor and causal-path hash;
- sorted unique required evidence; and
- normalized supporting diagnostics that contain no identity-order rank input.

The exact member identity is the canonical tuple `(stage, category, scope,
capability, code)`. A capability atom is `(category, scope, capability)`.
Clustering never uses case names, author identities, fixture coordinates, or
held-out information.

Before implementation, the generic plan declares a conservative effect closure
for every selected atom/member. The closure is derived only from static reverse
call graphs, registries, configuration and data references, catalog/model
consumers, and focused non-corpus runtime traces. Closure atoms and members
consume the same per-round and total budgets as directly selected members.
Unbounded dynamic lookup, an unmapped outcome-affecting consumer, or an effect
closure that cannot be proven before implementation rejects the plan.

During public evaluation:

- an unchanged nonselected/non-closure gap remains byte-identical;
- a changed gap must have been in the frozen effect closure;
- a selected or closure gap may disappear only when its obligation passes, or
  may advance to one through four successors with the same obligation anchor,
  the prior causal path as an exact prefix, a different current member, a same
  or higher causal stage, and equal or stronger required evidence; and
- zero successors for a still-failing obligation, more than four successors,
  path rewriting, unknown stages, or undeclared side effects retire V8.

This admits legitimate blocker refinement and fan-out without allowing silent
loss or unrelated beneficial side effects.

## 6. Baseline and ranking

Generation zero synthesizes all 18 discovery cases twice under one frozen
environment. Every complete export must be byte-identical. Every pass is
promoted twice from clean roots through the installed KiCad version. The
baseline publishes public per-case evidence, the aggregate outcome report,
complete root frontiers, canonical clusters, semantic co-rank-one candidates,
the selected bundle, its generic plan, and self-hashed commitments.

Candidates cover complete current frontier leaves for at least two active cases
in at least two reporting domains. Prior atoms are ineligible. Direct and
effect-closure atoms/members are deduplicated before eligibility and budget
checks. Candidates rank by:

1. completely covered active cases, descending;
2. covered reporting domains, descending;
3. covered circuit roles, descending;
4. covered safety weight, descending;
5. capability atoms including closure, ascending; and
6. exact members including closure, ascending.

Canonical identity is only a labeled deterministic fallback after a complete
semantic tie. The complete semantic co-rank-one set is published.

## 7. Bounded adaptive rounds

V8 permits at most three implementation rounds, nine total atoms, and 27 total
exact members, including effect closure. Each round permits at most three atoms
and nine members. The complete discovery cohort remains in regression scope.

A round advances only when all selected current leaves disappear for every case
they cover, at least two active cases across two domains advance, every changed
gap satisfies the frozen causal-path/effect-closure rules, no prior pass
regresses, no unsafe case becomes pass, and environment bytes remain frozen
except for a selected component/model transition explicitly sealed in advance.

The first round with strict total discovery pass uplift and at least one newly
passing active-cohort case is final public admission. Continuing after that
point is prohibited. A no-op round, ambiguous transition, missing frontier,
budget excess, environment drift, selected-member persistence, unexplained
side effect, regression, or exhausted round ceiling permanently retires V8
before held-out access.

## 8. Implementation boundary

Production code may dispatch only on generic typed contracts, capability
registries, and normalized electrical semantics. Fixture identifiers, corpus
IDs, author IDs, requirement hashes, family-specific block dispatch,
coordinates, path allowlists, and expected outcomes are prohibited.

Every changed production and verification path maps to one selected or closure
member. Representative, adversarial, boundary, deterministic-replay, and
fail-closed tests are required. Full local tests, lint, installed-KiCad protected
fixtures, and the exact historical public evidence projections run before the
implementation seal. Prism reviews the exact staged bytes through its configured
external provider; every high and medium finding is remediated or receives a
contract-cited reproducible disposition before commit.

## 9. One-time blind held-out final

Only final public admission unlocks one logical held-out evaluation. A separate
least-privilege custodian validates the committed ordered chain and three
distinct external 0600 keys: held-out source, held-out baseline, and final
result. Read-only preflight completes before the first authenticated plaintext
record or baseline payload is produced.

Success requires strict held-out pass uplift, at least one newly passing case
whose immutable obligation anchor and hidden blocker path are covered by the
public selected chain, exact case-set preservation, no regression, no
unsafe-to-pass transition, deterministic replay, and complete installed-KiCad
physical evidence. Only encrypted per-case evidence and non-revealing aggregate
counts are published. No held-out case, outcome bucket, gap, path, diagnostic,
timing, or membership detail is disclosed.

## 10. Terminal states

V8 ends in exactly one committed state:

- successful encrypted held-out aggregate admission;
- public retirement before held-out access; or
- consumed blind-final retirement with only non-revealing aggregate evidence.

There is no tuning after corpus synthesis, no selection override, no budget or
gate relaxation, no fixture-specific remediation, and no manual GitHub Actions
run. Local evidence is authoritative; hosted CI may verify the committed bytes
afterward but does not drive the workflow.
