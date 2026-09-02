# Generic Analysis, Model, and Solver Admission Specification

Status: implementation target

## 1. Purpose

KiCadAI must decide, before bounded topology search or physical generation,
whether every required electrical analysis can be executed with an exact,
reviewed model set and an available deterministic solver. Admission is an
evidence-producing trust boundary, not an optimistic capability hint.

This milestone is selected from the highest-impact public V18 failure family
recorded in `V19_CAPABILITY_SELECTION.md`: nine of the 23 nonpassing public
cases expose one or more of `dc_operating_point_model`,
`dc_operating_point_solver`, `ac_sweep_solver`, `dc_sweep_solver`,
`transient_solver`, or `stability_solver`.

V19 itself is permanently retired and its evaluator, report, manifests, and
versioned implementation remain immutable historical evidence. The new
admission behavior is a successor capability. Evaluation reuses the immutable
24-case public corpus and the frozen case order, limits, and two-replay
discipline; it must not replace, relabel, or mutate a V19 artifact.

## 2. Trust boundary

Provider-authored requirements may name only registered behavioral analyses.
They may not supply model identifiers, model text, equations, matrices,
executables, include paths, solver identifiers, tolerances, iteration limits,
or source priority.

Trusted model sources are:

1. the embedded reviewed provenance registry;
2. a reviewed project component overlay applied through the existing bounded
   onboarding contract; and
3. an explicitly configured reviewed overlay using the same contract.

Every source is strict-decoded, normalized, bounded, and content-addressed.
Records may refer only to compiled trusted model definitions whose canonical
content hash matches the record. Two sources that claim the same catalog/model
identity must be byte-equivalent after normalization or admission fails.
Source ordering is canonical and may not silently change a selected model.

## 3. Canonical analysis request

Admission derives the unique sorted analysis set from behavioral assertions.
Aliases are normalized only through a reviewed, versioned mapping. In
particular, `dc_sweep` is a DC operating-point workflow with an explicit sweep
contract; it is not a separate numerical solver.

For every required analysis the request records:

- the authored analysis;
- the canonical workflow analysis;
- the assertions and operating cases that require it;
- whether a sweep, periodic excitation, thermal boundary, or small-signal
  operating point is required; and
- the required component-model dependency analyses.

Unknown, empty, contradictory, or structurally incomplete analysis requests
fail before search.

## 4. Solver registry

Solver availability comes from a compiled, immutable registry. Each entry has
a stable solver ID, implementation version, canonical analysis kind, supported
workflow model IDs, deterministic flag, and a canonical content digest.

An entry is admissible only when:

- its analysis is registered;
- the numerical evaluator is implemented;
- the workflow model declares executable support for the analysis;
- required analysis-shape fields can be derived from the behavioral contract;
- every selected primitive model is compatible with the workflow; and
- the solver is enabled in the executing environment.

Environment configuration may disable a solver but may not add a new solver,
change its meaning, or override compatibility.

## 5. Model selection

Requirement-level admission proves that at least one reviewed inventory path
can satisfy each analysis. Candidate-level admission resolves every connected
component to exactly one compatible primitive model before a simulation plan
is built.

Selection is deterministic by canonical catalog ID, model ID, provenance
digest, and source digest. An ambiguity is a failure, not an invitation to pick
the first record. No fallback may substitute a different model after a solver
or simulation failure.

For every admitted component the evidence records:

- component and catalog identity;
- family and selected primitive model ID;
- exact normalized model parameters and their SHA-256 digest;
- provenance source, revision, review status, applicability bounds, and model
  content SHA-256;
- registry-source identity, kind, and content SHA-256;
- required dependency analyses; and
- a stable compatibility decision and reason.

## 6. Admission result and diagnostics

The canonical result contains the normalized request, selected solver for each
analysis, selected model evidence when candidate connectivity is known,
rejected claims with non-secret reasons, environment identity, and a hash over
the complete result excluding only the hash field itself.

The result status is `admitted` or `refused`. A refusal contains one or more
canonically sorted diagnostics using only these stable categories:

- `MISSING_MODEL`
- `INCOMPATIBLE_MODEL`
- `MISSING_ANALYSIS_DEFINITION`
- `UNSUPPORTED_ANALYSIS`
- `SOLVER_UNAVAILABLE`
- `SOLVER_MODEL_INCOMPATIBLE`
- `INVALID_MODEL_PARAMETERS`

Diagnostics include a stable code, path, analysis, component when applicable,
message, and actionable suggestion. Classification must use typed causes; it
must not infer a category by matching human-readable message substrings.

## 7. Integration

The public open-topology CLI runs requirement-level admission after strict
requirement, catalog, overlay, provenance, and inventory validation, but before
topology search. A refusal writes the complete admission evidence and does not
start synthesis or create a physical project.

Candidate simulation runs candidate-level admission after graph construction
and harness resolution, but before simulation-plan resolution or numerical
execution. Admission evidence is included in the simulation attempt and its
hash. Legacy versioned evaluators remain byte-identical unless an explicit
successor constructor opts into the new contract.

## 8. Determinism and tamper behavior

Identical requirement, catalog, overlay, registry, solver environment, and
component evidence must produce byte-identical admission JSON and hashes,
independent of source, record, component, assertion, or analysis input order.

Changing any model bytes, provenance field, parameter, applicability bound,
solver entry, enabled-solver set, analysis dependency, or source identity must
change the evidence hash or fail validation. Missing or malformed evidence
always fails closed.

## 9. Evaluation and preservation

Public evaluation must:

1. authenticate the immutable corpus and historical V18/V19 evidence;
2. execute all 24 discovery cases in manifest order exactly twice through the
   successor constructor under one committed environment;
3. compare the nine selected public cases against their complete historical
   typed frontiers;
4. report aggregate outcomes and exact typed transitions without claiming a
   held-out or arbitrary-circuit result;
5. preserve the V18 admitted pass and every historical unsafe result; and
6. require normal two-run installed-KiCad promotion for every new pass.

The milestone materially improves the selected family only if at least one
selected case passes or every selected generic availability leaf it touches is
replaced by a strictly later, more specific typed blocker. Merely renaming
`SIMULATION_INVALID` does not count.

## 10. Prohibitions

The implementation may not contain corpus or requirement IDs, fixture names,
coordinates, expected outcomes, circuit-family templates, component
allowlists, private primitive lists, special-case schemas, hidden solver
fallbacks, model substitutions, relaxed safety gates, or raised frozen search
limits.
