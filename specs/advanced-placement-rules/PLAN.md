# Advanced Placement Rules Implementation Plan

Date: 2026-06-24

This plan implements `specs/advanced-placement-rules/SPEC.md`.

Each phase should be implemented, reviewed with Prism, tested, and committed
before moving to the next phase.

## Phase 1: Rule Model And Normalization

### Goal

Add typed advanced placement rule models without changing placement decisions.

### Work

- Add rule-family types for:
  - thermal;
  - high-current;
  - creepage/clearance;
  - differential-pair;
  - controlled-impedance.
- Add a normalized advanced-rule container to placement requests/rules.
- Add rule IDs, severity, hard/soft enforcement, affected refs/nets/roles, and
  metadata completeness fields.
- Add deterministic sorting and normalization helpers.
- Add validation for missing required metadata.
- Keep empty-rule behavior identical to current placement.

### Tests

- Rule normalization is deterministic.
- Empty advanced rules preserve existing placement output.
- Missing required metadata produces stable validation issues.
- JSON round-trip keeps rule IDs, family, severity, and enforcement.

### Acceptance

- Advanced rules can be carried through placement requests and results as
  additive data.

### Commit

```text
Add advanced placement rule model
```

## Phase 2: Thermal And High-Current Scoring

### Goal

Use thermal and high-current intent during candidate scoring.

### Work

- Add `thermal` candidate score dimension.
- Add `high_current` candidate score dimension.
- Reject checkable hard thermal spacing violations.
- Prefer heat-source placement near declared edge/thermal regions where
  configured.
- Score high-current source/load paths by distance, sequence, and congestion
  pressure.
- Emit bounded evidence citing refs, nets, distances, limits, and rule IDs.

### Tests

- Heat source near protected sensor is rejected or penalized according to rule
  mode.
- Regulator prefers declared thermal region or board edge.
- High-current load/source candidates closer together score higher.
- Missing high-current source/sink metadata reports incomplete evidence.

### Acceptance

- Thermal and high-current rules can change candidate selection in controlled
  fixtures.

### Commit

```text
Score placement candidates for thermal and high-current rules
```

## Phase 3: Creepage And Clearance Scoring

### Goal

Prevent obvious domain-spacing violations and expose unsupported creepage proof.

### Work

- Add `creepage_clearance` candidate score dimension.
- Implement bounding-box clearance checks between rule domains.
- Reject hard placement candidates that violate checkable clearance limits.
- Penalize soft clearance proximity.
- Emit explicit unsupported-proof evidence for creepage geometry that placement
  cannot prove.
- Add repair hints for increasing domain spacing.

### Tests

- Hard clearance violation rejects a candidate.
- Soft clearance rule affects candidate ranking.
- Unsupported creepage proof appears as structured evidence.
- Domain matching works for refs, nets, net classes, and roles.

### Acceptance

- Clearance rules prevent unsafe placements in deterministic generated
  fixtures without claiming full creepage certification.

### Commit

```text
Enforce placement clearance rules
```

## Phase 4: Differential-Pair And Controlled-Impedance Scoring

### Goal

Improve high-speed placement readiness before routing.

### Work

- Add `differential_pair` candidate score dimension.
- Add `controlled_impedance` candidate score dimension.
- Prefer symmetric differential-pair endpoints and compatible orientation.
- Penalize pair endpoint skew proxies.
- Prefer controlled-impedance routing corridors with fewer obstacles and lower
  congestion.
- Emit missing stackup/reference-plane evidence as warnings, not success.
- Preserve deterministic tie-breaking.

### Tests

- Differential-pair sink placement with lower skew proxy scores higher.
- Orientation mismatch is reported in score evidence.
- Controlled-impedance path through dense corridor scores lower.
- Missing reference-plane metadata emits a structured warning.

### Acceptance

- High-speed placement readiness appears in candidate scores and diagnostics
  without pretending to validate routed impedance.

### Commit

```text
Score high-speed placement rules
```

## Phase 5: Diagnostics And Repair Hints

### Goal

Turn advanced rule evidence into actionable placement diagnostics.

### Work

- Add diagnostic categories for advanced placement rules.
- Report hard violations, weak soft scores, incomplete metadata, and
  unsupported proofs.
- Map diagnostics to repair hints:
  - move heat source;
  - shorten high-current path;
  - increase domain spacing;
  - improve differential-pair symmetry;
  - reserve high-speed corridor;
  - add missing metadata.
- Ensure diagnostics include rule family, rule ID, affected refs/nets, and
  severity.

### Tests

- Each rule family emits at least one diagnostic fixture.
- Diagnostics are stable and sorted.
- Repair hints are present for repairable placement problems.
- Unsupported proof diagnostics are not marked as repaired by placement alone.

### Acceptance

- Advanced placement failures are machine-readable and actionable for future AI
  repair loops.

### Commit

```text
Add diagnostics for advanced placement rules
```

## Phase 6: Design Workflow And CLI Evidence

### Goal

Expose advanced placement rule evidence to users and AI callers.

### Work

- Add advanced rule summaries to design workflow placement stage artifacts.
- Include counts by rule family, severity, hard violations, soft warnings,
  incomplete metadata, and unsupported proof.
- Carry advanced placement evidence into repair bundles where relevant.
- Expose structured JSON in CLI commands that already emit placement/workflow
  summaries.
- Keep human-facing CLI output concise and backward-compatible.

### Tests

- Design workflow JSON contains advanced rule counts.
- Repair bundle artifacts preserve advanced placement diagnostics.
- CLI golden fixtures cover selected advanced rule fields.
- Empty-rule workflows omit or show zero-value summaries deterministically.

### Acceptance

- AI callers can read advanced placement rule status without parsing prose.

### Commit

```text
Expose advanced placement rule evidence
```

## Phase 7: Golden Fixtures And Regression Corpus

### Goal

Lock down expected behavior for every advanced rule family.

### Work

- Add deterministic fixtures for:
  - thermal regulator near sensitive sensor;
  - high-current source/load path;
  - isolated domain clearance;
  - differential-pair connector-to-IC placement;
  - controlled-impedance high-speed corridor;
  - mixed-rule conflict and tie-breaking.
- Add golden outputs for score summaries and diagnostics.
- Verify existing placement, routing, and workflow tests remain stable.
- Document remaining proof gaps that require KiCad DRC, route validation, or
  fabrication review.

### Tests

- Run focused placement tests.
- Run workflow golden tests.
- Run `go test ./...`.

### Acceptance

- The regression corpus proves advanced placement rules are deterministic,
  inspectable, and compatible with existing placement behavior.

### Commit

```text
Add advanced placement rule goldens
```
