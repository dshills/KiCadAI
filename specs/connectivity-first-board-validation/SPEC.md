# Connectivity-First Board Validation Specification

## Purpose

Build a strict board-validation workflow that catches PCBs that are syntactically
valid and visually plausible but electrically wrong.

The validator must answer one practical question:

> Is this board electrically meaningful enough for an AI agent to continue,
> route, repair, or present it as a candidate design?

This project is not a replacement for KiCad DRC. It is a layered validation
gate that combines KiCadAI's in-process model checks, generated-board
connectivity checks, routing completion checks, zone checks, and KiCad DRC
evidence when available.

## Background

KiCadAI already has several relevant pieces:

- `internal/kicadfiles/pcb.Validate` validates PCB file structure, net
  references, pads, zones, layers, dimensions, and other modeled objects.
- `internal/kicadfiles/pcb.ValidateGeneratedConnectivity` checks same-net pad
  connectivity and dangling route endpoints for generated boards.
- `internal/routing.ValidateResult` checks route endpoint completion, obstacle
  intersections, board bounds, layer validity, via sanity, and inter-route
  clearance for routing-engine output.
- `internal/evaluate` reports coarse PCB semantic issues, including pads whose
  nets have no parsed track, arc, or zone usage.
- `internal/kicadfiles/checks` can run and parse KiCad CLI ERC/DRC checks, with
  deterministic parser/fake-runner coverage.
- `kicadai check drc` and `kicadai check project` expose KiCad DRC evidence.

These pieces are useful but fragmented. No single workflow currently combines
them into a board-readiness gate with explicit connectivity, route completion,
zone, and DRC evidence semantics.

## Goals

1. Provide one authoritative board-validation workflow for PCB electrical
   correctness.
2. Catch net-to-pad assignment errors, disconnected same-net pads, dangling
   route endpoints, and unrouted required nets.
3. Distinguish partially routed, fully routed, and intentionally unrouted nets.
4. Validate zone object correctness and surface zone connectivity limitations.
5. Integrate KiCad DRC evidence when available without requiring KiCad for normal
   unit tests.
6. Return structured, AI-actionable findings with references, nets, layers,
   object paths, and repair categories.
7. Add known-good and known-bad fixtures that prove the validator catches
   "looks fine but electrically wrong" boards.
8. Feed the result into project evaluation/readiness without overstating
   fabrication readiness.

## Non-Goals

- Replacing KiCad's full DRC algorithms.
- Implementing KiCad zone refill.
- Guaranteeing fabrication readiness from in-process checks alone.
- Solving all multi-board or panelized-project validation cases.
- Validating manufacturing outputs such as Gerbers, drill files, pick-and-place,
  or BOMs.
- Automatically repairing boards in this project.

## Terminology

- **Structural validation**: S-expression and object-model correctness: nets,
  layers, pads, zones, UUIDs, dimensions, and legal field values.
- **Connectivity validation**: Geometry-aware checks that same-net copper objects
  form the intended electrical graph.
- **Route completion**: Every required routed net has all required endpoints
  connected through tracks, arcs, vias, or acceptable copper.
- **Unrouted net**: A net with two or more connected pads/endpoints and no
  completed copper path.
- **Zone evidence**: Evidence that zone objects are valid, assigned to legal
  nets/layers, and either filled/connected or require KiCad DRC/refill evidence.
- **DRC evidence**: KiCad CLI DRC output parsed into stable findings, including
  command metadata and raw artifact references.
- **Board validation result**: The combined output of all validation stages.

## User-Facing Command

The preferred command is:

```sh
kicadai --json validate board <project-or-pcb>
```

If adding a new top-level `validate` namespace is too disruptive in the first
phase, the same workflow may be exposed initially as:

```sh
kicadai --json check board <project-or-pcb>
```

The command should accept:

```text
--kicad-cli string       KiCad CLI executable path for optional DRC evidence
--allow-missing-drc      Do not fail when KiCad DRC cannot run
--keep-artifacts         Keep DRC reports and copied workspaces
--artifact-dir string    Directory for retained validation artifacts
--allowlist string       KiCad check allowlist JSON path
--strict-zones           Treat zones without fill/connectivity evidence as blocking
--strict-unrouted        Treat all unrouted multi-pad nets as blocking
```

Default behavior should be strict for generated board validation but pragmatic
for local development:

- in-process structural/connectivity failures are blocking;
- KiCad DRC absence is reported as skipped unless `--require-drc` is introduced
  or an existing strict mode is enabled;
- DRC findings are blocking unless allowlisted;
- zone limitations are warnings at first, then can become blocking under
  `--strict-zones`.

## Result Model

Add a stable result model, likely in a new package:

```text
internal/boardvalidation
```

Core types:

```go
type Result struct {
    Target              string
    BoardPath           string
    ProjectPath         string
    Status              Status
    FabricationReady    bool
    FabricationReason   string
    Checks              []Check
    Nets                []NetStatus
    Zones               []ZoneStatus
    Issues              []reports.Issue
    Artifacts           []reports.Artifact
    Summary             Summary
}

type Status string

const (
    StatusPass    Status = "pass"
    StatusFail    Status = "fail"
    StatusSkipped Status = "skipped"
    StatusError   Status = "error"
)
```

`Check` names should be stable:

- `pcb_structural_validation`
- `net_to_pad_validation`
- `generated_connectivity`
- `unrouted_net_validation`
- `route_completion`
- `zone_validation`
- `kicad_drc`

Each `reports.Issue` should include as much context as safely available:

- `Code`
- `Severity`
- `Path`
- `Message`
- `Refs`
- `Nets`
- `Suggestion`
- `OperationID` when the board came from a transaction operation

Repair categories should be compatible with ERC/DRC feedback categories:

- `connectivity`
- `net_assignment`
- `routing`
- `zone`
- `clearance`
- `outline`
- `layer`
- `footprint`
- `drc`
- `unknown`

## Validation Stages

### 1. Target Resolution

The workflow must accept either:

- a `.kicad_pcb` file;
- a project directory containing `.kicad_pro` and `.kicad_pcb`;
- later, a transaction/apply result or in-memory `design.Design`.

Project target resolution must:

- locate exactly one project file;
- discover the board file using project name first;
- preserve project context for DRC invocation;
- report missing or ambiguous board files as structured errors.

### 2. Structural PCB Validation

Run the existing PCB reader and validator:

- parse `.kicad_pcb`;
- validate nets, layers, pads, tracks, vias, arcs, zones, board outline, and
  known object fields;
- map validation errors into structured `reports.Issue` values.

Structural failures block later electrical claims, but the workflow should still
run any safe independent checks that can produce useful diagnostics.

### 3. Net-to-Pad Validation

Validate that every connected pad is electrically meaningful:

- pad net code exists in the board net table;
- pad net name matches the board net table entry;
- pads intended to be connected are not accidentally left on net code `0`;
- non-plated or intentionally unconnected pads are not treated as failures;
- duplicate pad names within a footprint are either valid net-tie constructs or
  reported.

This stage should extend or wrap existing `pcb.Validate` output so users get a
board-validation check name and AI-readable repair category.

### 4. Generated Connectivity Validation

Run `pcb.ValidateGeneratedConnectivity` for boards that have enough modeled
geometry.

It should catch:

- same-net pads split into disconnected copper islands;
- dangling route endpoints;
- track/via/pad non-contact due to geometry near-misses;
- false positives from rectangular pad corners;
- track T-junction connectivity;
- via-to-pad and track-width overlap connectivity.

Current limitation:

- zone fills are not currently treated as full copper connectivity evidence in
  the same way KiCad's zone fill engine would.

### 5. Unrouted Net Validation

Add a board-level net status pass that reports each net as one of:

- `single_endpoint`
- `unconnected`
- `partially_routed`
- `fully_routed`
- `zone_dependent`
- `ignored`

For every net with two or more connected pads, determine whether all pads are in
one connectivity component using pads, tracks, arcs, vias, and optionally filled
zone polygons.

Blocking behavior:

- `unconnected` and `partially_routed` are blocking under default generated-board
  validation;
- `zone_dependent` is warning by default and blocking under `--strict-zones`;
- `single_endpoint` is not blocking;
- explicitly ignored nets require a documented policy, not a silent skip.

### 6. Route Completion Validation

For routing-engine results, keep using `routing.ValidateResult`.

For board files, add or expose equivalent route-completion evidence:

- all multi-endpoint nets have a connected copper graph;
- all route endpoints connect to same-net pads, vias, or track endpoints;
- no route segment is assigned to an unknown net;
- no route segment exists on an unroutable or unknown layer.

This stage should not duplicate every geometric clearance rule. Clearance remains
the routing validator and KiCad DRC's responsibility.

### 7. Zone Validation

Zone validation has two levels:

Structural:

- zone net code exists;
- zone net name matches the net table;
- zone layers are legal;
- zone polygons have enough points;
- filled polygons reference declared zone layers;
- keepout zones do not have copper net assignments;
- thermal, clearance, hatch, and island settings are in valid ranges.

Connectivity evidence:

- if filled polygons are present, determine whether pads/tracks/vias overlap the
  filled copper on matching layers;
- if filled polygons are absent, mark the zone `requires_fill_evidence`;
- when KiCad DRC is available, prefer KiCad zone/refill evidence over heuristics.

Initial implementation may report `requires_fill_evidence` as warning, with
`--strict-zones` making it blocking.

### 8. KiCad DRC Evidence

Integrate the existing `internal/kicadfiles/checks` DRC runner:

- run DRC for project or board targets when KiCad CLI is configured;
- preserve raw reports and command metadata when artifacts are requested;
- parse findings into board-validation issues;
- map connectivity, clearance, outline, zone, and net-class findings into stable
  repair categories;
- distinguish skipped DRC, DRC execution error, parser error, and DRC violations.

DRC cannot be the only validation layer because normal tests must remain KiCad
free and AI workflows need fast preflight checks. DRC is the strongest external
evidence layer when available.

## Integration Points

### CLI

Add a command that returns the unified result model. The JSON shape must follow
existing `reports.Result` conventions.

### Evaluate

`evaluate project` should be able to include board-validation evidence or a
skipped external-check placeholder. It should not mark a PCB fabrication-ready
unless required validation checks pass.

### Transactions

Transaction `plan`/`apply` should be able to run board validation after write
when requested. Operation-correlated issues should preserve operation IDs where
the source transaction operation is known.

### Routing

Routing output should remain validated by `routing.ValidateResult`, but the
board validator should provide a second check after routes are emitted into a
real PCB file.

### Circuit Blocks

Verified blocks should not be promoted to `erc_drc_verified` or a future
`board_validated` state unless board validation passes with required DRC
evidence or an explicit documented exception.

## Fixtures

Add deterministic fixtures under `examples/checks` or a dedicated validation
fixture directory.

Known-good fixtures:

- two-pad net fully routed with a track;
- via-assisted two-layer net;
- board with a simple GND zone and explicit filled polygon if writer supports it;
- generated schematic-to-PCB transfer board after placement and routing.

Known-bad fixtures:

- pad assigned to an unknown or mismatched net;
- two same-net pads with no route;
- route endpoint near but not touching a pad;
- dangling track segment;
- segment on unknown layer;
- malformed or unfilled zone;
- missing board outline;
- DRC sample with clearance violation;
- DRC sample with disconnected item if KiCad report format is stable.

Normal unit tests should use in-process fixtures and fake KiCad runner output.
Real KiCad CLI smoke tests should remain opt-in.

## Acceptance Criteria

The project is complete when:

1. A single command can validate a board or project and return structured JSON.
2. Structural PCB validation, generated connectivity, unrouted nets, route
   completion, zone validation, and DRC evidence are visible as separate checks.
3. Known-good in-process fixtures pass.
4. Known-bad in-process fixtures fail with stable issue codes and repair
   categories.
5. DRC parser/fake-runner fixtures prove DRC findings become board-validation
   issues.
6. Missing KiCad DRC evidence is explicit, never silently ignored.
7. `evaluate project` does not overstate readiness when board validation or DRC
   evidence is missing.
8. All normal tests pass without KiCad installed.
9. Optional KiCad smoke tests can be run locally and documented.

## Known Follow-Up Gaps

- Full KiCad-compatible zone-fill connectivity requires either invoking KiCad
  refill/DRC or implementing a much richer copper-fill model.
- Multi-instantiated hierarchical schematic net scoping is currently limited in
  schematic-to-PCB transfer and should be handled before claiming full
  schematic-driven board readiness.
- Manufacturing-output validation is a separate fabrication package project.
- Automatic repair suggestions can build on this result model but are not part
  of the first implementation.
