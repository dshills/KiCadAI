# V19 Generic Causal-Topology Repair Plan

This plan implements `V19_SPEC_ADDENDUM.md` in version-isolated phases. The
current goal ends after the design freeze; the phases below require a separate
implementation authorization.

## Phase 0: Freeze and authentication

1. Review the design and capability-selection report with Prism.
2. Authenticate the V18 contract, evaluator, public corpus, generation-one
   report, and evaluated/publication commits.
3. Record V19 contract and evaluator manifests before any public run.
4. Prove the working tree is clean and historical V18 bytes match their seals.

Exit: the design and all inherited inputs are immutable and independently
checkable.

## Phase 1: Directed graph semantics and invariants

1. Add V19-only derivation of directed causal edges from inventory terminal
   electrical/function roles.
2. Add validators for typed feedback cycles, active-output contention, domain
   and rating compatibility, reference closure, and registry/model provenance.
3. Add positive and negative synthetic tests for every invariant.
4. Add permutation/property tests proving input order cannot affect results.

Exit: invalid graphs are rejected deterministically before simulation, while
valid feedback and safe input fan-out remain representable.

## Phase 2: Atomic reusable operations

1. Implement role-complete stage insertion using the existing connection-map
   evidence shape.
2. Implement independent observation-cone allocation.
3. Implement role-aware terminal redirection.
4. Implement typed passive feedback insertion.
5. Compose existing value/polarity/passive operations with at most one new
   operation per two-change proposal.
6. Prove all candidate primitives come only from the supplied reviewed
   inventory and required-analysis support.

Exit: all five semantic synthetic fixtures can generate at least one complete,
invariant-valid candidate without production fixture dispatch.

## Phase 3: Deterministic compositional beam

1. Add the V19-only depth-four, width-eight diagnosis-driven beam.
2. Implement the monotonic expansion rule and canonical ordering tuples.
3. Charge every generation, repair, simulation, corner, and value action to the
   unchanged policy counters.
4. Add global graph-hash deduplication and replayable operation histories.
5. Add three-change composition, more-than-48-child truncation, maximum-size,
   and fixed-seed permutation tests.

Exit: the search composes at least three independent changes across multiple
proposals while never exceeding proposal width two, depth four, 48 evaluated
causal trials, beam width eight, or any inherited run-wide limit.

## Phase 4: Versioned synthesis and executor integration

1. Add `SynthesizeV19WithLegacy` using the exact V18 path first.
2. Gate V19 only on the typed `causal_topology_repair` frontier predicate.
3. Add V19 executor and runner adapters with separately bound V19 and exact V18
   environments.
4. Add byte-for-byte V18 delegation, unsafe-terminal, exhaustion, cancellation,
   and deterministic-replay tests.

Exit: all V18-ineligible results are byte-identical and eligible failures can
use only the bounded V19 extension.

## Phase 5: Local verification and evaluator freeze

1. Run focused operation, invariant, beam, executor, and runner tests.
2. Run the complete local Go suite and historical seal checks.
3. Build the V19 public runner and freeze its environment and manifests.
4. Run all deterministic, resource, fail-closed, and clean-root promotion
   harness self-tests without evaluating the public corpus.
5. Review the staged implementation with Prism and remediate valid findings.

Exit: production code is locally green and the evaluator is authenticated
before it can observe public outcomes.

## Phase 6: One bounded public evaluation

1. From a clean committed worktree, authenticate the exact 24-case V18 public
   boundary and committed V19 evaluator.
2. Evaluate all 24 cases exactly twice.
3. For every pass, perform two clean-root installed-KiCad promotions covering
   ERC, strict DRC, connectivity, route completion, writer correctness, and
   zero round-trip diffs.
4. Apply the five-of-five advancement gate, two-pass/two-domain minimum,
   preservation gates, and exact replay checks.
5. Publish either admission evidence or a fail-closed retirement report.

Exit: V19 is admitted only when every frozen gate passes. One bounded
implementation correction run is permitted; otherwise retire the capability
without corpus churn or limit changes.

## Implementation estimate and stop conditions

Expected implementation effort is 30–44 engineering hours (roughly 4–6
working days), as recorded in the capability-selection report. This is not a
single unattended tool runtime estimate.

Stop immediately if:

- a proposed fix requires case IDs, fixture names, coordinates, private
  allowlists, or circuit-family templates;
- a primitive or required analysis lacks reviewed registry evidence;
- historical V18 bytes or ineligible results would change;
- any frozen numeric limit must increase;
- a safety, invariant, fail-closed, KiCad, or round-trip gate would weaken; or
- the bounded public correction run still misses admission.

The next goal after this design freeze should implement Phases 1–4, run Phase 5
local verification, obtain Prism approval, and commit. Phase 6 remains a
separate explicit evaluation step because it observes public outcomes.
