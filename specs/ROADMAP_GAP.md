# Roadmap Gap Analysis

Date: 2026-06-17

This document reviews `specs/ROADMAP.md` against the current project state and
identifies where the roadmap is stale, where implementation exists but needs
hardening, and which gaps still block reliable AI-generated KiCad projects.

## Executive Summary

The roadmap is directionally correct, but it now mixes three different states:

- foundational work that has already been implemented;
- partially implemented systems that need quality, coverage, and completeness;
- major open gaps that still prevent autonomous schematic and PCB generation.

The largest remaining blockers are not the existence of a writer, CLI, placement
engine, routing engine, or circuit blocks. Those foundations exist. The largest
remaining blockers are component intelligence, full symbol semantics, validated
block quality, closed-loop repair, and manufacturing readiness. Writer
correctness is now also a first-class implemented gate: it provides project,
schematic, PCB, transfer, pad-net, copper-net, zone-net, and optional KiCad
round-trip checks, but it should continue to harden as new generated designs are
added.

The roadmap should be reorganized around the path from "can generate known
examples" to "can reliably generate useful KiCad projects from intent."

## Update: Writer Correctness Closeout Completed

Since the first version of this gap analysis, the Writer Correctness Closeout
project has been implemented.

Implemented evidence:

- `writer check` CLI command for project directories, `.kicad_pro`,
  `.kicad_sch`, and `.kicad_pcb` targets.
- `internal/writercorrectness` result model and validation orchestration.
- project structure discovery checks.
- schematic parse and generated-connectivity checks.
- schematic-to-PCB transfer checks.
- PCB net table and footprint pad-net checks.
- copper and zone net-reference checks.
- optional KiCad round-trip evidence.
- golden corpus coverage for expected passing and blocking cases.
- AI design workflow integration through the `writer_correctness` stage.
- README documentation for command usage, checks, flags, and limits.

Remaining writer-correctness follow-up work belongs under hardening rather than
new foundation work:

- expand golden corpus cases as new examples and blocks are added;
- keep closing PCB reader/writer hydration gaps exposed by real KiCad round
  trips;
- add compatibility aliases only if CLI naming becomes a real usability issue;
- increase KiCad-backed evidence where stable local fixtures exist;
- ensure every future AI design workflow path is gated by `writer check`.

## Update: Component Intelligence Foundation Implemented

The Component Intelligence Model project has moved from roadmap gap to
implemented foundation. The project now has a typed component catalog, curated
seed records, deterministic selection, confidence and acceptance gates,
resolver/pinmap evidence hooks, CLI commands, circuit-block annotations, and
`design create` integration through the `component_selection` stage.

Implemented evidence:

- `data/components/` seed catalog for passives, connectors, LEDs, diodes, and
  current placeholder active blocks.
- `internal/components` catalog model, loader, validator, selector, and
  resolver evidence checks.
- `component list`, `component show`, `component find`, `component select`, and
  `component validate` CLI commands.
- circuit block component metadata for selected built-in blocks.
- component selection gating in the AI design workflow before schematic or PCB
  writes.
- component selection examples under `examples/components/`.
- component intelligence guide under `docs/component-intelligence.md`.

Remaining component-intelligence follow-up work belongs under hardening and
catalog expansion rather than foundation work:

- expand verified manufacturer-grade records beyond the seed catalog;
- verify broader active components with symbol-footprint-pinmap evidence;
- add sourcing, lifecycle, derating, tolerance, and environmental constraints;
- grow golden corpus tests for selection output and unsafe choices;
- use exact part choices to drive richer schematic value/property output.

## Update: Symbol Resolver Hardening Implemented

The Symbol and Footprint Library Resolver roadmap item has also moved from
foundation gap to implemented foundation for symbol semantics. The project now
has a hardened symbol parser and resolver path that can parse representative
KiCad `.kicad_sym` files, model units, common pins, electrical types, power
symbols, inherited records, rough graphics bounds, project/global symbol
tables, and expose resolver evidence through stable CLI JSON.

Implemented evidence:

- `internal/libraryresolver` symbol records for units, pins, electrical types,
  hidden/common flags, power symbols, inherited metadata, and graphics bounds.
- project-local and global `sym-lib-table` parsing with KiCad path-variable
  expansion and deterministic precedence.
- component evidence validation against resolver-backed symbol pins and unit
  policy.
- schematic transaction hydration that can use resolver pin geometry for wire
  anchors.
- `library symbols list`, `library symbols show`, `library symbols pins`, and
  `library symbols validate` CLI commands.
- writer correctness integration through the `library_resolver` check and
  unresolved-symbol diagnostics when symbol roots are configured.
- checked-in CLI golden corpus for list/show/pins/blocked validation outputs.
- optional external KiCad symbol smoke test gated by
  `KICADAI_RUN_EXTERNAL_SYMBOL_TESTS=1` and `KICAD_SYMBOLS_DIR`.

Remaining symbol-resolver follow-up work belongs under hardening and library
coverage rather than foundation work:

- model additional KiCad symbol constructs as they appear in real corpus
  failures;
- add explicit safe policies for hidden power pins and multi-unit component
  usage;
- grow golden fixtures from real KiCad libraries when regressions are found;
- keep symbol, footprint, and component evidence aligned as the catalog expands;
- use resolver evidence more deeply in autonomous design repair loops.

## High-Priority Findings

### 1. Roadmap Status Is Stale

Several items still read as if they are future projects even though first-pass
implementations now exist.

Implemented or substantially started:

- Footprint Library Expansion
- Schematic-to-PCB Transfer
- Connectivity-First Board Validation
- Writer Correctness Closeout
- Circuit Block PCB Realization
- AI Design Workflow CLI
- Placement Engine foundation
- Routing Engine foundation
- ERC/DRC Feedback Loop foundation

These should move out of "Most Important Next Projects" and into an
"Implemented, Needs Hardening" section. The work remaining in these areas is
quality, completeness, validation, and autonomous repair, not initial
construction.

### 2. AI Design Workflow CLI Is No Longer Only a Proposal

`ROADMAP.md` still lists a command like:

```text
kicadai design create --request request.json --output ./out/project
```

as a next project. That workflow now exists as the deterministic AI-facing
entrypoint.

The remaining gap is not exposing the command. The gap is making the command
autonomous enough to:

- infer a design from higher-level intent;
- select appropriate blocks and parts;
- generate schematic and PCB artifacts;
- validate results;
- classify issues;
- repair the design;
- repeat until the result is acceptable or clearly blocked.

### 3. Component Intelligence Expansion Remains a Blocker

The project now has a component intelligence foundation and seed catalog, but
it does not yet have enough verified component breadth for general autonomous
design.

Remaining needs:

- verified symbol-to-footprint mappings for broad component families;
- pin function maps;
- package variants;
- polarity rules;
- electrical types;
- default passives by use case;
- voltage, tolerance, power, current, and temperature constraints;
- manufacturer part numbers;
- lifecycle and availability metadata;
- connector family conventions;
- regulator, op-amp, MCU, sensor, crystal, USB, RF, protection, and power
  component support.

Without expanding this layer, the AI can assemble known examples but cannot
safely choose parts for arbitrary user requirements.

### 4. Full Symbol Library Handling Still Trails Footprint Handling

Footprint expansion has advanced further than schematic symbol handling. The
roadmap correctly identifies this, but it should rank full symbol semantics as a
near-term blocker.

Remaining needs:

- complete `.kicad_sym` parsing;
- project and global `sym-lib-table` resolution;
- symbol inheritance handling;
- multi-unit symbols;
- alternate bodies;
- hidden pins;
- power symbols;
- pin geometry;
- pin electrical types;
- stable symbol instance generation matching KiCad conventions;
- preservation of embedded symbols;
- reliable symbol-pin to footprint-pad mapping.

This is required before the system can confidently generate more complex
schematics or validate schematic-to-PCB transfer for real parts.

### 5. Circuit Blocks Exist, But Need Verification Depth

`ROADMAP.md` says only LED and breakout flows exist and that there is no broad
library of reusable circuit blocks. That is stale. The project now has broader
circuit-block work and PCB realization support.

The remaining gap is verification depth and design richness.

Each block should eventually include:

- resolver-backed symbols and footprints;
- pinmap validation;
- parameterized design inputs;
- calculated values;
- required constraints;
- schematic fixture;
- PCB fixture;
- expected ERC/DRC behavior;
- expected netlist;
- local placement rules;
- local routing rules;
- known-good KiCad validation evidence.

The block library should be treated as a verified design corpus, not only a set
of code helpers.

### 6. Placement and Routing Are Foundations, Not Finished Engines

Placement and routing support now exist, but the current implementations are
still conservative and deterministic. That is appropriate for the foundation,
but the roadmap should distinguish "implemented foundation" from "quality and
autonomy."

Placement gaps:

- connector edge placement;
- decoupling capacitor proximity;
- crystal and oscillator proximity;
- power path grouping;
- analog/digital separation;
- thermal placement;
- keepouts;
- mounting holes;
- board outline inference from user intent;
- block-aware placement constraints.

Routing gaps:

- net classes;
- trace width and current calculations;
- differential pairs;
- length matching;
- via strategy;
- layer stack awareness;
- copper pours and thermal relief;
- clearance-aware pathfinding;
- obstacle avoidance;
- zone-aware routing;
- DRC-driven rerouting.

### 7. Validation Exists, But Closed-Loop Repair Is Missing

The project has validation infrastructure and optional KiCad checks. The missing
piece is an autonomous repair loop.

Needed loop:

```text
generate -> validate -> classify issue -> plan repair -> patch design -> revalidate
```

The repair loop should include:

- retry budgets;
- deterministic issue classes;
- stage ownership for repairs;
- rollback on failed repair;
- final blocked reports when repair is unsafe;
- regression fixtures for known failure patterns.

This is the difference between "the system reports errors" and "the system can
finish a design without human intervention."

### 8. Manufacturing Readiness Remains Mostly Open

The roadmap is accurate that manufacturing readiness is not complete.

Remaining needs:

- Gerber generation;
- drill generation;
- BOM export;
- CPL / position export;
- fabrication package manifest;
- board stackup;
- design rules;
- solder mask and paste handling;
- edge cuts validation;
- mounting holes;
- silkscreen cleanup;
- courtyard and collision checks;
- clearance evidence;
- fabrication readiness report.

This should become a dedicated project after schematic/PCB correctness and
validation repair are stronger.

### 9. Round-Trip Preservation Should Be Split Into Two Tracks

Round-trip preservation matters most when modifying existing KiCad projects.
Greenfield AI generation has a related but distinct need: producing clean,
valid, KiCad-native files from scratch.

Recommended split:

- Existing-project mutation safety
- Greenfield writer correctness

Existing-project mutation safety should focus on preserving unknown nodes,
ordering, unsupported objects, child sheets, and user-authored content.

Greenfield writer correctness should focus on producing KiCad-native project,
schematic, PCB, library table, netlist, and fabrication artifacts that pass
validation without relying on user cleanup.

### 10. Roadmap Structure Needs Cleanup

`Most Important Next Projects` appears twice with the same list. The roadmap
also includes stale current-state notes for areas that now have first-pass
implementations.

Recommended cleanup:

- remove duplicate project list;
- add status labels;
- distinguish foundation from hardening;
- add acceptance criteria for each next project;
- add explicit dependencies between projects;
- keep "AI Planner" gated behind component intelligence, symbol semantics,
  validation repair, and block verification.

## Current Gap Matrix

### Implemented, Needs Hardening

These areas exist and should now be improved rather than restarted:

- Footprint Library Expansion
- Schematic-to-PCB Transfer
- Connectivity-First Board Validation
- Writer Correctness Closeout
- Circuit Block PCB Realization
- AI Design Workflow CLI
- Placement Engine foundation
- Routing Engine foundation
- ERC/DRC Feedback Loop foundation

Main hardening work:

- more fixtures;
- more golden corpus coverage;
- stricter validation;
- better diagnostics;
- real KiCad CLI evidence;
- repair planning;
- preservation of generated correctness under rewrite;
- broader writer correctness corpus coverage and KiCad round-trip evidence.

### Partially Implemented

These areas have meaningful work but are not complete enough for broad
autonomous use:

- Circuit Block Library
- KiCad CLI validation loop
- Schematic quality rules
- Footprint resolver completeness
- Symbol resolver completeness
- Round-trip preservation
- Writer-backed design workflow output quality across larger component families

Main completion work:

- broader component support;
- resolver-backed symbol and footprint metadata in every generation path;
- expected ERC/DRC fixtures;
- block-level constraints;
- generated netlist evidence;
- full schematic-to-PCB net correctness for nontrivial, multi-block projects.

### Major Open Gaps

These are the highest-risk missing capabilities for full AI design generation:

- Component intelligence expansion and verification beyond the seed catalog
- Full symbol library semantics
- Natural-language intent-to-design planner
- Closed-loop autonomous validation repair
- Manufacturing export and readiness

## Recommended Next Priority Order

Active next project:

- `specs/symbol-resolver-hardening/SPEC.md`

### 1. Full Symbol Resolver Hardening

Bring schematic symbol semantics closer to footprint support.

Focus:

- `.kicad_sym` parser completeness;
- symbol inheritance;
- units;
- hidden pins;
- alternate bodies;
- electrical types;
- power symbols;
- project/global library table resolution;
- symbol placement based on real pin geometry.

Acceptance criteria:

- common symbols resolve with pin metadata;
- multi-unit or unsupported symbols block safely;
- schematic generation can use real symbol pin geometry for common parts;
- symbol-to-footprint assignment validation uses real symbol pins.

### 2. Circuit Block Verification Harness

Status: implemented foundation.

The project now has a circuit block verification harness and checked-in corpus.
All built-in blocks have at least one manifest under
`internal/blocks/testdata/verification`, the CLI can run one case, a suite, or
the built-in corpus through `block verify`, and design workflow planning can
cite each block's strongest built-in evidence level.

Implemented evidence:

- manifest model and validation for evidence levels, components, ports, nets,
  PCB expectations, writer expectations, and optional ERC/DRC expectations;
- semantic assertion engine for generated refs, roles, symbols, footprints,
  ports, nets, and pin memberships;
- PCB assertion support for placements, footprint IDs, pad nets, required
  routes, and required zones;
- writer correctness integration for generated block projects;
- optional KiCad ERC/DRC hooks with skipped, required, allowed, and expected
  issue behavior;
- `kicadai --json block verify --case|--suite|--builtins` CLI;
- golden report snapshots for built-ins, a passing case, a blocked case, and
  skipped ERC/DRC evidence;
- design workflow `block_planning` evidence summaries and fabrication-candidate
  evidence gating.

Remaining follow-up work:

- embed or package built-in verification manifests for installed binaries
  instead of relying on source-tree paths or `KICADAI_BLOCK_VERIFICATION_ROOT`;
- add singleflight-style cache population if design workflow block evidence
  lookup becomes hot under concurrent service traffic;
- raise more block manifests from `schematic_verified` to `pcb_verified` and
  `erc_drc_verified` as local PCB/DRC evidence matures;
- expand golden report cases as new blocks and evidence levels are added.

### 3. Closed-Loop Validation Repair

Add a repair engine that can respond to validation failures.

Focus:

- issue classification;
- repair actions;
- retry budgets;
- rollback;
- staged revalidation;
- blocked reports.

Acceptance criteria:

- common generated-design failures can be repaired automatically;
- unrecoverable issues produce clear blocked reports;
- repair behavior is deterministic and covered by tests.

### 4. Placement Constraint Engine

Improve placement from deterministic layout to constraint-aware layout.

Focus:

- block-local placement;
- connector edge rules;
- decoupling proximity;
- oscillator proximity;
- analog/digital grouping;
- power path grouping;
- keepouts and board outline constraints.

Acceptance criteria:

- blocks can emit placement constraints;
- the placement engine respects constraints or reports violations;
- validation can detect placement rule failures.

### 5. Routing Quality Engine

Improve routing from basic connectivity to rule-aware board routing.

Focus:

- net classes;
- clearance;
- width rules;
- via rules;
- differential-pair metadata;
- simple obstacle avoidance;
- zone connectivity;
- DRC-driven rerouting.

Acceptance criteria:

- generated routes use net classes;
- route validation catches wrong-net and unrouted cases;
- route repair can fix common local failures.

### 6. Writer Correctness Hardening

Keep the new writer correctness gate aligned with the expanding generator.

Focus:

- larger golden corpus;
- KiCad round-trip fixtures for representative schematics and boards;
- stricter cross-file netlist evidence;
- unsupported-object detection;
- generated workflow examples that exercise blocks, placement, and routing;
- diagnostics that point to responsible symbols, footprints, pads, tracks, or
  zones.

Acceptance criteria:

- every generated example has a writer-correctness expectation;
- every new writer feature ships with a blocking and passing corpus case;
- KiCad-backed round-trip evidence is available for stable fixtures;
- writer failures remain structured enough for future repair planning.

### 7. Fabrication Export Readiness

Once generated designs are electrically meaningful, add manufacturing output.

Focus:

- Gerbers;
- drills;
- BOM;
- CPL;
- position files;
- fabrication manifest;
- readiness report.

Acceptance criteria:

- a generated demo project can produce a complete fabrication package;
- outputs are validated for required layers and files;
- package report clearly states readiness and remaining warnings.

## Suggested Roadmap Rewrite Shape

The roadmap should be rewritten into these sections:

1. Completed foundations
2. Implemented but needs hardening
3. Current blockers to autonomous generation
4. Next projects in priority order
5. Manufacturing backlog
6. Long-term AI planner work

Each project should include:

- status;
- dependencies;
- acceptance criteria;
- current implementation evidence;
- known gaps;
- next concrete phase.

## Bottom Line

The project has moved past the initial file-writer phase. The next roadmap
should focus on correctness, component knowledge, validation evidence, and
repair. The path to autonomous AI-generated KiCad projects is now less about
adding more surface area and more about making the existing generation pipeline
trustworthy.
