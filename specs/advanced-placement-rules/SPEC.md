# Advanced Placement Rules Specification

Date: 2026-06-24

## Purpose

Extend placement hardening with explicit PCB-domain rules for thermal,
high-current, creepage/clearance, differential-pair, and controlled-impedance
intent.

The placement engine now has deterministic placement, block-derived placement
metadata, semantic candidate scoring, fanout and congestion scoring, routing
readiness reports, and repairable placement diagnostics. The next gap is that
placement still treats several fabrication and signal-integrity concerns as
generic proximity or congestion effects.

This project adds first-class rule models and evidence so generated boards can
make placement decisions using electrical and manufacturing intent before
routing and DRC. The goal is not to replace KiCad DRC or a field solver. The
goal is to make generated placement avoid obvious bad layouts and explain when
the design needs stricter downstream validation.

## Roadmap Link

This implements the next open Priority 4 placement hardening item from
`specs/ROADMAP.md`:

- add thermal, high-current, creepage/clearance, differential-pair, and
  controlled-impedance placement rules.

It builds directly on `specs/semantic-placement-scoring/SPEC.md`.

## Goals

1. Add stable placement rule models for advanced PCB concerns.
2. Let circuit blocks, design requests, and future AI workflows declare these
   rules without editing placement internals.
3. Enforce hard placement exclusions for rules that can be checked at placement
   time.
4. Add soft candidate scoring dimensions for placement preferences that improve
   routability, thermal behavior, and signal integrity.
5. Produce structured diagnostics when advanced rules are missing, violated, or
   only partially checked.
6. Surface rule evidence in placement results, design workflow artifacts, and
   CLI JSON.
7. Keep behavior deterministic and compatible with existing generated projects.
8. Add golden fixtures proving each rule family affects placement and evidence.

## Non-Goals

- Full thermal simulation.
- Controlled-impedance stackup calculation.
- Differential-pair routing implementation.
- High-current copper-area or temperature-rise proof.
- Creepage proof across arbitrary slots, cutouts, coatings, or pollution
  degree models.
- Requiring KiCad DRC in default unit tests.
- Mutating imported or user-authored projects.
- Replacing board-level DRC, ERC, route validation, or fabrication review.

## Existing Foundation

The implementation should reuse:

- `internal/placement` component, board, rule, keepout, group, candidate score,
  quality report, diagnostics, and retry hint models;
- `internal/pcbrules` net class, role, trace, clearance, length, zone, and
  coupled-net intent;
- circuit block metadata and local-route definitions;
- design workflow placement summaries and repair bundles;
- board validation and routing diagnostics for downstream evidence;
- existing golden placement and route fixtures.

No second placement engine should be introduced.

## Rule Families

### Thermal Placement Rules

Thermal rules describe components and regions that need heat-management-aware
placement.

Required fields:

- rule ID;
- affected refs or component roles;
- thermal role, such as `heat_source`, `thermal_sensitive`, `heat_sink`,
  `regulator`, `power_switch`, or `connector`;
- preferred region or edge, if any;
- minimum distance from thermally sensitive refs or roles;
- optional copper/zone proximity preference;
- optional airflow or board-edge preference;
- severity and hard/soft enforcement mode.

Placement behavior:

- hard rules reject candidates too close to protected refs or regions;
- soft rules penalize candidates near thermal-sensitive components;
- heat sources may prefer board edge, copper pour region, or declared thermal
  region;
- evidence must cite the heat source, protected subject, distance, limit, and
  enforcement mode.

### High-Current Placement Rules

High-current rules describe current paths that should be short, wide, and
direct.

Required fields:

- rule ID;
- affected nets or net roles;
- current class or current estimate;
- source refs/pads and sink refs/pads where known;
- maximum preferred path length;
- preferred layer or side constraints;
- required keepaway from sensitive analog/high-speed sections where declared;
- hard/soft enforcement mode.

Placement behavior:

- score candidates to shorten source-to-load current paths;
- prefer grouping high-current path components in sequence;
- penalize paths that cross sensitive regions or squeeze through congested
  areas;
- flag missing source/sink metadata as an incomplete rule, not silent success.

### Creepage And Clearance Placement Rules

Creepage/clearance rules describe required spacing between electrical domains.

Required fields:

- rule ID;
- domain A and domain B, expressed as refs, nets, net classes, or roles;
- minimum clearance distance;
- optional creepage distance;
- optional voltage or insulation class metadata;
- board-side applicability;
- hard/soft enforcement mode.

Placement behavior:

- hard clearance rules reject candidates that violate bounding-box spacing;
- soft rules report proximity penalties;
- rules must distinguish checked clearance from unsupported creepage geometry;
- unsupported creepage proof must produce explicit evidence requiring external
  DRC/fabrication validation.

### Differential-Pair Placement Rules

Differential-pair rules describe paired nets that should leave and arrive
symmetrically.

Required fields:

- rule ID;
- positive net and negative net;
- source ref/pads and sink ref/pads where known;
- preferred orientation relationship;
- maximum allowed source/sink skew at placement level;
- length matching target or tolerance where known;
- pair group ID.

Placement behavior:

- prefer source and sink candidates that keep pair endpoints adjacent and
  similarly oriented;
- penalize asymmetric placement around connectors, ICs, or pair endpoints;
- produce evidence for pair endpoint distance, skew proxy, and orientation;
- never claim routed length matching, only placement-level readiness.

### Controlled-Impedance Placement Rules

Controlled-impedance rules describe high-speed nets that need routing space and
stable layer intent.

Required fields:

- rule ID;
- affected nets, net classes, or roles;
- preferred layer or layer set;
- minimum corridor width or keepaway;
- source/sink refs and pads where known;
- maximum via count preference;
- optional reference-plane requirement;
- hard/soft enforcement mode.

Placement behavior:

- prefer direct routing corridors with fewer obstacles;
- penalize placements that force high-speed nets through dense or blocked
  regions;
- emit warnings when reference-plane or stackup evidence is unavailable;
- preserve deterministic behavior without running a router during placement.

## Rule Declaration Sources

Rules may come from:

- circuit block definitions;
- design request schema;
- schematic or PCB transaction metadata;
- inferred net roles from `internal/pcbrules`;
- future AI planner output.

The implementation must normalize all declarations into one placement rule
model before scoring. Missing optional metadata should degrade evidence quality
without panics. Missing required metadata should produce validation issues.

## Candidate Scoring

Add score dimensions for:

- `thermal`;
- `high_current`;
- `creepage_clearance`;
- `differential_pair`;
- `controlled_impedance`.

Each dimension must:

- be deterministic;
- include bounded structured evidence;
- use hard rejection only for checkable hard rules;
- keep soft penalties normalized with existing score dimensions;
- avoid hiding unknown or unsupported checks.

Candidate scoring must continue to respect:

- hard keepouts;
- board boundary;
- fixed placement and mobility;
- component collisions;
- existing edge, region, fanout, congestion, route-length, and semantic scores.

## Diagnostics

Advanced placement diagnostics must report:

- violated hard rules;
- weak soft-rule scores;
- missing required metadata;
- unsupported proof requirements;
- rule categories that need KiCad DRC, route validation, or fabrication review.

Diagnostics should map to repair hints where possible:

- move source/sink closer;
- increase spacing between domains;
- move heat source toward edge or thermal region;
- preserve differential-pair symmetry;
- reserve high-speed routing corridor;
- add missing rule metadata.

## Workflow And CLI Evidence

Design workflow outputs must include:

- advanced placement rules enabled/disabled;
- rule counts by family;
- hard violation count;
- soft warning count;
- unsupported proof count;
- worst-scoring refs/rules;
- per-family summary in placement stage JSON;
- carried evidence in repair bundles and validation summaries where relevant.

Human-facing CLI output may stay concise, but JSON output must expose enough
structured data for AI callers.

## Validation Strategy

Default tests must not require KiCad. Unit and golden tests should use small
deterministic generated boards with synthetic rules and block metadata.

Required fixtures:

- thermal regulator near sensitive sensor;
- high-current source/load path;
- clearance between power domains;
- differential-pair connector-to-IC layout;
- controlled-impedance high-speed corridor;
- mixed fixture with multiple rule families and deterministic tie-breaking.

Tests must prove:

- hard violations reject candidates;
- soft rules affect candidate selection;
- missing metadata emits structured issues;
- score evidence is stable;
- workflow summaries expose rule counts and violations;
- existing placement goldens remain compatible except for additive evidence.

## Acceptance Criteria

- Advanced placement rule models are stable and JSON-serializable.
- Rules can be attached through placement requests without touching scorer
  internals.
- Candidate scoring uses every rule family in controlled fixtures.
- Hard advanced rules can reject unsafe candidates.
- Soft advanced rules can change placement choices deterministically.
- Diagnostics distinguish violation, warning, incomplete metadata, and
  unsupported proof.
- Design workflow and CLI JSON expose advanced placement evidence.
- `go test ./...` passes.
- Prism review is run before each phase commit during implementation.
