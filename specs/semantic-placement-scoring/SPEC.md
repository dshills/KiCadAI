# Semantic Placement Scoring Specification

Date: 2026-06-24

## Purpose

Improve the placement engine so it uses design intent before selecting
placements, not only after placement through quality reports.

The current placement foundation can place components deterministically, carry
block groups and constraints, report quality, emit diagnostics, and feed
routing/repair loops. This project adds a candidate scoring layer that uses
component roles, electrical intent, congestion pressure, fanout pressure, and
existing block metadata while candidates are being selected.

The goal is to make first-attempt placement more routing-friendly and
electrically plausible, reducing the amount of downstream routing retry and
repair required.

## Background

The roadmap identifies this as the next open placement work:

- add richer candidate scoring from semantic component roles and the new
  congestion/fanout reports, not only post-placement quality reports;
- expand placement-routing retry with richer convergence criteria across
  larger boards;
- validate hardened placement outputs against KiCad DRC evidence in larger
  board-level golden projects.

This specification covers the first item: semantic candidate scoring. It must
remain compatible with later thermal/high-current/differential-pair and
KiCad-DRC-backed placement proof work.

## Goals

1. Add an explicit, inspectable candidate scoring model.
2. Score placement candidates using semantic component roles and net roles.
3. Prefer candidates that reduce predicted route length and congestion.
4. Prefer candidates that improve fanout and pad escape room.
5. Respect hard constraints before scoring soft preferences.
6. Preserve deterministic output for identical inputs.
7. Expose candidate-score evidence in placement results and design workflow
   summaries.
8. Keep the existing placement request/result and transaction pipeline
   backward-compatible.

## Non-Goals

- Replacing the deterministic placer with a global optimizer.
- Implementing simulated annealing, force-directed placement, or stochastic
  exploration.
- Solving full routing during placement.
- Adding KiCad DRC execution to the candidate scorer.
- Implementing thermal, creepage, controlled impedance, or differential-pair
  rules in this project.
- Mutating imported/user-authored projects.

## Existing Foundation

This project should build on:

- `internal/placement` request, component, board, group, keepout, mobility,
  quality, and diagnostics models;
- block-derived placement groups, proximity constraints, edge constraints, and
  local route metadata assembled in `internal/designworkflow`;
- placement quality reports for routing readiness, HPWL, congestion, fanout,
  group cohesion, keepouts, and mechanical checks;
- placement-routing retry summaries and placement retry hints;
- generated placement mobility policy;
- design workflow placement stage summaries.

No parallel placement engine should be introduced.

## Candidate Scoring Model

Add a candidate scoring layer that evaluates legal candidate placements before
the final candidate is chosen for a component.

### Candidate Score

Each scored candidate must include:

- component ref;
- component role;
- candidate index;
- candidate position, rotation, side/layer;
- total score;
- score dimensions;
- hard rejection reasons, if any;
- evidence strings or structured fields explaining important score drivers.

Recommended model:

```go
type CandidateScore struct {
    Ref        string                 `json:"ref"`
    Role       string                 `json:"role,omitempty"`
    Index      int                    `json:"index"`
    Placement  Placement              `json:"placement"`
    Total      float64                `json:"total"`
    Dimensions []CandidateScoreDimension `json:"dimensions,omitempty"`
    Rejected   bool                   `json:"rejected,omitempty"`
    Reasons    []CandidateScoreReason `json:"reasons,omitempty"`
}

type CandidateScoreDimension struct {
    Name     CandidateScoreDimensionName `json:"name"`
    Score    float64                     `json:"score"`
    Weight   float64                     `json:"weight"`
    Evidence []string                    `json:"evidence,omitempty"`
}
```

The exact shape may be adjusted to match existing placement report conventions,
but these fields must be represented somewhere machine-readable.

### Score Dimensions

Required score dimensions:

- `hard_constraints`: candidate legality; rejected candidates cannot win.
- `semantic_role`: role-specific placement preference.
- `group_cohesion`: proximity to group anchor and group members.
- `electrical_proximity`: closeness to electrically related refs or pads.
- `route_length`: predicted HPWL impact.
- `congestion`: predicted coarse-grid routing pressure.
- `fanout`: pad escape room and nearby obstacle pressure.
- `edge`: edge/connector satisfaction.
- `region`: analog/digital/power/user/clock region preference.
- `mobility`: respect generated mobility policies and avoid moving fixed refs.

Weights must be deterministic and configurable through placement rules where
reasonable. Default weights must be conservative and documented in tests.

## Semantic Inputs

The scorer may use existing data from:

- component role (`mcu`, `decoupling_capacitor`, `connector`, `regulator`,
  `sensor`, `feedback_resistor`, `crystal`, `pullup`, etc.);
- block placement groups and anchor roles;
- local routes and required local-route metadata;
- net role (`power`, `ground`, `signal`, `i2c`, `clock`, `reset`,
  `programming`, `analog`, `high_current`);
- pad summaries and footprint geometry;
- proximity rules and region rules already present in placement requests;
- generated placement mobility policy;
- board area, margins, keepouts, and edge constraints.

The scorer must tolerate missing semantic data. Missing data should reduce or
omit soft score dimensions, not block placement unless the relevant rule is
explicitly required.

## Candidate Generation And Rejection

Candidate scoring happens after candidate generation but before candidate
selection.

Hard rejection criteria:

- outside board;
- collision with fixed/hard placed components;
- hard keepout intersection;
- hard edge/side/rotation violation;
- mobility policy violation;
- impossible required group constraint.

Rejected candidates must be reported when they are diagnostically useful, but
reports should be bounded so JSON does not become enormous.

## Determinism Requirements

The scorer must be deterministic:

- stable component ordering;
- stable candidate ordering;
- stable dimension ordering;
- stable tie-breaking by total score, hard-rejection state, candidate index,
  component ref, and role;
- no map iteration order in output;
- no unseeded randomness.

Scores should be rounded or normalized enough that floating-point noise does not
create unstable goldens.

## Evidence And Reporting

Placement results must expose:

- winning candidate score for each placed component;
- bounded alternative candidate scores for debugging;
- aggregate dimension totals;
- top score penalties;
- candidate rejection counts by reason;
- whether semantic scoring was enabled;
- score version or policy name for future compatibility.

Design workflow placement summaries should include:

- candidate scoring enabled/disabled;
- average winning score;
- lowest winning score;
- dominant score penalty categories;
- rejected candidate count;
- refs with weak fanout, congestion, or semantic-role scores.

## Validation And Diagnostics

Existing placement diagnostics should consume candidate score evidence where it
improves actionability.

Examples:

- decoupling capacitor far from MCU supply pins: cite semantic/electrical score
  evidence;
- connector has edge candidates rejected: cite edge rejection count and best
  failed edge;
- part has poor fanout: cite fanout score and nearest blocking refs/keepouts;
- group is spread out: cite group cohesion score and anchor ref.

Candidate scoring must not hide existing blocking issues. It should provide
better explanation and better initial candidate selection.

## Integration Requirements

### Placement Package

`internal/placement` should own:

- score model;
- scorer configuration;
- candidate score calculation;
- deterministic score sorting;
- bounded score reporting;
- unit tests and golden tests.

### Design Workflow

`internal/designworkflow` should:

- pass existing block/role/net metadata into placement request scoring inputs;
- expose scoring summary in the placement stage;
- keep existing output shape backward-compatible by adding fields, not
  replacing existing summaries.

### Retry Loop

The bounded placement-routing retry loop should:

- preserve candidate scoring evidence across retry attempts;
- use score evidence to explain why a retry improved or failed;
- avoid treating scoring-only changes as progress unless placement or routing
  quality actually improves.

## Acceptance Criteria

- Existing placement behavior remains deterministic.
- Existing generated examples and placement goldens continue to pass or are
  intentionally updated with better evidence.
- Candidate scoring is visible in placement results and workflow summaries.
- Hard constraints always dominate soft score preferences.
- Semantic scoring improves at least two checked-in representative fixtures:
  one decoupling/proximity case and one connector/fanout or routing-readiness
  case.
- Full `go test ./...` passes.
- Prism review has no unresolved high or medium findings.

## Risks

- Score weights can become arbitrary and hard to tune.
- Too much candidate evidence can make JSON output noisy.
- Soft scoring can accidentally override hard safety constraints if not
  separated carefully.
- Existing goldens may become brittle if floating-point scores are not
  normalized.

## Open Questions

- Should candidate scoring be always enabled once implemented, or guarded by a
  placement rule flag for one release?
- How many alternative candidates should be retained in normal JSON output?
- Should semantic weights be user-configurable in structured design requests,
  or only internal placement rules initially?
- Which larger generated board fixture should become the first KiCad
  DRC-backed placement proof after this project?
