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

Amplifier inventory is intentionally broader than the currently implemented
amplifier blocks. The roadmap-backed family now names input buffer, gain stage,
bias network, Class AB output pair, output protection, supply decoupling,
headphone output, and speaker output entries. Several of those entries are
explicit `unsupported` readiness records that point to the existing narrow
`opamp_gain_stage`, `class_ab_output_stage`, or `headphone_output_protection`
implementations where appropriate. Treat those entries as AI-facing capability
boundaries: headphone-scale slices may be modeled, while speaker and power
amplifier requests remain blocked until SOA, thermal, protection, layout, and
KiCad-backed evidence are implemented.

`amplifier_input_buffer` is the first implemented family-level front-end
contract. It emits a passive AC-coupled input conditioning and bias-reference
fragment with input impedance and high-pass cutoff evidence. It is structural
connectivity evidence only; active buffering, noise review, source impedance
matching, and fabrication-readiness claims remain blocked until later amplifier
phases.

`amplifier_bias_network` is implemented as a structural two-diode Class AB
headphone bias string with rail feed resistors, exported `BIAS_P`/`BIAS_N`
nodes, and an `AMP_OUT` placement anchor for downstream output-pair alignment.
It intentionally blocks VBE multiplier requests, speaker or power-amplifier
use, numeric quiescent-current claims, and unconstrained thermal placement
until simulation-backed and KiCad-backed evidence exists.

`class_ab_output_pair` is implemented as a structural headphone-class
complementary emitter follower using the verified MMBT3904/MMBT3906 seed pair,
emitter resistors, rail connections, and load-reference anchoring. It estimates
peak output current and resistor dissipation for evidence reporting, but blocks
speaker/power-amplifier use and any request outside the derated low-current
envelope until SOA, thermal, copper-width, and protection evidence mature.

`headphone_output_protection` and `headphone_output_connector` now form the
load-interface slice for headphone designs. The protection block emits an
AC-coupled output fragment, calculates output high-pass cutoff, checks coupling
capacitor voltage margin, and blocks speaker/bridge/fault-protection claims.
The connector block provides a mono three-pin board-edge interface with
`HP_OUT`, `LOAD_RET`, and `LOAD_REF` anchors. Speaker output connectors remain
unsupported until active DC fault protection, current, and thermal evidence are
available.

`amplifier_supply_decoupling` provides the local rail evidence slice for active
amplifier stages. It emits VCC-to-GND ceramic decoupling, optional local bulk
capacitors, and dual-supply VEE decoupling when requested. The block checks rail
polarity assumptions through positive rail-voltage validation and requires
capacitor voltage rating to be at least 1.5x the rail voltage. This is
structural placement/connectivity evidence, not full power-integrity proof.

The amplifier package also provides a simulator-neutral Class AB headphone
simulation model. The current output artifact is a SPICE3/ngspice-oriented
netlist with parameter-derived feedback, output-coupling, load, rail, and
simple op-amp/output-device models; an optional runner can normalize
measurements for gain, high-pass cutoff, output DC offset, quiescent current,
load current, output swing, and stability margin. When a workflow includes a
`simulation` stage, the promotion process uses that evidence to gate
candidate/pass readiness. The simulation stage artifacts are
`.kicadai/amplifier-simulation-netlist.cir`,
`.kicadai/amplifier-simulation.json`, and optional raw simulator output at
`.kicadai/amplifier-simulation-raw.txt`. Missing simulator configuration skips
cleanly unless the fixture or workflow explicitly requires simulation. This is
not a substitute for KiCad ERC/DRC, SOA/thermal analysis, or
speaker/power-amplifier protection proof.

```sh
kicadai block list
kicadai block show led_indicator
kicadai --request ./examples/blocks/requests/led_indicator.json block instantiate led_indicator
kicadai --request ./examples/blocks/requests/led_indicator.json block realize-pcb led_indicator
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
kicadai --builtins block verify
kicadai --case ./internal/blocks/testdata/verification/led_indicator_default/manifest.json block verify
kicadai --suite ./internal/blocks/testdata/verification --output ./out/block-verification --overwrite block verify
kicadai --builtins --kicad-corpus --kicad-corpus-tier smoke block verify
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
kicadai block list
kicadai block show led_indicator
kicadai \
  --request examples/blocks/requests/led_indicator.json \
  --output ./out/led_indicator \
  --name led_indicator \
  --overwrite \
  block instantiate led_indicator
kicadai \
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

The `voltage_regulator` block is tied into verified component selection for the
current fixed 3.3 V LDO catalog slice. Its regulator, input-capacitor, and
output-capacitor component roles derive selection requirements from block
parameters such as `output_voltage`, `input_voltage`, `output_current`,
`input_voltage_min`, `input_voltage_max`, `input_capacitance`,
`output_capacitance`, `enable_mode`, and package
preferences. Capacitance parameters use the same parsed value strings as the
catalog and examples, such as `10u`, `100n`, or explicit SI farad values.
Today, `output_voltage` acts as a selection constraint: requests outside the
verified 3.3 V catalog coverage must block for connectivity-oriented
acceptance instead of pretending another fixed voltage is proven.

Supported regulator profiles are intentionally narrow:

- AMS1117-family fixed 3.3 V SOT-223 with VIN/VOUT/GND pins.
- AP2112K-3.3 SOT-23-5 with VIN, VOUT, GND, EN tied to VIN, and NC marked with
  an explicit KiCad no-connect marker.

AP2112K is selected by the intent planner only for 3.3 V rails fed from inputs
at or below 6 V and at or below 150 mA. This conservative automatic-selection
limit is lower than the part's catalog current rating because the current
planner does not yet prove SOT-23-5 thermal dissipation against copper area and
ambient temperature. `enable_mode: tied_input` is required
for the AP2112K block profile until exported enable-control ports are modeled.
`enable_mode: none`, ADJ variants, BYP variants, and manufacturer
do-not-connect pins without explicit evidence block rather than guessing.
Optional power-LED roles are active only when `include_power_led` is true, so
omitted indicator LEDs do not force unnecessary component selection. The
resulting selections are written into the generated transaction and reported in
the `component_selection` stage. Regulator and capacitor selections now include
structured evidence summaries: AMS1117 reports ESR-window stability review
requirements, AP2112K reports ceramic-stable output-cap status plus review
requirements, and selected MLCC capacitors report DC-bias/effective-capacitance
review status. Fabrication-candidate workflows block on unproven stability,
thermal, or derating evidence.

The generated block examples are structural schematic/project outputs; they are
not yet fabrication-ready PCB designs. See
[docs/circuit-block-library.md](circuit-block-library.md) for request
formats, verification levels, resolver requirements, examples, AI usage, and
known limitations. The current release-readiness gap matrix is in
[docs/circuit-block-readiness.md](circuit-block-readiness.md).
