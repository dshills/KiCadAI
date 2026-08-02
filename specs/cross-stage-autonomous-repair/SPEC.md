# End-to-End Cross-Stage Autonomous Repair Specification

## Objective

KiCadAI must diagnose, select, apply, validate, and sequence the smallest safe
repair across electrical simulation, schematic/ERC, placement, routing,
connectivity, DRC, writer, and round-trip failures without circuit identity or
human-authored case logic.

The coordinator unifies existing electrical, physical, and persisted-file
repair mechanisms. It does not replace their domain-specific proposal or
application logic. It owns ordering, budgets, checkpoints, rollback,
stage re-entry, affected-scope preservation, confirmation, and evidence.

## Frozen Evaluation

Before production behavior changes, freeze an independently authored corpus
with at least one failure rooted in each of:

1. trusted simulation;
2. schematic construction;
3. ERC;
4. placement;
5. routing;
6. connectivity;
7. DRC;
8. writer semantics; and
9. KiCad round trip.

Corpus files may describe test-only fault injection, but production code may
not contain a corpus ID, path, expected result, component reference, exact
fixture value, coordinate, allowlist, or case-specific branch.

## Structured Contract

Every diagnostic must contain a workflow stage, stable code, category,
severity, evidence hash, and affected semantic scope. Free-form messages may be
reported but cannot authorize a repair.

Every proposal must contain:

- the diagnostic hash it addresses;
- a registered operator;
- every affected stage and semantic scope;
- the earliest stage that must be re-entered;
- expected measurable effects;
- bounded change count and normalized magnitude;
- authorization or a stable rejection reason.

The declared re-entry stage must equal the earliest affected stage. A proposal
that omits affected stages, targets an unknown stage, or attempts to resume
after an affected stage fails closed.

## Deterministic Coordination

At each iteration the coordinator:

1. captures a restorable checkpoint;
2. normalizes diagnostics and selects the earliest blocked workflow stage;
3. gathers authorized proposals for that stage;
4. evaluates candidates in deterministic order under global and per-diagnostic
   budgets;
5. restores the checkpoint after every trial;
6. rejects candidates that fail validation, preserve the original diagnostic,
   alter unrelated scope, or regress any protected gate or safety margin;
7. ranks remaining candidates by resolved blocking evidence, passed gates,
   change count, normalized magnitude, re-entry stage, and stable proposal ID;
8. reapplies and confirms the winner; and
9. repeats until all required gates pass or no authorized improvement remains.

Repeated proposal/state pairs are never evaluated twice. Context cancellation,
budget exhaustion, restore failure, nondeterministic confirmation, missing
evidence, and unsupported diagnostics fail closed.

## Transaction And Preservation Rules

The target must expose byte-restorable checkpoints. Candidate state is never
committed before confirmation. A rejected or failed candidate must leave the
target at the exact checkpoint hash.

Each checkpoint includes hashes for protected semantic scopes. Every scope not
declared by the proposal must remain byte-identical. Domain adapters may add
stronger invariants, including:

- unrelated schematic symbols, fields, wires, labels, and hierarchy;
- unrelated placement and mechanical constraints;
- unrelated route operations, vias, zones, and net classes;
- selected parts, pin maps, rating evidence, and simulation models;
- project metadata and user-owned content.

## Regression Rules

A candidate is rejected when it:

- changes a previously passing required gate to warning, blocked, or missing;
- introduces a new blocking diagnostic;
- reduces a declared safety, thermal, SOA, or operating-corner margin beyond
  the target's deterministic tolerance;
- removes a required gate or margin;
- changes protected unrelated scope; or
- cannot reproduce the same candidate and result hashes during confirmation.

Improving one assertion is insufficient. The complete required gate vector and
all protected margins participate in acceptance.

## Evidence And Replay

The report schema is `kicadai.cross-stage-autonomous-repair.v1`. It records:

- policy and budget consumption;
- normalized diagnostics and proposals;
- every checkpoint, candidate, restore, rejection, selection, and confirmation;
- affected and preserved scope hashes;
- before/after gate and margin vectors;
- stage re-entry decisions;
- final stop reason and state hash; and
- a canonical content hash.

Two runs from the same input, policy, catalogs, models, and toolchain must
produce byte-identical reports and generated projects.

## Acceptance

- The frozen corpus remains byte-identical after production work begins.
- At least one unseen failure in every frozen stage recovers through the same
  production coordinator without corpus identity.
- Mixed-stage cases prove earliest-stage ordering and prevent a later repair
  from being invalidated by an earlier one.
- Negative cases prove rollback for electrical, thermal, SOA, corner, physical,
  unrelated-scope, and nondeterministic regressions.
- Existing electrical causal repair, physical correction, validation repair,
  and unsupported outcomes remain passing and fail closed where appropriate.
- Every recoverable physical case passes connectivity, writer correctness,
  installed-KiCad ERC, strict DRC, and zero-difference round trip locally.
- The complete local lane fits its documented timeout and is not delegated to
  GitHub Actions.
- Project status and roadmap reflect the verified result.
- Prism reviews the staged diff, actionable findings are remediated, and the
  milestone is committed.
