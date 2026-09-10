# V23 solver layer

Status: solver implementation and independent tests complete; generation
integration, final certificates, and the successor corpus freeze remain pending.
No new corpus or installed-KiCad pass is claimed by this layer.

## Correction

`solveBoundedOpAmpDCFromStateV23` first invokes the unchanged historical active-set
solver. Historical successes and unrelated refusals are returned unchanged.
Only its exact bounded cycle/iteration refusal, with an inherited op-amp clamp,
can trigger recovery. Nonlinear devices are excluded from this correction.

Recovery removes only inherited op-amp clamp entries from a cloned seed. Other
active-device states are retained. It checks the existing stability proof,
rebuilds the same finite-gain equations, and tries one bounded alternative seed.
Acceptance requires the existing MNA residual, operating-limit, supply, and
resolved active-state consistency checks. If any check fails, the original
system, solution, states, and diagnostic are returned. No electrical bound,
tolerance, model parameter, primitive, circuit connection, or public search limit
is changed.

The additional-solve upper bound per eligible point is:

`op_amp_count + 2 + min(active_device_count * 6 + 2, 32)`

This includes one stability solve per op-amp, one explicit rebuild, at most one
rebuild for retained non-op-amp clamps, and the historical iteration bound.
It is a conservative work bound, not a measurement of actual iteration counts.

## Version and provenance boundary

`EvaluateWithSolverV23` is an explicit opt-in API. Only linear-MNA plans containing
a DC sweep use the new sweep path; nonlinear and other workflows delegate to
the original engine. Primitive-model/registry identities and all default APIs
remain unchanged. Version-isolated dispatch/corner adapters preserve historical
analysis ordering, worst-case selection, deduplication, worker bounds, and
fail-closed aggregation. No concurrency policy was changed.

The returned `SolverExecutionV23` binds the exact plan, report, diagnostics,
solver ID/contract digest, selected execution path, and recovery work bounds.
The source freeze will separately authenticate the implementation/build bytes.
The legacy-shaped numerical report alone is **not** a V23 provenance claim.
Consumers must bind and verify the execution record and independently require
all admission, diagnostic, numerical, safety, and physical acceptance gates.
`VerifySolverExecutionV23` authenticates this binding; it is not a numerical
rerun or a substitute for the generation certificate.

The next integration step must carry these records through every evaluated
assertion/case/corner and into the selected repair certificate. It must reject
missing, substituted, mismatched, or rehashed solver-policy evidence. The
application remains on the historical engine until that explicit V23 path exists.

## Local validation

Passing independent tests cover low/high rail release, cascaded and resistive
negative feedback, finite-gain analytical accuracy, preserved comparator seeds,
unchanged legitimate saturation and cold results, rejected unstable alternatives,
preserved bounded refusal, deterministic full bidirectional sweeps, worst-case
load corners, genuine output-span failures, missing uncertainty evidence,
invalid/rehashed model parameters, invalid registry/unserializable plans,
historical nonlinear/DC delegation, and solver/plan/report provenance tampering.
Independent device-value sweeps also cover every forward/reverse point against
the analytical resistor-divider equation, with and without an existing override;
numerical results match the historical engine exactly and inputs remain immutable.

- Focused V23 and historical-reproducer race tests: pass.
- Entire `internal/simmodel` short and non-short test suites: pass.
- V23 diagnostic/publication audits and both V22 audits: pass.
- Historical V17/V18 and V20 contract audit packages: pass.
- Scoped simulation-package lint: zero issues.

A direct checksum of V17's old live environment correctly rejects the newer
security-release `go.mod`/`go.sum`. Those files are unchanged by V23. The committed
historical dependency snapshots and production-source audit boundary are verified
by `TestHistoricalDependenciesAndAuditHarness`; production source is never
substituted. V23 does not refresh or weaken an old manifest to accommodate itself.
