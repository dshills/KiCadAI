# Connectivity-First Board Validation Implementation Plan

## Objective

Implement a unified board-validation workflow that combines structural PCB
validation, net-to-pad checks, generated connectivity checks, unrouted-net
analysis, route completion, zone validation, and KiCad DRC evidence into one
AI-actionable result.

The end state is a command that catches boards that parse and look plausible but
are electrically wrong.

## Implementation Rules

- Keep normal tests independent of KiCad CLI.
- Use fake KiCad runners and checked-in sample reports for deterministic DRC
  tests.
- Preserve existing `check erc`, `check drc`, and `evaluate` behavior unless the
  phase explicitly changes it.
- Return structured `reports.Issue` values with stable codes, paths, refs, nets,
  and repair categories where possible.
- Treat in-process structural/connectivity failures as blocking.
- Treat missing DRC evidence as explicit skipped evidence unless strict DRC mode
  is enabled.
- Do not silently ignore zones, unrouted nets, missing board outlines, or unknown
  net assignments.
- Commit after each phase once tests and review pass.

## Phase 1: Package Skeleton And Result Model

### Goal

Create the board-validation package boundary and stable result types without
changing CLI behavior yet.

### Tasks

- Add `internal/boardvalidation`.
- Define `Status`, `Check`, `Result`, `Summary`, `NetStatus`, `ZoneStatus`, and
  `Options`.
- Define stable check names:
  - `pcb_structural_validation`
  - `net_to_pad_validation`
  - `generated_connectivity`
  - `unrouted_net_validation`
  - `route_completion`
  - `zone_validation`
  - `kicad_drc`
- Add status aggregation helpers:
  - pass when all required checks pass;
  - fail when any required check has blocking/error issues;
  - skipped when a check could not run by policy;
  - error for parse/tool/setup failures.
- Add helpers to convert `kicadfiles.ValidationErrors` and generic errors into
  `reports.Issue`.

### Tests

- Result status aggregation.
- Check ordering is deterministic.
- Required versus optional check semantics.
- Issue conversion preserves paths and codes.

### Acceptance Criteria

- Package compiles and has unit coverage.
- No CLI behavior changes.
- `go test ./internal/boardvalidation` passes.

## Phase 2: Target Resolution

### Goal

Resolve board-validation inputs consistently for `.kicad_pcb` files and project
directories.

### Tasks

- Add target resolver in `internal/boardvalidation`.
- Support:
  - direct `.kicad_pcb` path;
  - project directory with `.kicad_pro` and board file;
  - future room for in-memory `design.Design`.
- Reuse existing project discovery behavior where possible.
- Return structured errors for:
  - missing target;
  - missing project file;
  - multiple project files;
  - missing board file;
  - ambiguous board files;
  - unreadable board file.
- Preserve project context for later DRC execution.

### Tests

- Direct board path.
- Project directory with matching board.
- Missing board.
- Multiple project files.
- Ambiguous board files.

### Acceptance Criteria

- Target resolution is deterministic and safe.
- Existing project read tests continue to pass.

## Phase 3: Structural And Net-To-Pad Validation

### Goal

Wrap existing PCB parser/model validation in the board-validation result and add
explicit net-to-pad semantics.

### Tasks

- Run `pcb.ReadFile` and `pcb.Validate`.
- Add `pcb_structural_validation` check.
- Add `net_to_pad_validation` check that reports:
  - pad net code not in board net table;
  - pad net name mismatch;
  - connected copper item net code/name mismatch if not already covered;
  - connected pad on net `0` when other evidence implies it should be routed,
    when safely detectable;
  - invalid duplicate pad names outside supported net-tie policy.
- Map findings into repair categories:
  - `net_assignment`;
  - `footprint`;
  - `layer`;
  - `unknown`.

### Tests

- Known-good minimal routed board.
- Unknown pad net code.
- Pad net name mismatch.
- Unconnected NPTH pad does not fail.
- Duplicate pad/net-tie behavior matches current PCB model support.

### Acceptance Criteria

- Structural and net-to-pad failures are visible as separate checks.
- Known-good structural fixture passes.
- Known-bad net assignment fixtures fail with stable paths.

## Phase 4: Generated Connectivity Integration

### Goal

Integrate `pcb.ValidateGeneratedConnectivity` as a first-class board-validation
check.

### Tasks

- Add `generated_connectivity` check.
- Run existing generated connectivity validation when structural validation
  succeeds enough to make geometry safe.
- Convert connectivity validation errors into structured issues.
- Preserve existing connectivity semantics:
  - same-net disconnected pads;
  - dangling route endpoints;
  - arcs, tracks, vias, and pad geometry.
- Add repair suggestions for disconnected pads and dangling endpoints.

### Tests

- Existing `CorrectnessFixturePCB` passes.
- Removing route/arc creates disconnected pad failure.
- Moving track endpoint creates dangling route endpoint failure.
- Board-validation result includes check name and issue category.

### Acceptance Criteria

- Existing PCB connectivity tests still pass.
- Board-validation tests prove generated connectivity failures are blocking.

## Phase 5: Unrouted Net Analysis

### Goal

Report per-net route/connectivity status, not only raw validation errors.

### Tasks

- Build a connectivity graph over pads, tracks, arcs, and vias.
- For each nonzero net, collect endpoint pads and connected copper objects.
- Classify each net as:
  - `single_endpoint`;
  - `unconnected`;
  - `partially_routed`;
  - `fully_routed`;
  - `zone_dependent`;
  - `ignored`.
- Add `unrouted_net_validation` check.
- Add `Result.Nets` summary rows.
- Treat `unconnected` and `partially_routed` multi-pad nets as blocking by
  default.
- Keep `single_endpoint` non-blocking.

### Tests

- Single endpoint net is not blocking.
- Two pads with no copper are `unconnected`.
- Three pads with only two connected are `partially_routed`.
- Fully routed net passes.
- Via-assisted net passes.

### Acceptance Criteria

- The validator reports net-level status even when no DRC evidence exists.
- Known-bad unrouted fixtures fail before KiCad DRC runs.

## Phase 6: Route Completion For Board Files

### Goal

Make route-completion evidence explicit for board files, complementing
`routing.ValidateResult`.

### Tasks

- Add `route_completion` check.
- Reuse the net connectivity graph from Phase 5.
- Report:
  - route endpoint not connected to same-net copper;
  - segment on unknown/unroutable layer;
  - via with invalid layer span;
  - route item assigned to unknown net.
- Avoid duplicating all clearance checks; keep clearance in routing validation
  and DRC.

### Tests

- Dangling track endpoint.
- Track on unknown layer.
- Via with invalid layer span.
- Unknown track net.
- Fully routed board passes.

### Acceptance Criteria

- Board-file validation can independently prove route completion.
- Routing-engine output remains validated by `routing.ValidateResult`.

## Phase 7: Zone Validation And Zone Evidence

### Goal

Surface zone correctness and zone-connectivity limitations explicitly.

### Tasks

- Add `zone_validation` check.
- Reuse existing PCB zone structural validation.
- Add `ZoneStatus` rows with:
  - zone name;
  - net;
  - layers;
  - polygon count;
  - filled polygon count;
  - status;
  - evidence source.
- Report:
  - missing polygons;
  - missing layers;
  - invalid keepout net assignments;
  - filled polygons on undeclared layers;
  - zone with no fill evidence.
- Add option `StrictZones`.
- Treat missing fill evidence as warning by default, blocking under
  `StrictZones`.

### Tests

- Valid copper zone with polygon.
- Keepout zone with copper net fails.
- Zone with no polygons fails.
- Zone with no fill evidence creates warning by default.
- Strict zone mode makes missing fill evidence blocking.

### Acceptance Criteria

- Zone limitations are explicit and machine-readable.
- The validator no longer silently treats zones as equivalent to routed copper
  unless evidence exists.

## Phase 8: KiCad DRC Evidence Integration

### Goal

Integrate KiCad DRC results into board validation using the existing checks
package.

### Tasks

- Add `kicad_drc` check to board-validation result.
- Call `checks.RunDRC` when KiCad CLI is configured or strict DRC is requested.
- Preserve artifacts from `checks` in board-validation result.
- Convert DRC findings into board-validation issues.
- Map DRC categories:
  - clearance;
  - connectivity;
  - outline;
  - zone;
  - net assignment;
  - unknown.
- Add options:
  - `RequireDRC`;
  - `AllowMissingDRC`;
  - `AllowlistPath`;
  - artifact options.
- Ensure DRC execution errors are distinct from DRC violations.

### Tests

- Fake DRC runner pass.
- Fake DRC runner connectivity failure.
- Fake DRC runner clearance failure.
- DRC skipped when KiCad unavailable and not required.
- DRC missing is blocking when required.
- Allowlisted DRC finding is suppressed but still recorded in summary.

### Acceptance Criteria

- Board validation can include DRC evidence without normal tests needing KiCad.
- DRC absence is explicit, never silent.

## Phase 9: CLI Command

### Goal

Expose the unified validator through the CLI.

### Tasks

- Add preferred command:
  - `kicadai --json validate board <project-or-board>`
- If the existing command parser makes a new namespace expensive, add:
  - `kicadai --json check board <project-or-board>`
- Add flags:
  - `--require-drc`;
  - `--allow-missing-drc`;
  - `--strict-zones`;
  - existing KiCad CLI/artifact/allowlist flags.
- Return `reports.Result` with `command` and board-validation payload.
- Nonzero exit when required checks fail.
- Text output can remain minimal or unsupported initially if JSON-only matches
  current structured command patterns.

### Tests

- Missing target.
- Known-good board returns ok.
- Known-bad board returns structured issues.
- DRC skipped appears in JSON.
- Required DRC absence returns nonzero.

### Acceptance Criteria

- Users and agents have one command to validate board electrical meaning.
- CLI output is deterministic and documented enough for README update.

## Phase 10: Evaluation And Readiness Integration

### Goal

Make project evaluation reflect board-validation evidence without overstating
fabrication readiness.

### Tasks

- Add optional board-validation check to `evaluate project` or add a separate
  readiness path that can consume board-validation results.
- Update fabrication readiness rules:
  - structural PCB validation must pass;
  - generated connectivity/unrouted/route completion must pass;
  - zone strictness follows options;
  - DRC evidence must be present only when required by options or verification
    level.
- Keep external DRC skipped placeholders when not running KiCad.
- Update README status caveats.

### Tests

- Evaluate project without DRC does not claim DRC-clean.
- Evaluate project with board-validation payload reflects blocking failures.
- Generated good board can become board-validation-ready in in-process checks.

### Acceptance Criteria

- Readiness semantics are accurate and conservative.
- Existing evaluate behavior remains compatible unless stricter mode is chosen.

## Phase 11: Fixtures And Golden Reports

### Goal

Add durable known-good and known-bad fixtures that prove electrical correctness
checks catch real failure modes.

### Tasks

- Add fixture builder helpers for small boards.
- Add examples or testdata for:
  - fully routed two-pad board;
  - unrouted two-pad board;
  - partial three-pad route;
  - dangling endpoint;
  - via-assisted connection;
  - unknown net assignment;
  - zone warning/failure;
  - DRC parser failure samples.
- Add golden JSON snippets only where stable and not too brittle.

### Tests

- Unit tests assert check names, statuses, net statuses, and key issue fields.
- Optional KiCad smoke fixture documented separately.

### Acceptance Criteria

- Regressions in connectivity-first behavior are caught by normal tests.
- Fixtures are small and explain the intended failure mode.

## Phase 12: Documentation And Handoff

### Goal

Document the new validator for human use and AI integration.

### Tasks

- Update README with:
  - command examples;
  - strict mode examples;
  - DRC caveats;
  - interpretation of statuses.
- Update package docs for `internal/boardvalidation`.
- Add a short status note for remaining gaps:
  - full KiCad zone refill;
  - manufacturing output validation;
  - automatic repair.
- Add examples to `examples/checks` if appropriate.

### Acceptance Criteria

- A user can run the validator from README instructions.
- AI agents can consume the JSON result without reading code.

## Commit Plan

Suggested commit sequence:

1. `Add board validation result model`
2. `Resolve board validation targets`
3. `Add structural net pad validation`
4. `Integrate generated PCB connectivity checks`
5. `Report unrouted board nets`
6. `Add board route completion checks`
7. `Add zone validation evidence`
8. `Integrate KiCad DRC board evidence`
9. `Expose board validation CLI`
10. `Integrate board validation readiness`
11. `Add connectivity validation fixtures`
12. `Document board validation workflow`

Each commit should pass:

```sh
GOCACHE=/private/tmp/kicadai-go-cache go test ./...
git diff --check
prism review staged
```

## Open Questions

1. Should the first user-facing command be `validate board` or `check board`?
2. Should missing DRC evidence be warning by default or blocking for generated
   boards?
3. Should zones without filled polygons be warning by default until KiCad refill
   is automated?
4. Should board validation run automatically after `transaction apply` when a PCB
   is written?
5. What is the first stable real KiCad DRC fixture for local smoke testing:
   clearance, missing outline, disconnected item, or zone fill?
