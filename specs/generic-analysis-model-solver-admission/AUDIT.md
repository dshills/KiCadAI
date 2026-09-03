# Generic Analysis, Model, and Solver Admission Audit

Status: complete; frozen public advancement gate passed

Starting commit: `ff8ac638a`

## Frozen evidence

- V18 public generation-one population: 24 cases, 1 pass and 23 nonpassing.
- Selected public failure family: 9 affected cases across 5 reporting domains.
- Exact atoms: `dc_operating_point_model`, `dc_operating_point_solver`,
  `ac_sweep_solver`, `dc_sweep_solver`, `transient_solver`, and
  `stability_solver`.
- Retired V19 result: 0 pass, 12 unsupported, 1 unsafe, and 11 exhausted.
- V19 remains immutable historical evidence and is not the implementation
  target for this milestone.

## Implementation evidence

- `kicadai.simulation-admission.v1` deterministically derives analyses, selects
  immutable enabled solvers, authenticates bundled/project/configured model
  sources, and admits exact inventory plus graph/harness models.
- Admission evidence binds request, environment, analysis, solver, model,
  parameter, source, provenance-record, and compatibility digests.
- Seven stable refusal categories fail before synthesis or numerical work; no
  implicit model substitution is allowed.
- V20 is version-isolated. Historical V18/V19 evaluator and synthesis sources
  remain byte-identical to their frozen manifests.

## Frozen public evaluation

The committed evaluator ran all 24 authenticated discovery cases serially and
exactly twice from clean roots with installed KiCad 10.0.3. The authenticated
report is `internal/capabilityfeedback/testdata/closed_loop_open_set_v20_generation_zero/report.json`
with file SHA-256 `6ea9b697f8852cb8f4d752f75e5fa44aca93de7bbad9bb5e5fc6c063b10ff6aa`
and report hash `6e4b732925aa158d811e4d15041d6b9ce3d138c7a273409b41efde72fba76d24`.

- V20: 1 pass, 5 unsupported, 1 unsafe, 17 exhausted.
- V18 comparison: 1 pass, 7 unsupported, 1 unsafe, 15 exhausted.
- The V18 pass and unsafe result were preserved. The pass again completed two
  installed-KiCad promotions with every physical and replay gate true.
- Two unsupported cases advanced to bounded topology-repair exhaustion. One
  is the selected `dc_operating_point_model` case, which moved to the later
  generic `causal_topology_repair` blocker. This satisfies the frozen material-
  improvement rule without renaming a failure.
- The other selected solver leaves remain visible as numerical/topology work;
  V20 admits their exact model-and-solver combinations but does not claim those
  circuits pass.

An initial sandboxed execution was rejected before publication because macOS
terminated the installed KiCad zone-refill subprocess. The unchanged frozen
evaluator was rerun outside that sandbox from a fresh root; only the valid
aggregate above is retained.
