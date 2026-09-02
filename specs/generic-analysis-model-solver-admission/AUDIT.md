# Generic Analysis, Model, and Solver Admission Audit

Status: in progress

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

## Phase evidence

Implementation and verification evidence will be recorded here as each phase
completes.
