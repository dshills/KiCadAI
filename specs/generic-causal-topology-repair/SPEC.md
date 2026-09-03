# Generic Causal Topology Completion and Bounded Repair

## Purpose

V21 is a version-isolated successor to the frozen V20 public evaluation. It
adds deterministic, circuit-family-neutral completion and repair of candidate
electrical graphs. It does not expand the component catalog, simulation model
set, solver set, physical-routing policy, or the v1 supported surface.

The selected public population is frozen in `V21_PUBLIC_TOPOLOGY_POPULATION.json`.
It contains 47 `causal_topology_repair` frontier occurrences and nine
`complete_topology` occurrences across eight public discovery cases and five
reporting domains. These are occurrences, not 56 distinct requirements.

## Contract-derived obligations

The planner derives obligations only from normalized ports, domains,
behavioral assertions, terminal contracts, the current graph, and typed
simulation diagnoses. In canonical order it checks:

1. every required external port is represented and electrically reachable;
2. every non-reference domain has a compatible reference closure;
3. every observed port owns a measurable, non-conflicting observation cone;
4. every explicit excitation has a directionally valid causal path to its
   observation;
5. source, supply, control, signal, output, load, and reference relationships
   are directionally coherent;
6. requirements sharing an excitation may diverge into independent observed
   branches, and compatible partial paths may converge;
7. required subgraphs are connected to the contract rather than isolated;
8. active cycles exist only when typed feedback evidence authorizes them.

## Generic operations

V21 may connect an unreachable port, complete a causal path, extend or split an
observation cone, join compatible partial paths, introduce a branch or
convergence node, redirect an incompatible terminal, attach a reference, or
remove/replace a causally irrelevant fragment. An operation is admissible only
when a typed obligation identifies its scope and the resulting graph passes
the invariant boundary.

Operations may refer to semantic roles and primitive terminal contracts. They
must never refer to a public case ID, project name, circuit family, expected
solution, coordinate, preferred component identity, or hard-coded net.

## Deterministic bounded search

The V21 planner uses canonical obligation and operation ordering, canonical
graph hashes, graph-plus-obligation state keys, duplicate and ancestor-cycle
rejection, conservative dominance pruning, and explicit depth, width, work,
candidate, and retained-memory limits. Worker count may change execution
parallelism only; it cannot change ordering, selected evidence, or hashes.

An accepted child must preserve every satisfied critical obligation and must
strictly improve the ordered structural vector or the diagnosed electrical
penalty. Plateau exploration is bounded and cannot dominate a strictly better
state.

## Provenance and refusal

Every proposed operation records the failed invariant, obligation, graph
scope, operation kind, before/after hashes, parent hash, work consumption, and
accept/reject reason. V20 admission decisions remain attached to every
simulation attempt.

V21 fails closed with stable diagnostics for no applicable operation, a
contradictory path, invalid terminal/domain/observation relationships, a
duplicate or cycle, a frozen bound, or work-budget exhaustion. A diagnostic
rename is not an advancement.

## Evaluation boundary

V21 reuses the authenticated public V10 corpus. It has no held-out access
surface. Before evaluation, evaluator source, environment, selected
population, advancement rules, case/replay counts, and search limits are
sealed. All 24 public cases run exactly twice in manifest order. Historical
V18–V20 source, reports, manifests, and evidence remain byte-identical.

`source_report_sha256` is the SHA-256 digest of the report file bytes;
`source_report_hash` is the report's canonical content hash. The population
file is a selection freeze, not an evaluator case manifest: its
`selected_cases` array intentionally contains only the eight cases carrying
the selected V20 topology frontier. The evaluator continues to take all 24
cases, in order, from the authenticated corpus manifest.

Material improvement requires at least three selected cases across at least
two reporting domains to pass or advance beyond topology exhaustion to a
strictly later typed blocker. No capability-expansion phase follows until this
milestone is merged.
