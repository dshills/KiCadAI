# Generic Multi-Branch Analog Topology Synthesis Specification

Status: complete
Date: 2026-08-03
Frozen baseline: `8f6ac90426b308b69cfcccbda9146be9bf6cc5f0`

## Objective

Extend primitive-only open-topology synthesis so behavior-only requirements can
produce and repair analog circuits containing multiple feedback, reference,
threshold, and output-combination branches. The milestone closes the original
eight-case benchmark at 8/8 and proves that the new behavior generalizes to an
independently frozen neutral corpus.

## Baseline

The frozen baseline deterministically passed six of the eight original
held-out cases. Adjustable voltage regulation and voltage-window monitoring now
pass. Adjustable current output and hysteretic detection exhaust because their
best candidates narrowly miss transfer or threshold assertions. No selected
candidate currently records a graph-changing electrical repair.

That historical baseline is evidence, not an acceptance relaxation. The existing assertion
bounds, safety constraints, simulation trust rules, work budgets, and physical
gates remain authoritative.

## Required Capability

### Topology invention

The search shall derive from port roles, operating cases, behavioral metrics,
primitive terminal contracts, ratings, and trusted model evidence. It shall be
able to construct:

- feedback dividers and polarity-correct control loops;
- stable reference branches;
- upper and lower threshold branches;
- multiple decision stages with deterministic output combination;
- high-side controlled-current paths selected by compliance, thermal, and
  safe-operating-area evidence rather than a named device family.

### Value derivation

Analytic seeds shall use the selected primitive model's bounded behavior where
that behavior materially affects the equations. In particular, hysteresis
sizing shall use the modeled output swing and high-side pass sizing shall
account for terminal roles and available compliance. Analytic values remain
advisory; trusted simulation over every required corner is the only pass
authority.

### Graph-changing repair

Diagnosis-driven repair shall support bounded deterministic operations that can:

- add a passive branch between relevant nodes;
- redirect a branch endpoint;
- split an existing two-terminal branch with a new internal node and insert a
  compatible primitive in series;
- combine compatible anonymous branch nodes;
- substitute a terminal-compatible primitive.

Repair endpoints shall come from the diagnosed source-observation cone and
semantic graph roles. Proposal order, identifiers, graph deltas, consumption,
and replay shall be deterministic. Every selected repair shall rerun graph
validation, value enumeration, and all required analyses.

## Independent Generalization Corpus

The two behavior-only requirements under
`internal/opentopologysynthesis/testdata/multi_branch_analog_corpus` are frozen
against the pre-implementation commit:

- an outside-window supply guard with thresholds distinct from the original
  voltage-window case;
- a 3.3 V regulated rail with independent line, load, startup, stability,
  thermal, and safe-operating-area bounds.

The requirement files may describe only behavior, external interfaces,
operating conditions, measurable bounds, safety limits, and acceptance gates.
They shall contain no primitive identity, topology, internal node, component
value, geometry, routing, provider, simulation-model, or repair instruction.

## Acceptance

1. All eight original frozen held-out requirements pass trusted simulation at
   every required corner; the promotion test requires exactly 8/8 rather than
   the former 6/8 floor.
2. Adjustable voltage regulation, voltage-window monitoring, adjustable current
   output, and hysteretic detection all pass without changing their frozen
   requirement files or assertion bounds.
3. At least one selected open-topology result contains a real graph-changing
   repair with preserved before/after hashes and byte-identical replay.
4. Both independent neutral cases pass and produce materially multi-branch
   primitive graphs without implementation leakage.
5. DC accuracy, line/load behavior, startup, stability, thresholds, hysteresis,
   transient response, thermal limits, safe operating area, and declared fault
   behavior are evaluated wherever required by each requirement.
6. The affected open-topology, architecture-generalization, simulation,
   lowering, routing, writer, and round-trip regressions remain green locally,
   and a sharded repository-wide run introduces no failure relative to the
   frozen baseline. Any unrelated baseline failure must be reproduced from the
   frozen commit and recorded rather than hidden.
7. Passing promotion designs complete two clean installed-KiCad runs with clean
   ERC, strict DRC, complete routing/connectivity, writer correctness, zero
   normalized round-trip diffs, and identical retained evidence.
8. A production-code scan finds no held-out or neutral case IDs, fixture names,
   physical coordinates, hardcoded target values, block-family substitutions,
   or case-specific repair rules.
9. The roadmap and milestone audit accurately describe the final result.
10. The staged diff receives Prism review and all actionable findings are
    remediated before commit and push.

## Non-Goals

- weakening a behavioral assertion, safety limit, strict DRC rule, or trust
  gate;
- adding a named compact block in place of primitive topology invention;
- teaching the search a fixture-specific circuit or coordinate;
- claiming arbitrary analog design outside the proven corpus;
- using GitHub Actions as the primary test runner for this milestone.
