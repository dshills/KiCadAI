# KiCadAI Roadmap

Date: 2026-06-18

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
- Component intelligence foundation with catalog records, confidence gates,
  resolver evidence, CLI commands, and design workflow integration.
- Circuit block library foundation and PCB realization for verified blocks.
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
- `design create` workflow for structured block-based design requests.
- README and focused docs for current CLI capabilities.

### Still Not Ready

KiCadAI is not yet ready for unconstrained "make me any board" autonomous
design. The remaining blockers are mostly breadth, quality gates, and closed
loop confidence:

- component catalog breadth is still small;
- many active parts need verified symbol, footprint, pinmap, and design-rule
  evidence;
- circuit blocks need deeper electrical validation and layout constraints;
- placement and routing need stronger rules for real PCB quality;
- KiCad-backed validation needs to be used more consistently;
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

### Current Foundation

The component catalog, resolver evidence checks, confidence gates, CLI commands,
and workflow integration exist.

### Remaining Work

- Expand the verified catalog for passives, connectors, regulators, op-amps,
  MCUs, sensors, crystals, USB-C parts, protection devices, LEDs, diodes, and
  power components.
- Add manufacturer part numbers, lifecycle, availability, voltage/current/power
  ratings, tolerance, temperature, and package constraints.
- Add explicit polarity, pin function, electrical type, and footprint pad maps.
- Connect catalog selections to richer schematic properties and PCB rules.
- Add golden tests for component selection, unsafe choices, and resolver
  evidence failures.

### Acceptance Gates

- Common design intents can select concrete parts without placeholder active
  components.
- Every selected part has symbol, footprint, and pinmap evidence.
- Unsafe or under-evidenced selections block the workflow with actionable
  issues.

## Priority 2: Circuit Block Library Expansion

### Objective

Build a reusable library of verified schematic and PCB design fragments.

### Current Foundation

Circuit blocks exist, and PCB realization can produce fragments, placements,
footprints, routed local connections, and constraints for known blocks.

### Remaining Work

- Harden and expand blocks for:
  - LED indicators;
  - regulators;
  - MCU minimal systems;
  - USB-C power;
  - I2C sensors;
  - op-amp gain stages;
  - connector breakouts;
  - crystals/oscillators;
  - reset/programming headers;
  - ESD and reverse-polarity protection.
- Encode electrical rules: required decoupling, pull-ups, enable pins, boot
  straps, thermal constraints, analog biasing, and rail compatibility.
- Encode PCB rules: placement groups, proximity, edge constraints, keepouts,
  route priorities, zone requirements, and local DRC constraints.
- Add block verification manifests with ERC/DRC evidence where possible.

### Acceptance Gates

- Each block can emit schematic and PCB fragments with validation evidence.
- Blocks fail closed when required components, footprints, nets, or constraints
  are missing.
- Block-level tests cover both generated files and design-rule expectations.

## Priority 3: Post-Repair Validation Adapters

### Objective

Make persisted repair success depend on full project validation, not only
transaction validation.

### Current Foundation

Repair can classify issues, mutate safe transaction operations, replay generated
projects, gate overwrite, clean stale managed files through manifests, and run
post-apply validation hooks.

### Remaining Work

- Add built-in post-apply adapters for:
  - writer correctness;
  - connectivity-first board validation;
  - KiCad ERC;
  - KiCad DRC;
  - optional round-trip checks.
- Persist repair bundles from `design create` or add a dedicated CLI export
  command so target-based repair apply is fully CLI-reproducible.
- Add retry budgets across generate, validate, repair, and revalidate cycles.
- Implement KiCad zone refill only under explicit KiCad CLI policy.
- Preserve richer before/after validation evidence and issue deltas.

### Acceptance Gates

- `repair apply --target ...` can produce final validation evidence without
  caller-supplied fake validators.
- Repairs report `repaired` only when post-write validation is clean.
- Worsened, repeated, or unresolved blocking issues stay blocked or partial.

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

1. Build post-repair validation adapters and bundle export support.
2. Expand verified component records for the next target design families.
3. Harden circuit blocks with electrical and layout constraints.
4. Improve placement rules for block-aware layouts.
5. Improve routing rules and DRC-driven route repair.
6. Add fabrication export/readiness gates.
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
