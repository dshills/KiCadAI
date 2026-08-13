# V8 Round 1 Generic Implementation Plan

## Selected capability boundary

The generation-zero public frontier nominates the exact member
`(simulation, simulation, simulation, dc_sweep_solver, SIMULATION_INVALID)`.
Its atom is `(simulation, simulation, dc_sweep_solver)`. The committed effect
plan contains no closure member: the bounded change is confined to deriving a
DC sweep source and range from already-declared semantic electrical envelopes.

The implementation must not name V8 cases, circuit families, author slots,
coordinates, block templates, or expected outcomes. It must accept only one
unambiguous external semantic excitation or load/supply target, require finite
strictly ordered bounds from the public requirement, and fail closed for
missing, conflicting, unbounded, or unsupported axes.

## Static effect proof

`sweepSourceAndRange` has one production caller, `simulationIntentParts`.
That result flows through `evaluateAssertionCorner` and `EvaluateCandidate`.
The latter is consumed by top-level synthesis, bounded causal repair, and the
cross-stage target adapter. All consumers retain the same simulation member;
none creates an independently separable topology, component, model, physical
design, or verification effect.

No registry, configuration loader, catalog/model resolver, or data file is
consulted by the range derivation. The complete source hashes, reverse edges,
focused non-corpus consumers, and explicit empty reference sets are frozen in
`V8_ROUND_1_EFFECT_PLAN.json`.

## Implementation steps

1. Introduce one generic bounded semantic-range resolver for DC sweep axes.
2. Preserve the existing precedence for explicit excitation conditions,
   bounded port voltage envelopes, line-regulation supplies, and
   load-regulation loads.
3. Extend only to already-supported external semantic targets whose declared
   condition or electrical envelope supplies a finite strict range and whose
   source mapping is unique.
4. Reject target ambiguity, axis ambiguity, absent graph mapping, nonfinite
   bounds, equal/reversed bounds, and multiple candidate sources.
5. Add focused table-driven tests for voltage, current, load, control,
   ambiguous, missing, and malformed bounds without using corpus cases.
6. Run focused and full local tests, deterministic replay, historical public
   projections, and installed-KiCad protected fixtures. Prism-review the exact
   staged implementation before committing.

## Required round evidence

The post-implementation public round must remove every selected leaf in each
covered case or append a protocol-valid same/higher-stage successor with the
same obligation anchor. It must advance at least two active cases across two
reporting domains and two circuit roles, preserve every unrelated frontier
byte-for-byte, introduce no pass or safety regression, and satisfy the frozen
round budgets. Anything else retires V8.
