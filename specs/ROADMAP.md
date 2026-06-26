# KiCadAI Roadmap

Date: 2026-06-25

This roadmap replaces the older roadmap and gap analysis now archived as
`specs/OLD_ROADMAP.md` and `specs/OLD_ROADMAP_GAP.md`.

## Goal

KiCadAI should let an AI agent read, evaluate, repair, and eventually create
complete KiCad schematic and PCB projects from intent.

The target workflow is:

```text
user intent
  -> requirements and constraints
  -> verified circuit blocks and components
  -> schematic transaction
  -> footprint assignment
  -> PCB placement
  -> routing and zones
  -> writer correctness
  -> ERC/DRC/round-trip validation
  -> deterministic repair loop
  -> fabrication-ready outputs
```

The project is now past the "can we write KiCad files?" stage. The main work is
hardening correctness, expanding verified design knowledge, and closing the loop
from validation feedback to safe automatic repair.

## Current State

### Implemented Foundations

- Direct KiCad project, schematic, and PCB writers.
- Project directory structure generation and inspection.
- KiCad IPC client for connection, version, document, and capability probes.
- Transaction model with validation, planning, apply, operation IDs, and
  operation-correlated issue feedback.
- Schematic-to-PCB transfer workflow.
- Resolver-backed footprint hydration, footprint graphics, pads, and models.
- Symbol resolver foundation with project/global symbol table support and
  KiCad symbol metadata.
- Verified component and part intelligence foundation with catalog coverage
  reports, richer metadata, confidence gates, rating/function-aware selection,
  resolver/pinmap evidence checks, golden coverage, and design workflow
  evidence output.
- Circuit block library expansion foundation with inventory, readiness gaps,
  electrical rules, PCB constraints, required local routes, verification corpus
  guards, PCB realization and board-validation evidence hooks, hardened
  optional/required KiCad ERC/DRC policy, and design workflow evidence output.
- Placement engine hardening foundation with deterministic model support,
  block-derived placement intent, proximity/region/mechanical/routing-readiness
  quality reports, coarse congestion reports, component fanout/escape readiness
  reports, semantic candidate scoring, advanced placement rules for thermal,
  high-current, creepage/clearance, differential-pair, and
  controlled-impedance intent, repairable diagnostics, workflow evidence, and
  golden corpus coverage.
- Routing engine hardening foundation with shared `internal/pcbrules`,
  per-net quality reports, net-class and role-aware routing, obstacle
  diagnostics, via/layer policy diagnostics, length policy evidence, explicit
  zone policy behavior, coupled-net intent reporting, workflow integration, and
  golden corpus coverage.
- Bounded placement-routing retry foundation with routing diagnostic to
  placement hint mapping, explicit retry policy, deterministic adjustment
  builder, best-attempt selection, repeated-state detection, and workflow retry
  summaries, plus golden coverage for fixture loading, retry summary shape,
  supported placement retry categories, unsupported skips, selected stop
  conditions, CLI output, evidence of improvement in pad-backed full-board
  seeds, generated hydrated-pad and no-eligible-hint boundaries, generated
  multi-block pad-hydration and net-intent boundaries, and hard-constraint
  preservation under retry adjustment. Generated placement participants now
  carry explicit mobility policy, including `fixed`, `group_transform`,
  `local_rebuild`, `soft_preferred`, and `unowned` classes, and retry evidence
  reports eligible refs, blocked refs, and `local_route_mobility` handling.
- Connectivity-first board validation for pad nets, unrouted nets, route
  completion, outlines, zones, and DRC evidence hooks.
- ERC/DRC feedback loop foundation using `kicad-cli` where configured.
- Writer correctness closeout foundation with project, schematic, PCB,
  transfer, pad-net, copper-net, zone-net, and optional KiCad round-trip gates.
- Schematic round-trip compatibility foundation.
- Closed-loop validation repair foundation with planning, executors, persisted
  generated-project apply, target-based CLI support, workflow integration, and
  repair goldens.
- Post-repair validation adapters for writer correctness, board validation,
  optional KiCad ERC/DRC, optional KiCad round-trip evidence, persisted
  validation summaries, issue deltas, retry budget evidence, and design
  workflow repair bundle artifacts, plus a dedicated `repair export-bundle`
  command for non-`design create` generated targets, with CLI golden coverage
  for bundle export, target apply, validation summaries, delta statuses, and
  optional vs required KiCad DRC policy.
- Fabrication export/readiness gate foundation with readiness model, package
  manifest serialization, project evaluation, deterministic BOM/CPL reports,
  safe dry-run/execute export service, CLI wiring, KiCad CLI evidence policy,
  BOM/CPL identity and placement evidence, BOM/CPL consistency validation,
  built-in generic assembly manufacturer profile checks, and `design create`
  fabrication-candidate acceptance integration.
- `design create` workflow for structured block-based design requests.
- README and focused docs for current CLI capabilities.

### Still Not Ready

KiCadAI is not yet ready for unconstrained "make me any board" autonomous
design. The remaining blockers are mostly breadth, quality gates, and closed
loop confidence:

- component catalog breadth is still intentionally small beyond the verified
  seed families;
- many production-ready active parts still need datasheet-backed electrical,
  thermal, lifecycle, availability, and design-rule evidence;
- circuit block coverage now includes concrete structural generators for
  crystal/canned oscillator, standalone reset/programming, ESD, and
  reverse-polarity protection families. It also includes conditional
  realization for the default reset/programming ISP fixture, manifest-level PCB
  realization evidence, and deterministic timing/local-route checks where
  modeled. ESD and reverse-polarity protection now include entry-anchor and
  power-path local-route evidence plus board-level connector/interface binding
  evidence in `design create`. These newer blocks still need more variants and
  stronger KiCad-backed layout proof;
- placement now models several PCB-quality rule families and first-pass
  timing-sensitive crystal, canned oscillator, and reset/programming fixture
  evidence, but larger-board KiCad DRC-backed proof and broader
  timing-sensitive block variants are still needed;
- placement-routing retry now includes focused pad-backed seeds, generated
  workflow coverage, larger generated fixture families, selected attempt
  evidence, optional DRC evidence, and generated mobility policy coverage. It
  still needs broader real KiCad DRC-clean layout proof before it can be
  treated as production layout quality;
- KiCad-backed validation exists in the repair and workflow loops, but needs
  broader golden evidence and richer parser-to-repair category mapping;
- repair can persist generated-project changes, but imported-project mutation
  remains blocked by preservation safety;
- fabrication export now provides conservative readiness gates with BOM/CPL
  identity, consistency, and local manufacturer-profile evidence, but does not
  yet guarantee complete manufacturer acceptance;
- structured intent planning now exists, but natural-language parsing and
  broader semantic design synthesis are not yet first-class pipelines.
  Deterministic rationale reports now explain supported planner decisions and
  blockers, but they do not replace broader synthesis.

## Roadmap Principles

- Prefer deterministic, inspectable workflows before LLM orchestration.
- Treat KiCad file correctness as a hard gate, not a best effort.
- Only mutate generated projects unless preservation support is explicit.
- Expand AI autonomy through verified blocks, components, and constraints.
- Keep every repair action tied to validation evidence.
- Do not report "ready" unless writer correctness, board validation, and
  configured KiCad checks agree.

## Priority 1: Verified Component And Part Intelligence

### Objective

Make part selection reliable enough for AI-generated designs beyond demos.

### Status

Implemented foundation.

### Current Foundation

The component catalog now has verified or policy-allowed coverage for common
passives, pin headers, LEDs, diodes, a regulator, an op-amp, an MCU, an I2C
sensor, a crystal, USB-C power-only, and a protection part. It includes:

- catalog coverage reporting through `component coverage`;
- richer metadata for lifecycle, ratings, tolerances, temperature, companion
  components, placement hints, routing hints, and schematic properties;
- verified built-in pinmaps for the seed active and connector records;
- rating-aware selection with min/max checks;
- required function checks;
- concrete-part and companion metadata gates;
- rejected-candidate diagnostics;
- stronger evidence validation for required pin-to-pad mappings and polarity;
- workflow component-selection output with manufacturer, MPN, confidence,
  pinmap evidence, companions, and rejected alternatives;
- golden coverage and representative selection snapshots.

### Remaining Work

- Expand from seed records to larger verified families and real alternatives.
- Add availability/lifecycle source integration when a trusted local or remote
  source is chosen.
- Replace remaining structural placeholders where verified concrete parts are
  needed.
- Improve MCU function names from generic GPIO placeholders to datasheet port
  and peripheral roles.
- Emit selected component properties into generated schematic symbols when the
  transaction/writer model supports arbitrary symbol properties.
- Convert component placement/routing hints into downstream rule enforcement.
- Add KiCad-library-backed evidence runs for the full checked-in catalog when
  external library roots are configured.

### Acceptance Gates

- Seed common design intents can select concrete or policy-allowed parts without
  relying on placeholder active components.
- Selected connectivity-level active seed parts have symbol, footprint, and
  pinmap evidence.
- Unsafe or under-evidenced selections block the workflow with actionable
  issues.

## Priority 2: Circuit Block Library Expansion

### Objective

Build a reusable library of verified schematic and PCB design fragments.

### Status

Implemented foundation.

### Current Foundation

- Circuit block inventory reports roadmap seed families, implemented status,
  readiness, validation rules, PCB rules, required roles, exported ports, and
  explicit unsupported gaps.
- LED indicator, connector breakout, voltage regulator, MCU minimal, USB-C
  power, I2C sensor, op-amp gain-stage, crystal oscillator, canned oscillator,
  reset/programming header, ESD protection, and reverse-polarity protection
  blocks declare electrical rules, PCB constraints, and required local-route
  expectations where supported by the current realization model.
- PCB realization metadata includes placement groups, keepouts, proximity
  constraints, route width constraints, edge-facing constraints, and local
  route definitions for supported block-local nets.
- MCU minimal realization now includes power decoupling, AREF decoupling,
  reset pull-up, and local route evidence for its supported ATmega328P-A
  template.
- Canned oscillator realization includes local decoupling, enable pull-up,
  local routes, timing evidence, and workflow repair suggestions for missing or
  distant decoupling/control evidence.
- Reset/programming realization supports conditional default ISP/header/reset
  switch PCB evidence, route-length timing evidence, and programming-header
  ground-reference findings without breaking UART/no-switch instantiation.
- Crystal, canned oscillator, and reset/programming verification manifests now
  assert required local routes and satisfied timing fixtures.
- ESD and reverse-polarity protection verification manifests now require PCB
  realization evidence for the currently modeled placement, entry anchors, and
  power-path local routes.
- `design create` routing summaries now include board-level anchor binding
  evidence for realized entry anchors, including physical endpoint discovery
  for connector/interface pads, derived board-edge endpoints, explicit
  `external_endpoints` for board-edge and imported-mechanical points,
  endpoint-to-anchor route operations, and structured
  missing/ambiguous/invalid/unsupported/net-mismatched/not-routable diagnostics.
- Block verification can require internal board validation and has explicit
  optional versus required KiCad ERC/DRC behavior with skipped/blocking stage
  summaries.
- Representative protection and timing block manifests now declare optional
  KiCad ERC/DRC proof intent. Deterministic fake-runner tests cover missing
  CLI, required findings, allowed findings, artifact-producing pass cases, and
  global `RequireERC`/`RequireDRC` strengthening.
- ERC/DRC verification records stable report artifact descriptions, contextual
  check failure messages, generated-project freshness sentinels, and separate
  check-context signatures for resolved `kicad-cli`, version, units, and
  allowlists.
- A local opt-in block smoke test runs selected manifests through real
  `kicad-cli` only when `KICADAI_RUN_KICAD_CLI=1` is set; normal `go test
  ./...` remains KiCad-independent.
- A named opt-in KiCad block corpus now exists in `block verify` through
  `--kicad-corpus` and `--kicad-corpus-tier`. The initial smoke corpus includes
  `led_indicator_default` and `connector_breakout_4pin`, reports selected
  case/status summaries, writes `corpus-summary.json` and per-case
  `corpus-result.json` artifacts when `--output` is set, and has fake-runner
  plus opt-in real KiCad smoke coverage.
- The general block verification corpus covers every built-in block and has regression
  guards ensuring that required routes, timing fixtures, and realization stages
  remain defined.
- `design create` block-planning output exposes block readiness, verification
  level, rule IDs, required routes, and known gaps for AI-facing explanation.

### Remaining Work

- Promote more block manifests into the KiCad corpus, raise selected cases from
  optional `skip` candidates to required `pass` candidates as generated PCB
  quality improves, and broaden DRC-clean evidence beyond the initial smoke
  tier.
- Broaden board-edge/imported-mechanical anchor binding proof with larger
  KiCad-backed generated fixtures and repair suggestions for bad endpoint
  declarations.
- Replace remaining structural/generic active templates with concrete
  component-catalog-backed parts where fabrication readiness is desired.
- Convert more semantic PCB constraints into downstream placement, routing,
  DRC, and repair enforcement.
- Broaden block variants beyond the initial verified/default topologies.

### Acceptance Gates

- Supported seed blocks emit schematic and PCB fragments with declared rule and
  local-route evidence.
- Unsupported seed families appear as explicit, tested gaps.
- Block-level tests cover rule metadata, PCB constraints, required route
  definitions, verification-corpus coverage, and workflow evidence output.

## Priority 3: Post-Repair Validation Adapters

### Objective

Make persisted repair success depend on full project validation, not only
transaction validation.

### Current Foundation

Repair can classify issues, mutate safe transaction operations, replay generated
projects, gate overwrite, clean stale managed files through manifests, and run
post-apply validation hooks.

Post-repair validation adapters now include writer correctness,
connectivity-first board validation, optional KiCad ERC/DRC, and optional
KiCad round-trip evidence. Persisted repair results include validation adapter
summaries, before/after issue deltas, retry budget evidence, and generated
repair bundle artifacts from `design create` repair runs. `repair
export-bundle` now packages stage issue JSON against generated targets for
non-workflow repair evidence, with dry-run-by-default behavior, inside-target
output policy, overwrite gates, malformed request handling, generated ownership
checks, hydrated transaction provenance, and selected-field CLI goldens.
Generated projects now persist `.kicadai/transaction.json`, and generated
manifests record that provenance hash so target-based repair can fail closed on
missing, stale, malformed, imported, or unsupported targets. CLI golden
fixtures now lock down generated bundle parseability, target apply overwrite
policy, post-apply validation adapter names, repaired/partial delta status
behavior, target-provenance export/apply replay, and optional versus required
missing KiCad DRC evidence.

### Remaining Work

- Continue enriching parser-specific repair categories where KiCad introduces
  new ERC/DRC report shapes.
- Extend retry budgets from persisted repair apply into full generate,
  validate, repair, and revalidate loops.
- Use explicit KiCad zone refill policy and normalized convergence evidence in
  higher-level retry loops.
- Keep broadening checked-in and opt-in KiCad ERC/DRC artifact fixtures as new
  local KiCad versions are validated.

### Acceptance Gates

- `repair apply --target ...` can produce final validation evidence from
  built-in adapters.
- `repair apply --target ...` exposes normalized findings and convergence stop
  reasons for AI callers.
- Repairs report `repaired` only when post-write validation is clean.
- Repairs report `partial` when only non-blocking issues remain or when
  blocking issue counts improve but are not eliminated.
- Repairs report `blocked` when blocking issue counts do not improve or get
  worse.

## Priority 4: Placement Engine Hardening

### Objective

Move from deterministic placement that works for small examples to placement
that produces reasonable boards.

### Current Foundation

Placement hardening foundation is implemented. The engine now carries explicit
intent and quality evidence for:

- block-derived placement groups, keepouts, constraints, and local-route
  proximity hints;
- proximity scoring with pad-distance evidence and center fallback;
- group-cohesion scoring;
- connector edge constraint scoring;
- hard and optional mechanical keepouts;
- analog/digital/power/user region preference reports;
- per-net HPWL and routing-readiness reports;
- coarse congestion reports with deterministic grid utilization evidence;
- component fanout and escape-readiness reports with edge, keepout, neighbor,
  and escape-demand pressure;
- semantic candidate scoring for role, group, electrical proximity,
  route-length, congestion, fanout, edge, region, and mobility dimensions;
- advanced placement rules for thermal spacing/edge preferences, high-current
  source-load proximity, creepage/clearance domain spacing, differential-pair
  placement-readiness, and controlled-impedance corridor/reference-plane
  evidence;
- hard candidate rejection for checkable advanced-rule violations, including
  thermal spacing, high-current length limits, and clearance spacing;
- repairable placement diagnostics for missing placements, keepouts, regions,
  proximity, routing readiness, fanout, estimated footprint geometry, grouping,
  advanced placement rules, and validation issues;
- design workflow placement summaries for advanced rule dimension counts, worst
  scores, hard violations, warnings, and unsupported/missing proof evidence;
- golden placement corpus coverage for LED, regulator, MCU minimal, USB-C
  power, I2C sensor, op-amp gain-stage, and connector-breakout layouts;
- mixed advanced-rule regression coverage for thermal, high-current,
  clearance, differential-pair, and controlled-impedance scoring evidence;
- crystal/canned oscillator timing fixture metadata, reset/programming timing
  metadata, realized timing evidence, local route length checks, load-cap and
  decoupling proximity/symmetry evidence, enable/control presence,
  ground-reference evidence, timing-sensitive placement scoring, and workflow
  `timing_results` issue reporting;
- pad-backed full-board retry seed coverage for spacing improvement,
  reduce-distance rule evidence, safe non-improvement stops, hard-constraint
  preservation, generated pad hydration, generated placement
  mobility, `local_route_mobility`, and selected CLI evidence boundaries.

### Remaining Work

- Expand timing-sensitive fixture coverage beyond the current crystal, canned
  oscillator, and reset/programming paths into other timing-critical blocks and
  KiCad-backed DRC-clean fixture evidence.
- Add spatial indexing or equivalent acceleration for very large
  advanced-rule hard-constraint sets.
- Add structured advanced-rule issue metadata instead of message-based
  diagnostic bridging.
- Expand placement-routing retry with richer convergence criteria across
  larger boards.
- Validate hardened placement outputs against KiCad DRC evidence in larger
  board-level golden projects.

### Acceptance Gates

- Placement output is deterministic and reproducible.
- Known block layouts pass proximity and board-edge rules.
- Placement failures produce repairable structured issues.

## Priority 5: Routing Engine Hardening

### Objective

Route generated boards in ways that are electrically meaningful and DRC-aware.

### Current Foundation

Routing hardening foundation is implemented. The engine now includes:

- shared `internal/pcbrules` rule resolution for net classes, role defaults,
  trace/via/layer policy, clearance matrix metadata, neckdown metadata, length
  limits, zone policy, and coupled-net intent;
- per-net route quality reports with status, endpoint counts, segment/via
  counts, length, layers, search nodes, failure categories, repair suggestions,
  and coupled group IDs;
- net class and role-aware routing for trace width, via geometry, via limits,
  preferred layer, and allowed layer policy;
- obstacle diagnostics that report nearby blocker kind/source where available;
- explicit zone handling for ignore, obstacle, unsupported, and conservative
  zone-sufficient blockers;
- length warning/failure policy and search-pressure quality scoring;
- repairable routing diagnostics for rules, zones, length policy, via policy,
  layer access, clearance, connectivity, and external checks;
- design workflow routing summaries with quality score, route report counts,
  and repair diagnostic counts;
- routing diagnostics mapped to placement retry hints for spacing, fanout,
  distance, edge, unsupported, and rule-only failures;
- opt-in bounded placement-routing retry summaries in `design create`;
- golden route corpus coverage for straight routes, keepout detours, via
  routes, length-policy blockers, and zone-policy blockers.

### Remaining Work

- Enforce full class-to-class clearance matrix behavior during occupancy and
  validation.
- Implement DRC-grade neckdown geometry around constrained pads.
- Add true same-net through-hole zero-cost layer transitions rather than only
  conventional generated vias.
- Add conservative zone-sufficient proof evidence before allowing zones to
  satisfy route segments.
- Broaden golden routing cases to more full circuit block boards and KiCad DRC
  evidence.
- Broaden iterative route/placement retry loops with more generated full-board
  fixtures, richer route-quality ranking, and KiCad DRC-backed improvement
  evidence now that generated pad summaries hydrate.

### Acceptance Gates

- Routed boards have no disconnected pads or unrouted required nets.
- Route quality reports explain routed, failed, skipped, and weak nets with
  per-net rule, length, layer, via, and search-pressure evidence.
- Shared PCB rule resolution stays deterministic across routing, workflow
  summaries, diagnostics, and future writer/DRC integrations.
- DRC failures become structured, operation-correlated issues.
- Route repair can replace generated route operations safely.

## Priority 6: Schematic Semantics And ERC

### Objective

Make schematic generation and evaluation closer to KiCad/ERC expectations.

### Current Foundation

Schematic writer, parser, generated connectivity checks, symbol resolver
foundation, and schematic round-trip compatibility exist.

### Remaining Work

- Expand `.kicad_sym` handling as real libraries expose unsupported constructs.
- Add stronger policies for multi-unit symbols, hidden pins, power symbols, and
  alternate bodies.
- Add schematic-level checks for required pins, power nets, decoupling,
  duplicate refs across hierarchy, label consistency, no-connect markers, and
  value/rating sanity.
- Support richer hierarchical sheets and cross-sheet connectivity.
- Keep schematic round-trip preservation growing with real KiCad projects.

### Acceptance Gates

- Schematic-only evaluation can catch common electrical mistakes before PCB
  generation.
- Symbol-unit and power-pin behavior is explicit and tested.
- Generated schematics round-trip through KiCad without lossy diffs for covered
  objects.

## Priority 7: Imported Project Preservation

### Objective

Allow safe AI review and eventually safe edits of user-authored KiCad projects.

### Current Foundation

Readers preserve many known structures, unsupported content is detected, and
repair apply blocks imported or preservation-only targets.

### Remaining Work

- Preserve unknown schematic and PCB nodes across read/write.
- Preserve ordering-sensitive sections and user-authored local libraries.
- Track ownership of generated vs user-authored objects.
- Add safe edit transactions for imported projects with explicit scope.
- Add conflict reports when requested edits touch unsupported objects.

### Acceptance Gates

- AI can evaluate imported projects without changing files.
- Any imported-project write is blocked unless ownership and preservation are
  proven.
- Round-trip diffs are acceptable or explicitly allowlisted.

## Priority 8: Fabrication Readiness

### Objective

Define "done" as manufacturable, not merely parseable.

### Current Foundation

Writers, board validation, ERC/DRC hooks, and fabrication export/readiness gates
exist. The `export preview`, `export bom`, and `export fabrication` commands
evaluate project readiness, model `blocked`/`candidate`/`ready` status,
generate deterministic BOM/CPL reports where evidence exists, hydrate BOM rows
with component identity evidence, hydrate CPL rows with BOM linkage, side, and
rotation evidence, validate BOM/CPL consistency, enforce safe
dry-run/execute/overwrite behavior, emit package manifests/readiness reports,
generate and validate KiCad-CLI-backed Gerber/drill artifacts, emit
`physical-rules.json`, run modeled physical fabrication checks for stackup, net
classes, solder mask/paste pad policy, Edge.Cuts containment, courtyard
presence/overlap, silkscreen board clearance, and mounting-hole
geometry/edge clearance, expose optional or required KiCad CLI evidence policy,
and optionally apply the built-in `generic_assembly` manufacturer profile
through `--manufacturer-profile`.
`design create` treats `validation.acceptance: fabrication-candidate` as a
request to prove fabrication readiness; partial readiness status (`candidate`
or `blocked`) downgrades achieved acceptance and leaves
`acceptance.fabrication_ready` false. The `fabrication_readiness` workflow stage
now includes compact physical-rule status, blocker/warning counts, profile, and
report path where available.

### Remaining Work

- Expand physical DFM coverage beyond currently modeled checks, especially
  annular ring policy, copper slivers, solder mask slivers, castellations,
  impedance constraints, panelization, board finish, and fabrication notes.
- Add richer manufacturer profile presets and profile import once specific
  fabricator rule sources are selected.
- Compare physical-rule evidence with optional KiCad DRC categories and
  manufacturer outputs where those tools expose machine-readable findings.

### Acceptance Gates

- A generated board can produce a fabrication package with KiCad-CLI-backed
  Gerber/drill evidence when a local KiCad CLI is configured.
- Missing fab artifacts or failed checks block "ready" status.
- Output package contents, identity evidence, consistency evidence, and local
  profile evidence are deterministic and test-covered.

## Priority 9: Intent Planner And AI Orchestration

### Objective

Turn higher-level requests into validated KiCad projects with explainable
decisions.

### Status

Implemented foundation.

### Current Foundation

`design create` accepts structured requests and orchestrates block planning,
component selection, schematic/PCB generation, placement, routing, validation,
optional bounded placement-routing retry, and optional repair behavior.

The `intent` command family now adds a structured planning layer above
`design create`:

- `intent plan` validates a structured intent request, derives requirements,
  chooses supported circuit blocks, applies component/validation/routing policy
  defaults, and emits `intent-plan.json` plus `generated-request.json` when an
  output directory is provided.
- `intent explain` returns a compact summary of requirements, selected blocks,
  assumptions, known gaps, and blocking issues for AI callers.
- `intent create` plans intent, refuses blocking or clarification-required
  plans, runs `design create`, and persists planner artifacts under
  `.kicadai/` in the generated project.
- `intent rationale` builds a consolidated AI-facing rationale report from a
  structured request, natural-language draft, text file, or generated target.
  It explains source evidence, interpreted intent, selected blocks/components,
  connections, assumptions, clarifications, known limits, validation summary,
  artifact references, and next actions.

Golden fixtures in `examples/intent/` cover sensor breakout, MCU programmer,
power module, amplifier module, and fabrication-oriented sensor requests.
Planning status is conservative: unsupported families, unsafe ambiguity, and
missing proof become structured issues or known gaps instead of guessed design
decisions.

Structured semantic mapping is now implemented for target, bus, and supply
intent:

- request fields can identify support targets, logical buses, and named supply
  domains;
- the supported ATmega328P-A seed MCU exposes planner-visible I2C,
  programming, UART, reset, power, and clock-capable roles;
- MCU/I2C sensor/connector intent produces concrete SDA/SCL connections when a
  compatible target is unambiguous;
- reset/programming support maps ISP and UART connections through semantic port
  roles with target-scoped signal nets;
- voltage-domain planning records selected supply source/net evidence and fails
  closed on unknown explicit supply aliases;
- external MCU clock intent reports a precise non-internal-clock topology
  limitation instead of a vague missing pin-assignment gap;
- golden fixtures cover supported semantic plans, partial known-gap plans, and
  intentionally blocked multi-MCU ambiguity.
- `intent draft` now converts supported natural-language text into structured
  intent with extraction evidence, confidence, and clarifications.
- `--text/--file ... intent explain` drafts first, stops on blocking
  clarifications, and otherwise reuses planner explanation output.
- `--text/--file ... intent create` drafts first, refuses blocking
  clarifications, runs the existing design workflow, and persists draft/source
  artifacts under `.kicadai/`.
- `intent create` now persists `.kicadai/workflow-result.json` and
  `.kicadai/design-rationale.json` alongside planner artifacts so generated
  targets can be explained after creation.

### Remaining Work

- Broaden natural-language coverage beyond deterministic seed phrases and add
  future LLM adapter auditing against the draft result shape.
- Expand semantic mapping beyond the seed MCU template, especially
  resolver-backed MCU alternate functions, additional bus peripherals, and
  safe external-clock topology generation.
- Select more blocks and parts from verified catalogs as coverage grows.
- Calculate values and check ratings.
- Connect bounded placement-routing retry, validation repair, and future
  fabrication checks into a higher-level generate/validate/repair loop.
- Store deeper part, footprint, routing, repair, and fabrication decision
  evidence in rationale artifacts as those stages expose richer provenance.

### Acceptance Gates

- The AI can explain every selected block, part, footprint, net class, and
  repair.
- Ambiguous or unsupported user intent becomes a structured clarification or
  blocked issue, not a guessed design.
- The workflow can stop safely with partial artifacts and precise blockers.

## Near-Term Recommended Sequence

1. Expand verified component and block coverage alongside each new block
   family.
2. Add stackup, courtyard, solder mask/paste, silkscreen, and mounting-hole
   fabrication checks.
3. Expand structured intent planning into a semantic design synthesis loop with
   richer MCU, interface, voltage-domain, and fabrication evidence.

## Definition Of Autonomous Ready

KiCadAI is ready for full autonomous schematic and PCB generation when:

- user intent can be converted into structured constraints;
- selected components are verified against libraries and pinmaps;
- generated schematics pass schematic validation and configured ERC;
- generated PCBs pass writer correctness, board validation, and configured DRC;
- repair loops can improve generated designs without damaging user content;
- fabrication outputs can be generated and validated;
- every unresolved issue is structured, attributable, and actionable.

Until those gates are met, the recommended AI workflow is assisted generation:
produce deterministic artifacts, validate aggressively, repair only generated
content, and ask for human review when evidence is incomplete.
