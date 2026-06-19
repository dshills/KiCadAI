# KiCadAI Roadmap

Date: 2026-06-19

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
  guards, and design workflow evidence output.
- Placement engine foundation with deterministic placement model support.
- Routing engine foundation for small generated PCB nets.
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
  workflow repair bundle artifacts.
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
- circuit block coverage still needs more variants and concrete generators for
  crystal/oscillator, standalone reset/programming, ESD, and reverse-polarity
  protection families;
- placement and routing need stronger rules for real PCB quality;
- KiCad-backed validation exists in the repair and workflow loops, but needs
  broader golden evidence and richer parser-to-repair category mapping;
- repair can persist generated-project changes, but imported-project mutation
  remains blocked by preservation safety;
- fabrication export is still not a complete release gate;
- natural-language intent planning is not yet a first-class pipeline.

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
  power, I2C sensor, and op-amp gain-stage blocks declare electrical rules,
  PCB constraints, and required local-route expectations.
- PCB realization metadata includes placement groups, keepouts, proximity
  constraints, route width constraints, edge-facing constraints, and local
  route definitions for supported block-local nets.
- MCU minimal realization now includes power decoupling, AREF decoupling,
  reset pull-up, and local route evidence for its supported ATmega328P-A
  template.
- Block verification corpus covers every built-in block and has regression
  guards that required routes remain defined.
- `design create` block-planning output exposes block readiness, verification
  level, rule IDs, required routes, and known gaps for AI-facing explanation.

### Remaining Work

- Add concrete generators for crystal/oscillator, standalone reset/programming,
  ESD, and reverse-polarity protection blocks.
- Replace remaining structural/generic active templates with concrete
  component-catalog-backed parts where fabrication readiness is desired.
- Add KiCad-backed ERC/DRC evidence to block verification manifests where
  external KiCad CLI is available.
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
repair bundle artifacts from `design create` repair runs.

### Remaining Work

- Expand post-repair adapters to include richer parser-specific evidence once
  ERC/DRC findings are mapped to repair categories consistently across all
  flows.
- Add a dedicated repair bundle export command for non-`design create` flows.
- Extend retry budgets from persisted repair apply into full generate,
  validate, repair, and revalidate loops.
- Implement KiCad zone refill only under explicit KiCad CLI policy.
- Add golden CLI fixtures for post-repair validation summaries and issue deltas.

### Acceptance Gates

- `repair apply --target ...` can produce final validation evidence from
  built-in adapters.
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

Placement planning and model support exist, and design workflows can place
generated footprints.

### Remaining Work

- Group components by circuit block and net role.
- Place connectors based on edge/orientation constraints.
- Place decoupling capacitors near power pins.
- Keep crystals and oscillators near MCUs.
- Separate analog, digital, high-current, and noisy regions.
- Model mounting holes, board outlines, keepouts, and mechanical constraints.
- Add congestion, route-length, thermal, and DRC feedback scores.
- Support iterative placement repair from validation findings.

### Acceptance Gates

- Placement output is deterministic and reproducible.
- Known block layouts pass proximity and board-edge rules.
- Placement failures produce repairable structured issues.

## Priority 5: Routing Engine Hardening

### Objective

Route generated boards in ways that are electrically meaningful and DRC-aware.

### Current Foundation

Routing request/result models, small-net routing, validation, and transaction
emission exist.

### Remaining Work

- Add net classes with trace width, clearance, via, and layer constraints.
- Add power, signal, analog, and high-current routing policies.
- Improve obstacle avoidance around footprints, keepouts, zones, and board
  edges.
- Support two-layer routing with via policy and return-path awareness.
- Add differential pair and length-matching foundations.
- Feed DRC and connectivity findings back into route repair.
- Improve zone and copper pour handling.

### Acceptance Gates

- Routed boards have no disconnected pads or unrouted required nets.
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

Writers, board validation, ERC/DRC hooks, and export command placeholders exist.

### Remaining Work

- Generate and validate Gerbers, drill files, BOM, CPL/position files, and a
  fabrication package manifest.
- Add stackup, net class, solder mask, paste, edge cuts, courtyard, silkscreen,
  and mounting-hole checks.
- Add fabrication-readiness score and blocking issue taxonomy.
- Integrate manufacturer profile constraints when available.

### Acceptance Gates

- A generated board can produce a complete fabrication package.
- Missing fab artifacts or failed checks block "ready" status.
- Output package contents are deterministic and test-covered.

## Priority 9: Intent Planner And AI Orchestration

### Objective

Turn higher-level requests into validated KiCad projects with explainable
decisions.

### Current Foundation

`design create` accepts structured requests and orchestrates block planning,
component selection, schematic/PCB generation, placement, routing, validation,
and optional repair behavior.

### Remaining Work

- Add an intent schema above block-level requests.
- Parse user requirements into electrical, mechanical, manufacturing, and
  validation constraints.
- Select blocks and parts from verified catalogs.
- Calculate values and check ratings.
- Produce a design rationale and known-limit report.
- Add iterative generate/validate/repair loops with bounded budgets.
- Store all decisions, assumptions, and validation evidence in artifacts.

### Acceptance Gates

- The AI can explain every selected block, part, footprint, net class, and
  repair.
- Ambiguous or unsupported user intent becomes a structured clarification or
  blocked issue, not a guessed design.
- The workflow can stop safely with partial artifacts and precise blockers.

## Near-Term Recommended Sequence

1. Improve placement rules for block-aware layouts and component placement
   hints.
2. Improve routing rules, net classes, and DRC-driven route repair.
3. Add golden CLI fixtures for post-repair validation summaries, issue deltas,
   and generated repair bundles.
4. Add a dedicated repair bundle export command for non-`design create` flows.
5. Add fabrication export/readiness gates.
6. Expand verified component and block coverage alongside each new block
   family.
7. Add intent-level planning only after the above gates are reliable.

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
