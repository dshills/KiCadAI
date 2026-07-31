# Open-Topology Synthesis Gap Audit

Date: 2026-07-30

## Current Boundary

| Area | Existing capability | Missing for this milestone |
| --- | --- | --- |
| Behavioral input | Strict v1-v5 open-set requirements, typed ports/domains, operating cases, events, assertions, and canonical hashes | A contract that carries no objective/provider capability and explicitly binds semantic excitations to observations |
| Architecture generation | Deterministic bounded search over obligations expanded by registered `FragmentProvider` implementations | Search that creates primitive terminal connectivity instead of selecting a pre-authored fragment expansion |
| Catalog evidence | Reviewed symbols, footprints, pin/pad maps, ratings, tolerances, thermal facts, and model claims | A deterministic primitive-only inventory that excludes functional compact models |
| Electrical models | Graph-derived resistor, capacitor, inductor, diode, zener, BJT, MOSFET, op-amp, comparator, source, nonlinear, transient, stability, thermal, and SOA support | Direct binding from generated primitive graphs and semantic external ports into those models |
| Value solving | Preferred-series passive solving and catalog variant alternatives inside registered expansions | Value domains attached to arbitrary generated primitive instances |
| Closed loop | Deterministic candidate evaluation and bounded changes to declared values, parameters, and variants | Addition, deletion, substitution, or redirection of graph structure after a typed diagnosis |
| Lowering | `FragmentRealization` lowers provider payloads to resolved circuit graphs | A provider-independent primitive graph adapter with equivalent provenance and semantic bindings |
| Physical promotion | Placement, routing, connectivity, writer, round trip, ERC, DRC, replay, and promotion bundles | Evidence that generated graph structure—not a named block—survives the complete path |

## Evidence From The Current Tree

- `internal/architecturesearch/search.go` expands semantic obligations only
  through providers selected by capability.
- `internal/architecturesearch/catalog_provider*.go` contains the current
  reusable but named functional expansion families.
- `internal/architecturesearch/realization.go` carries a selected expansion's
  concrete instances and connections into lowering.
- `internal/closedloopsynthesis/model.go` represents repairable variables and
  catalog alternatives but has no candidate-graph delta.
- `internal/simmodel/mna_registry.go` already owns the reviewed primitive
  terminal definitions and graph-derived analysis boundary needed by the new
  lane.
- `internal/compositionlowering/lower.go` and the downstream design workflow
  already provide the required physical and KiCad-backed gates.

## Architectural Decision

The new lane will not add one provider per held-out behavior. It will add a
primitive inventory, canonical candidate-graph kernel, and deterministic
terminal-level search. Passing graphs will be adapted into the existing
architecture/lowering evidence boundary only after topology and value search
has completed.

Named providers remain the preferred, mature path for capabilities already in
their verified envelope. Open-topology synthesis is a separate experimental
path until its own promotion evidence satisfies this specification.

## Untouched Baseline Expectation

The eight new requirements are intentionally unsupported by the current public
contract because they contain no provider objectives. The expected untouched
baseline is therefore zero complete open-topology passes, with all cases
stopping before architecture selection. The Phase 0 baseline test will record
the exact current codes rather than treating an empty provider-backed candidate
as success.
