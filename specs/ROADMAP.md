# KiCadAI Roadmap

Date: 2026-07-02

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
- KiCad round-trip preservation closeout foundation with `.kicad_pro`
  modeled-section raw-key preservation, PCB track arc and slotted/oval drill
  readback, schematic DNP/unit/body-style/simulation/BOM/placement flag
  readback, symbol instance preservation, label shape/rotation preservation,
  and focused regression coverage.
- Schematic layout readability foundation with deterministic role/stage/lane
  classification, conservative readable schematic placement, orthogonal
  schematic wire routing, label fallback for long/shared nets, overlap and
  diagonal-wire diagnostics, design workflow readability evidence, wider
  op-amp gain-stage block coordinates, and design API orthogonal schematic
  connection emission. Checked-in simple examples now have standard readability
  gates, amplifier examples have strict signal-flow/rail/feedback/orthogonal
  wiring gates, and generated LED/op-amp workflow paths have summary regression
  coverage.
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
  built-in generic assembly manufacturer profile checks, physical-rule
  fabrication profile schema/validation, built-in physical-rule profile
  registry, local JSON profile loading, profile discovery/validation CLI,
  physical-rule threshold integration, profile provenance in readiness reports
  and package manifests, and `design create` fabrication-candidate acceptance
  integration.
- `design create` workflow for structured block-based design requests.
- Amplifier example/test corpus foundation with checked-in Class AB, Class A,
  and op-amp headphone-buffer schematic fixtures; amplifier semantic landmark
  checks; structured intent and text fixtures; explicit block inventory gaps;
  a draft generated op-amp headphone-buffer design request; op-amp PCB
  constraint/routing evidence checks for feedback, decoupling, input/output
  separation, high-current output width, and thermal review; and an optional
  KiCad-backed `expected_fail` fabrication-candidate amplifier fixture.
- Runnable `examples/design/*.json` requests are now covered by automated
  regression tests that strict-decode each request, run `design create`, verify
  generated project artifacts, read back generated schematic/PCB files, and
  check generated schematic component identity properties. An optional
  `examples/design/kicad-backed/` tier now validates metadata-backed fixtures
  only when `KICADAI_KICAD_CLI` is configured. The initial fixtures are
  `expected_fail` cases that keep richer generated boards visible without
  making the default test suite depend on KiCad. Generated design PCB net
  assignment now propagates pad/copper net names through placement operations,
  resolves KiCad 10 name-only net references during readback, and exposes
  workflow net-assignment evidence. `KICADAI_KICAD_CLI` supplies the executable
  path for design workflow checks; `KICADAI_RUN_KICAD_CLI=1` remains the
  separate boolean opt-in used by lower-level block smoke tests.
- KiCad-backed design promotion reports now exist for generated design
  workflows and optional fixture runs. `.kicadai/design-promotion.json`
  records declared vs achieved readiness, metadata/stage/writer/connectivity/
  KiCad/route/physical/artifact gates, referenced artifacts, issue codes, and
  repair guidance. `design create` includes a compact `data.promotion` summary
  when the report artifact is written. Current KiCad-backed fixtures are
  classified as `expected_fail` with explicit promotion blockers rather than
  silently skipped or accidentally promoted.
- Multi-endpoint generated inter-block routing now models route groups,
  resolves deterministic route trees, routes branch requests with same-net
  copper evidence, and reports group-level completion semantics. The
  `inter_block_routing` summary exposes `multi_endpoint_nets`,
  `required_endpoints`, `proven_endpoints`, `branches_planned`,
  `branches_attempted`, `branches_completed`, `graph_component_count`,
  `missing_required_endpoints`, `complete_groups`, `partial_groups`, and
  `blocked_groups`.
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
  broader golden evidence and richer parser-to-repair category mapping.
  Promotion reports now make missing or failed evidence explicit, but the
  current optional fixtures still need real ERC/DRC-clean runs before they can
  be promoted to `candidate` or `pass`;
- schematic readability now has checked-in simple/amplifier example gates and
  generated workflow evidence, but still needs broader example regeneration,
  exact KiCad text-justification geometry, hierarchy/page splitting, and safe
  imported-schematic layout mutation;
- repair can persist generated-project changes, but imported-project mutation
  remains limited to explicitly preservation-reviewed cases;
- fabrication export now provides conservative readiness gates with BOM/CPL
  identity, consistency, local assembly manufacturer-profile evidence, and
  physical-rule fabrication-profile threshold/provenance evidence, but does not
  yet guarantee complete manufacturer acceptance;
- structured intent planning now exists, and the first semantic design
  synthesis trace records topology, bus, voltage-domain, component-policy, and
  calculation evidence. Natural-language parsing remains deterministic and
  narrow, and broader topology synthesis is still intentionally limited.
  Deterministic rationale reports now explain supported planner decisions and
  blockers, but they do not replace broader synthesis.
- the default design examples are intentionally small LED workflows. LED and
  connector/LED optional KiCad-backed scenarios are now candidate fixtures with
  warning-only KiCad evidence. I2C sensor breakout and amplifier
  fabrication-candidate scenarios remain `expected_fail` fixtures. They now
  progress past the previous writer-correctness pad/copper net-code blocker,
  and the I2C fixture now progresses past stale routing-skip and local
  sensor_* net-alias blockers. LED and connector/LED fixtures also prove
  block-local route endpoint binding to physical same-net pad anchors.
  Generated inter-block routing now reports endpoint-contact evidence and
  counts completion only when a same-net contact graph connects the required
  endpoints. Multi-endpoint route-tree execution now owns the I2C fixture's
  VCC/GND/SDA/SCL nets, removes them from fallback net-level routing, and
  reports managed nets plus branch-attempt evidence. Route-tree endpoint
  access now drives branch routing through ranked pad/local-route access
  candidates, bounded synthetic access-pad attempts, selected access-role
  evidence, and snap-exempt local-anchor route operations. Route-tree endpoint
  access and contact graph summaries now expose pad access, local-route merge
  anchors, required/proven endpoints, and graph group completion evidence.
  Route-tree repair classifies branch/contact failures, emits repairable
  hints, feeds bounded placement retry, and selects attempts using route-tree
  completion evidence. The latest selected attempt proves 9 of 12 required
  contacts after failed route branches are excluded from contact proof, with
  one complete route-tree contact group and three partial groups. The current
  selected-attempt blockers are VCC/SDA same-net graph splits plus VCC/GND
  branch-scoped pathfinding blockers. Route-tree diagnostics now separate
  fixed-net skip notices and missing-net-class warnings from repairable
  blockers. The remaining layout-quality blockers are route-tree branch
  pathfinding/contact graph proof for VCC/GND/SDA, richer generated-board
  validation, and KiCad ERC/DRC-clean evidence.
- amplifier generation is currently evidence-oriented rather than
  fabrication-ready. The draft op-amp headphone-buffer request uses supported
  blocks, but Class A/Class AB output stages, headphone DC-blocking/protection,
  stability networks, verified output-device selection, load-drive limits,
  thermal/current layout proof, and optional KiCad ERC/DRC-clean promotion are
  still open blockers.

## Roadmap Principles

- Prefer deterministic, inspectable workflows before LLM orchestration.
- Treat KiCad file correctness as a hard gate, not a best effort.
- Only mutate generated projects unless preservation support is explicit.
- Expand AI autonomy through verified blocks, components, and constraints.
- Keep every repair action tied to validation evidence.
- Do not report "ready" unless writer correctness, board validation, and
  configured KiCad checks agree.

## Parallel Execution Model

Near-term AI-generation work is tracked in the machine-readable AI readiness
matrix under `data/ai-readiness/matrix/*.json`. Records may declare a
`parallel_group` and sorted `depends_on` prerequisites so contributors or agents
can split work without losing promotion order.

Current groups:

- `fixture_promotion`: KiCad-backed fixture promotion and validation evidence.
- `catalog_block_expansion`: verified component, footprint, pinmap, and block
  catalog work.
- `engine_hardening`: placement, routing, layout, and validation engine
  hardening.
- `intent_ai_ux`: intent planner and AI orchestration workflow polish.
- `documentation`: user and agent documentation.

The Go validator rejects unknown groups, dangling dependencies, duplicate
dependencies, self-dependencies, cycles, and `verified` records whose
dependencies are not also `verified`. This lets the roadmap support parallel
implementation while keeping final readiness claims conservative.

## Priority 1: Verified Component And Part Intelligence

### Objective

Make part selection reliable enough for AI-generated designs beyond demos.

### Status

Foundation plus first verified family-expansion slice implemented.

### Current Foundation

The component catalog now has verified, blocked, or policy-allowed coverage for
common passives, pin headers, LEDs, signal/Schottky/TVS diodes, a regulator, an
op-amp, small-signal BJT amplifier seeds, an MCU, an I2C sensor, a crystal,
USB-C power-only, and protection parts. It includes:

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
- local lifecycle/availability source snapshots through `--source-dir`,
  with deterministic validation, component selection gates, workflow
  procurement summaries, rationale evidence, and BOM/fabrication snapshot
  reporting;
- verified concrete alternatives for the first expanded family slice:
  0603/0805 resistor and capacitor seeds, 1x02 through 1x06 Samtec pin
  headers, 0603/0805 Lite-On indicator LEDs, signal and Schottky diodes, a
  concrete SOD-323 ESD/TVS protection diode, and onsemi MMBT3904/MMBT3906
  SOT-23 BJT amplifier seeds. Selection has deterministic
  concrete-vs-generic behavior, equivalence metadata validation, alternative
  coverage reporting, workflow procurement evidence, and BOM procurement
  snapshot propagation;
- an NPN TO-220 power-output BJT placeholder that is blocked by default
  until pinout, package, thermal, Safe Operating Area (SOA), and layout
  constraints are modeled;
- verified regulator-path coverage that connects fixed 3.3 V linear regulator
  blocks to catalog-backed AMS1117 SOT-223 and AP2112K SOT-23-5 selection,
  capacitor selection, rating requirements, generated request overrides,
  AP2112K EN/NC handling, headroom blocking, persisted workflow evidence,
  structured regulator/capacitor stability evidence, connectivity warnings,
  fabrication-candidate blockers for unproven AMS1117 ESR-window evidence,
  AP2112K MLCC derating evidence, and thermal review evidence;
- selected component identity propagation into generated schematic symbol
  properties, including component ID, variant ID, role, block ID,
  manufacturer, MPN, confidence, source, lifecycle, availability, and pinmap
  evidence where known;
- component placement/routing hint enforcement evidence: supported placement
  `near` hints feed placement proximity rules, supported routing `net_class`
  hints are checked against realized local-route widths, AP2112K EN/NC hints
  are reported as block-satisfied when emitted operations match selected
  function pins, and all unsupported/skipped/failed hint outcomes surface in
  workflow diagnostics and rationale output;
- BOM/fabrication identity extraction that prefers the generated schematic
  identity properties while retaining legacy property-name fallbacks;
- golden coverage and representative selection snapshots.

### Remaining Work

- Continue expanding from the first verified family slice to broader values,
  packages, voltages, tolerances, and real second-source alternates.
- Broaden the verified regulator path beyond the current 3.3 V AMS1117-style
  and AP2112K LDO slices, including additional voltages, adjustable/BYP
  networks, exported enable control, regulator-specific proven capacitor
  networks, and thermal/layout evidence strong enough to unblock
  fabrication-candidate regulator designs.
- Add trusted provider importers or remote snapshot adapters once a source is
  selected; current support is local snapshot JSON only.
- Replace remaining structural placeholders where verified concrete parts are
  needed.
- Promote blocked output-stage and high-power records only after pinout,
  thermal, SOA, and layout constraints are represented and tested.
- Improve MCU function names from generic GPIO placeholders to datasheet port
  and peripheral roles.
- Broaden component hint enforcement beyond the initial supported subset and
  tie hint failures to required-hint severity once catalog metadata can
  distinguish hard constraints from review guidance.
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

Implemented foundation with a documented first AI-controlled generation lane.

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
- Optional `design create` fixtures under `examples/design/kicad-backed/` now
  exercise full generated-design workflows with metadata-declared readiness,
  expected stages, expected artifacts, and required ERC/DRC policy. Current
  readiness:
  - `expected_fail`: `i2c_sensor_breakout_candidate` and
    `opamp_headphone_buffer_kicad_candidate`;
  - `candidate`: `led_indicator_kicad_smoke` and
    `connector_led_kicad_smoke`;
  - `pass`: none yet;
  - `blocked`: none yet.
- `connector_led_kicad_smoke` now has routing-enabled regression coverage for
  promoted inter-block route candidates, endpoint-contact diagnostics, and
  same-net contact graph completion semantics. `led_indicator_kicad_smoke`
  now exercises standalone exported-port labels, schematic electrical checks,
  and LED local-route contact proof. Both are candidate fixtures, not pass
  fixtures, because KiCad still reports warning-level evidence.
  `i2c_sensor_breakout_candidate` now enables routing and bounded retry, proves
  block-local VCC/GND/SDA/SCL alias propagation into local routes, and remains
  expected-fail on incomplete multi-endpoint inter-block route completion.
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
- Promote optional design examples from `expected_fail` to `candidate` and then
  `pass` by closing full inter-block routing, richer route-completion
  validation, and KiCad-clean layout blockers. Generated PCB pad/copper net
  assignment now has workflow evidence and no longer blocks the LED smoke
  fixture at writer correctness. Generated block-local route endpoints now bind
  to physical same-net pad anchors for LED, connector/LED, and I2C local-route
  fixtures. Connector/LED has candidate-level inter-block contact evidence, and
  I2C sensor breakout now has promoted VCC/GND/SDA/SCL alias propagation,
  route-tree-managed inter-block nets, endpoint-access evidence, local-route
  and same-net copper merge evidence, contact graph completion evidence,
  classified route-tree repair hints, retry selection based on route-tree
  completion evidence, and explicit selected-attempt VCC/SDA graph-split plus
  VCC/GND branch pathfinding blockers. The
  `i2c_sensor_breakout_candidate` name identifies it as a promotion candidate
  even though its current readiness is `expected_fail`.
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
- generated inter-block route contact targets, endpoint-contact proofs,
  contact summaries, same-net contact graph completion semantics, and repair
  classification for route-contact diagnostics;
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
foundation, schematic round-trip compatibility, and deterministic schematic
electrical rules exist. `design create` now emits a `schematic_electrical`
workflow stage before PCB realization, promotion reports include a matching
gate, and `evaluate schematic` / `evaluate project` expose a
`schematic_electrical` check. Current rule coverage includes references, labels,
no-connect markers, required pin intent, power rail source/sink metadata,
PWR_FLAG attachment, and decoupling/value/rating evidence hooks.

### Remaining Work

- Expand `.kicad_sym` handling as real libraries expose unsupported constructs.
- Add stronger policies for multi-unit symbols, hidden pins, power symbols, and
  alternate bodies.
- Expand schematic-level checks across hierarchy and feed richer block/component
  metadata into decoupling, power, value, and rating policies.
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
inspection/evaluation now expose structured preservation reports. Transaction
planning adds operation-level preservation reviews for imported targets, default
imported apply is fail-closed, and explicit imported apply uses locking, atomic
writes, and post-write readback validation. Fixture coverage exercises clean,
schematic-preserved, and PCB-preserved imported projects.

### Remaining Work

- Preserve unknown schematic and PCB nodes across read/write.
- Preserve ordering-sensitive sections and user-authored local libraries.
- Expand ownership tracking beyond file/object summaries into individual
  schematic and PCB elements.
- Broaden safe edit transactions beyond isolated add/write cases.
- Add conflict reports when requested edits touch unsupported objects.
- Preserve more ordering-sensitive local library and project-table content.

### Acceptance Gates

- AI can evaluate imported projects without changing files.
- Any imported-project write is blocked unless ownership and preservation are
  proven and explicit imported apply approval is supplied.
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
classes, solder-mask/paste pad policy, solder-mask web estimates, annular
rings, copper feature widths, polygonal copper width and edge-clearance
evidence, polygonal solder-mask web evidence, Edge.Cuts containment,
courtyard presence/overlap, silkscreen board clearance, mounting-hole
geometry/edge clearance, edge-plating/castellation policy, and
controlled-impedance and differential-pair evidence gaps. It also includes
fabrication metadata evidence for board finish, panelization, and fabrication
notes, exposes optional or required KiCad CLI evidence policy, applies built-in
or local physical-rule
fabrication profiles through `--manufacturer-profile` and
`--manufacturer-profile-dir`, records profile hash/source provenance in
readiness reports and package manifests, optionally applies the built-in
`generic_assembly` assembly manufacturer profile evidence, and enriches BOM
rows with local procurement snapshot fields when `--source-dir` is supplied.
Snapshot evidence is explicitly local and does not claim live stock, price, or
supplier acceptance.
`design create` treats `validation.acceptance: fabrication-candidate` as a
request to prove fabrication readiness; partial readiness status (`candidate`
or `blocked`) downgrades achieved acceptance and leaves
`acceptance.fabrication_ready` false. The `fabrication_readiness` workflow stage
now includes compact physical-rule status, blocker/warning counts, profile, and
report path where available.

### Remaining Work

- Expand physical DFM coverage beyond the current conservative polygon-backed
  checks, especially exact constructive-geometry sliver proof for every KiCad
  feature, specific board-house rule source curation, deeper edge-plating
  evidence, and solver-grade impedance proof.
- Add reviewed board-house profile snapshots once specific fabricator rule
  sources are selected, and keep them clearly labeled as local constraints
  rather than manufacturer acceptance guarantees.
- Add live or provider-backed procurement import once a trusted source and
  cache/update policy are selected.
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

Public `examples/design/*.json` requests are now executable regression fixtures
for `design create`. The default set exercises supported LED workflows and
proves that checked-in examples remain aligned with the current request schema,
block contracts, project writer, schematic/PCB readers, and component identity
property propagation. Optional KiCad-backed design examples now live under
`examples/design/kicad-backed/`; LED and connector/LED are candidate fixtures,
while richer I2C and amplifier generated boards still record `expected_fail`
evidence. I2C now reaches route-tree-managed inter-block routing with clean
local-route alias/contact proof, route-tree endpoint access, contact graph
completion evidence, route-tree repair hints, and selected retry evidence. It
still blocks on selected-attempt VCC graph-split/pathfinding evidence; the
protected Class AB headphone amplifier fixture now verifies the
`headphone_output_protection` block summary and verified
LMV321/MMBT3904/MMBT3906 selection path, then stops at schematic electrical
validation on expected label-alias conflicts (`headphones_SIG` versus
`output_amp_out` and `output_lower_emitter` versus `output_upper_emitter`). The
next amplifier promotion task is schematic alias cleanup so PCB realization and
real KiCad ERC/DRC evidence can run.

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
- Prompt-driven `intent create` now exposes `data.ai_status` and persists
  `.kicadai/validation-summary.json`, `.kicadai/retry-state.json`, and
  `.kicadai/manifest.json` for AI control loops. Status values distinguish
  `ready`, `candidate`, `blocked`, `needs_clarification`, `unsupported`, and
  `tool_error`.
- The first AI-controlled prompt lane is locked down by golden tests for simple
  LED indicator, connector breakout with power LED, and 3.3 V I2C sensor
  breakout prompts. Unsafe mains/high-voltage and ambiguous battery prompts
  fail closed instead of receiving guessed low-voltage defaults.
- Retryable blockers now include deterministic `retry_key`, repair category,
  and optional repair bundle path when a generated repair bundle exists.
  Agents should construct repair apply commands from trusted executable and
  workspace paths; revalidation remains required after repair.
- `intent-plan.json` now includes `synthesis`, a deterministic trace with
  topology decisions, bus/target/voltage-domain evidence, component policy
  constraints, value-calculation records, applied/deferred/blocked calculation
  status, and fail-closed synthesis gaps.
- Supported value calculations now mutate generated block parameters for LED
  resistors, I2C pull-ups, and crystal load capacitors. Regulator headroom and
  op-amp gain are retained as explicit calculated requirements. Component
  rating overrides are emitted where the checked-in catalog models the rating,
  including LED current, LED resistor power, and op-amp supply voltage.
- Semantic synthesis fixtures cover explicit MCU/I2C supply domains, UART
  programming topology, blocked unknown supply aliases, and blocked external
  clock topology.

### Remaining Work

- Broaden natural-language coverage beyond deterministic seed phrases and add
  future LLM adapter auditing against the draft result shape.
- Promote the first prompt-driven LED lane from current precise placement
  blocker to passing/candidate once placement repair or board sizing policy can
  close the generated placement issue.
- Expand semantic synthesis beyond the seed MCU template, especially
  resolver-backed MCU alternate functions, additional bus peripherals,
  supported GPIO assignment, and safe external-clock topology generation.
- Select more blocks and parts from verified catalogs as coverage grows.
- Expand calculated rating enforcement as the component catalog gains broader
  voltage, capacitance, power, current, and tolerance metadata across more
  families.
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
