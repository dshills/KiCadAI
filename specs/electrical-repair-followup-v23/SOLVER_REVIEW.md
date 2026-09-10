# V23 solver-layer review

Scope: the eight staged solver implementation, regression, and scope/seal files
based on `fd06ef1b3b37c1ab91647712adeb9a9676731837`. Generation integration and
the successor corpus evaluation are not complete and no new corpus pass is claimed.

Prism used the authorized configured external Gemini provider.

## First review

Run `fd9fb1b6266292d0132c166157ced637`; raw JSON SHA-256
`9a38011cf5e1e2ba31f2fa82418afda971e888d33cee66d690bf302dd3aa1be4`.

- `022cf72b79c623df`: not a defect. MNA routines receive the current explicit
  `pointAnalysis`; they do not select the original analysis from `plan.Analyses`.
  `planWithAnalysisOverrides` applies device overrides only and does not replace
  `Plan.Analyses`. Calling it in the source branch would not perform the suggested
  update. The full 102-point bidirectional source regression checks every value
  against the finite-gain analytical result and both rail exits.
- `497f39629c461d12`: not a device-sweep control-flow defect. Both branches reach
  the solver; the copied nonlinear branch was unreachable after early delegation.
  Removed that dead branch for clarity. Added an independent 22-point analytical
  divider sweep, with and without an existing device override. Every forward and
  reverse point matches the historical numerical result, and the plan is unchanged.

## Second review

Run `7c6b50141fed12fd26130910a967ac0a`; raw JSON SHA-256
`39712835cb7bd30e1df2acc272def23f152a2d529ed825fbb01298c30e764d67`.
Zero high/medium findings; two low suggestions deliberately not applied:

- `443ad6aef4d10918`: accepting a convergence message among unrelated diagnostics
  would widen the frozen trigger and risk discarding an independent failure. The
  unchanged historical refusal returns exactly one diagnostic. Other combinations
  must retain the original refusal; the negative trigger tests require this.
- `b646bda9fd7ac847`: the historical bounded-state diagnostics have no dedicated
  code. Their exact path/message contract is sealed and independently exercised
  by the frozen reproducer and recovery tests. No stable typed code can be used
  without changing historical production. String changes would fail these tests,
  rather than silently expanding admission. A future API redesign is outside V23.

No unresolved valid findings. After remediation, focused V23/historical-reproducer
race tests, the complete simulation test suite, and scoped lint passed. Historical
audit packages also passed; the dependency-snapshot distinction is recorded in
`SOLVER_IMPLEMENTATION.md`. No installed-KiCad or successor corpus run occurred
for this solver-only checkpoint.
