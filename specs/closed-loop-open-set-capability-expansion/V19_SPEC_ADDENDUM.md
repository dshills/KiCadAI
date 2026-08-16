# V19 Generic Causal-Topology Repair Design Freeze

Status: design frozen for implementation; no V19 production code, corpus
mutation, synthesis run, or KiCad evaluation is authorized by this document.

## Objective

V19 extends the existing `causal_topology_repair` capability with deterministic,
bounded graph operations that can compose complete active signal paths. The
extension addresses the five public V18 cases selected in
`V19_CAPABILITY_SELECTION.md` without introducing circuit-family templates,
case identifiers, coordinates, outcome knowledge, or a private primitive list.

The V19 implementation must remain a versioned adapter. It must preserve V18
results byte-for-byte unless the V18 result is nonpassing and its only typed
frontier capability is `causal_topology_repair`. A V19 repair may expose a later
typed blocker; it may never convert missing evidence into a pass.

## Authenticated public evidence

The design uses only these public V10 discovery requirements and their entries
in the committed V18 generation-one report:

| Public case | Requirement SHA-256 | Missing reusable graph behavior |
|---|---|---|
| 009 | `04fec91fc4561e4af8ad44ff0cf74e0c05b36ddb5425f6ccb8b0acdc6642b84e` | Branch one analog excitation into independent analog-transfer and digital-decision output cones. |
| 016 | `ab515daffe69b018a12cb63a92dafb0506631200c13ce145aac376025571f702` | Build a decision stage and a bounded intentional feedback path for distinct rising and falling thresholds. |
| 017 | `cf713a33f41fb9005482946631fa52bc2743130c4e7c0543d0610d34d65be435` | Build independent voltage-source and current-source output cones from shared supply and reference resources. |
| 018 | `6b06ff730742a0c4ec773f9f315b48bcec24f54ca511853b6c2bb2d4b726051b` | Insert a role-complete powered analog transfer stage between an excitation and an observation. |
| 021 | `37d77403b6712a22d930a249e2271b1fbfe01994196fd4ca61e006a98f015aa8` | Branch one monitored input into two independent, oppositely directed decision cones without output contention. |

The hashes and immutable paths are authenticated by the V18 report and corpus
manifests. They are evidence only and must not be used as implementation
selectors.

The current repair implementation explains the common failure:

- it can adjust values and polarity;
- it can add, redirect, split, or substitute only two-terminal passive
  primitives; and
- it evaluates one generation plus coordinated pairs of those first-generation
  changes rather than repeatedly expanding improving child graphs.

It therefore cannot atomically add a registry-backed active primitive with all
required signal, power, and reference terminals, allocate independent output
cones, or introduce a controlled feedback edge. Those are graph-language gaps,
not five separate circuit-family gaps.

No held-out record, key, outcome, or aggregate influenced this design.

## V19 eligibility and delegation

The V19 entry point must first execute the exact V18 path with the exact legacy
and V18 environments bound by the future evaluator manifest. The V19 extension
is eligible only when all of the following are true:

1. V18 did not pass and did not return unsafe.
2. Every terminal typed frontier path ends at the exact capability atom
   `causal_topology_repair`.
3. The requirement and inventory pass their existing public contract checks.
4. The V18 run contains replayable causal-repair diagnosis rather than an
   untyped error.

Otherwise V19 returns the V18 result byte-for-byte. Eligibility must be derived
from typed run evidence, never from a requirement ID, file path, domain, role,
metric name alone, expected outcome, or graph hash.

Unsafe V18 results are terminal. V19 must not attempt to repair them.
This is a preservation boundary, not a claim that V18 invariants are perfect:
the selected public evidence contains only nonpassing exhausted cases, and
reclassifying historical unsafe outcomes was not selected or authorized.

## Reusable operation model

A V19 logical operation is an atomic, replayable transformation from one
complete candidate graph to another complete candidate graph. It records its
kind, diagnosed obligation, selected inventory primitive when applicable,
created internal nodes, complete terminal connection map, before and after
graph hashes, and canonical cost. Intermediate incomplete graphs must never be
simulated or retained.

The existing `GraphOperation.Connections` representation can record a complete
multi-terminal insertion. V19 should extend behavior in new versioned files;
it should not widen the historical meaning of V18 operations.

### 1. `insert_role_complete_stage`

Insert one active or passive primitive and bind every required terminal in one
logical operation.

Inputs are a diagnosed upstream node or external excitation, one unresolved
observation, compatible supply/reference domains, and the required analysis
set. Candidate primitives come only from the environment-supplied inventory.
They must provide a terminal-role signature compatible with the requested
causal transfer, for example analog-to-analog transfer, analog-to-digital
decision, power-to-voltage excitation, or power-to-current excitation.

The operation must:

- bind the causal input/control terminal to the diagnosed upstream node;
- bind the causal output terminal to the unresolved observation cone;
- bind all required power and reference terminals to compatible declared
  resources;
- apply registry defaults only where the inventory explicitly permits them;
- reject a primitive lacking models for any required analysis; and
- produce a complete graph that passes every V19 invariant before simulation.

The role signature is derived from terminal electrical/function metadata and
the requirement's public port/observation semantics. Production code may not
switch on component names or circuit-family labels.

### 2. `allocate_independent_observation_cone`

Allocate an observation-specific cone from a shared causal source. The shared
node may feed multiple high-impedance input/control terminals, but active
outputs are never shorted together and one output driver is not silently used
to satisfy heterogeneous observations.

This planner operation emits one or two atomic stage insertions or terminal
bindings. A two-cone proposal therefore consumes the frozen maximum of two
logical changes. More than two cones require a later accepted proposal and are
subject to the compositional-depth limit.

Cone selection is based on unresolved observation electrical type, direction,
analysis obligations, and driver compatibility. It must not infer a named
block such as a window detector, voltage reference, or current source.

### 3. `redirect_role_terminal`

Generalize the existing terminal redirection operation from two-terminal
passives to any inventory-declared terminal whose role permits rebinding. The
operation changes one terminal binding and must preserve the primitive's full
required-terminal contract.

Power, reference, and causal-output terminals may be redirected only when the
new node has a compatible domain and the result passes contention, reference,
and cycle validation. The operation cannot change terminal roles or invent a
domain bridge.

### 4. `insert_typed_feedback_path`

Add one registry-backed passive path from a stage output or observation node to
a compatible control/input node in that same causal cone. This is the only V19
operation allowed to introduce a directed back-edge.

It is eligible only when an unresolved public obligation explicitly requires
feedback-sensitive behavior such as hysteresis, stability margin, closed-loop
gain, or a direction-specific threshold pair. The proposed loop must contain
at least one passive element, identify a deterministic loop-break input, avoid
power/reference nodes, and satisfy the cycle invariant below. An arbitrary
back-edge remains invalid.

### 5. Existing value, polarity, passive, and substitution operations

V19 retains the existing value, polarity, two-terminal passive, and compatible
substitution operations. They are not redefined. They can be paired with one
new topology operation when the pair contains no more than two logical changes.

## Mapping the five selected failures

The mapping is explanatory evidence, not executable dispatch:

| Public case | Required operation composition |
|---|---|
| 009 | Allocate two observation cones from the shared input; insert one analog-transfer stage and one digital-decision stage; bind both to compatible supply/reference domains. |
| 016 | Insert or complete a digital-decision stage; add a typed passive feedback path; coordinate a bounded value change if threshold separation remains unsatisfied. |
| 017 | Allocate two independent observation cones from the supply/reference resources; insert voltage-excitation and current-excitation stages chosen by terminal-role signatures. |
| 018 | Insert one role-complete analog-transfer stage; coordinate a bounded value or compatible primitive substitution only when the causal topology becomes complete. |
| 021 | Allocate two digital observation cones from the monitored input; insert independent decision stages with observation-specific polarity; keep their outputs electrically separate. |

The implementation must pass synthetic equivalents even if all five public
case files are absent from the test process.

## Deterministic bounded composition

V19 replaces the current one-generation coordination behavior only for eligible
V19 runs with a real diagnosis-driven beam. It does not raise any V18 limit.

### Fixed limits

Every limit below is a global run-wide ceiling unless its label explicitly says
proposal, depth, or beam. In particular, 4,096 value trials is not a
per-candidate allowance, and value-driven simulations also consume the global
50,000 candidate-simulation ceiling. Reaching either ceiling first produces
no partial-candidate evidence. A complete passing candidate found before the
ceiling remains eligible for selection; typed exhaustion is returned only when
no fully evaluated passing candidate exists.

- maximum logical changes per proposal: 2;
- maximum accepted proposal depth: 4;
- maximum beam width at each depth: 8;
- maximum evaluated causal-repair candidates for the entire V19 invocation:
  48;
- maximum topology repairs for the entire synthesis run: 128;
- maximum retained candidates: 16;
- maximum primitive instances: 32;
- maximum internal nodes: 32;
- maximum expanded states: 20,000;
- maximum generated graphs: 50,000;
- maximum candidate simulations: 50,000;
- maximum corner evaluations: 65,536; and
- maximum value trials: 4,096.

Depth counts accepted parent-to-child proposals, not individual low-level
terminal bindings. Thus a path can contain at most eight logical changes while
no proposal contains more than two. The 128 topology-repair count remains an
independent, stricter run-wide accounting gate.

### Search procedure

1. Evaluate the V18-selected base graph and seed depth zero.
2. Visit parents in canonical parent order.
3. Derive operations only from that parent's unresolved typed obligations and
   causal diagnosis.
4. Canonicalize operations, apply at most two as one proposal, validate the
   resulting complete graph, and deduplicate by canonical graph hash.
5. Order valid children by the canonical child tuple below before consuming
   trial budget.
6. Admit at most the deterministic per-depth evaluation quota defined below,
   then evaluate that entire slice or until a stricter run-wide numeric budget
   is exhausted. Never stop merely because a traversal happened to encounter a
   pass first.
7. Reject unsafe children and children that regress a satisfied critical
   obligation. Retain only monotonic children as defined below.
8. Select at most eight children for the next depth, including the bounded set
   of structurally progressing score-plateau children defined below. Retain
   passing children for final ranking but do not expand them.
9. After finishing the admitted slice, stop and select the top-ranked passing
   child if at least one exists. Otherwise continue until depth four, no
   expandable child, or numeric exhaustion. Return a typed exhaustion result
   when no candidate passes.

The global visited set includes the base graph and all generated complete graph
hashes. A duplicate consumes generated-graph accounting as required by the
existing policy but never consumes a simulation twice.
When multiple proposals produce the same graph hash, retain the proposal with
the bytewise smallest canonical cumulative operation-history key before
simulation. If both history keys are identical, compare `CanonicalGraphJSON`;
if that is also identical, the candidates are semantically identical and
compact to one.

The 48 evaluated-candidate ceiling is divided into four base quotas of 12, one
for each depth. Unused quota rolls forward only: the quota at depth `d` is 12
plus unused quota from lower depths. Later depths cannot borrow from an earlier
depth, so a broad first generation cannot consume all 48 trials and prevent an
otherwise available depth-four path. Within a depth, the canonical child tuple
selects the admitted slice. This deliberately bounded beam does not promise to
evaluate every child of all eight parents; exhaustion is a valid outcome.

### Candidate validation and loop exit

Candidate handling is a fixed state machine:

1. **Pre-simulation reject:** reject a proposal that exceeds any resource
   limit, cannot be applied atomically, does not yield a complete canonical
   graph, fails `ValidateCompleteGraph`, or fails a V19 cycle, contention,
   domain, reference, rating, or registry invariant.
2. **Evaluate:** run every required analysis and authored corner through the
   unchanged simulation environment, charging all existing counters. A
   canceled, incomplete, non-finite, or limit-truncated evaluation cannot pass.
3. **Post-simulation reject:** reject unsafe results and any result that
   regresses a previously satisfied critical assertion.
4. **Pass:** a search candidate passes only when every authored assertion,
   required analysis, and safety gate applicable to synthesis passes with
   complete evidence. Unsupported evidence is a typed nonpass, never an omitted
   obligation. A pass is retained but never expanded.
5. **Expandable nonpass:** a safe nonpass may enter the next beam only under
   the monotonic expansion rule. All other safe nonpasses remain diagnostic
   evidence and are not expanded.
6. **Depth exit:** finish the one already admitted slice for the current depth.
   If it contains passes, choose exactly one by the canonical child tuple, skip
   every later depth, and end causal search. If it has no pass, choose the next
   beam by the same tuple and continue. “Do not stop on the first pass” means
   only that the current admitted slice is completed; it never requires a
   second slice or deeper search after that slice yields a pass.

A search-stage pass remains subject to every downstream synthesis, physical,
and KiCad promotion gate; it is not by itself a published circuit pass.

### Monotonic expansion rule

A safe, invariant-valid child is expandable only if it does not turn any
previously satisfied critical assertion into a failure and it makes strict
progress by the first applicable condition:

1. reduces the number of unsatisfied critical obligations;
2. reduces the total number of unsatisfied obligations;
3. lexicographically reduces the typed structural-defect vector: missing
   required terminal bindings, unallocated required observation cones,
   missing diagnosed typed-feedback paths, and unreachable required
   observations, in that order;
4. makes a required observation causally reachable with a valid driver;
5. reduces the worst normalized violation by more than the existing causal
   noise tolerance; or
6. improves a diagnosis-specific normalized margin by more than that same
   fixed noise tolerance without regressing any already satisfied obligation.

A child satisfying condition 3 may be expanded when its simulation score is
unchanged; this is the sole deterministic plateau lane and enables a later
proposal to complete a multi-step repair. Strict score-improving children fill
the beam first. Structurally progressing plateau children may then fill every
remaining beam slot, with at most two from one parent, selected by the canonical
child tuple. A child that ties both the simulation score and
structural-defect vector is retained only as diagnostic evidence and is not
expanded. Probabilistic neutral moves and patience counters are prohibited
because they would weaken replay determinism. Unsafe or invariant-invalid
graphs are never expanded.

The inherited causal noise tolerance is a deterministic numeric comparison
threshold, not a statistical-significance claim. No sampling test, p-value, or
probabilistic acceptance criterion participates in candidate selection.

### Canonical ordering

Operation candidates are sorted by this tuple:

1. operation rank: existing value, existing polarity, role-terminal redirect,
   existing passive add/split/substitute, role-complete stage insertion,
   independent-cone allocation, typed feedback insertion, coordinated pair;
2. unresolved critical obligation before noncritical obligation;
3. observation ID;
4. upstream node ID;
5. primitive kind;
6. primitive inventory key;
7. canonical terminal-connection tuple;
8. canonical value encoding; and
9. proposed after-graph hash.

The canonical terminal-connection tuple is the list of `(terminal ID, node ID)`
pairs sorted bytewise by terminal ID and then node ID, matching the existing
`compareTerminalConnections` order. IDs are the normalized graph IDs produced
by `NormalizeGraph`; length-delimited fields are used when forming a history
key so concatenation cannot collide. A finite `float64` value is encoded with
the existing `strconv.FormatFloat(value, 'g', -1, 64)` representation; absent
values encode as `-`. Negative zero and every non-finite value are rejected
before encoding. Graph hashes use the existing `CanonicalGraphJSON` and
`GraphHash` functions rather than a new V19 serializer.

Evaluated children and beam parents are sorted by:

1. safe pass before safe nonpass;
2. fewer unsatisfied critical obligations;
3. fewer total unsatisfied obligations;
4. fewer unreachable required observations;
5. lower worst normalized violation;
6. lower cumulative logical-change count;
7. lower cumulative added-primitive and added-node count;
8. canonical operation-history key; and
9. graph hash.

The cumulative operation-history key is a length-delimited canonical encoding
of every operation in path order, including kind, diagnosed obligation,
primitive kind/key, created normalized nodes, sorted connection tuple,
canonical value, and before/after hashes. If all nine ranking fields are equal,
compare `CanonicalGraphJSON` bytewise. Equal canonical graph bytes identify one
semantic candidate and are compacted rather than ordered by discovery time,
goroutine completion, parent slice position, or memory address.

All strings use bytewise ascending order, all numeric comparisons use the
existing canonical finite-number rules, and no map iteration may influence an
ordered result. Input primitive, node, terminal, assertion, and diagnostic
orders must be normalized before generation.

Replay byte identity is required inside the single evaluator environment whose
Go toolchain, OS, architecture, model artifacts, and KiCad version are frozen
by manifest. V19 must not hash raw intermediate floating-point accumulators:
graph/value identities use the canonical encodings above, and comparisons use
the existing finite `float64` rules plus fixed causal noise tolerance. A new
decimal or software-floating-point implementation would change historical
simulation semantics and is outside this capability. Cross-environment
equivalence is established by manifest equality, not by claiming arbitrary CPU
architectures produce identical unbound simulation bytes.

## Safety and graph invariants

Every proposal must be checked before simulation and again before selection.
The checks extend, rather than replace, `ValidateCompleteGraph`.

### Causal cycles

Build a directed causal graph from inventory terminal electrical/function
roles. Input/control terminals point into a primitive; driven/source terminals
point out. Supply and reference bindings do not create causal signal edges.

Reject every directed cycle unless it was created by a recorded
`insert_typed_feedback_path` operation and all of these hold:

- the cycle remains within one observation cone;
- it contains exactly one declared loop-break input/control terminal;
- it contains at least one passive registry primitive;
- it contains no supply or external-reference node;
- every primitive in the cycle has reviewed models for the required analysis;
  and
- the originating unresolved obligation is explicitly feedback-sensitive.

At most one typed feedback path may exist in one observation cone. Multiple
independent cones may each contain one such path, subject to the same proposal,
depth, repair, and graph limits. Nested, cross-cone, output-to-output, or
untyped cycles are invalid.

### Active-output contention

A node may have at most one actively driven push-pull/source terminal. Passive
connections and high-impedance inputs do not count as drivers. Shared
open-drain/open-collector outputs are allowed only when the registry explicitly
declares that drive mode, every driver is compatible with the same domain, a
valid pull resource exists, and aggregate current remains within all ratings.

Observation-cone allocation shares inputs or supply/reference resources, never
active outputs. Each required source output must have one unambiguous driver
cone.

### Electrical domains and ratings

Every terminal binding must be compatible with the node's declared voltage,
current, signal/reference class, and direction. Worst-case domain limits must
fit the primitive registry ratings before simulation. A domain crossing is
valid only through a registry primitive whose terminal contract explicitly
models translation or isolation for every required analysis.

No operation may silently merge external supply domains or create a new
external domain.

### References

Every analog or control cone must trace to exactly one compatible declared
external reference, or to an explicitly isolated secondary reference created
by a registry primitive that models the isolation boundary. V19 cannot invent
an external return, use a supply as an implicit signal reference, or leave an
active reference/power terminal floating.

Two external references may not be merged unless the public requirement
declares them compatible and the graph contains an allowed connection.

### Primitive registry and physical evidence

Every inserted or substituted primitive must come from the supplied
`PrimitiveInventory` and must retain its catalog/model-provenance identity.
Eligibility requires:

- a complete terminal contract and all required terminal roles;
- a finite catalog-backed value domain where applicable;
- voltage/current/power ratings covering the declared domains;
- reviewed model provenance for every required analysis; and
- the existing physical-evidence and promotion eligibility.

V19 must not add a primitive allowlist, component-name switch, fallback ideal
device, synthetic simulation model, or unreviewed default.

### Accounting and replay

Every proposal records canonical before/after hashes and its complete operation
history. All graph generation, rejection, deduplication, simulation, corner,
value, and topology-repair consumption must be charged to the existing policy
fields. Reaching a limit returns typed exhaustion; it never truncates required
corners and never authorizes a pass from partial evidence.

## Synthetic fixture design

These fixtures are semantic tests, not production templates. Their names and
construction must not appear in production dispatch.

1. `shared_input_mixed_observations`: one analog excitation, one analog
   transfer observation, and one digital state observation. It requires two
   independent role-complete cones and proves safe input fan-out.
2. `directional_threshold_with_memory`: one monitored analog input and one
   digital output with distinct rising/falling thresholds. It requires a typed
   feedback path and proves arbitrary cycles are rejected.
3. `shared_supply_heterogeneous_sources`: one external supply/reference and two
   source observations, one voltage and one current. It proves heterogeneous
   independent output cones and rating checks.
4. `powered_conditioning_stage`: one analog input/output transfer with gain,
   bandwidth, noise, and swing obligations. It proves role-complete active
   insertion and supply/reference binding.
5. `opposed_threshold_outputs`: one monitored input and two isolated digital
   outputs with lower and upper decision behavior. It proves two decision cones,
   observation-specific polarity, and no active-output contention.

Negative fixtures must independently reject:

- two push-pull outputs on one node;
- an arbitrary or nested causal cycle;
- a cross-domain binding without a reviewed translator/isolator;
- a floating or multiply merged reference;
- an absent, unreviewed, or analysis-incomplete primitive;
- a proposal containing three logical changes; and
- a fifth accepted proposal depth.

Determinism fixtures must run original, reversed, every cyclic rotation, and
256 fixed-seed Fisher-Yates permutations of primitive/node/assertion input
order. Candidate order, hashes, rejection counts, selected history, and all
consumption fields must be byte-identical.

A composition fixture must require three independent logical changes and pass
only through at least two accepted proposals. A branching stress fixture must
generate more than 48 eligible children and prove truncation to 48 evaluated
trials, beam width eight, proposal width two, and all run-wide ceilings.

## Public V18 advancement gate

The later V19 evaluation must authenticate the exact committed 24-case V18
public boundary and evaluate every case exactly twice under one committed V19
environment.

Admission requires all of the following:

1. All five selected public cases (009, 016, 017, 018, and 021) either pass or
   advance to a deterministic later typed frontier. Any remaining
   `causal_topology_repair` frontier fails admission.
2. At least two of the five selected cases become complete passes across at
   least two reporting domains.
3. Every V18 pass, unsafe result, and satisfied typed obligation is preserved.
4. V18-ineligible requirements delegate byte-for-byte to V18.
5. Both synthesis replays are byte-identical for every case.
6. Every pass completes two clean-root installed-KiCad promotions with clean
   ERC, strict DRC, connectivity, route completion, writer correctness, and
   zero round-trip differences.
7. All local Go, historical seal, deterministic replay, fail-closed, resource
   accounting, and invariant tests pass.

The evaluator must publish failure rather than relax an invariant or begin a
new corpus cycle. One implementation run and one bounded correction run are the
maximum authorized evaluation attempts for the later implementation goal.

## Version isolation and likely implementation surface

Historical V18 files and manifests should remain byte-identical. The expected
production additions are:

- `internal/opentopologysynthesis/synthesis_v19.go`: exact V18 delegation and
  typed-frontier eligibility;
- `internal/opentopologysynthesis/causal_repair_v19.go`: bounded multi-depth
  beam, monotonic expansion, ranking, and accounting;
- `internal/opentopologysynthesis/causal_operations_v19.go`: role-complete
  insertion, cone allocation, generalized terminal redirection, and feedback;
- `internal/opentopologysynthesis/causal_invariants_v19.go`: directed-cycle,
  contention, domain, reference, and registry validation;
- `internal/capabilityexecutorv10/v19_executor.go` and `v19_runner.go`: V19-only
  wiring while retaining exact V18 inputs; and
- `cmd/kicadai-discovery-baseline-v19/main.go`: frozen public runner.

Expected focused tests are the matching `_test.go` files plus synthetic
operation, invariant, compositional-depth, permutation, accounting, delegation,
and runner tests. A future evaluator freeze will add V19 contract/protocol,
manifest, and contract-test files under this specification directory.

No change is expected in `search_model.go`: `GraphOperation.Connections` can
already represent a complete multi-terminal insertion. No V19 component catalog
or model-provenance extension is authorized; the capability must use the
environment's existing reviewed inventory.

If implementation proves that a historical file must change, stop before that
edit and amend this design with a version-isolation justification.

## Non-goals

V19 does not authorize:

- a circuit-family, fixture, requirement-ID, or coordinate-specific block;
- new components, ideal models, analysis solvers, or model evidence;
- routing, placement, writer, or KiCad semantic changes;
- changed assertion bounds, expected outcomes, or evaluation data;
- increased search, simulation, corner, value, graph, node, primitive, repair,
  beam, proposal-width, or retained-candidate limits;
- weakened safety, invariant, strict-DRC, connectivity, writer, round-trip, or
  fail-closed gates; or
- held-out evaluation or GitHub Actions execution.

Any of those requires a separate selection and authorization.
