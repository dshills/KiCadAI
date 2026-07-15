# Generic Circuit Repair Guidance Specification

Date: 2026-07-15
Status: Proposed

## Goal

Expose deterministic, fail-closed repair guidance with generic-circuit
preflight results. An AI agent must be able to discover supported patch
operations, inspect a failed preflight, construct a bounded patch, re-run
preflight, and then invoke provider-free project creation without reading
KiCadAI source or editing KiCad files.

## Scope

This specification changes only `generic-circuit-v1` capability output,
preflight result data, and the existing constrained `circuit patch` loop. It
does not add components, circuit blocks, provider dispatch, natural-language
parsing, fixture selection, automatic patch application, or KiCad-file edits.

## Non-Goals

- Automatically choosing or applying a repair.
- Offering executable repairs for electrical, analog, thermal, safety,
  fabrication, KiCad ERC/DRC, or round-trip findings.
- Claiming that preflight implies KiCad-backed or fabrication-ready evidence.
- Deriving alternatives from project names, fixture paths, generated
  references, or coordinate tables.

## Public Contract

`kicadai capability generation --json` gains generic repair-contract metadata:

```json
{
  "generic_repair_contract": {
    "patch_schema": "kicadai.circuit-patch.v1",
    "supported_operations": ["replace_component", "replace_endpoint", "replace_pcb_region"],
    "policy": "preflight reports candidates but never applies them"
  }
}
```

Every `circuit preflight` response gains `repair_options`:

```json
{
  "repair_options": [
    {
      "diagnostic_id": "graph-...",
      "code": "GRAPH_UNIT_INVALID",
      "path": "nets[5].endpoints[2].unit",
      "operation_template": {"op": "replace_endpoint", "net": "GAIN_INPUT"},
      "required_values": ["endpoint", "replacement"],
      "allowed_values": {"replacement.unit": ["A", "B", "P"]},
      "rationale": "endpoint unit is not declared by its component",
      "stage": "connectivity",
      "retry_scope": "connectivity",
      "disposition": "agent_selectable"
    }
  ]
}
```

All lists and maps are serialized deterministically. `operation_template` is a
partial `PatchOperation`, never a free-form JSON Patch. The agent must supply
every listed required value, encode a normal `PatchDocument`, and let `circuit
patch` remain the final authority.

## Guidance Policy

`agent_selectable` guidance is emitted only when all candidates are derived
from trusted catalog or graph evidence and one bounded patch operation can be
described without changing immutable graph identity.

Initial eligible families:

1. Catalog-resolution selection errors: `replace_component` template scoped to
   the existing component ID, with catalog-supported component IDs and variants.
2. Endpoint selector/function errors: `replace_endpoint` template scoped to
   the existing net and component, with catalog-supported selectors for that
   component/unit.
3. Invalid declared unit on an endpoint: `replace_endpoint` template scoped to
   that endpoint, with the component's declared units.
4. Invalid PCB-region bounds: `replace_pcb_region` template scoped to the
   existing region and bounded by the graph board envelope.

Guidance is omitted for unknown or ambiguous component alternatives, net
topology conflicts, duplicate/immutable identity findings, unsupported patch
operations, and every electrical-review or external-evidence finding. Omission
is intentional fail-closed behavior; the original diagnostic continues to
state the required review or retry scope.

## Integration

`generationcapability.BuildDocument` owns the static patch schema, operation
vocabulary, and evidence boundaries. `circuitgraph` owns graph-derived allowed
values. `evaluateCircuitPreflight` invokes the repair planner whenever it has
a decoded graph and one or more stage-tagged diagnostics. `circuit patch` is
unchanged except that it consumes the documented same patch contract.

Provider context and CLI capability output must serialize the same capability
document. Preflight never writes a graph, project, or KiCad artifact.

## Validation and Safety

- Every suggested operation must itself pass `ValidatePatch` once required
  values are supplied.
- No guidance may contain a project name, fixture path, component reference,
  or hard-coded board coordinates unrelated to the graph's own declared
  region/bounds.
- Suggestions are advisory; `circuit patch`, strict decode, catalog
  resolution, lowering, placement, and routing remain mandatory gates.
- Diagnostics without stable ID/code/path/stage/retry scope cannot produce an
  executable candidate.

## Test Strategy

- Unit tests prove deterministic guidance generation and omission of ambiguous
  or forbidden cases.
- Capability tests prove provider and CLI documents have identical repair
  contracts.
- CLI corpus covers unknown component, invalid pin/function, invalid unit, and
  invalid placement-region preflights, then verifies each accepted patch reaches
  ready preflight.
- Negative tests prove unsafe substitutions, conflicts, immutable graph fields,
  and external/electrical findings produce no executable repair option.
- Existing generic RC, protected LED, BMP280, LMV321, dual-LMV321, and LM358
  direct/provider paths remain in the default regression suite. Optional KiCad
  promotion remains environment-gated.

## Acceptance Criteria

- Capability output lets an agent discover patch operation limits without source
  access.
- Preflight emits stable, deterministic, bounded repair options only when safe.
- Every supported corpus repair can pass through `circuit patch` and
  re-preflight without a provider.
- Unsupported and ambiguous conditions fail closed with no executable option.
- Documentation explains `capability -> graph -> preflight -> repair option ->
  patch -> re-preflight -> circuit create -> optional KiCad promotion`.
