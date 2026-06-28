# Generated Design PCB Net Assignment Specification

Date: 2026-06-28

## Summary

Generated design-level PCBs must carry complete, consistent net assignment
evidence before they are handed to writer correctness, board validation, or
optional KiCad ERC/DRC checks.

This specification closes the blocker currently visible in the optional
KiCad-backed design examples: generated footprints, pads, tracks, vias, zones,
and local-route copper can be structurally present while still lacking
trustworthy net codes or net names. The result is a board that may be
parseable, but is not electrically meaningful enough for KiCad validation.

The goal is to make generated schematic-to-PCB workflows produce a canonical
PCB net table, assign every known pad and copper object against that table, and
emit deterministic diagnostics for every unresolved or ambiguous mapping.

## Problem

The design workflow now produces small generated KiCad projects and optional
KiCad-backed fixtures, but some richer examples are still marked
`expected_fail`. The failures occur before meaningful KiCad validation because
post-write checks detect incomplete net evidence, including failures such as:

- footprint pads missing net codes;
- pad net names that do not match the board net table;
- copper objects missing net codes;
- tracks or local routes that cannot be proven to connect the intended pads;
- warnings that hydrated footprint pads exist, but no reliable pin-to-pad net
  assignment was propagated into the generated board.

This is an important correctness boundary. A board that visually contains
components and tracks but lacks valid net assignments can look plausible while
being electrically wrong.

## Goals

- Build one canonical net assignment path for generated design PCBs.
- Derive a deterministic PCB net table from schematic/design intent,
  block ports, explicit connections, required local routes, and generated
  power nets.
- Assign stable positive net codes to all named nets while reserving net code
  `0` for no-net KiCad objects.
- Assign matching net names and net codes to generated footprint pads.
- Assign matching net names and net codes to generated copper objects,
  including tracks, vias, local-route segments, and zones where modeled.
- Reuse existing resolver-backed footprint hydration and pad summary evidence.
- Reuse verified component pinmaps and block component pin definitions.
- Preserve writer correctness as the hard gate that detects bad assignments.
- Emit deterministic diagnostics for unresolved, ambiguous, or conflicting
  pad and copper mappings.
- Promote optional KiCad-backed design examples only when writer correctness,
  board validation, and optional KiCad checks provide evidence.

## Non-Goals

- Do not solve arbitrary KiCad global library resolution for every symbol or
  footprint family.
- Do not infer unsafe pin mappings for unknown components.
- Do not make every generated board route-complete or DRC-clean in this
  project alone.
- Do not bypass writer correctness, board validation, or KiCad checks.
- Do not mutate imported user projects or preservation-sensitive content.
- Do not treat visual wire/track proximity as electrical proof without pad and
  copper net assignments.

## Current Foundations

The project already has several pieces that this work should use rather than
replace:

- schematic and PCB writers;
- schematic-to-PCB transfer workflow;
- transaction and operation-correlated validation issues;
- resolver-backed footprint hydration;
- footprint pad summary generation;
- verified component pinmaps;
- block component pin definitions and ports;
- placement and routing engines;
- board validation for pad nets, unrouted nets, route completion, outlines,
  zones, and DRC evidence hooks;
- writer correctness checks for project, schematic, PCB, transfer, pad-net,
  copper-net, and zone-net issues;
- optional KiCad-backed design examples gated by `KICADAI_KICAD_CLI`.

This project bridges those foundations into a single generated-design net
assignment contract.

## Required Model

### Net Identity

Generated design PCBs must have a canonical net identity model with:

- `NetName`: exact KiCad-facing net name;
- `NetCode`: integer PCB net code, with `0` reserved for no-net;
- `Source`: schematic connection, block port, required local route, generated
  power net, zone, or explicit route endpoint;
- `References`: component references and pin/pad endpoints contributing to the
  net;
- `Confidence`: verified, derived, generated, ambiguous, or unresolved;
- `Issues`: deterministic diagnostics for aliases, conflicts, or missing
  evidence.

Net codes must be deterministic. If the source design does not provide stable
codes, codes should be assigned in a repeatable order from normalized net
names, while preserving KiCad no-net behavior.

### Pad Assignment

Each generated footprint pad that participates in the design must have an
assignment record:

- component reference;
- footprint identity;
- pad number/name;
- component pin or role when known;
- assigned net name;
- assigned net code;
- evidence source;
- confidence;
- issue path when unresolved or ambiguous.

Pad assignment may use:

- verified component pinmaps;
- block component pin definitions;
- selected component identity and pinmap evidence;
- hydrated footprint pad names/numbers;
- schematic-to-PCB transfer bindings;
- explicit block port bindings;
- required local route endpoints.

Pad assignment must not silently guess when a part has unknown or conflicting
pin semantics. Unknown mappings may remain no-net only when the component is
intentionally unused and the diagnostic explains why.

### Copper Assignment

Each generated copper object that is part of an intended connection must have
an assignment record:

- object kind: track, via, zone, or local-route segment;
- object index or operation ID;
- assigned net name;
- assigned net code;
- source route or endpoint evidence;
- endpoint pad evidence when available;
- issue path when unresolved or mismatched.

Tracks, vias, and zones must reference a net code that exists in the board net
table. Where endpoints are known, copper net identity must agree with the pad
nets at both ends or produce a validation issue.

## Workflow Requirements

### Design Create

`kicadai design create` must run net assignment before final PCB write
validation. The generated workflow summary should include:

- number of nets;
- number of assigned pads;
- number of unresolved pads;
- number of assigned copper objects;
- number of unresolved copper objects;
- number of mismatched pad/copper assignments;
- optional path to persisted evidence when workflow artifacts are enabled.

### Writer Correctness

Writer correctness remains the authoritative hard gate. It must fail generated
boards when:

- a pad has a non-empty net name with no matching net code;
- a pad has a net code that does not match the board net table;
- a copper object has no net code but belongs to a generated route;
- a copper object references an unknown net code;
- a route endpoint conflicts with the endpoint pad net;
- a zone references an unknown or mismatched net.

### Optional KiCad-Backed Fixtures

The optional fixtures under `examples/design/kicad-backed/` should remain
skipped unless `KICADAI_KICAD_CLI` is configured. This work should promote
fixtures gradually:

- `expected_fail`: fixture documents a known blocker;
- `candidate`: generated project passes internal writer correctness and board
  validation, but KiCad evidence is still being evaluated;
- `pass`: fixture passes internal validation and configured KiCad checks.

The LED fixture should be the first promotion target. Connector/LED should be
next. I2C sensor breakout may remain `expected_fail` if it exposes a deeper
component, placement, or routing blocker.

## Diagnostics

Diagnostics must be stable and actionable. They should identify:

- component reference;
- pad number/name;
- expected pin role when known;
- selected component ID or variant ID when available;
- net name/code;
- evidence source;
- missing resolver, missing pinmap, ambiguous pin, missing route endpoint, or
  net table mismatch category.

Example issue paths:

- `pcb.footprints[U1].pads[3].net_code`
- `pcb.footprints[D1].pads[A].pinmap`
- `pcb.tracks[4].net_code`
- `design.net_assignment.refs[R1].pads[2]`

## Acceptance Criteria

- Default tests run without KiCad installed.
- Optional KiCad-backed tests still skip when `KICADAI_KICAD_CLI` is unset.
- Generated PCB net tables are deterministic across repeated runs.
- Generated pad net names and net codes match the board net table.
- Generated copper net codes match the board net table.
- Writer correctness fails unknown, ambiguous, or conflicting assignments with
  actionable diagnostics.
- The simple KiCad-backed LED design progresses past the current missing
  pad/copper net assignment blocker.
- Connector/LED and I2C fixtures either progress or retain narrower documented
  blockers.
- Existing imported-project round-trip and preservation behavior remains
  unchanged.
- `go test ./...` passes.

## Open Questions

- Should generated evidence be persisted by default, or only when workflow
  artifact output is requested?
- Should net codes preserve schematic creation order when available, or always
  use sorted names for determinism?
- How much no-net tolerance should be allowed for intentionally unused pins on
  active components?
- Should zone assignment remain strict from the beginning, or initially warn
  for generated designs that do not yet emit filled copper zones?
