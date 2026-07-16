# Catalog-backed nonlinear DC operating-point analysis

## Objective

Extend graph-derived trusted simulation with bounded DC operating-point analysis for reviewed diode, NPN BJT, and PNP BJT catalog primitives. The circuit graph supplies connectivity and bounded source values only. Device equations, parameters, iteration policy, limits, and failure behavior remain checked-in trusted data and code.

## Requirements

1. Add a distinct `nonlinear_circuit_dc_v1` workflow. It accepts only DC operating-point analyses and must contain at least one reviewed nonlinear primitive. The existing linear workflow remains unchanged and continues to reject connected nonlinear devices.
2. Add reviewed Shockley diode and forward/reverse Ebers-Moll BJT primitives. NPN and PNP are separate primitive identifiers so polarity cannot be provider-controlled.
3. Resolve a device only from a unique compatible catalog primitive claim. Reject missing, ambiguous, wrong-family, incomplete-terminal, or invalid-parameter evidence.
4. Catalog parameters are finite, bounded, canonical named values. Model parameters include saturation current, emission coefficient, temperature and operating limits; BJTs additionally include forward and reverse beta. Provider output cannot supply models, parameters, equations, matrices, includes, code, solver settings, or topology classifications.
5. Solve with deterministic Newton linearization and a fixed source/gmin continuation schedule. Iterations, exponential range, step damping, unknown count, coefficients, solutions, update tolerance, residual tolerance, and total work are bounded.
6. Emit deterministic solver evidence per operating point, including total iterations, continuation-stage count, and final maximum update and residual. Failure diagnostics identify the analysis, continuation stage, iteration bound, and largest update/residual unknown, with actionable advice for floating nodes, missing bias paths, source conditions, or catalog model incompatibility.
7. Validate converged diode current/reverse voltage and BJT collector current/collector-emitter voltage against catalog-backed model limits. Fail closed before reporting assertions when a limit is exceeded.
8. Preserve plan hashing, tamper detection, replay byte determinism, all legacy reports, and all existing linear MNA results.

## Numerical policy

- Thermal voltage is derived from checked-in temperature with fixed physical constants.
- Exponentials use a bounded argument with deterministic linear continuation above the bound.
- Continuation stages are fixed in code and ordered from strong gmin/reduced sources to nominal sources/minimal gmin.
- Every stage has a fixed iteration limit and maximum voltage update. Convergence requires both update and nonlinear residual bounds.
- No random initial conditions, adaptive iteration budgets, wall-clock stopping conditions, host-dependent parallel reduction, or external simulator invocation is permitted.

## Held-out proof

Add an AI fixture for a catalog-backed transistor switch or bias circuit that does not declare a block family or topology classification. Its recorded response must resolve through the normal catalog and graph pipeline, converge reproducibly, satisfy structured voltage assertions, route without incomplete operations, and pass real KiCad ERC, strict DRC, connectivity, writer correctness, and zero-diff round-trip validation. Existing protected I2C and LED fixture evidence must remain green.

## Non-goals

- Transient analysis, nonlinear AC linearization, temperature sweeps, noise, breakdown, capacitance, Early effect, self-heating, SOA, or fabrication sign-off.
- Provider-supplied SPICE models or arbitrary equations.
- Fixture-specific coordinates, allowlists, schemas, routing cases, or topology recognizers.
