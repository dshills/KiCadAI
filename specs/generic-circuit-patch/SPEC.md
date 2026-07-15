# Generic Circuit Graph Patch Specification

Date: 2026-07-15
Status: Proposed

## Purpose

`generic-circuit-v1` already provides an AI-facing graph, strict preflight, and
provider-free project creation. This specification adds a constrained mutation
contract so an agent can correct a rejected graph from machine-readable
diagnostics without editing KiCad files, invoking a provider, or applying
arbitrary JSON Patch operations.

The patch result is always a new normalized graph. It must be strict-decoded
and evaluated by the existing catalog resolution, schematic lowering,
readability, placement, and routing path before it is declared ready to write.

## Goals

- Give agents a deterministic `circuit patch` command between preflight and
  `circuit create`.
- Use typed selectors and graph-local IDs rather than JSON Pointer paths.
- Preserve component identity, project identity, and trusted catalog authority.
- Fail closed before graph or KiCad-project output for unsupported or unsafe
  changes.
- Return enough evidence for an agent to correct only the reported scope.

## Non-Goals

- Natural-language repair, provider retries, arbitrary JSON Patch, or scripts.
- Automatically choosing a replacement component or changing electrical intent.
- Editing KiCad projects, generated files, or library records.
- Claiming ERC, DRC, round-trip, fabrication, analog, thermal, or safety pass
  without the existing evidence gates running.
- Generic graph topology synthesis beyond the currently supported contract.

## CLI Contract

```sh
kicadai circuit patch \
  --request graph.json --patch changes.json --output corrected-graph.json
```

`circuit patch` reads two strict JSON documents and, only after all operations
and the corrected graph pass validation, writes `--output`. It never writes a
KiCad project. `circuit create` remains the only direct-graph writer.

The command response includes:

- input graph SHA-256 and normalized patch operations;
- corrected normalized graph and critical graph projection;
- preflight data and `ready_for_write`;
- changed critical projection fields;
- structured, stage-tagged diagnostics and retry scopes;
- remaining external evidence requirements.

The command returns a non-success result for a rejected patch or a corrected
graph that is not ready. No output graph is created in either case.

## Patch Document

The v1 patch schema is `kicadai.circuit-patch.v1`, version `1`:

```json
{
  "schema": "kicadai.circuit-patch.v1",
  "version": 1,
  "operations": [
    {
      "op": "replace_endpoint",
      "net": "FILTER_IN",
      "endpoint": {"component": "r1", "selector_kind": "symbol_pin", "selector": "999"},
      "replacement": {"component": "r1", "selector_kind": "symbol_pin", "selector": "1"}
    }
  ]
}
```

Unknown fields, duplicate operation identities, invalid typed values, trailing
JSON, documents above the bounded size, and unsupported operation names are
schema failures. Operations are applied in the supplied array order so an agent
can make an explicit remove-then-add repair. Each operation's typed fields are
normalized, and the same graph plus ordered patch produces identical output;
the final corrected graph, not a transient intermediate state, is the
validation boundary.

## Supported Operations

### `replace_component`

Targets one immutable graph instance `component` ID. The replacement may change
only the catalog selector `component_id`, `variant_id`, `query`, `value`,
`symbol`, `footprint`, and `units`. Exactly one of catalog `component_id` or
`query` must remain present; both or neither are an operation error. The graph
instance ID, reference, role,
population, ratings, required functions, manufacturer, MPN, properties, and
extensions are immutable in v1. A replacement selector must still resolve to a
catalog record compatible with that immutable role and reference; it cannot
silently turn `R1` into a capacitor.

### `replace_endpoint`

Targets an exact endpoint inside one named net and replaces it with a typed
endpoint on the same component instance. The operation is appropriate for
pin/function/unit diagnostics. It cannot rename the net, change its role, move
a connection to a different component, or silently remove another endpoint.
An endpoint may be transiently present in a conflicting net or no-connect while
ordered operations apply, but the final strict graph must contain it in exactly
one allowed location.

### `add_net_endpoint` and `remove_net_endpoint`

Target one existing net and add or remove one typed endpoint. Duplicate
endpoints and removal of a non-existent endpoint are operation errors. Nets
with fewer than two endpoints are not pruned or implicitly removed; they are
rejected by the final strict graph validation and preflight.

### `add_no_connect` and `remove_no_connect`

Add or remove one typed endpoint from explicit `no_connects`. The existing
strict graph rules remain authoritative for conflicting connected/no-connect
pins and unit validity.

### `replace_pcb_placement`

Targets one component placement and changes only `region`, `near`, `edge`,
`priority`, and `max_distance_mm`. The component identity and region identity
remain immutable. The corrected graph must pass placement preflight.

### `replace_pcb_region`

Targets one region ID and changes only `role` and bounded `bounds`. Region IDs
are immutable; board dimensions remain graph-level immutable data.

### `replace_policy`

Changes only one explicitly named boolean in `policy`. Unsupported policy
fields fail closed. Policy cannot relax catalog, graph, connectivity, writer,
or external validation requirements.

## Immutable Data

Patch v1 cannot change graph `schema`, `version`, project name/title/
description/acceptance/board, component IDs/references/roles, net names/roles
or electrical constraints, buses, power flags, schematic flow/lanes/groups/
placements/rules, zones, keepouts, extensions, or any trusted catalog evidence.
Adding and removing complete components or nets is deliberately deferred. All
endpoint operations target an existing net; a missing net must be repaired by a
new reviewed graph revision rather than implicit net creation. These operations
have broad effects on layout, electrical completeness, and routing and require
a separate reviewed contract.

## Safety And Validation

1. Strict-decode the input graph and patch independently.
2. Normalize and validate patch operation IDs and immutable-field boundaries.
3. Apply each operation to an in-memory clone only.
4. Strict-encode and strict-decode the corrected graph to enforce the same
   parser and normalization contract used for agent input.
5. Run existing preflight. Catalog selection, library evidence, schematic
   electrical/readability, placement, and routing remain authoritative.
6. Write the corrected graph atomically only when every prior stage passes.
   Output uses the existing CLI's explicit user-authorized path model and is
   never derived from graph data.

The command never relies on project names, fixture paths, generated references,
or hard-coded coordinates. Patch diagnostics use stable codes, paths, stages,
suggestions, and retry scopes so agents can repair the smallest scope.

## Evidence And Determinism

The critical projection contains project identity, normalized component
selection fields, nets/endpoints, no-connects, PCB regions/placements, and
policy. The response exposes before/after projections and changed projection
paths. Repeated identical input graph and patch documents must produce bytewise
identical corrected graph JSON and semantically identical preflight evidence.

`ready_for_write` means only strict graph, catalog, schematic, placement, and
routing preflight gates passed. Writer correctness and internal connectivity
run after `circuit create`; KiCad ERC/DRC and normalized KiCad round-trip remain
external requirements unless specifically invoked through existing flags.

## Test Strategy

- Unit tests: strict patch parse, normalization, every permitted operation,
  immutable-field rejection, duplicate/conflicting operations, and no mutation
  of the input document.
- CLI tests: invalid component, pin, multi-unit, and placement graphs repaired
  to ready; failed patch leaves no output graph/project.
- Regression corpus: RC, protected USB-C LED, BMP280, LMV321, dual-LMV321, and
  LM358 graphs wherever their existing catalog/library fixtures allow direct
  preflight.
- End-to-end: failing graph -> patch -> ready preflight -> direct create,
  deterministic repeated outputs, and optional KiCad-backed BMP280 path.

## Open Questions

- Add/remove complete components and nets should be considered only after this
  bounded replacement vocabulary has stable agent evidence.
- A future patch provenance artifact may record the external agent identity,
  but no secrets, prompts, or credentials belong in it.
