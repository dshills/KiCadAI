# V9 Effect-Exposure and Causal-Round Engine

Status: implementation prepared with synthetic outcome-neutral tests; real V9
corpus frontiers have not been loaded or evaluated.

## Purpose

V8 treated the cases fully covered by a candidate as though they were the
candidate's complete effect surface. That was insufficient when the same
generic atom also appeared on one path of a partially covered case beside an
unselected sibling. V9 separates unlock coverage from effect exposure and
commits both before implementation.

## Frozen policy

The version-isolated V9 engine requires exactly 24 discovery cases and the six
canonical gap stages/categories. It permits at most two rounds, six cumulative
atoms, 18 cumulative exact members, three atoms and nine exact members per
round, and four causal successors per selected path. Candidate support and
advancement each require at least two cases, two reporting domains, and two
circuit roles.

Complete candidate enumeration is bounded at all `2^24` possible active-case
subsets. Overflow fails closed; the engine does not switch to sampling or a
heuristic. Frozen-policy equality is exact.

## Selection evidence

For every eligible candidate, the engine derives:

- `fully_covered_case_ids`, where the entire active frontier is covered by the
  candidate's direct plus mechanically proven closure members;
- `effect_exposure_case_ids`, where any current path leaf shares a direct or
  closure atom or exact member;
- selected path hashes and every nonselected sibling path hash for each exposed
  case; and
- a canonical complete-case hash for every non-exposed case.

The exposure set must be a superset of the fully covered set. Atom exposure is
recognized even when an exposed path has a different exact diagnostic code.
Closure members count toward full coverage. All 24 cases appear in exactly one
of the exposure or non-exposed commitments.

Ranking uses full unlock count, reporting-domain diversity, circuit-role
diversity, and safety weight descending, followed by exposed-but-not-covered
case count, sibling burden, atom count, and exact-member count ascending.
Canonical candidate identity breaks only a complete semantic tie. V9 does not
apply V8's pre-ranking dominance pruning because it could discard a candidate
whose committed collateral exposure is better.

## Exactly-once round gates

Before comparing a round, the engine mechanically rederives the selected
candidate's full coverage, exposure, selected paths, sibling paths, non-exposed
hashes, diversity, burden, and identity from the prior frontier. Any omission,
addition, or substitution fails.

For every exposed case:

- every selected path must disappear because its obligation is satisfied or
  advance to one through four append-only same-anchor successors;
- successors append exactly one different same-or-higher-stage member with
  nonweaker evidence;
- every nonselected sibling path remains structurally identical; and
- no unexplained new path is admitted.

Every non-exposed case must reproduce its complete canonical case hash,
including metadata, outcome, satisfied obligations, path identities, evidence,
and diagnostics. Prior satisfied obligations cannot regress. Passing cases
cannot regress, unsafe cases cannot become passes, and all case metadata and
case membership remain fixed.

Public admission still requires strict total pass uplift and a newly passing
member of the original active cohort. Otherwise continuation requires diverse
advancement and remaining round/cumulative budget. Exhausting the frozen budget
without admission fails closed.

## Preparation evidence

Synthetic tests cover closure-aware full coverage, atom-level partial exposure,
collateral-exposure ranking, selected-path advancement, sibling preservation,
non-exposed drift, exposure omission, append-only successors, bounded fan-out,
stage monotonicity, pass/unsafe regression, satisfied-obligation preservation,
canonical hashing, input-order determinism, budget enforcement, and exact
frozen-policy rejection.

The package imports no corpus publication, synthesis, evaluation, or held-out
key path. It consumes only typed frontier values supplied by the later frozen
public evaluator.
