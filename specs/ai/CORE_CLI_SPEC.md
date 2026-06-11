# KiCadAI Core CLI and AI Integration Specification

## 1. Purpose

This specification defines the next product layer for KiCadAI: a command-line
tool and reusable Go core that let an AI agent inspect, evaluate, modify, and
generate KiCad projects through structured commands.

The immediate focus is the CLI and the internal packages that power it. MCP,
GUI integration, web services, and remote agent protocols are intentionally out
of scope for this specification. Those interfaces can be considered later only
after the CLI feels complete, stable, testable, and useful for real projects.

The intended workflow is:

```text
user goal or AI plan
  -> structured CLI request or transaction file
  -> Go core operation
  -> KiCad-native project/schematic/PCB files
  -> deterministic validation and KiCad-backed checks where available
  -> structured report for human and AI review
```

The tool must make AI-assisted KiCad work evidence-driven. AI should not be
asked to freehand `.kicad_sch` or `.kicad_pcb` files as the normal workflow.
It should use inspection, transaction, generation, and validation commands that
return structured results.

## 2. Goals

### 2.1 Primary Goals

- Let AI and humans inspect existing KiCad schematic and PCB designs.
- Let AI and humans evaluate designs and receive structured feedback.
- Let AI and humans add, remove, and modify schematic and PCB objects through
  safe operations.
- Let AI generate a complete project, schematic, and PCB from a structured
  design request.
- Keep all generated output KiCad-native and openable in KiCad.
- Preserve user-authored KiCad content wherever the writer does not fully model
  a feature.
- Make every operation reproducible, testable, and suitable for CI.
- Return machine-readable errors that an AI can use to correct its next step.

### 2.2 Secondary Goals

- Support iterative AI design loops where the tool reports validation failures
  and the AI proposes corrections.
- Support human review workflows by producing summaries, checklists, reports,
  and artifact paths.
- Support future domain-specific circuit generators such as breakout boards,
  filters, supplies, headphone amplifiers, sensor nodes, and connector boards.
- Maintain compatibility with direct KiCad file generation, KiCad CLI
  round-trip checks, and corpus-driven writer correctness tests.

## 3. Non-Goals

This phase does not attempt to:

- Implement an MCP server.
- Replace KiCad's GUI.
- Hide KiCad files from the user.
- Guarantee electrical correctness of generated circuits without review.
- Implement full autorouting.
- Implement SPICE simulation.
- Provide regulatory, safety, EMC, medical, automotive, aerospace, or mains
  compliance certification.
- Support arbitrary natural-language design directly inside the CLI. Natural
  language may be handled by an AI agent, but the CLI boundary should remain
  structured.

## 4. Existing Foundation

The current project already provides substantial infrastructure that this spec
must build on rather than replace.

### 4.1 Direct KiCad File Writers

Implemented writer areas:

- `.kicad_pro` project writer.
- `.kicad_sch` schematic writer.
- `.kicad_pcb` PCB writer.
- Project directory writer with safe generated-file inventory and overwrite
  behavior.
- Project-local symbol and footprint library table generation.
- KiCad 10-style generated files that open in KiCad without old-format
  warnings after prior writer corrections.

### 4.2 Schematic Writer Coverage

Implemented schematic capabilities include:

- Root schematic files.
- Child schematic files.
- Symbols, properties, pins, pin anchors, instances, and embedded library
  symbols.
- Wires, labels, global labels, hierarchical labels, junctions, no-connects,
  buses, bus entries, polylines, texts, and sheets.
- Raw schematic items for unsupported or future KiCad nodes.
- Stable KiCad item ordering.
- Schematic validation and generated connectivity checks.
- Hierarchical symbol-library validation.
- Hierarchical reference duplicate validation.
- Project-wide UUID uniqueness validation including nested schematic objects.

### 4.3 PCB Writer Coverage

Implemented PCB capabilities include:

- Board headers, layers, setup, nets, footprints, pads, footprint properties,
  footprint text, footprint graphics, drawings, tracks, track arcs, vias, zones,
  dimensions, groups, images, tables, targets, embedded files, component
  classes, and preservation-only raw structures where modeled support is not
  complete.
- Board outline generation through drawing primitives.
- Pad, footprint, zone, route, layer, and dimension validation.
- Generated PCB connectivity validation for disconnected pads, route endpoints,
  vias, track bodies, arcs, overlapping pads, and T-junction-like cases.
- Corpus scanner for KiCad demo files and PCB object coverage reports.
- Round-trip validation against KiCad CLI where available.
- Artifact retention controls for round-trip diffs and summaries.
- Project-wide UUID uniqueness validation including nested PCB footprint
  objects.

### 4.4 High-Level Go Design API

The current `internal/kicadfiles/designapi` builder provides:

- `AddSymbol`
- `Connect`
- `AssignFootprint`
- `PlaceFootprint`
- `Route`
- `AddZone`
- `Design`
- `WriteProject`

It also provides deterministic IDs, net merging, footprint replacement,
default pad generation, side-aware pad layers, through-hole defaults, duplicate
physical pad support, and pad-to-symbol-pin validation.

### 4.5 Existing CLI

The existing `cmd/kicadai` CLI supports lower-level KiCad IPC probes and some
file-generation workflows:

- `config`
- `ping`
- `version`
- `documents`
- `capabilities`
- `plan-led-demo`
- `draw-led-demo`
- `generate-led-demo`
- `generate-project`

This spec extends the CLI into a first-class AI tooling surface.

### 4.6 Existing Test Infrastructure

Existing test assets include:

- Unit tests for writers and validators.
- Golden and fragment tests.
- Project writer tests.
- Round-trip tests with fake KiCad CLI helpers.
- Optional integration tests using real `kicad-cli`.
- PCB corpus scanner tests.
- PCB object correctness fixture tests.
- Design API tests.

## 5. Explicit Gaps in Current Writer and Tooling

This section is intentionally direct. These gaps are not failures; they are the
work needed before AI can safely inspect, modify, and generate arbitrary KiCad
designs.

### 5.1 Missing Full Readers

The project has strong writers, validators, raw preservation hooks, and corpus
scanners, but it does not yet have full structured readers for existing KiCad
projects.

Needed:

- Read `.kicad_pro` into the project model.
- Read `.kicad_sch` into the schematic model.
- Read `.kicad_pcb` into the PCB model.
- Preserve unknown nodes and original ordering during read-modify-write.
- Surface unsupported nodes in inspection reports.

Impact:

- AI can generate new projects now.
- AI cannot yet reliably inspect and modify arbitrary existing user projects
  without a reader/import layer.

### 5.2 Partial Round-Trip Preservation

The writers contain preservation mechanisms, and round-trip validation exists,
but there is not yet a complete general-purpose read-modify-write preservation
engine.

Needed:

- Unknown node retention by source location.
- Preservation of comments where meaningful and possible.
- Stable ordering for imported files.
- Conflict reporting when a transaction modifies a region containing
  unsupported or preserved content.

Impact:

- Generated files are increasingly robust.
- Existing user-authored projects still need cautious import boundaries.

### 5.3 Limited Semantic Inspection

The code can validate modeled files and summarize some PCB structures, but it
does not yet provide high-level inspection outputs designed for AI consumption.

Needed:

- Schematic summary: symbols, refs, values, libraries, footprints, pins, nets,
  sheets, labels, unconnected anchors, and power symbols.
- PCB summary: footprints, pads, nets, board outline, tracks, vias, zones,
  dimensions, drawings, placement, unrouted or disconnected pads.
- Project summary: files, library tables, generator versions, KiCad versions,
  known unsupported features, and validation state.

Impact:

- The writer can generate and validate.
- AI still lacks a concise, stable view of what exists in a project.

### 5.4 Limited Mutation Model

The high-level builder supports creation workflows well, but arbitrary
read-modify-write mutation is not complete.

Needed:

- Transaction operations for adding components to existing designs.
- Transaction operations for removing components and cleaning up nets,
  footprints, routes, zones, and labels.
- Update operations for values, footprints, nets, placement, routes, and board
  geometry.
- Conflict detection for manual edits and unsupported preserved nodes.

Impact:

- AI can build new designs through the builder.
- AI cannot yet safely edit all parts of an existing design.

### 5.5 Missing Structured Error Codes

Validators currently return useful error strings, but AI workflows need stable
codes and precise object paths.

Needed:

- Error code catalog.
- Structured validation issue type.
- Stable object paths such as `schematic.symbols[R1].footprint`.
- Severity levels.
- Suggested corrective action fields.

Impact:

- Humans can understand current errors.
- AI correction loops will be more reliable after structured issues exist.

### 5.6 Missing KiCad ERC/DRC Command Integration as Core CLI Features

Round-trip and integration tests can use KiCad CLI where configured, but the
user-facing CLI does not yet expose complete ERC/DRC workflows.

Needed:

- Discover `kicad-cli`.
- Run schematic ERC where KiCad supports it.
- Run PCB DRC with structured report output.
- Normalize KiCad report formats into KiCadAI issue structures.
- Handle missing KiCad CLI gracefully.

Impact:

- Internal validation exists.
- KiCad-backed validation is not yet a complete user-facing CLI workflow.

### 5.7 Missing Rendering and Fabrication Export

The current project focuses on file writers and validation, not artifact
generation.

Needed:

- Schematic SVG/PDF export.
- PCB front/back SVG export.
- Optional 3D render hooks.
- BOM export.
- Position file export.
- Gerber and drill export.
- Fabrication zip packaging.

Impact:

- Generated projects can be opened in KiCad.
- Human/AI review artifacts need additional CLI support.

### 5.8 Limited Domain-Level Circuit Knowledge

The builder has generic operations. It does not yet know how to create a
breakout board, op-amp stage, regulator section, Class AB amplifier, or other
domain blocks from high-level intent.

Needed:

- Reusable block generators.
- Component-role metadata.
- Net-role metadata.
- Symbol-footprint-pinmap libraries.
- Design-rule presets for common board classes.

Impact:

- AI can call primitives.
- "Make me a breakout board that does these things" requires a higher-level
  generation layer.

## 6. Core Architecture

The CLI should be a thin wrapper around reusable Go packages. The core should
not depend on terminal output.

Recommended package direction:

```text
internal/kicadfiles/...          existing KiCad-native models, writers, validators
internal/kicadfiles/designapi    existing high-level generated-design builder
internal/inspect                 future project/schematic/PCB inspection summaries
internal/evaluate                future structured design feedback and lint checks
internal/transactions            future mutation operation planner and executor
internal/generate                future high-level board/schematic generators
internal/reports                 future structured report and issue formatting
cmd/kicadai                      CLI entrypoint over the reusable packages
```

The CLI should never contain business logic that cannot be called from tests.
Every command should call an internal package function and render the result as
JSON or human text.

## 7. Core Data Contracts

### 7.1 Result Envelope

Every JSON-producing command must return a stable envelope:

```json
{
  "ok": true,
  "command": "inspect-project",
  "version": "0.1.0",
  "data": {},
  "issues": [],
  "artifacts": []
}
```

Failure example:

```json
{
  "ok": false,
  "command": "apply-transaction",
  "version": "0.1.0",
  "data": null,
  "issues": [
    {
      "code": "MISSING_FOOTPRINT",
      "severity": "error",
      "path": "schematic.symbols[R12].footprint",
      "message": "R12 has no assigned footprint.",
      "suggestion": "Assign a footprint before generating the PCB."
    }
  ],
  "artifacts": []
}
```

### 7.2 Issue Model

All validators, inspectors, generators, and transaction executors should return
issues in this shape:

```json
{
  "code": "DISCONNECTED_PAD",
  "severity": "error",
  "path": "pcb.footprints[J1].pads[\"1\"]",
  "message": "Pad J1.1 is assigned to GND but is not connected to any routed copper.",
  "refs": ["J1"],
  "nets": ["GND"],
  "suggestion": "Route GND to J1.1 or mark the pad intentionally unconnected."
}
```

Severity values:

- `info`
- `warning`
- `error`
- `blocked`

### 7.3 Artifact Model

Generated files and reports should be returned as artifacts:

```json
{
  "kind": "kicad_project",
  "path": "output/blink/blink.kicad_pro",
  "description": "Generated KiCad project file"
}
```

Artifact kinds should include:

- `kicad_project`
- `schematic`
- `pcb`
- `symbol_library_table`
- `footprint_library_table`
- `validation_report`
- `roundtrip_report`
- `drc_report`
- `erc_report`
- `preview`
- `bom`
- `gerber`
- `drill`
- `fabrication_package`

### 7.4 Transaction Model

Transactions should be JSON files containing ordered operations:

```json
{
  "name": "add-led-output",
  "project": "demo",
  "operations": [
    {
      "op": "add_symbol",
      "ref": "D1",
      "library_id": "Device:LED",
      "value": "LED",
      "pins": [
        {"number": "1", "x_mm": -5, "y_mm": 0},
        {"number": "2", "x_mm": 5, "y_mm": 0}
      ],
      "at": {"x_mm": 40, "y_mm": 20}
    },
    {
      "op": "assign_footprint",
      "ref": "D1",
      "library_id": "LED_SMD:LED_0805_2012Metric"
    }
  ]
}
```

The transaction executor must report the index of the failing operation when a
transaction fails.

## 8. CLI Command Families

### 8.1 Project Generation

Commands:

```text
kicadai generate project --request request.json --output ./out --json
kicadai generate breakout --request request.json --output ./out --json
kicadai generate example --name led_indicator --output ./out --json
```

Required behavior:

- Create a KiCad project directory using safe writer rules.
- Validate the generated design before writing.
- Return structured artifacts.
- Support `--overwrite`.
- Support deterministic seeds.

Initial implementation should wrap the existing design API and current
`generate-led-demo` / `generate-project` behavior.

### 8.2 Inspection

Commands:

```text
kicadai inspect project ./project --json
kicadai inspect schematic ./project/project.kicad_sch --json
kicadai inspect pcb ./project/project.kicad_pcb --json
```

Required behavior:

- Read or scan the target file or project.
- Return a compact summary suitable for AI context.
- Include unsupported or preservation-only nodes.
- Include validation issues when cheap to compute.

Initial inspection can be scan-based for PCB and generated-design-based for
known generated files. Full structured inspection depends on reader work.

### 8.3 Evaluation

Commands:

```text
kicadai evaluate project ./project --json
kicadai evaluate schematic ./project/project.kicad_sch --json
kicadai evaluate pcb ./project/project.kicad_pcb --json
```

Required behavior:

- Run KiCadAI internal validation where possible.
- Run connectivity checks for generated or readable models.
- Run KiCad CLI checks when requested and available.
- Return structured issues.
- Distinguish between "not supported yet" and "design failed."

Evaluation should be useful even before full KiCad readers exist. For example,
PCB corpus scanning and KiCad CLI parse/DRC checks can provide feedback without
fully importing the board into the model.

### 8.4 Transactions

Commands:

```text
kicadai transaction apply ./project tx.json --json
kicadai transaction plan ./project tx.json --json
kicadai transaction validate tx.json --json
```

Required behavior:

- Validate transaction schema before applying.
- Apply operations in order.
- Stop on first fatal issue.
- Return operation index and issue details.
- Write through the safe project writer.
- Preserve unsupported content where possible.
- Refuse unsafe modifications when preservation cannot be guaranteed.

Initial transaction support should target generated projects first. Existing
projects require reader and preservation work before broad support.

### 8.5 Round-Trip

Commands:

```text
kicadai roundtrip schematic ./project/project.kicad_sch --json
kicadai roundtrip pcb ./project/project.kicad_pcb --json
kicadai roundtrip project ./project --json
```

Required behavior:

- Copy inputs to an artifact workspace.
- Run KiCad CLI upgrade/save path where available.
- Compare normalized output.
- Emit diffs only when requested with artifact retention.
- Return clear skipped status when KiCad CLI is unavailable.

This should wrap the existing `internal/kicadfiles/roundtrip` package.

### 8.6 Export

Commands:

```text
kicadai export bom ./project --output bom.csv --json
kicadai export fabrication ./project --output fab.zip --json
kicadai export preview ./project --output previews --json
```

Required behavior:

- Use KiCad CLI where possible.
- Return artifact paths.
- Normalize errors into structured issues.

This is a later CLI family. It should not block inspection, transactions, or
generation.

## 9. Required Operation Set

The first transaction executor should support operations that map directly onto
the existing `designapi.Builder`.

### 9.1 Creation Operations

- `create_project`
- `set_paper`
- `set_board_outline`

### 9.2 Schematic Operations

- `add_symbol`
- `remove_symbol`
- `set_symbol_value`
- `connect`
- `disconnect`
- `add_label`
- `add_power_symbol`
- `add_no_connect`
- `add_sheet`

Initial support may exclude removal and sheets until readers and mutation
planning are ready.

### 9.3 Footprint Operations

- `assign_footprint`
- `clear_footprint`
- `verify_pinmap`

### 9.4 PCB Operations

- `place_footprint`
- `remove_footprint`
- `route`
- `remove_route`
- `add_via`
- `add_zone`
- `set_net_class`
- `add_mounting_hole`
- `add_test_point`

Initial support should prioritize explicit placement, simple segments, zones,
and generated board outlines because those are closest to existing writer
support.

## 10. AI-Oriented Design Generation

Full one-shot generation from "I need a breakout board that does these things"
should be built as a layered generator, not as raw file writing.

### 10.1 Design Request

A design request should be structured:

```json
{
  "kind": "breakout_board",
  "name": "sensor_breakout",
  "requirements": {
    "connectors": [
      {"ref": "J1", "pins": ["VCC", "GND", "SCL", "SDA"]}
    ],
    "components": [
      {"ref": "U1", "type": "sensor", "symbol": "Sensor:Example", "footprint": "Package:Example"}
    ],
    "power": {"input": "VCC", "voltage": "3.3V"},
    "board": {"width_mm": 50, "height_mm": 30}
  }
}
```

Natural language can be stored as `intent`, but the executable request should
be structured enough to validate.

### 10.2 Generator Pipeline

Generators should follow this pipeline:

```text
request validation
  -> component selection
  -> symbol and footprint assignment
  -> schematic graph construction
  -> board outline creation
  -> placement
  -> routing or partial routing
  -> internal validation
  -> optional KiCad CLI validation
  -> write project and reports
```

### 10.3 First Generators

Recommended initial generators:

1. LED indicator board.
2. Two-connector breakout board.
3. Sensor breakout board.
4. Op-amp buffer board.
5. Class AB headphone amplifier prototype, only after pinmap and analog layout
   lint infrastructure improves.

## 11. Evaluation and Feedback

AI feedback should be based on explicit checks.

### 11.1 Structural Checks

- Missing project files.
- Missing schematic or PCB.
- Invalid library IDs.
- Duplicate references.
- Duplicate UUIDs.
- Missing symbol libraries.
- Missing footprint libraries.
- Missing footprints.
- Missing board outline.
- Invalid layers.
- Invalid net assignments.

### 11.2 Connectivity Checks

- Schematic disconnected wire endpoints.
- No-connect markers not on symbol pins.
- PCB pads assigned to nets but disconnected from routed copper.
- Route endpoints that do not touch pads, vias, or tracks.
- Net mismatch between schematic and PCB where comparable.
- Single-pin nets unless explicitly allowed.

### 11.3 Writer Compatibility Checks

- Unsupported KiCad objects.
- Preservation-only objects.
- Round-trip diffs.
- KiCad CLI parse/save failures.
- KiCad DRC/ERC failures where available.

### 11.4 Design-Intent Checks

These are future checks but should shape the issue model now:

- Decoupling capacitor too far from target pin.
- Input/output separation violation.
- Output trace too narrow for declared current.
- Feedback trace too long.
- Missing test points.
- Ambiguous ground strategy.
- Missing mounting holes.
- Silkscreen reference readability.

## 12. Human Review Output

Every generated project should be able to produce a review summary:

```text
Project: sensor_breakout
Generated files: OK
KiCad parse check: OK
Internal validation: 0 errors, 2 warnings
DRC: skipped, kicad-cli unavailable
ERC: skipped, not implemented
Fabrication readiness: no
Blocking issues:
  - U1 pinmap is not verified
Warnings:
  - No mounting holes
  - No test point for VCC
```

The JSON form should contain the same facts as structured fields.

## 13. Safety Model

The CLI must never mark a design fabrication-ready simply because files were
written successfully.

Fabrication readiness requires:

- KiCadAI validation passes.
- KiCad CLI checks pass where configured.
- Required footprints are assigned.
- Critical pinmaps are verified.
- Board outline exists.
- No blocking writer or preservation issues exist.
- Human review checklist is generated.
- Any waived warnings include rationale.

High-voltage, mains, medical, automotive, aerospace, and regulatory-sensitive
designs must be reported as requiring qualified human review.

## 14. Testing Requirements

### 14.1 Unit Tests

Required tests:

- Command argument parsing.
- JSON request parsing.
- Transaction schema validation.
- Result envelope formatting.
- Structured issue code mapping.
- Generator request validation.
- Inspection summaries.
- Evaluation summaries.

### 14.2 Writer Regression Tests

Existing writer tests must remain green. New CLI work must not weaken:

- Schematic writer tests.
- PCB writer tests.
- Project writer tests.
- Design validation tests.
- Design API tests.
- Round-trip tests.
- Corpus scanner tests.
- PCB connectivity tests.

### 14.3 CLI Golden Tests

Each CLI JSON command should have golden-style output tests for:

- success
- validation failure
- missing file
- unsupported operation
- skipped external tool

### 14.4 Integration Tests

KiCad CLI integration tests should remain optional and gated by environment
variables. Normal tests must not require KiCad.

## 15. Implementation Phases

### Phase 1: Structured CLI Result Foundation

- Add shared result envelope package.
- Add structured issue type and code catalog.
- Update or wrap existing CLI commands to emit result envelopes in JSON mode.
- Keep human text output available.

Acceptance:

- Existing CLI behavior remains compatible where possible.
- JSON outputs are stable and tested.

### Phase 2: Inspection Commands

- Add project inspection command.
- Add schematic inspection command for generated/readable models.
- Add PCB inspection command using current model support and corpus/scanner
  summaries where full reading is unavailable.
- Report unsupported features explicitly.

Acceptance:

- AI can ask "what is in this project?" and receive bounded structured output.

### Phase 3: Evaluation Commands

- Add internal validation command family.
- Add generated connectivity checks where models are available.
- Add round-trip command wrappers.
- Add KiCad CLI detection and skipped/failed/pass states.

Acceptance:

- AI can ask "what is wrong with this design?" and receive structured issues.

### Phase 4: Transaction Schema and Planner

- Define transaction JSON schema.
- Validate transaction files.
- Plan operations without writing.
- Return operation-level issues.

Acceptance:

- AI can validate an edit plan before applying it.

### Phase 5: Generated-Project Transaction Apply

- Apply transactions to projects generated by KiCadAI.
- Support add/connect/assign/place/route/zone operations through
  `designapi.Builder`.
- Write updated projects safely.

Acceptance:

- AI can modify a generated design and receive artifacts plus validation issues.

### Phase 6: Full Project Readers

- Implement structured readers for project, schematic, and PCB files.
- Preserve unsupported content.
- Build imported models suitable for inspection and cautious mutation.

Acceptance:

- AI can inspect existing user-authored projects without relying only on scans.

### Phase 7: Existing-Project Mutation

- Support safe add/remove/update transactions on imported projects.
- Detect unsupported conflict regions.
- Refuse unsafe edits with actionable issues.

Acceptance:

- AI can modify existing KiCad projects without silently damaging unsupported
  content.

### Phase 8: First High-Level Generators

- Add structured request schema for simple generated boards.
- Implement breakout board generator.
- Implement sensor/connector board generator.
- Produce review summary and artifacts.

Acceptance:

- A user can provide a structured board request and receive a complete KiCad
  project that opens in KiCad.

### Phase 9: External Artifact Commands

- Add KiCad CLI ERC/DRC wrappers.
- Add preview/export wrappers.
- Add BOM and fabrication package wrappers.

Acceptance:

- A generated project can produce the review artifacts needed for human
  inspection.

## 16. Acceptance Criteria for This Spec

This CLI/core tools effort is complete when:

- The CLI can inspect generated and imported KiCad projects with structured
  output.
- The CLI can evaluate designs and return stable issue codes.
- The CLI can apply structured transactions to generated projects.
- The CLI can safely reject edits it cannot preserve.
- The CLI can generate at least one complete non-trivial schematic and PCB from
  a structured request.
- The output opens in KiCad.
- The project passes internal validation.
- Round-trip checks can be run where KiCad CLI is available.
- AI agents can use JSON output to correct failed operations without parsing
  human prose.
