# AI Design Workflow CLI Specification

## 1. Purpose

The AI Design Workflow CLI adds a high-level command that turns structured
design intent into a KiCad project:

```sh
kicadai design create --request request.json --output ./out/project
```

The command should orchestrate the lower-level systems KiCadAI already has:

- circuit block selection and parameterization;
- schematic generation;
- footprint assignment;
- schematic-to-PCB transfer;
- PCB block realization;
- placement;
- routing;
- project writing;
- ERC/DRC and connectivity validation;
- structured feedback for repair loops.

The goal is not to hide complexity behind a fragile one-shot generator. The
goal is to create a deterministic workflow surface that an AI agent can call,
inspect, repair, and rerun until the generated design reaches the requested
acceptance level.

## 2. Current Foundation

KiCadAI already has the following building blocks:

- `internal/blocks` for reusable circuit blocks, block composition, schematic
  transactions, PCB realization metadata, and block PCB realization output.
- `internal/libraryresolver` for KiCad symbol/footprint discovery and
  compatibility checks.
- `internal/pinmap` for symbol-to-footprint readiness checks.
- `internal/schematicpcb` for schematic-to-PCB transfer.
- `internal/placement` for component placement requests, placement results,
  quality metrics, and `place_footprint` operation emission.
- `internal/routing` and `internal/routingadapters` for route requests, routed
  copper, route operations, and route diagnostics.
- `internal/transactions` for validation, planning, and project apply.
- `internal/kicadfiles/designapi` and lower-level writers for KiCad project,
  schematic, and PCB files.
- `internal/boardvalidation` for connectivity-first PCB validation.
- `internal/kicadfiles/checks` for KiCad CLI ERC/DRC execution and parsed
  feedback when `kicad-cli` is available.
- CLI command families for `block`, `place`, `route`, `transaction`, `check`,
  `validate`, `evaluate`, and `inspect`.

The missing layer is a design workflow coordinator that can call these pieces
in a predictable order and preserve enough intermediate state for AI feedback
and repair.

## 3. Goals

The AI Design Workflow CLI must:

- define a stable request schema for design creation;
- support explicit block-composition requests first;
- support higher-level intent fields without requiring free-form natural
  language inside the core deterministic workflow;
- choose or validate circuit blocks and block parameters;
- generate a schematic transaction;
- assign footprints through block metadata and library resolver evidence;
- realize block-local PCB fragments;
- combine fragments into a board-level placement/routing candidate;
- write a KiCad project directory;
- run deterministic validation after each major stage;
- run optional KiCad ERC/DRC checks when requested and available;
- return machine-readable feedback that identifies the failed stage, offending
  block, operation, reference, net, and suggested repair action where possible;
- distinguish draft output from fabrication-readiness claims;
- be useful to AI agents without requiring hidden state or manual GUI actions.

## 4. Non-Goals

The first implementation does not need to:

- interpret arbitrary natural-language design prompts directly;
- perform component sourcing, pricing, or manufacturer part selection;
- solve dense production autorouting;
- certify fabrication readiness without KiCad DRC evidence;
- handle high-speed, RF, impedance, safety, thermal, or compliance-sensitive
  design rules beyond explicit constraints;
- replace existing lower-level CLI commands;
- require a running KiCad GUI;
- require network access.

## 5. Command Surface

### 5.1 Primary Command

```sh
kicadai --json design create \
  --request request.json \
  --output ./out/project \
  --overwrite
```

`--json` is required for the initial implementation, consistent with other
structured commands.

### 5.2 Useful Flags

The command should reuse existing flags where possible:

- `--request`: required request JSON path.
- `--output`: required output project directory.
- `--name`: optional project name override.
- `--seed`: deterministic seed.
- `--overwrite`: allow replacing existing output.
- `--kicad-cli`: KiCad CLI executable for ERC/DRC.
- `--timeout`: KiCad CLI timeout.
- `--allowlist`: ERC/DRC allowlist.
- `--require-drc`: require KiCad DRC evidence.
- `--strict-zones`: make missing zone fill evidence blocking.
- `--strict-unrouted`: make unrouted multi-pad nets blocking.
- `--library-cache`, `--refresh-library-cache`, `--symbols-root`,
  `--footprints-root`, `--klc-root`, `--templates-root`: resolver inputs.

New flags may include:

- `--acceptance draft|structural|connectivity|erc-drc|fabrication-candidate`
- `--max-repair-iterations int`
- `--keep-intermediates`
- `--intermediate-dir string`
- `--skip-routing`
- `--skip-kicad-checks`

## 6. Design Request Schema

### 6.1 Top-Level Shape

```json
{
  "version": "0.1.0",
  "name": "sensor_breakout",
  "intent": {
    "summary": "I2C sensor breakout with USB-C power",
    "category": "breakout"
  },
  "board": {
    "width_mm": 50,
    "height_mm": 30,
    "layers": 2,
    "edge_clearance_mm": 1.0
  },
  "libraries": {
    "require_resolver": false,
    "symbol_roots": [],
    "footprint_roots": []
  },
  "blocks": [
    {
      "id": "usb_power",
      "block_id": "usb_c_power",
      "params": {
        "include_fuse": true,
        "include_tvs": true
      }
    },
    {
      "id": "regulator",
      "block_id": "voltage_regulator",
      "params": {
        "input_voltage": "5V",
        "output_voltage": "3.3V"
      }
    },
    {
      "id": "sensor",
      "block_id": "i2c_sensor",
      "params": {
        "i2c_address": "0x48"
      }
    },
    {
      "id": "header",
      "block_id": "connector_breakout",
      "params": {
        "pin_names": ["VCC", "GND", "SDA", "SCL", "INT"]
      }
    }
  ],
  "connections": [
    {"from": "usb_power.VBUS_OUT", "to": "regulator.VIN"},
    {"from": "usb_power.GND", "to": "regulator.GND"},
    {"from": "regulator.VOUT", "to": "sensor.VCC", "net_alias": "VCC_3V3"},
    {"from": "regulator.GND", "to": "sensor.GND"},
    {"from": "sensor.SDA", "to": "header.SDA"},
    {"from": "sensor.SCL", "to": "header.SCL"},
    {"from": "sensor.INT", "to": "header.INT"},
    {"from": "regulator.VOUT", "to": "header.VCC"},
    {"from": "regulator.GND", "to": "header.GND"}
  ],
  "constraints": {
    "route_width_mm": 0.25,
    "clearance_mm": 0.2,
    "prefer_top_layer": true,
    "allow_back_layer": true
  },
  "validation": {
    "acceptance": "connectivity",
    "require_erc": false,
    "require_drc": false,
    "strict_unrouted": true,
    "strict_zones": false
  }
}
```

### 6.2 Schema Rules

- `version` is required.
- `name` must be a valid project name after normalization.
- `blocks[].id` must be unique and stable.
- `blocks[].block_id` must reference a known block.
- `connections[].from` and `connections[].to` use `instance.port` syntax.
- `board.width_mm` and `board.height_mm` must be positive.
- `board.layers` must be `1`, `2`, or a supported future layer count.
- `validation.acceptance` controls which validation failures are blocking.
- Unknown fields should be rejected in strict mode once the schema stabilizes.

### 6.3 Explicit First, AI Later

The first workflow accepts explicit block composition. Later, a planner can
transform higher-level intent into that same explicit structure. The core
workflow should not depend on an LLM to be deterministic or testable.

## 7. Workflow Stages

The command runs a stage pipeline. Every stage emits:

- `name`;
- `status`: `ok`, `warning`, `blocked`, or `skipped`;
- inputs summary;
- outputs summary;
- issues;
- artifacts;
- optional repair suggestions.

### 7.1 Parse And Normalize Request

Responsibilities:

- read `request.json`;
- validate schema;
- normalize project name, block IDs, ports, dimensions, flags;
- merge CLI validation policy overrides;
- generate deterministic workflow seed.

Failure examples:

- malformed JSON;
- duplicate block instance IDs;
- invalid board dimensions;
- unknown acceptance level.

### 7.2 Library Context

Responsibilities:

- optionally load resolver cache;
- optionally refresh resolver cache;
- record resolver availability;
- expose symbol/footprint roots used;
- warn when geometry must be estimated.

The workflow must be able to run without external KiCad libraries for tests,
but it should report lower confidence when resolver evidence is unavailable.

### 7.3 Block Planning And Selection

Initial responsibilities:

- validate explicit `blocks[]`;
- validate each block request;
- instantiate each block;
- compose block ports and connections;
- produce a schematic transaction.

Future responsibilities:

- map higher-level intent to block IDs;
- choose defaults based on board category;
- propose missing support circuitry;
- reject unsafe underspecified requests.

### 7.4 Schematic Generation

Responsibilities:

- build a transaction containing:
  - `create_project`;
  - `add_symbol`;
  - `connect`;
  - `assign_footprint`;
  - `write_project`;
- apply transaction to the output directory;
- preserve operation IDs;
- emit schematic artifact paths.

### 7.5 PCB Fragment Realization

Responsibilities:

- call block PCB realization for each instance;
- apply block-local transforms;
- collect component placements, local routes, keepouts, zones, and constraints;
- report unsupported block behavior;
- reject missing footprint assignments or missing route endpoint pins.

The initial implementation may place block fragments on a simple grid. Later
implementations should use a real floorplanner.

### 7.6 Schematic-To-PCB Transfer

Responsibilities:

- ensure every assigned schematic symbol has a PCB footprint candidate;
- transfer refs, values, nets, and footprint IDs into PCB operations;
- reconcile block realization refs with schematic refs;
- emit board outline operations.

### 7.7 Placement

Responsibilities:

- build a board-level `placement.Request`;
- hydrate footprint bounds and pads when resolver data exists;
- include block placement hints and keepouts;
- place unfixed components;
- emit `place_footprint` operations;
- return placement quality feedback.

### 7.8 Routing

Responsibilities:

- convert placed board output to `routing.Request`;
- include local routes from block realization;
- route required board-level nets;
- emit route operations;
- report partial routes, unrouted nets, clearance failures, and blocked nets.

Initial routing can be skipped or limited to small known-good requests. The
workflow must make skipped routing explicit in stage results.

### 7.9 Project Write

Responsibilities:

- write `.kicad_pro`, `.kicad_sch`, `.kicad_pcb`, `sym-lib-table`, and
  `fp-lib-table` where required;
- use safe output directory handling;
- preserve deterministic IDs where possible;
- emit artifact list.

### 7.10 Validation

Responsibilities:

- run transaction validation before apply;
- inspect generated project;
- evaluate generated project;
- run connectivity-first board validation;
- run KiCad ERC/DRC when configured and available;
- apply allowlists;
- evaluate acceptance policy.

### 7.11 Feedback

Responsibilities:

- collect issues from all stages;
- group issues by stage, block instance, operation ID, ref, net, and artifact;
- include actionable repair hints;
- distinguish:
  - user request errors;
  - unsupported capability;
  - writer bug;
  - placement failure;
  - routing failure;
  - ERC/DRC finding;
  - validation policy failure.

## 8. Output Contract

The command returns a `reports.Result` envelope.

Example data shape:

```json
{
  "ok": false,
  "command": "design create",
  "version": "0.1.0",
  "data": {
    "project": {
      "name": "sensor_breakout",
      "output_dir": "./out/project"
    },
    "acceptance": {
      "requested": "connectivity",
      "achieved": "structural",
      "fabrication_ready": false
    },
    "stages": [
      {
        "name": "placement",
        "status": "ok",
        "summary": {
          "component_count": 12,
          "placed_count": 12
        }
      },
      {
        "name": "routing",
        "status": "blocked",
        "summary": {
          "net_count": 8,
          "routed_count": 7,
          "unrouted_count": 1
        }
      }
    ],
    "feedback": {
      "summary": {
        "blocking_count": 1,
        "warning_count": 2
      },
      "repairs": [
        {
          "stage": "routing",
          "net": "SDA",
          "action": "increase board width or allow back-layer routing"
        }
      ]
    }
  },
  "issues": [],
  "artifacts": []
}
```

## 9. Acceptance Levels

### 9.1 `draft`

Requirements:

- request parsed;
- schematic project written;
- known unsupported behavior reported.

### 9.2 `structural`

Requirements:

- schematic and PCB files parse;
- required project files exist;
- refs and UUIDs are valid;
- footprints assigned for PCB-bearing symbols.

### 9.3 `connectivity`

Requirements:

- `structural` requirements pass;
- net-to-pad assignments are valid;
- no blocking disconnected pads;
- no blocking unrouted required nets;
- required block-local routes are present.

### 9.4 `erc-drc`

Requirements:

- `connectivity` requirements pass;
- KiCad ERC and DRC run successfully;
- unsuppressed ERC/DRC findings are below configured thresholds.

### 9.5 `fabrication-candidate`

Requirements:

- `erc-drc` requirements pass;
- board outline present;
- zones filled or explicitly not required;
- fabrication caveats are empty or accepted;
- result clearly states that human review is still recommended.

## 10. Feedback And Repair Model

Feedback should be structured for iterative AI repair.

Each repair item should include:

- `stage`;
- `severity`;
- `code`;
- `message`;
- `block_id`;
- `instance_id`;
- `operation_id`;
- `refs`;
- `nets`;
- `artifact`;
- `suggested_action`;
- `retry_scope`.

`retry_scope` values:

- `request`: modify design request;
- `block`: modify block selection or parameters;
- `placement`: rerun placement with changed constraints;
- `routing`: rerun routing with changed constraints;
- `writer`: internal writer issue;
- `external`: requires KiCad CLI, library root, or user review.

## 11. Determinism

The workflow must be deterministic for a fixed:

- request JSON;
- CLI flags;
- library cache;
- KiCadAI version;
- seed.

Deterministic behavior includes:

- block instance ordering;
- generated refs;
- generated UUIDs where controlled by the writer;
- placement tie-breaking;
- route ordering;
- report stage ordering.

## 12. Error Handling

The workflow must:

- never silently skip a requested block;
- never silently downgrade acceptance level;
- never claim fabrication readiness without the required validation evidence;
- keep partial artifacts when `--keep-intermediates` is set;
- remove or avoid incomplete output when safe write semantics require it;
- return `ok=false` for blocking issues.

## 13. Testing Strategy

Tests should cover:

- request schema validation;
- explicit block composition vertical slice;
- LED plus connector breakout generated project;
- regulator plus sensor plus connector generated project;
- placement failure feedback;
- routing partial failure feedback;
- missing footprint feedback;
- missing KiCad CLI behavior;
- fake ERC/DRC runner behavior;
- deterministic output snapshots for stable JSON fields;
- generated project inspect/evaluate/validate loops;
- no network or GUI dependency in normal tests.

Golden tests should use small fixtures and avoid depending on local KiCad
library installations unless guarded behind explicit integration flags.

## 14. Security And Safety

The workflow must:

- only write under the requested output directory;
- require `--overwrite` for existing output directories;
- avoid shelling out except through explicit KiCad CLI runner paths;
- report external tool command paths in metadata;
- avoid embedding secrets or environment tokens in output reports.

## 15. Open Questions

- Should `design create` support pure natural-language `intent.prompt` in the
  deterministic CLI, or should that remain an external planner concern?
- Should acceptance policies be embedded in request JSON, flags, or both?
- Should block fragment placement start with simple grid transforms or a
  dedicated floorplanner package?
- How should optional block subcomponents be represented when schematic
  instantiation conditionally emits them?
- What is the minimum validation evidence required before the CLI can use the
  phrase `fabrication-candidate`?

