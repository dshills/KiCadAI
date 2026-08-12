# V7 Round 1 Implementation Plan

Status: frozen before outcome-affecting implementation

This plan is bound to the generation-zero V7 discovery artifacts:

- input frontier SHA-256: `871e673e3b07c38b7369a0e595dcc8b48976c335f5e3a10ada519bf4a027837b`
- selection SHA-256: `87915abf1d8371b94a652c939df8122e05ee0d0bd18d2b674af7504f9813738d`
- generic plan SHA-256: `c020b6116610d8d392eb580cbaeebfa39655cf65219617781d698827364e1d93`

The implementation is limited to the three selected exact members and the
single shared-invariant prerequisite below. Paths listed here are the maximum
admitted mutation set; the reviewed implementation seal will record the exact
changed subset and its before/after hashes.

## 1. DC operating-point solver

Selected member:

- stage `simulation`
- scope `simulation`
- capability `dc_operating_point_solver`
- code `SIMULATION_INVALID`

Generic change:

- permit a DC operating-point input-impedance assertion when its input node,
  reference node, and excitation-source component resolve;
- calculate the impedance from the solved differential input voltage and the
  solved excitation-source current;
- preserve `maximumTrustedOpenCircuitImpedanceOhm` at its existing `1e15` ohm
  post-processing-only reporting ceiling; it is never stamped into the MNA
  matrix. Reject only an indeterminate operating point after all three scopes
  resolve and both differential voltage and source current normalize to zero
  under the existing `normalizedMNAFloat` `1e-15` floor. When current
  normalizes to zero but differential voltage does not, report the finite
  open-circuit ceiling without division. A zero/zero measurement returns an
  assertion-scoped undefined diagnostic and deterministic numeric zero without
  invalidating the otherwise converged MNA analysis. The existing
  `(value, diagnostic)` API requires consumers to discard the value whenever
  the diagnostic is non-nil. All calculated impedance values are clamped to
  the same finite reporting ceiling;
- retain deterministic normalization and all existing MNA work bounds.

Admitted production paths:

- `internal/simmodel/mna_registry.go`
- `internal/simmodel/mna_measurements.go`
- `internal/simmodel/mna_solver.go`

Admitted verification paths:

- `internal/simmodel/mna_dc_input_impedance_test.go`
- `internal/opentopologysynthesis/dc_input_impedance_simulation_test.go`

## 2. Causal topology repair

Selected member:

- stage `topology_repair`
- scope `topology`
- capability `causal_topology_repair`
- code `OPEN_TOPOLOGY_REPAIR_EXHAUSTED`

Generic change:

- derive topology repair targets only from normalized diagnoses and the
  source-to-observation causal cone. For a diagnosed discontinuity, bridge
  endpoints may include the source and observation weak components plus
  declared external nodes needed to join those components;
- add the smallest bounded deterministic repair proposal needed to close a
  diagnosed causal discontinuity or polarity error that existing passive
  value and edge mutations cannot close. Proposal cost is ordered by fewest
  graph changes, then fewest added primitives, then fewest added internal
  nodes, then the existing canonical repair key;
- require complete-graph validation, hash-chain replay, improvement evidence,
  regression rejection, existing count budgets, and stable ordering;
- do not add circuit-family dispatch, fixture identities, coordinates,
  allowlists, or unbounded search.

Admitted production paths:

- `internal/opentopologysynthesis/repair.go`
- `internal/opentopologysynthesis/causal_repair.go`
- `internal/opentopologysynthesis/causal_topology_repair.go`

Admitted verification paths:

- `internal/opentopologysynthesis/causal_repair_test.go`
- `internal/opentopologysynthesis/repair_test.go`
- `internal/opentopologysynthesis/causal_topology_repair_test.go`

## 3. Multi-obligation composition

Selected member:

- stage `requirement_realizability`
- scope `topology`
- capability `multi_obligation_composition`
- code `OPEN_TOPOLOGY_MULTI_CONTROL_COMPOSITION_REQUIRED`

Generic change:

- classify only externally independent non-power excitation ports as controls;
- decompose a source-output obligation by independent control while retaining
  shared power, reference, constraint, operating-case, and assertion context;
- synthesize the bounded sub-obligations with the existing policy and merge
  them through the existing deterministic graph-operation replay machinery;
- reject conflicts, incomplete graphs, combination overflow, invalid merged
  references, or non-deterministic ordering. The combination ceiling is
  exactly `max(1, MaxRetainedCandidates *
  multiOutputCombinationRetentionMultiplier)`, using the existing named
  constant whose frozen value is `2`. Reusing that bounded multi-output policy
  prevents a second, independently tunable composition budget. Subsearches consume one
  shared aggregate `MaxExpandedStates`/`MaxGeneratedGraphs` pool in canonical
  obligation order: each receives `remaining budget / remaining subsearches`,
  with a minimum of one when budget remains, so unused work is returned while
  later obligations retain an equitable share. Allocation first requires a
  strictly positive remaining-subsearch count; zero terminates allocation
  without division. Aggregate consumption may never exceed the original caller
  policy. Later obligations intentionally may consume unused surplus from
  simpler earlier obligations. Subsearches execute sequentially to ensure
  deterministic budget allocation. Across every
  obligation and composition layer, at most
  `max(1, MaxRetainedCandidates * 2)` merged candidate paths are materialized,
  not that many paths per layer.

Admitted production paths:

- `internal/opentopologysynthesis/realizability.go`
- `internal/opentopologysynthesis/search.go`
- `internal/opentopologysynthesis/multi_output_composition.go`
- `internal/opentopologysynthesis/multi_obligation_composition.go`

Admitted verification paths:

- `internal/opentopologysynthesis/realizability_test.go`
- `internal/opentopologysynthesis/multi_output_composition_test.go`
- `internal/opentopologysynthesis/search_test.go`
- `internal/opentopologysynthesis/multi_obligation_composition_test.go`

## 4. Admitted shared-invariant prerequisites

### 4.1 V5 historical evidence projection

The V5 reviewed-implementation seal includes `mna_registry.go` because the V5
electrothermal capability used the shared MNA assertion registry. The selected
V7 `dc_operating_point_solver` member must extend that shared assertion validation, so
the V5 live-worktree byte comparison would otherwise reject an authorized V7
change even though V5 evidence remains immutable.

The admitted prerequisite changes only the V5 verification harness. Every V5
artifact except the selected shared `mna_registry.go` remains byte-compared
with its V5 hash. The shared file is not ignored: the harness projects it
through the original V5 tests and frozen public-evidence replay, while the V7
implementation seal records its exact V5-before/V7-after hashes and selected
DC-solver member mapping. V7 final admission fails on any V5 replay regression,
missing original V5 verification, or missing replacement hash coverage.

Admitted path:

- `internal/capabilityfeedback/v5_reviewed_implementation_test.go`

Selected-member mapping:

- `dc_operating_point_solver`, because its assertion validator is implemented
  in the shared MNA registry file.

Static consumer boundary:

- `TestClosedLoopV5ReviewedImplementationSealIsFrozen`
- V5 frozen public-evidence verification

### 4.2 V6 historical evidence projection

The V6 reviewed-implementation seal currently compares historical V6 artifact
hashes with the live worktree. That makes any later, selected evolution of a
shared topology artifact look like corruption of V6 evidence. The admitted
prerequisite changes only the V6 verification harness. Every V6 artifact not
shared with this selected V7 implementation remains byte-compared with its V6
hash. A selected shared path is not ignored: the harness projects it through
the frozen V6 public-admission replay and original V6 verification suite, while
the V7 implementation seal records its exact V6-before/V7-after hashes and
selected-member mapping. V7 final admission fails on any V6 replay regression,
missing original V6 verification, or missing replacement hash coverage. The
harness still validates the immutable checksum-protected V6 seal, its recorded
hashes, and its recorded implementation commit. It introduces no Git
executable or history dependency and does not alter the V6 seal, selection,
corpus, evaluator, production runtime, or any synthesis outcome.

Admitted path:

- `internal/capabilityfeedback/v6_reviewed_implementation_test.go`

Selected-member mapping:

- `causal_topology_repair`, because the repair implementation consumes shared
  topology graph behavior;
- `multi_obligation_composition`, because composition evolves shared V6
  topology artifacts.

Static consumer boundary:

- `TestClosedLoopV6ReviewedImplementationSealIsFrozen`
- `TestUpdateClosedLoopV6ReviewedImplementationSeal`
- V6 public-admission verification

The prerequisite has no production or dynamic-registry consumer. Focused test
execution must reproduce this verification-only boundary before the round is
sealed.

## 5. Immutable environment and exit gates

This round does not change the corpus, frozen selection, evaluator, policy,
budgets, seeds, catalog, inventory, component/model provenance registry,
physical gates, toolchain, or promotion environment. The assertion-validation
logic in `mna_registry.go` is part of the selected simulation capability, not a
catalog or primitive-model registry transition.

Before the reviewed implementation seal is written:

1. focused unit, determinism, replay, bound, conflict, and fail-closed tests
   pass;
2. the full local Go suite and lint pass;
3. protected installed-KiCad fixtures required by the frozen protocol pass;
4. public discovery replays are deterministic and preserve all prior passes;
5. production and verification changes are mapped to a selected exact member;
6. Prism reviews the staged bytes through the configured provider and all
   actionable findings are remediated;
7. the implementation seal records exact before/after hashes and the static
   and focused-runtime consumer evidence for the prerequisite.
