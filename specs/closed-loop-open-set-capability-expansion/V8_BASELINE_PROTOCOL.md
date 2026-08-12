# V8 Baseline and Adaptive Evaluation Protocol

## 1. Preconditions

The evaluator begins only from a clean committed tree containing:

- the V8 contract hash manifest and passing verifier;
- one authenticated fresh corpus manifest with exactly 18 discovery and 18
  encrypted held-out records;
- public obligation-anchor commitments for all discovery assertions and only
  aggregate commitments for held-out assertions;
- frozen evaluator, gap registry, selector, synthesis policy, toolchain,
  inventory, catalog, model registry, KiCad promotion environment, seeds,
  worker count, locale, timezone, floating-point mode, and resource ceilings;
- absent generation-zero, retirement, and final output paths; and
- the committed V7 retirement object with `held_out_opened=false`.

Any missing, extra, mismatched, mutable, or preexisting terminal artifact fails
before synthesis. The evaluator never opens V7 held-out material.

## 2. Deterministic generation-zero discovery baseline

Cases run in manifest order with outcome-affecting subprocess concurrency one.
Each discovery case runs twice in separate clean roots. Complete exports,
normalized diagnostics, candidate inventories, root frontiers, and result hashes
must be byte-identical. Every pass then receives two installed-KiCad promotions
from separate clean roots.

Promotion requires clean ERC, strict DRC, exact connectivity, route completion,
writer correctness, all required simulations/assertions, safety evidence, and
zero read/write/read round-trip diffs. A pass without complete promotion is
invalid, not downgraded.

The baseline publishes one canonical evidence record per discovery case plus a
self-hashed aggregate report. Each case is exactly one of:

- `pass`: all 14 gates and both promotions pass;
- `unsupported`: a required generic capability has no admitted implementation;
- `unsafe`: a critical safety assertion fails or required safety evidence is
  absent; or
- `exhausted`: bounded generic search completes without a valid passing design.

Unknown, partial, timeout-as-outcome, skipped, or conflicting classifications
invalidate the baseline.

## 3. Complete typed root frontiers

Every nonpassing eligible case exposes a nonempty normalized root frontier.
Every gap leaf belongs to exactly one category:

- `topology`;
- `component`;
- `model`;
- `simulation`;
- `physical_design`; or
- `verification`.

Each leaf contains the immutable obligation anchor, active exact member,
append-only causal path, path hash, sorted unique evidence requirements, and
normalized diagnostic references. Generation-zero paths contain exactly their
root leaf. Leaves sort by obligation anchor, path hash, canonical member key,
and evidence. Duplicate leaves or contradictory leaves for the same exact path
invalidate the baseline.

The evaluator independently recomputes every public obligation anchor from the
frozen manifest and behavior IDs. The failure classifier cannot alter anchors
or include outcome/capability fields in them.

## 4. Clustering, candidate closure, and ranking

Gap atoms are `(category, scope, capability)`. Exact members add `(stage,
code)`. Clusters contain only eligible discovery cases and publish complete
case/domain/role/safety support. Unsafe cases remain in regression scope but do
not provide unlock or ranking credit.

Candidate generation computes the complete deduplicated union closure of active
case frontiers with exact dominance pruning. With 18 discovery cases, at most
262,143 nonempty case subsets exist; the frozen 262,144 candidate ceiling
covers the complete construction without truncation. Overflow fails closed.

A pre-implementation plan adds its statically proven effect closure. Direct and
closure atoms/members are deduplicated and all count against budgets. A
candidate is eligible only when it:

- covers every active leaf for at least two eligible cases;
- covers at least two reporting domains and two circuit roles;
- uses at most three atoms and nine exact members including closure;
- contains no prior selected atom;
- has one complete executable generic plan for all members; and
- has a finite, mechanically proven effect closure with no unmapped consumer.

Rank fields, in order, are covered active cases descending, reporting domains
descending, circuit roles descending, safety weight descending, atom count
ascending, and member count ascending. The selector publishes all semantic
co-rank-one candidates and uses canonical bundle-key byte order only as a
labeled fallback among semantically equal candidates.

## 5. Implementation seal

The selected plan freezes direct members, closure members, required evidence,
path-to-member mappings, production paths, verification paths, prerequisite
consumers, static reverse edges, focused non-corpus runtime traces, catalog/model
transitions, and before/after hashes.

No corpus synthesis or outcome inspection may be used to construct the effect
closure. A path with any outcome-affecting consumer outside the selected or
closure members rejects the seal. A prerequisite with an independent separable
effect is a capability member and consumes budget; it is not hidden as plumbing.

The staged bytes pass focused and full local tests, lint, historical public
evidence projections, deterministic replay, and all installed-KiCad protected
fixtures. Prism reviews the exact staged diff. Every high/medium finding is
remediated or receives a reproducible contract-cited disposition before the
seal and implementation commit.

## 6. Single adaptive public round

Each committed implementation round is evaluated exactly once:

1. rerun all 18 discovery cases twice under the frozen environment;
2. promote every pass twice from clean roots;
3. reproduce exact case sets, anchors, aggregate evidence, and environment;
4. require every unchanged nonselected/non-closure gap to remain byte-identical;
5. require every changed gap to be a selected or frozen closure member;
6. for each removed selected/closure leaf, require either a passing obligation
   or one through four current successors with the same anchor, exact prior path
   prefix, one appended unique leaf with a different current member, same or
   higher causal stage, and equal or stronger evidence;
7. require each retained nonpassing obligation to expose a nonempty complete
   frontier;
8. require all selected leaves to disappear in every covered case;
9. require at least two advanced active cases across two domains and two roles;
10. require no pass regression or unsafe-to-pass transition; and
11. require only seal-bound environment transitions.

The causal stage order is topology, component, model, simulation,
physical_design, verification. A successor cannot move below the prior stage;
same-stage refinement is admitted only through a different appended member.
One-to-many refinement is bounded to four and each child must have a unique
current member/path. Many-to-one path rewriting, path truncation,
anchor change, zero successors for a still-failing obligation, successor
overflow, or an unknown stage retires V8.

Strict total pass uplift plus at least one newly passing active-cohort case is
final public admission. Otherwise every gate must pass, budgets must remain,
and the next frontier/ranking/plan/effect closure/selection must be atomically
committed before another implementation. Any other result is permanent public
retirement. The maximum is three rounds, nine total atoms, and 27 total members,
including closure.

## 7. Blind held-out baseline

After generation-zero public selection and before implementation, an isolated
custodian opens the V8 held-out source key, creates a distinct external 0600
baseline key, and evaluates the exact 18 held-out records under the frozen
generation-zero environment. It publishes authenticated encrypted per-case
baseline evidence and only aggregate counts/commitments that reveal no outcome
bucket, case, gap, anchor, path, diagnostic, frontier, membership, or timing
breakdown.

The implementation process cannot access the source key, baseline key,
plaintext, decrypted records, or evaluation workspace. A post-reveal failure
consumes the baseline unless an authenticated automatic checkpoint satisfies
the frozen infrastructure-only resume contract.

## 8. Public admission and blind final

Public admission binds the complete ordered round chain, all obligation/path
transitions, effect closures, selections, implementation seals, discovery
evidence, and environment commitments. It requires strict pass uplift, a newly
passing active-cohort case, exact case preservation, no regression, no
unsafe-to-pass transition, and complete physical evidence.

Only then may the isolated final custodian use distinct external 0600 source,
baseline, and result keys for one logical blind evaluation. Success requires:

- strict held-out pass-count uplift;
- at least one newly passing held-out case whose hidden obligation/path is
  covered by the public selected chain;
- exact case-set preservation;
- no baseline pass regression;
- no unsafe-to-pass transition;
- deterministic replay; and
- complete installed-KiCad promotion for every final pass.

The repository receives encrypted final per-case evidence plus non-revealing
aggregate admission/retirement artifacts only. No held-out detail is printed,
returned, logged, reviewed by Prism, or made available to the implementation
context.

## 9. Atomic publication and terminal behavior

Every baseline, round, retirement, and final is written to a fresh sibling
temporary directory, fsynced where supported, checksum-verified, atomically
renamed without replacement, and then verified read-only. Success and retirement
paths are mutually exclusive. A preexisting output is never overwritten.

There is no corpus mutation, retry after a consumed round, selection override,
manual expected-result edit, budget increase, gate relaxation, fixture-specific
implementation, held-out disclosure, or manual GitHub Actions run. Any ambiguity
fails closed into the protocol-defined terminal artifact.
