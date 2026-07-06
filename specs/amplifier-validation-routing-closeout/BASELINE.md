# Amplifier Validation and Routing Baseline

Captured from:

```sh
GOCACHE=$(pwd)/.cache/go-build \
  go run ./cmd/kicadai \
  --request examples/design/kicad-backed/class_ab_headphone_protected.json \
  --output examples/generated/amplifier_validation_routing_baseline \
  --overwrite \
  design create
```

The generated output directory is intentionally ignored.

## Current Stage Progression

The protected Class AB headphone fixture reaches:

- `block_planning`: `ok`
- `component_selection`: `warning`
- `schematic`: `ok`
- `schematic_electrical`: `ok`
- `pcb_realization`: `ok`
- `placement`: `ok`
- `routing`: `blocked`
- `project_write`: `skipped`
- `writer_correctness`: `skipped`
- `validation`: `skipped`
- `kicad_checks`: `skipped`

Promotion currently stops at `routing`. This confirms the previous schematic,
PCB realization, placement, and endpoint-binding closeout work is still
effective enough to attempt required inter-block route-tree completion.

## Current Routing Evidence

Fixture request state:

- `validation.skip_routing`: `false`
- `validation.skip_kicad_checks`: `true`

The routing stage reports:

- route connectivity endpoints resolved: `40`
- route connectivity unresolved endpoints: `0`
- endpoint contacts proven: `40`
- local routes bound: `20`
- emitted track segments: `20`
- required inter-block nets: `7`
- graph-complete required inter-block nets: `6`
- partial required inter-block nets: `1`
- proven required inter-block endpoints: `23 / 24`
- required-net classification missing endpoints: `1`
- repairable route-tree failures: `2`
- route-tree missing endpoint traces: `1`
- route-tree repair hints: VCC-only, repairable `graph_split`

## Current Route-Completion Blocker

Routing is enabled and reports explicit required-net classification:

- `AMP_OUT_DC_BIASED`: complete, `2 / 2` endpoints proven
- `AUDIO_IN`: complete, `2 / 2` endpoints proven
- `DRIVER_OUT`: complete, `2 / 2` endpoints proven
- `HP_OUT`: complete, `2 / 2` endpoints proven
- `HP_RET`: complete, `2 / 2` endpoints proven
- `LOAD_REF`: complete, `9 / 9` endpoints proven
- `VCC`: partial, `4 / 5` endpoints proven, missing `output.3`

The first remaining blocker is VCC route-tree/contact completion:

- `ROUTE_GRAPH_INCOMPLETE` on
  `design.inter_block_contact.nets[5].endpoints[1].segment`
- `VALIDATION_FAILED` on
  `design.inter_block_route_groups["VCC"].branches[1].nets.VCC`
- missing endpoint trace: `VCC` endpoint `output.3`, instance `output`, graph
  status `split`, with nearest same-net access evidence
- route-tree repair action: repairable `graph_split` hint for `VCC`, scoped to
  routing, with action `connect same-net graph components for this route group`

Project write, writer correctness, structural validation, and KiCad checks are
skipped with reason `routing did not complete` until this blocker is resolved.
Promotion keeps `route_completion` as the failed gate and reports downstream
writer, connectivity, and KiCad gates as skipped until the VCC route-tree
contact graph is complete.

## Current KiCad Evidence

KiCad checks are skipped before promotion-quality ERC/DRC evidence can run. The
next valid promotion target is not fabrication readiness; it is writer-level
structural evidence that is clean enough for KiCad ERC/DRC to become the next
authority.
