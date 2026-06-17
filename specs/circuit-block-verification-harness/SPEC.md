# Circuit Block Verification Harness Specification

## 1. Purpose

Build a verification harness that turns KiCadAI's circuit blocks into a
repeatable, machine-readable, regression-tested design corpus.

The current block system can define blocks, instantiate schematic operations,
compose block requests, emit PCB realization metadata, and generate example
projects. The missing layer is durable evidence that each block is electrically
and structurally meaningful across schematic, PCB, transfer, placement, routing,
and validation surfaces.

The harness must answer this question for every supported block:

```text
Given this block request, what exact schematic/PCB output is expected, what
validation evidence proves it, and what failures should block autonomous use?
```

## 2. Context

Existing foundations:

- `internal/blocks` defines block metadata, parameters, components, nets,
  ports, validation rules, schematic emission, composition, component selection,
  and PCB realization.
- Built-in blocks exist for:
  - `led_indicator`;
  - `voltage_regulator`;
  - `mcu_minimal`;
  - `usb_c_power`;
  - `i2c_sensor`;
  - `opamp_gain_stage`;
  - `connector_breakout`.
- `examples/blocks/` contains generated block projects and request JSON.
- `internal/writercorrectness` can validate project structure, schematic parse,
  generated connectivity, schematic-to-PCB transfer, PCB pad-net correctness,
  copper/zone net references, and optional KiCad round-trip evidence.
- `internal/libraryresolver` can provide symbol and footprint evidence.
- `internal/components` can validate component selection evidence.
- `internal/kicadfiles/checks` provides ERC/DRC feedback where KiCad CLI is
  available.
- `design create` can orchestrate block composition, schematic generation, PCB
  realization, placement, routing, and writer correctness stages.

The harness should connect these pieces into block-scoped proof.

## 3. Goals

The verification harness must:

- define a stable fixture format for block verification cases;
- generate or load block requests deterministically;
- instantiate each block through the same public block APIs used by workflows;
- optionally write schematic-only and schematic-plus-PCB projects;
- run writer correctness checks against generated artifacts;
- compare generated block summaries to golden expectations;
- validate expected nets, references, ports, footprints, routes, zones, and
  constraints;
- record verification level and readiness per block;
- distinguish passing, warning, blocked, skipped, and unsupported evidence;
- produce JSON reports usable by AI agents;
- support optional KiCad-backed ERC/DRC checks without requiring KiCad in
  default tests;
- make it difficult for an unverified block to claim fabrication readiness.

## 4. Non-Goals

This project does not:

- implement new circuit blocks;
- implement a new placement or routing engine;
- replace writer correctness, board validation, ERC, or DRC checks;
- guarantee manufacturing readiness;
- require external KiCad libraries in default tests;
- perform natural-language planning;
- repair broken designs automatically.

Closed-loop repair and manufacturing export remain follow-on projects.

## 5. Definitions

### 5.1 Verification Case

A verification case is a checked-in test definition for one block request.

It includes:

- case ID;
- block ID;
- request parameters;
- expected references;
- expected symbols and footprints;
- expected exported ports;
- expected nets and pin memberships;
- expected PCB placements;
- expected local routes;
- expected zones or copper expectations;
- expected validation outcome;
- optional ERC/DRC expectations;
- fixture classification.

### 5.2 Verification Manifest

A manifest is the machine-readable case file. It should live under a stable
fixture directory such as:

```text
internal/blocks/testdata/verification/<case-id>/manifest.json
```

The manifest owns the test contract. Generated `.kicad_*` outputs may be used
as examples or optional goldens, but the manifest should be the primary source
of expected semantic evidence.

### 5.3 Verification Run

A verification run executes one or more manifests through the harness and
returns a structured report.

Stages:

1. load manifest;
2. validate request;
3. instantiate block;
4. compose when needed;
5. write project when requested;
6. run semantic assertions;
7. run writer correctness;
8. optionally run ERC/DRC;
9. aggregate evidence and readiness.

### 5.4 Semantic Assertions

Semantic assertions are block-specific checks that do not depend on KiCad CLI.

Examples:

- `R1` exists and has `Device:R`;
- `D1` exists and has `Device:LED`;
- net `LED_A` connects resistor pin `2` to LED pin `1`;
- connector pin `1` exports `VIN`;
- regulator output capacitor is on `VOUT`;
- op-amp feedback resistor connects output to inverting input;
- USB-C CC resistors connect CC pins to ground or pull-down values;
- local route `led_series` exists and is required.

### 5.5 Evidence Level

Evidence levels should remain conservative:

- `definition_only`: block definition validates, but no generated output is
  proven.
- `schematic_verified`: schematic output matches semantic expectations and
  parses.
- `transfer_verified`: schematic-to-PCB transfer preserves expected refs,
  footprints, pads, and nets.
- `pcb_verified`: generated PCB passes writer correctness expectations.
- `erc_drc_verified`: optional KiCad ERC/DRC evidence exists and passes or
  matches allowlisted expectations.
- `reference_verified`: compared against a curated known-good KiCad project or
  external reference fixture.

A block may only claim a level that the harness has proven for the current
case.

## 6. Manifest Format

Initial JSON shape:

```json
{
  "id": "led_indicator_default",
  "block_id": "led_indicator",
  "description": "Default status LED with series resistor",
  "acceptance": "connectivity",
  "request": {
    "params": {
      "supply_voltage": "3.3V",
      "led_forward_voltage": "2.0V",
      "led_current": "5mA"
    }
  },
  "expected": {
    "references": ["R1", "D1"],
    "components": [
      {
        "role": "resistor",
        "ref_prefix": "R",
        "symbol_id": "Device:R",
        "footprint_id": "Resistor_SMD:R_0805_2012Metric"
      }
    ],
    "ports": [
      {"name": "VCC", "direction": "power"},
      {"name": "GND", "direction": "power"}
    ],
    "nets": [
      {
        "name": "LED_A",
        "visibility": "local",
        "pins": [
          {"ref": "R1", "pin": "2"},
          {"ref": "D1", "pin": "1"}
        ]
      }
    ],
    "pcb": {
      "placements": [
        {"ref": "R1", "x_mm": 10.0, "y_mm": 10.0, "tolerance_mm": 0.01}
      ],
      "required_routes": ["led_series"],
      "required_zones": []
    },
    "writer": {
      "ok": true,
      "allow_unrouted": false
    },
    "erc_drc": {
      "required": false
    }
  }
}
```

Rules:

- `id` must be stable, lowercase, and path-safe.
- `block_id` must resolve to a registered block.
- `request.params` must validate through the block request validator.
- Expected refs may use exact refs or role-based expectations where reference
  allocation is intentionally dynamic.
- Expected nets must identify pin memberships using resolved refs and pin
  numbers.
- Expected placements should support tolerances because placement engines may
  improve while retaining block semantics.
- Expected writer behavior must declare whether blocking issues are allowed.
- Optional KiCad-backed checks must be opt-in and skipped by default.

## 7. Go API

Add a package or subpackage, preferably:

```text
internal/blocks/verification
```

Primary types:

```go
type Manifest struct { ... }
type Expected struct { ... }
type ExpectedComponent struct { ... }
type ExpectedNet struct { ... }
type ExpectedPCB struct { ... }
type RunOptions struct { ... }
type RunResult struct { ... }
type StageResult struct { ... }
```

Primary functions:

```go
func LoadManifest(path string) (Manifest, []reports.Issue)
func ValidateManifest(manifest Manifest, registry blocks.Registry) []reports.Issue
func RunCase(ctx context.Context, manifest Manifest, opts RunOptions) RunResult
func RunSuite(ctx context.Context, root string, opts RunOptions) RunResult
```

`RunOptions` should include:

- registry;
- output directory;
- overwrite;
- keep artifacts;
- library roots;
- library cache;
- writer correctness options;
- KiCad CLI check options;
- update goldens;
- acceptance level;
- random seed, if any.

## 8. Report Shape

The harness should return `reports.Result` compatible JSON.

Data payload:

```json
{
  "suite": "built_in_blocks",
  "case_count": 7,
  "pass_count": 5,
  "warning_count": 2,
  "blocked_count": 0,
  "cases": [
    {
      "id": "led_indicator_default",
      "block_id": "led_indicator",
      "evidence_level": "pcb_verified",
      "stages": [
        {"name": "manifest", "status": "pass"},
        {"name": "instantiate", "status": "pass"},
        {"name": "semantic_assertions", "status": "pass"},
        {"name": "writer_correctness", "status": "pass"}
      ],
      "artifacts": []
    }
  ]
}
```

Issues must include enough fields to support future repair:

- manifest ID;
- block ID;
- stage;
- role;
- ref;
- net;
- pin;
- path;
- expected value;
- actual value;
- suggested repair category when obvious.

## 9. CLI

Add a user-facing command after the Go API is stable:

```sh
kicadai --json block verify --case internal/blocks/testdata/verification/led_indicator_default/manifest.json
kicadai --json block verify --suite internal/blocks/testdata/verification
kicadai --json block verify --builtins
```

Useful flags:

```text
--output string
--overwrite
--keep-artifacts
--artifact-dir string
--update-goldens
--require-writer
--require-erc
--require-drc
--allow-unrouted
--kicad-cli string
--symbols-root string
--footprints-root string
--library-cache string
--refresh-library-cache
```

Default behavior:

- no external KiCad CLI requirement;
- no external KiCad library roots required;
- deterministic checked-in fixture tests pass;
- optional ERC/DRC stages report skipped unless explicitly required.

## 10. Built-In Cases

Initial suite should include one default case per built-in block:

- `led_indicator_default`;
- `connector_breakout_4pin`;
- `voltage_regulator_3v3`;
- `i2c_sensor_pullups`;
- `opamp_gain_stage_noninverting`;
- `usb_c_power_5v_sink`;
- `mcu_minimal_basic`.

Each case should start with the strongest evidence currently practical. It is
acceptable for early cases to stop at `schematic_verified` or
`transfer_verified` if PCB realization is intentionally partial, but the report
must say so explicitly.

## 11. Validation Rules

### 11.1 Manifest Validation

Block on:

- missing case ID;
- invalid case ID;
- unknown block ID;
- invalid request params;
- duplicate expected refs;
- duplicate expected net names;
- malformed pin membership;
- invalid evidence level;
- expected writer/DRC settings that contradict acceptance.

### 11.2 Schematic Assertions

Block on:

- missing expected component;
- wrong symbol;
- wrong value when value is specified;
- missing footprint assignment when expected;
- missing required port;
- missing expected local or exported net;
- expected pin not connected to expected net.

Warn on:

- extra generated component not listed in expectations, unless strict mode;
- extra local nets, unless strict mode;
- optional port omitted.

### 11.3 PCB Assertions

Block on:

- expected footprint missing from PCB;
- wrong footprint ID;
- expected pad net mismatch;
- required route missing;
- required zone missing;
- writer correctness blocking issue when writer checks are required;
- local route assigned to wrong net.

Warn on:

- placement outside expected tolerance when not acceptance-critical;
- estimated geometry use;
- optional local route missing;
- optional KiCad DRC skipped.

### 11.4 ERC/DRC Assertions

When required:

- missing KiCad CLI is blocking;
- command failure is blocking unless the manifest expects a specific failure;
- unallowlisted ERC/DRC violation is blocking.

When optional:

- missing KiCad CLI produces skipped evidence, not failure;
- raw reports may be retained when requested.

## 12. Golden Outputs

Goldens should be small and semantic:

- manifest validation report;
- block output summary;
- expected net summary;
- writer correctness summary;
- full CLI report for one passing case;
- full CLI report for one intentionally blocked case.

Do not golden entire `.kicad_sch` or `.kicad_pcb` files unless the writer
contract itself is under test. Prefer semantic snapshots to avoid brittle
format churn.

Use an update flag consistent with the existing CLI golden tests.

## 13. Integration With Existing Systems

### 13.1 Blocks

The harness must instantiate blocks through public block APIs and not rely on
private implementation details.

### 13.2 Writer Correctness

Every case that writes a project should run writer correctness by default unless
the manifest explicitly disables it for a narrow reason.

### 13.3 Library Resolver

The harness should use resolver evidence when roots are configured. Default
tests should use compact checked-in fixtures or built-in fallback data.

### 13.4 Design Workflow

Once stable, `design create` should be able to cite block verification evidence
for the blocks it used.

### 13.5 Future Repair Loop

Issue paths and categories must be stable enough for a future repair planner to
map failures back to block request params, symbols, footprints, nets, routes, or
constraints.

## 14. Documentation

Add or update docs with:

- what block verification proves;
- what it does not prove;
- how to run built-in verification;
- how to add a new manifest;
- how to update goldens;
- how optional KiCad ERC/DRC checks are enabled;
- how verification levels map to autonomous design readiness.

## 15. Acceptance Criteria

This project is complete when:

- a manifest schema exists and is documented;
- a Go harness can load and validate manifests;
- one default manifest exists for each built-in block;
- semantic assertions check expected refs, symbols, footprints, ports, nets,
  and pin memberships;
- PCB assertions check expected placements, pad nets, local routes, and zones
  where the block claims PCB evidence;
- writer correctness runs for project-writing cases;
- optional ERC/DRC hooks are available and skipped by default;
- CLI can run one case or the built-in suite;
- golden tests cover at least one passing and one blocked verification report;
- README or docs explain usage and limitations;
- full `go test ./...` passes.

## 16. Risks

Risk: Golden files become brittle and slow development.

Mitigation: Golden semantic summaries, not raw KiCad files.

Risk: Blocks overclaim readiness.

Mitigation: Evidence levels are derived from harness stages, not manually
asserted by block definitions alone.

Risk: Optional KiCad checks make normal tests flaky.

Mitigation: KiCad-backed stages are opt-in and report skipped by default.

Risk: Harness duplicates writer correctness.

Mitigation: Harness owns block-specific expectations; writer correctness owns
generic KiCad file correctness.

Risk: Fixture burden slows block development.

Mitigation: Start with minimal default cases and grow cases as bugs are found.
