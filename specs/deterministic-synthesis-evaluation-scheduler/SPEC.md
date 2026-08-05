# Deterministic Synthesis-Evaluation Scheduler

## Goal

Bound the cost of evaluating a growing architecture search without weakening
promotion evidence. The scheduler must reject inexpensive failures before
expensive analyses, reuse only byte-identical trusted work, and make its
ordering, budgets, cache decisions, and dominance decisions replayable.

## Trust Boundary

The scheduler consumes only repository-owned typed requirements, resolved
simulation plans, catalog/model provenance, and workflow stages. A provider
cannot supply stage order, cache keys, budgets, dominance rules, solver
controls, corner subsets, physical coordinates, or promotion exceptions.

## Required Stage Order

Every candidate attempt uses this monotonic order:

1. `structural` — resolution, plan validation, model provenance, and required
   assertion/link coverage;
2. `dc` — DC operating point and DC sweep work;
3. `ac` — AC sweep, noise, and stability work;
4. `transient` — transient, startup, and distortion work;
5. `thermal_soa` — thermal and coupled electrothermal/SOA work;
6. `exhaustive_promotion` — confirmation that every required plan, analysis,
   operating case, assertion, and worst-case corner completed;
7. `physical_lowering` — the existing schematic, placement, routing,
   connectivity, writer, validation, and round-trip pipeline; and
8. `kicad_verification` — installed-KiCad ERC and strict DRC.

The simulation scheduler may stop after the first failed electrical stage for
a rejected or repairable attempt. A selected candidate must reach
`exhaustive_promotion`; the existing workflow must then reach the required
physical and KiCad stages before promotion.

## Determinism

- Candidates, states, plans, analyses, and evidence are canonically ordered.
- Concurrency may change wall time but not selection, rejection, accounting,
  hashes, persisted evidence, or artifacts.
- Cache keys are SHA-256 hashes of the complete typed resolved plan. Cache
  reuse is allowed only for an exact key match and returns defensive copies.
- Cache capacity and eviction are deterministic and bounded.
- Fail-fast results retain the failed stage, evaluated plan hashes, assertion
  evidence, unevaluated stage list, and reason.

## Budgets

The policy must separately bound:

- candidates and repairs;
- total candidate-state evaluations;
- candidate-state evaluations per candidate;
- total trusted analysis executions;
- trusted executions per analysis kind; and
- plans admitted by one candidate-state evaluation.

Cache hits are recorded but do not consume trusted-execution budget. Budget
exhaustion is a stable fail-closed result and can never be promoted.

## Conservative Dominance

Dominance may remove a candidate from finalist consideration only when another
fully evaluated candidate is no worse in every persisted electrical and
static ranking dimension and strictly better in at least one. The report must
name the dominating candidate, enumerate the compared dimensions, and retain
both candidates' complete evaluation evidence. No unevaluated electrical
property may be inferred from a static score.

## Exhaustive Corners

Staged execution may defer expensive analyses but may not reduce their
registered worst-case corners. The selected candidate's final attempt must
contain a complete replayable transcript for every resolved plan and required
corner. Partial attempts cannot satisfy the promotion validator.

## Physical And KiCad Integration

Only the unique, exhaustively verified electrical finalist enters the existing
physical workflow. That workflow remains fail-closed and ordered: schematic
electrical checks precede PCB realization, placement precedes routing, routing
precedes writing and writer correctness, validation includes connectivity and
round-trip checks, and installed-KiCad ERC/strict DRC is last.

## Acceptance

1. Unit tests prove stage order, fail-fast behavior, exact-key cache reuse,
   cache isolation, every budget boundary, conservative dominance, exhaustive
   finalist verification, cancellation, and concurrency-independent bytes.
2. `go test ./... -timeout 20m` passes locally on the reference machine.
3. The protected current-output, protected voltage-output, architecture-
   generalization, and previously promoted synthesis corpora retain their
   required deterministic and installed-KiCad gates.
4. Selected reports replay from a cold process and have zero normalized
   round-trip differences.
5. Production code contains no fixture identities, fixture coordinates,
   allowlists, schemas, or block-family special cases.

## Explicit Non-Goals

This milestone does not add new device physics, architecture families,
unreviewed models, RF/high-speed qualification, mains safety, or fabrication
approval. Nonlinear and switching architecture expansion follows this work.
