# Generic Autonomous Placement And Routing Correction Specification

## 1. Purpose

Define a bounded, deterministic correction loop for generated
`generic-circuit-v1` projects. The loop may recover from supported placement,
endpoint-access, route-tree, and required-net completion failures without
asking the provider to rewrite geometry or changing circuit intent.

This project extends the existing placement-routing retry foundation. It does
not introduce a second placer, router, repair framework, or provider schema.

## 2. Current Baseline

KiCadAI already provides:

- strict generic circuit graph decoding and trusted-catalog resolution;
- lowering to `designworkflow.Request` with `explicit_circuit` evidence;
- deterministic placement and routing;
- placement retry hints and bounded `routing_retry` policy;
- best-attempt selection and repeated placement-state detection;
- route-tree branch, endpoint-access, contact-graph, and repair summaries;
- generated-project repair bundles and post-repair validation;
- provider retry-state artifacts for provider/schema failures;
- KiCad-backed promotion gates.

The remaining gaps are:

- generic graph lowering does not enable placement-routing retry;
- retry diagnostics use several subsystem taxonomies rather than one stable
  correction category model;
- retry planning and request mutation are coupled;
- `stop_on_repeated_signature` is modeled but not enforced by the retry loop;
- attempts do not expose stable retry keys or before/after invariant hashes;
- correction evidence is nested in `workflow-result.json` rather than written
  as a dedicated artifact;
- no identity-neutral generic stress case proves automatic recovery;
- routing-only transformations such as branch reordering and via relocation do
  not yet have a guarded correction contract.

## 3. Goals

The implementation must:

1. Classify correction diagnostics into a stable generic taxonomy.
2. Produce deterministic correction plans before applying changes.
3. Apply only explicitly authorized transformations.
4. Preserve electrical and physical intent invariants.
5. Enforce attempt budgets, stable retry keys, repeated-state detection, and
   fail-closed stop conditions.
6. Persist complete correction evidence under `.kicadai/`.
7. Enable the loop only for generated `generic-circuit-v1` requests in this
   milestone.
8. Prove one identity-neutral recoverable stress case.
9. Preserve every existing provider-backed pass fixture.

## 4. Non-Goals

This milestone does not:

- mutate imported or user-authored projects;
- ask an AI provider to repair PCB geometry;
- change component selection, symbols, footprints, pin mappings, schematic
  topology, net membership, net classes, widths, clearances, or required nets;
- implement push-and-shove routing or dense-board autorouting;
- relax ERC, DRC, writer, round-trip, connectivity, or physical checks;
- suppress genuine findings;
- make fabrication-ready or analog-performance claims;
- add component families, circuit blocks, topology dispatch, fixture checks,
  or production coordinate tables.

## 5. Scope Identification

A request is eligible only when all of the following hold:

- it was lowered from `generic-circuit-v1`;
- `explicit_circuit` is present;
- `intent.category` is `explicit_circuit_graph`;
- catalog and resolution hashes are valid;
- the project is being generated into a managed output workspace;
- routing is enabled;
- correction policy is enabled and valid.

The request itself carries the eligibility evidence so workflow code does not
inspect project names, fixture paths, prompts, or provider response text.

Existing non-generic `routing_retry` behavior remains compatible. The richer
autonomous correction artifact is emitted only for eligible generic requests.

## 6. Correction Taxonomy

`CorrectionCategory` values are stable machine-readable strings:

| Category | Meaning | V1 action |
| --- | --- | --- |
| `component_overlap` | Placement overlap or insufficient component spacing | Increase bounded spacing |
| `inaccessible_pad` | Pad cannot obtain legal route access | Increase fanout room or move eligible component |
| `blocked_escape_direction` | Edge, keepout, or copper blocks available escape | Move eligible component within constraints |
| `route_tree_branch_order` | Route-tree branch sequence prevents completion | Rebuild with deterministic alternate order when supported; otherwise stop |
| `missing_layer_transition` | A required legal layer transition is absent | Apply proven layer-access policy when supported; otherwise stop |
| `same_net_branch_merge` | Same-net branches remain separate contact components | Rebuild the route tree after an authorized placement/access correction |
| `required_net_disconnected_endpoint` | A required endpoint is not contacted | Apply `reduce_endpoint_distance` when the diagnostic proves both endpoint refs and at least one is movable, then rebuild; endpoint snapping is reserved and unsupported in v1 |
| `routing_region_exhaustion` | Search space is exhausted by distance or congestion | Bounded spacing or proximity adjustment |
| `unsupported_geometry` | Geometry or evidence cannot be safely corrected | Never apply automatically |

Subsystem diagnostics map into this taxonomy. The original issue code,
category, action, refs, nets, normalized path, and evidence remain available.

## 7. Correction Diagnostics

Each normalized diagnostic contains:

- category;
- source subsystem and source category;
- issue code and severity;
- affected refs and nets in sorted order;
- normalized issue path;
- concise message;
- evidence strings in sorted order;
- whether an automatic action is supported;
- unsupported reason when no action is authorized.

Diagnostics must not depend on project names or absolute output paths. Raw
coordinates are excluded from stable keys unless quantized by an existing
geometry contract. Persisted messages and evidence are constructed from a
closed set of KiCadAI issue codes, categories, actions, normalized paths,
refs, nets, and bounded engine-authored evidence tokens. Provider text and raw
external-tool output are not accepted as correction evidence.

## 8. Stable Retry Keys

The correction retry key is a SHA-256 digest of canonical data:

- schema version;
- normalized diagnostic categories;
- issue codes;
- normalized paths;
- sorted refs and nets;
- selected authorized action kinds;
- current invariant fingerprint;
- current movable-placement state hash.

Messages and absolute paths are not key inputs. Equivalent diagnostics in
different workspaces must produce the same key. Placement-state coordinates
are canonical decimal values quantized to `0.0001 mm`, rotations to
`0.01 degree`, and layers to canonical KiCad names before hashing. This matches
the existing placement-state contract and prevents raw floating-point noise
from changing a retry key. Canonical fields are written incrementally to the
hash; the implementation must not build a full-board JSON serialization solely
to compute the key. Quantization converts values to fixed-point integers with
an explicitly tested half-away-from-zero rounding rule before hashing.

When `stop_on_repeated_signature` is enabled, a repeated key stops before
another mutation. A correction history may not contain the same applied retry
key twice.

## 9. Correction Plan

Planning is pure and does not mutate requests or files. A plan contains:

- schema version;
- attempt number and remaining budget;
- retry key;
- normalized diagnostics;
- ordered proposed actions;
- authorization decision;
- expected invariant fingerprint;
- stop reason when no safe action exists.

Supported action kinds in v1 are:

- `adjust_relative_spacing`;
- `move_within_declared_region` through existing placement retry hints;
- `improve_endpoint_fanout` through existing placement retry hints;
- `reduce_endpoint_distance` through existing placement retry hints;
- `rebuild_route_tree` after an authorized placement/access change.

`required_net_disconnected_endpoint` selects its action from source evidence:
distance/search-exhaustion evidence maps to `reduce_endpoint_distance`, pad or
layer-access evidence maps to `improve_endpoint_fanout`, and explicit
clearance/congestion evidence maps to `adjust_relative_spacing`. The diagnostic
must carry a required net and resolved endpoint refs. The plan selects a
deterministic movable anchor/target pair using sorted references and existing
placement semantics. If the source category is ambiguous, endpoint identity is
not proven, all endpoints are fixed, or a required region prevents the action,
the plan stops instead of attempting incidental recovery.

The following action kinds are reserved but fail closed until their shared
router contracts are implemented and tested:

- `reorder_route_tree_branches`;
- `insert_layer_transition`;
- `relocate_layer_transition`;
- `snap_route_endpoint`.

Reserved actions must never be silently translated into a placement change.

## 10. Guarded Application

Application accepts the current request, placement request, correction plan,
and expected invariant fingerprint.

Before mutation it must verify:

- generic scope eligibility;
- plan authorization;
- retry budget;
- expected invariant fingerprint;
- no repeated applied retry key;
- all affected components are movable;
- fixed positions, required regions, edge intent, and keepouts remain hard;
- the proposed action maps to an existing deterministic adjustment.

When a conflict contains one fixed and one movable component, only the movable
component is eligible for an action and the fixed component remains the
constraint anchor. When all affected components are fixed, planning stops with
`fixed_constraint_conflict`. A diagnostic with multiple movable candidates is
ordered by normalized reference and existing mobility priority; it does not
choose a candidate randomly.

After constructing an adjusted request it must verify:

- the invariant fingerprint is unchanged;
- the request validates;
- spacing increases by at most `1.0 mm` per correction and `2.0 mm` total;
- generated proximity constraints do not exceed `25.0 mm`;
- clearance values are not reduced;
- keepout bounds are not moved or shrunk;
- required regions are not moved, expanded, or removed;
- fixed positions and edge constraints are unchanged;
- board dimensions, layers, and edge clearance are unchanged.

Spacing and fanout actions update soft minimum constraints and rerun the
existing deterministic global placer. They do not directly push one component.
The resulting placement must pass the normal overlap, board-boundary, keepout,
region, and routing-readiness checks, which catches secondary collisions with
components not named by the original diagnostic.

The static spacing and proximity caps deliberately match the existing retry
engine. They do not replace or scale electrical clearance rules. If a
technology profile or physical rule requires more movement than these caps,
v1 stops with `no_safe_adjustment`; changing those limits requires a separate
evidence-backed policy change.

The adjusted placement and routing run in memory. Generated project files are
written only for the selected best attempt through the existing workflow.

## 11. Preserved Invariants

The invariant fingerprint covers canonical representations of:

- explicit circuit resolution and catalog hashes;
- component IDs, references, values, footprint IDs, schematic units, and pads;
- net names, roles, classes, endpoint membership, required flags, currents,
  widths, and clearances;
- schematic transaction intent;
- board dimensions and layer count;
- regions, keepouts, and zones;
- fixed placement and edge constraints;
- validation requirements.

Soft placement rules and retry policy are excluded because correction may
adjust them. Provider text, output paths, timestamps, and generated UUIDs are
excluded.

Board dimensions are fixed in v1. If an authorized spacing or movement action
cannot fit within the existing board, regions, edge clearance, and keepouts,
application stops with `no_safe_adjustment`. Automatic board expansion is a
reserved future action, not an implicit fallback. A board that is physically
too small for its declared parts and rules is an invalid input constraint, not
a recoverable placement/routing failure; the report must distinguish that case
from budget exhaustion.

## 12. Budget And Stop Conditions

The default generic correction budget is three total placement/routing
attempts: one initial attempt and at most two corrections. Two corrections
allow the existing bounded spacing adjustment to progress from `1.0 mm` to its
`2.0 mm` cap while still preventing open-ended iteration. A request may select
a smaller valid budget. Future work may raise the maximum only after evidence
supports it.

The loop stops on:

- routed success;
- budget exhaustion;
- no correction diagnostics;
- no authorized action;
- ambiguous diagnostics;
- repeated retry key;
- repeated placement state;
- invariant mismatch;
- invalid adjusted request;
- placement blocker;
- new board-validation or DRC regression;
- non-improvement when policy requires stopping;
- context cancellation.

The best ranked attempt remains selected when a later attempt regresses.

Attempt improvement uses the existing strict lexicographic ranking, in this
order:

1. required DRC evidence is clean when the policy requires it;
2. fewer board-validation blockers;
3. fewer DRC blockers;
4. fewer blocked route-tree groups;
5. more complete route-tree groups;
6. more proven required endpoints;
7. more routed route-tree branches;
8. fewer route-tree contact misses and route-tree issues;
9. better routing status (`blocked` before `partial` before `routed`);
10. fewer failed nets;
11. more routed nets;
12. route-quality improvement meeting `min_routing_score_delta`;
13. the earlier attempt when all evidence is equal.

Placement score alone cannot select an electrically worse attempt.
This ordering is retained from the proven retry foundation rather than replaced
with weighted scores: a lower-priority quality gain can never outweigh a
higher-priority DRC, connectivity, or route-tree regression.

## 13. Evidence Artifact

Eligible generic workflows write:

```text
.kicadai/autonomous-correction.json
```

The report contains:

- schema version and scope;
- enabled state and attempt budget;
- initial and final invariant fingerprints;
- attempt count, applied count, selected attempt, and stop reason;
- one record per attempted plan;
- diagnostics, actions, retry key, authorization, application status,
  before/after placement hashes, route metrics, regression flags, and selection
  reason;
- final preservation status;
- artifact references for workflow, connectivity, route completion, writer,
  ERC/DRC, and round-trip evidence when present.

The report must be deterministic except for explicitly identified
workspace-specific artifact paths. It must never contain prompts, API keys,
authorization headers, or hidden provider reasoning.

## 14. Workflow Integration

The generic path becomes:

```text
provider graph
  -> strict decode
  -> trusted catalog/library resolution
  -> explicit-circuit request
  -> initial placement and routing
  -> normalize correction diagnostics
  -> plan correction
  -> guarded apply
  -> retry placement and routing
  -> select best attempt
  -> write selected project
  -> run validation and promotion gates
  -> persist autonomous-correction report
```

Provider correction attempts remain limited to malformed provider output and
schema diagnostics before project generation. Engineering correction never
returns PCB geometry diagnostics to the provider.

## 15. Stress Fixture

Add one internal, identity-neutral stress fixture derived from an existing
generic graph. It must:

- use generic component and net semantics rather than production fixture IDs;
- perturb relative placement or spacing so the first attempt produces a
  supported correction diagnostic;
- recover through the generic plan/apply path;
- prove the final circuit projection and invariant fingerprint match the input;
- avoid project-name, fixture-path, or reference-specific production logic;
- run offline by default;
- support an optional KiCad-backed lane using the existing environment gate.

## 16. Test Strategy

Focused tests must cover:

- every correction category mapping;
- stable ordering and retry keys;
- key independence from workspace paths and messages;
- pure plan construction;
- unsupported and ambiguous fail-closed plans;
- guarded application and invariant rejection;
- repeated retry-key and repeated-state stops;
- budget exhaustion;
- best-attempt preservation;
- stress fixture recovery;
- deterministic repeated runs;
- artifact schema and CLI reporting;
- regression coverage for all existing generic fixtures.

Default tests remain offline and credential-free. KiCad-backed promotion is
optional and environment-gated.

## 17. Completion Criteria

The milestone is complete when:

- one recoverable generic stress case reaches pass within budget;
- unsupported and ambiguous cases stop with structured evidence;
- correction cannot alter protected circuit or physical-rule invariants;
- correction history and retry keys are persisted;
- internal connectivity and route completion pass;
- optional KiCad ERC/DRC are clean for the stress lane;
- writer correctness and normalized round-trip checks pass;
- existing provider-backed fixtures remain pass;
- `go test ./...` and `make lint` pass;
- Prism has no unresolved high findings;
- documentation explains operation, evidence, and limits.

## 18. Future Work

After this milestone, add routing-only actions one contract at a time, starting
with deterministic branch ordering and proven layer-transition insertion. The
next broader project is calculated rating and physical/fabrication evidence,
not additional topology accumulation.
