# Fabrication Physical Rule Checks Specification

## 1. Purpose

KiCadAI can now generate KiCad projects, produce BOM/CPL reports, run internal
writer and board checks, optionally collect KiCad ERC/DRC evidence, and export
Gerber/drill fabrication artifacts. That proves a generated project can produce
manufacturing files, but it does not yet prove the board has the physical data
and layout hygiene expected before a fabrication-ready claim.

This project adds deterministic physical fabrication-rule checks for generated
PCBs. The checks should make stackup, net-class, solder mask, solder paste, edge
cut, courtyard, silkscreen, and mounting-hole problems visible as structured
readiness evidence.

## 2. Goals

This project must:

- add a reusable physical fabrication-rule result model;
- evaluate generated `.kicad_pcb` files for modeled fabrication and assembly
  risks;
- report rule results with stable IDs, categories, severity, evidence, and
  repair hints;
- integrate physical-rule evidence into `export preview`, `export fabrication`,
  `design create` fabrication-candidate acceptance, and package manifests;
- keep default tests hermetic and independent of KiCad CLI;
- allow optional KiCad DRC evidence to support, but not replace, deterministic
  internal checks;
- avoid claiming manufacturer acceptance from heuristic checks alone.

## 3. Non-Goals

This project does not:

- implement a full CAM, Gerber, or DFM geometry engine;
- certify any specific manufacturer will accept the board;
- add panelization, impedance solver, copper balancing, or IPC class
  certification;
- require network access or external manufacturer APIs;
- mutate imported projects;
- solve unresolved component lifecycle, sourcing, or procurement evidence;
- replace KiCad DRC.

## 4. Current Baseline

Implemented foundations include:

- KiCad project, schematic, and PCB writers;
- PCB parser and writer correctness checks;
- board validation for nets, routes, outlines, zones, and DRC evidence hooks;
- resolver-backed footprint hydration with pads, graphics, courtyard, fab,
  silkscreen, and models where library data is available;
- placement and routing diagnostics;
- fabrication readiness model, package manifest, BOM/CPL reports, manufacturer
  profile checks, Gerber/drill plotting, and artifact validation;
- `design create` fabrication-candidate integration.

Current gaps:

- fabrication readiness does not inspect stackup beyond basic writer validity;
- net classes are not treated as fabrication evidence in export readiness;
- solder mask and paste policy is not checked against pads and footprints;
- courtyard presence and courtyard-to-courtyard spacing are not exported as a
  fabrication gate;
- silkscreen over pads, off-board silkscreen, and edge-clearance issues are not
  modeled;
- mounting-hole requirements and edge clearance are not checked;
- physical-rule issues are not represented in package manifests or readiness
  summaries.

## 5. Rule Model

Add a physical-rule report model under `internal/fabrication` or a focused
subpackage such as `internal/fabrication/physicalrules`.

### 5.1 Report

The top-level report should include:

- `schema`: `kicadai.fabrication.physical_rules.v1`;
- `status`: `pass`, `warning`, `blocked`, or `skipped`;
- `profile`: manufacturer or local policy profile ID when provided;
- `board`: project-relative PCB path and detected board metadata;
- `summary`: per-category counts and aggregate status;
- `checks`: ordered list of individual check results;
- `issues`: existing `reports.Issue` entries suitable for CLI and artifact
  output;
- `evidence`: optional links to KiCad DRC, package artifacts, source object IDs,
  or generated project manifests.

### 5.2 Check Result

Each check result should include:

- `id`: stable machine-readable ID, for example
  `physical.edge_cuts.closed_outline`;
- `category`: one of `stackup`, `net_class`, `solder_mask`, `solder_paste`,
  `edge_cuts`, `courtyard`, `silkscreen`, or `mounting_hole`;
- `status`: `pass`, `warning`, `blocked`, or `skipped`;
- `severity`: existing report severity where a check emits an issue;
- `message`;
- `suggestion`;
- `references`: component references when applicable;
- `nets`: net names when applicable;
- `layers`: KiCad layer names when applicable;
- `objects`: stable object IDs, UUIDs, or generated identifiers when available;
- `measurements`: normalized millimeter values for distances, widths, and
  clearances;
- `source`: `writer`, `parser`, `profile`, `board_validation`, `kicad_drc`, or
  `heuristic`.

### 5.3 Status Rules

- `blocked`: a modeled rule fails in a way that should prevent fabrication
  readiness.
- `warning`: a modeled rule is incomplete, conservative, or risky but does not
  necessarily block candidate output.
- `pass`: the modeled rule has enough deterministic evidence to pass.
- `skipped`: the rule does not apply or the required source data is not
  available. Skipped required checks become warnings or blockers depending on
  profile and acceptance level.

The aggregate report status is the worst status across required checks.

## 6. Rule Categories

### 6.1 Stackup

The stackup checks should verify:

- copper layer count is valid and supported by the generated board;
- enabled copper layers match plotted copper Gerbers when fabrication artifacts
  exist;
- board thickness is positive when modeled;
- solder mask minimum width is non-negative;
- top/bottom copper layers exist for two-layer boards;
- unsupported multilayer stackup details are reported as warnings rather than
  silently ignored;
- generated routing/via policies are compatible with the available copper
  layers.

Blocking examples:

- no copper layers;
- invalid layer count;
- missing required top or bottom copper for a routed board;
- negative stackup or mask values.

### 6.2 Net Classes

Net-class checks should verify:

- a default net class exists in project settings when available;
- generated nets with route intent have an effective trace width, clearance,
  via diameter, and via drill;
- routed segments use widths compatible with assigned net rules;
- high-current, power, differential, clock, and controlled-impedance net roles
  have explicit rule evidence when those roles are modeled;
- board validation pad-net and copper-net evidence is reflected in physical
  readiness.

Blocking examples:

- routed generated net has no effective class;
- via drill is greater than or equal to via diameter;
- copper item net assignment conflicts with pad net assignment;
- required high-current net uses default/unclassified dimensions.

### 6.3 Solder Mask

Solder-mask checks should verify:

- F.Mask/B.Mask Gerber layers exist when artifact validation is enabled;
- SMD pads expose mask on the correct side;
- through-hole pads do not require impossible side-specific mask assumptions;
- mask expansion values are non-negative where modeled;
- global solder mask bridge policy is visible in evidence;
- pad, footprint, or board-level mask overrides are represented when parsed.

Blocking examples:

- required mask Gerber is missing from an executed package;
- pad layer data is inconsistent with its mounting side;
- mask settings are malformed.

Warnings:

- mask expansion is unknown and falls back to KiCad defaults;
- footprint has uncommon mask override not fully modeled.

### 6.4 Solder Paste

Solder-paste checks should verify:

- SMD assembly pads include paste layer intent on their assembly side;
- through-hole and NPTH mechanical holes do not incorrectly require paste;
- F.Paste/B.Paste Gerber presence matches assembly sides when plotted;
- paste margin and ratio values are non-pathological when parsed;
- footprints that intentionally omit paste are marked with explicit evidence.

Blocking examples:

- assembly SMD pad has no paste layer and no explicit no-paste reason;
- paste layer is present on a non-assembly mechanical hole;
- required paste plot is missing for a board with SMD assembly on that side.

Warnings:

- paste reductions are unknown;
- bottom-side assembly is present but manufacturer profile only accepts top-side
  assembly.

### 6.5 Edge Cuts

Edge-cut checks should verify:

- a board outline exists on `Edge.Cuts`;
- the outline can be interpreted as closed or sufficiently closed for the
  current geometry model;
- outline area is positive;
- footprints, pads, vias, tracks, zones, silkscreen, and mounting holes are
  inside the board or intentionally edge-mounted;
- copper-to-edge, hole-to-edge, and silkscreen-to-edge clearances meet profile
  thresholds where measurable.

Blocking examples:

- missing outline;
- obviously open outline;
- zero-area outline;
- generated pad or route outside board bounds;
- drilled hole violates required edge clearance.

Warnings:

- complex outline cannot be fully interpreted by the first geometry model;
- exact arc closure is not yet supported.

### 6.6 Courtyard

Courtyard checks should verify:

- normal assembly footprints have courtyard graphics or explicit exemption;
- hydrated footprint courtyard bounds are carried into placed footprints;
- courtyard boxes do not overlap unless the footprint is exempt or the block
  explicitly allows shared mechanical area;
- courtyard-to-courtyard spacing meets the active profile threshold when known.

Blocking examples:

- overlapping non-exempt courtyards on generated placements;
- missing courtyard on a non-exempt assembly footprint for fabrication-candidate
  acceptance.

Warnings:

- courtyard is absent on a connector or mechanical footprint that may be
  acceptable after human review;
- geometry is more complex than bounding-box checks can fully evaluate.

### 6.7 Silkscreen

Silkscreen checks should verify:

- reference/value and user silkscreen graphics are on valid silkscreen layers;
- silkscreen does not overlap pads or required mask openings when measurable;
- silkscreen is inside board outline with edge clearance;
- required reference designators remain present unless intentionally hidden by
  policy;
- silkscreen line width and text size are above profile thresholds where
  modeled.

Blocking examples:

- silkscreen lies outside the board outline;
- silkscreen overlaps pads on generated footprints under a strict fabrication
  profile;
- required reference text is missing for assembly-critical components.

Warnings:

- silkscreen-to-pad clearance cannot be fully measured;
- silkscreen is small but still parseable.

### 6.8 Mounting Holes

Mounting-hole checks should verify:

- required mounting holes from design intent, board template, or profile are
  present;
- mounting holes have expected plated/NPTH classification;
- drill diameter is positive and within profile limits;
- holes have edge clearance and local keepout evidence;
- mounting holes are excluded from BOM/CPL assembly rows unless intentionally
  included.

Blocking examples:

- design intent requires mounting holes but none are present;
- mounting hole drill is invalid;
- mounting hole edge clearance violates profile threshold;
- mounting hole appears as an assembly component without explicit policy.

Warnings:

- no mounting holes are requested and no profile requires them;
- keepout evidence is missing but the hole is otherwise valid.

## 7. Profile Policy

The first implementation should extend the local built-in profile mechanism
rather than adding remote manufacturer integrations.

The `generic_assembly` profile may define thresholds such as:

- minimum copper-to-edge clearance;
- minimum hole-to-edge clearance;
- minimum silkscreen-to-pad clearance;
- minimum silkscreen line width;
- minimum courtyard spacing;
- accepted assembly sides;
- whether mounting holes are required by default.

If no profile is selected, KiCadAI should still run generic structural checks
and mark profile-specific checks as skipped or warning. A selected profile may
promote selected warnings to blockers.

## 8. Integration

### 8.1 Fabrication Export

`export preview` and `export fabrication` should include physical-rule evidence
in the readiness result:

- add summary field such as `physical_rules`;
- add artifact kind or evidence record for the physical-rule report;
- include physical-rule issues in the existing issue list;
- write `fabrication/physical-rules.json` during executed fabrication export;
- reference the report from `package-manifest.json`.

`ready` must require physical-rule pass when fabrication checks are enabled.
`candidate` may allow warnings, but not blockers.

### 8.2 Design Workflow

`design create` with `acceptance: "fabrication-candidate"` should run the
physical-rule checks after board generation, placement, routing, validation, and
fabrication preview. Its output should include:

- aggregate physical-rule status;
- blocking issue count;
- warning issue count;
- profile used;
- path to the report when a project is written.

### 8.3 Validation And Repair

Physical-rule issues should use stable paths and suggestions so later repair
phases can map them to actions:

- stackup/rules -> adjust board setup or net classes;
- edge cuts -> generate or repair outline;
- courtyard -> adjust placement or footprint selection;
- silkscreen -> hide, move, shrink, or clip text/graphics;
- mounting holes -> add, move, resize, or add keepout;
- mask/paste -> adjust pad or footprint policy.

This project only creates the evidence and taxonomy. Automatic repair is a
future project unless the necessary repair hook already exists.

## 9. Testing Requirements

Default tests must not require KiCad CLI or external KiCad libraries.

Required test coverage:

- rule model JSON stability;
- stackup pass/fail fixtures;
- edge-cut missing/open/valid fixtures;
- net-class missing/default/role-specific fixtures;
- mask and paste pad-layer fixtures;
- courtyard missing/overlap/pass fixtures;
- silkscreen over-pad/off-board/pass fixtures;
- mounting-hole missing/invalid/edge-clearance/pass fixtures;
- fabrication export integration with dry-run and execute paths;
- `design create` fabrication-candidate summary behavior;
- package manifest references to physical-rule reports.

Optional tests may use real KiCad CLI when an environment variable is set, but
they must be skipped by default.

## 10. Acceptance Criteria

The project is complete when:

- generated PCBs can be evaluated with a physical-rule report;
- physical-rule failures are visible as structured issues with stable IDs;
- `export preview` includes physical-rule readiness;
- executed `export fabrication` writes `physical-rules.json` and records it in
  the package manifest;
- fabrication readiness cannot become `ready` when required physical-rule
  blockers exist;
- `design create` fabrication-candidate output includes physical-rule evidence;
- tests cover representative pass, warning, and blocking cases for every rule
  category;
- README and roadmap describe the new gate and remaining DFM limits.

## 11. Open Questions

- Should missing courtyards be blocking by default, or only under
  `generic_assembly` and stricter profiles?
- Should mounting holes be required only when requested by design intent, or
  should board-size/profile heuristics recommend them?
- How much geometry should be implemented before relying on KiCad DRC as
  supplemental evidence for complex shapes?
- Should silkscreen over solder mask be warning by default and blocking only
  under strict profile settings?
