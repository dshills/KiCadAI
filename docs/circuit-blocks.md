# Circuit Blocks

Reusable verified block workflows and block-library CLI behavior.

### Circuit Blocks

The block library contains reusable schematic templates for LED indicators,
connector breakouts, voltage regulators, I2C sensors, op-amp gain stages,
USB-C power input, a minimal ATmega328P-A system, crystal and canned
oscillators, reset/programming headers, 5 V ESD shunts, and series-diode
reverse-polarity input protection. Blocks can be listed, inspected, validated,
instantiated, composed, and now realized as PCB fragments.

Built-in blocks now declare machine-readable electrical rules, PCB rules, local
route requirements, placement/proximity constraints, verification evidence, and
readiness gaps. `design create` includes that block readiness in its
`block_planning` stage so agents can explain selected blocks, missing evidence,
required local routes, and known limitations before writing a project.

```sh
kicadai --json block list
kicadai --json block show led_indicator
kicadai --json --request ./examples/blocks/requests/led_indicator.json block instantiate led_indicator
kicadai --json --request ./examples/blocks/requests/led_indicator.json block realize-pcb led_indicator
```

`block realize-pcb` returns:

- the normal block instantiation output with schematic transactions;
- `realization.components[]` with refs, footprints, role names, and relative
  PCB placements;
- `realization.entry_anchors[]` with `id`, `port` (the block-level logical
  port associated with the anchor), `net_name`, `description`,
  block-origin-relative `placement.x_mm` and `placement.y_mm`, and string
  `placement.layer` values such as `"F.Cu"` for block-boundary entry/exit
  points such as connector-adjacent ESD inputs and protected power-path
  endpoints;
- `realization.local_routes[]` with `id`, `net_name`, endpoint refs/pins, and
  endpoint objects that use either component refs/pins or anchor references
  through `anchor_id`, plus route operations for verified local nets;
- `placement_request`, a ready input for the placement engine.

This is the first PCB-fragment layer for the circuit block library. It does not
yet claim fabrication readiness for complete boards; global block composition,
board outline selection, route conflict resolution, zone refill, and KiCad DRC
evidence are still required before generated block PCBs should be treated as
manufacturing candidates.

Circuit blocks also have a verification harness with checked-in manifests for
all built-in blocks. It verifies schematic semantics, PCB placement/pad/route
expectations where declared, PCB realization metadata, internal board
validation, writer correctness when requested, and optional KiCad ERC/DRC
evidence. The oscillator and reset/programming manifests now
assert realized local routes and timing fixtures for local decoupling,
reset/programming route length, enable/control presence, and ground-reference
checks where the current realization model can prove them. ESD and
reverse-polarity protection manifests now require modeled entry-anchor route
and power-path local-route evidence via `expected.pcb.required_local_routes`.
`design create` now adds board-level `anchor_bindings` evidence in the routing
stage when realized blocks expose entry anchors. The workflow discovers placed
physical pad endpoints, derives `board_edge_point` endpoints from edge-constrained
pads, accepts request-declared `external_endpoints`, resolves required
protection anchors to connector, `board_edge_point`, or
`imported_mechanical_point` endpoints,
emits endpoint-to-anchor route operations when both coordinates are known, and
reports bound, missing, ambiguous, invalid, unsupported, net-mismatched, routed,
or not-routable bindings as structured evidence. This binding evidence prevents
synthetic anchors from being mistaken for proven external interfaces, but it is
not a substitute for KiCad DRC, surge/thermal analysis, DFM checks, or
fabrication readiness gates.

In the root object of a `design create` request JSON, request-declared external
endpoints use this shape:

```json
{
  "external_endpoints": [
    {
      "id": "edge_vin",
      "kind": "board_edge_point",
      "net_name": "VIN_RAW",
      "roles": ["power_entry", "edge"],
      "layers": ["F.Cu"],
      "point": {"x_mm": 0, "y_mm": 12.5},
      "edge": "left",
      "required": true
    }
  ]
}
```

Use `kind: "imported_mechanical_point"` for user/importer-supplied interface
coordinates. `id` is required for every external endpoint, normalized by
trimming whitespace, lowercasing, replacing every run of non-`[a-z0-9_]`
characters with `_`, trimming leading/trailing `_`, and must be unique within
the `external_endpoints` array after normalization; missing or duplicate IDs
fail request validation. IDs are used as stable diagnostic and evidence
identifiers, not as references from other request fields yet. The optional
`edge` field is descriptive evidence for
`board_edge_point` endpoints; values normalize to lowercase and the valid values
are `left`, `right`, `top`, and `bottom` in the rectangular board coordinate
space, where `top` is the minimum Y edge and `bottom` is the maximum Y edge.
Non-rectangular boards should use the nearest cardinal direction until
polygon-edge endpoint support exists. `point.x_mm` and `point.y_mm` are
millimeters in the board coordinate frame used by the generated PCB. This
follows KiCad's positive-down Y convention, so smaller Y values are visually
above larger Y values. `roles` describe external-interface intent; useful values
include `connector`, `edge`, `external`, `power_entry`, `mechanical_interface`,
and `castellated`. Layer names are normalized to KiCad canonical names, so
`f.cu` becomes `F.Cu`; unsupported copper or technical layers fail validation.
For physically required endpoints, prefer an explicit copper layer list such as
`["F.Cu"]` or `["B.Cu"]` that matches the real interface copper. Omitted
`layers`, `null`, or `[]` is permissive fallback behavior: it acts as a binding
wildcard that can match an anchor on any available copper layer, but it does not
prove the physical endpoint exists on every copper layer. Optional endpoints can
omit `point` or `net_name` and remain visible as advisory evidence, but they
cannot produce route evidence until both are known. Endpoints marked
`"required": true` fail request validation unless `point` is a non-null object
with finite numeric `x_mm` and `y_mm` values and `net_name` is a non-empty
string.

```sh
kicadai --json --builtins block verify
kicadai --json --case ./internal/blocks/testdata/verification/led_indicator_default/manifest.json block verify
kicadai --json --suite ./internal/blocks/testdata/verification --output ./out/block-verification --overwrite block verify
kicadai --json --builtins --kicad-corpus --kicad-corpus-tier smoke block verify
```

KiCad-backed checks are skipped unless a manifest or flag requires them. Use
`--kicad-cli`, `--require-erc`, `--require-drc`, `--keep-artifacts`, and
`--artifact-dir` when external ERC/DRC evidence is required. Optional ERC/DRC
expectations are visible as skipped when no output directory or KiCad CLI is
available; required ERC/DRC fails verification with an explicit reason. A
skipped optional ERC/DRC stage means the block remains structurally verified by
the built-in harness, but it is not KiCad-clean or fabrication-ready evidence.
The opt-in KiCad corpus currently seeds `led_indicator_default` and
`connector_breakout_4pin` as smoke-tier candidates. Corpus mode emits a
`kicad_corpus` summary and, with `--output`, writes `corpus-summary.json` plus
per-case `reports/corpus-result.json` files. Normal `go test ./...` remains
KiCad-independent by default; real KiCad smoke tests require
`KICADAI_RUN_KICAD_CLI=1` and can use
`KICADAI_KICAD_CLI=/path/to/kicad-cli` for non-default installs.
When checks run, generated project freshness records the project signature and
a separate ERC/DRC check-context signature for the resolved `kicad-cli` path
and version, measurement units, and allowlist contents. Golden report snapshots
live under `cmd/kicadai/testdata/golden/block_verification` and can be
refreshed with:

```sh
go test ./cmd/kicadai -run TestRunBlockVerificationGoldens -update-block-verification-goldens
```

The `./internal/...` paths above are source-tree paths for repository
development. For general use, prefer `--builtins` or pass your own manifest
path through `--case` or `--suite`.

See [docs/circuit-block-verification.md](circuit-block-verification.md)
for evidence levels, manifest structure, and extension guidance.


### Circuit Block Library

The `block` command exposes reusable circuit primitives for AI-assisted
schematic generation. Blocks declare parameters, ports, required libraries, and
verification levels.

```sh
kicadai --json block list
kicadai --json block show led_indicator
kicadai \
  --json \
  --request examples/blocks/requests/led_indicator.json \
  --output ./out/led_indicator \
  --name led_indicator \
  --overwrite \
  block instantiate led_indicator
kicadai \
  --json \
  --request examples/blocks/requests/composed_sensor_breakout.json \
  --output ./out/composed_sensor_breakout \
  --name composed_sensor_breakout \
  --overwrite \
  block compose
```

Current built-in blocks include LED indicator, connector breakout, voltage
regulator, I2C sensor, op-amp gain stage, USB-C power input, MCU minimal
system, crystal oscillator, canned oscillator, reset/programming header, 5 V
ESD protection, and reverse-polarity input protection. These blocks now carry
electrical and PCB rule metadata for required companions, decoupling, pull-ups,
rail compatibility, enable/reset/programming handling, edge constraints,
keepouts, proximity constraints, route priorities, conditional realization, and
required local routes where the current realization model supports them. The
newer protection and timing blocks remain structural/partial: they use verified
seed records and checked-in verification manifests, but need more variants and
stronger KiCad-backed layout evidence before fabrication-readiness claims.

The generated block examples are structural schematic/project outputs; they are
not yet fabrication-ready PCB designs. See
[docs/circuit-block-library.md](circuit-block-library.md) for request
formats, verification levels, resolver requirements, examples, AI usage, and
known limitations. The current release-readiness gap matrix is in
[docs/circuit-block-readiness.md](circuit-block-readiness.md).
