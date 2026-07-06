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
- `routing`: `skipped`
- `project_write`: `ok`
- `writer_correctness`: `warning`
- `validation`: `blocked`
- `kicad_checks`: `skipped`

Promotion currently stops at `validation`. This confirms the previous PCB
realization, placement, endpoint-binding, project-write, and writer-correctness
closeout work is still effective enough to reach structural validation.

## Current Routing Policy Evidence

Fixture request state:

- `validation.skip_routing`: `true`
- `validation.skip_kicad_checks`: `true`

The routing stage reports:

- reason: `routing skipped`
- route connectivity endpoints resolved: `40`
- route connectivity unresolved endpoints: `0`
- endpoint contacts proven: `40`
- local routes bound: `20`
- emitted track segments: `20`
- graph-complete route groups: `0`
- blocked route groups: `7`
- missing required endpoints in route-tree evidence: `24`

The next implementation phases should replace this stale skip policy with an
explicit routing decision. Routing may run only when the route request has
concrete endpoint anchors and required-net classification.

## Current Validation Blockers

Structural validation reports disconnected pads, dangling generated route
endpoints, and partially routed or unconnected nets. The key current net-level
blockers are:

- `AMP_OUT_DC_BIASED`: partially routed
- `HP_OUT`: partially routed
- `VCC`: partially routed
- `LOAD_REF`: partially routed
- `AUDIO_IN`: partially routed
- `DRIVER_OUT`: partially routed
- `HP_RET`: partially routed
- `output_upper_drive`: unconnected
- `output_lower_drive`: unconnected
- `output_upper_emitter`: unconnected
- `output_lower_emitter`: unconnected

Generated track endpoint blockers currently appear on:

- `tracks.3.start` on `LOAD_REF`
- `tracks.5.end` on `LOAD_REF`
- `tracks.10.start` on `VCC`
- `tracks.15.start` on `AMP_OUT_DC_BIASED`
- `tracks.16.end` on `HP_OUT`
- `tracks.17.start` on `HP_RET`
- `tracks.18.end` on `DRIVER_OUT`
- `tracks.19.start` on `AUDIO_IN`

## Current KiCad Evidence

KiCad checks are skipped before promotion-quality ERC/DRC evidence can run. The
next valid promotion target is not fabrication readiness; it is writer-level
structural evidence that is clean enough for KiCad ERC/DRC to become the next
authority.
