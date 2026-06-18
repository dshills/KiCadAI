# Circuit Block Library Expansion Implementation Plan

## Phase 1: Inventory And Gap Classification

### Objectives

- Compare current built-in blocks against the Priority 2 requirements.
- Classify each block family as ready, partial, blocked, or unsupported.
- Produce a machine-readable inventory that future phases can test against.

### Work

- Add or extend block catalog introspection so each block reports:
  - supported variants;
  - required parameters;
  - required component roles;
  - exported ports;
  - electrical rules;
  - PCB rules;
  - verification level.
- Add tests that assert the inventory includes every roadmap seed block family.
- Add explicit gap records for crystal/oscillator, reset/programming, ESD, and
  reverse-polarity protection if they do not yet have concrete generators.
- Ensure CLI or report output can show block readiness without writing a
  project.

### Acceptance

- A single test or command can list block families and readiness.
- Missing roadmap block families appear as actionable gaps, not silent absence.
- Existing block behavior remains unchanged.

## Phase 2: Shared Rule And Evidence Model

### Objectives

- Normalize block electrical rules, PCB rules, and evidence requirements.
- Make fail-closed behavior consistent across block families.

### Work

- Add shared Go types for:
  - electrical requirements;
  - PCB requirements;
  - evidence requirements;
  - rule outcomes;
  - blocker/warning severity;
  - readiness levels.
- Connect rule outcomes to existing report/result issue structures.
- Add helper validators for common requirements:
  - component confidence;
  - resolver evidence;
  - pinmap evidence;
  - required companion components;
  - rail/rating compatibility;
  - required ports and nets;
  - placement proximity;
  - local route presence.
- Add tests for rule evaluation and deterministic issue ordering.

### Acceptance

- Blocks can return structured blockers and warnings without ad hoc strings.
- Under-evidenced active components block connectivity/fabrication readiness.
- Rule output is stable for golden tests.

## Phase 3: LED, Connector, And Passive-Oriented Blocks

### Objectives

- Harden lower-risk blocks first.
- Prove the shared rule model against simple but useful circuits.

### Work

- Update LED indicator rules:
  - resistor calculation or verified selection;
  - current and power checks;
  - polarity evidence;
  - local route requirement.
- Update connector breakout rules:
  - pin count validation;
  - pin-to-net assignment checks;
  - pin numbering evidence;
  - optional edge orientation.
- Add negative tests for invalid current, unresolved LED polarity, connector pin
  mismatch, and duplicate net assignments.
- Add verification manifests for positive LED and connector cases.

### Acceptance

- LED and connector blocks emit schematic and PCB fragments with rule evidence.
- Invalid LED and connector requests fail closed.
- Verification reports include expected nets, ports, placements, and routes.

## Phase 4: Regulator And USB-C Power Blocks

### Objectives

- Harden power-entry and power-conversion blocks.
- Enforce companion components and rail compatibility.

### Work

- Update regulator rules:
  - VIN/VOUT/current/dropout checks;
  - required input and output capacitors;
  - capacitor voltage rating and polarity checks;
  - enable pin handling;
  - capacitor proximity constraints.
- Update USB-C power rules:
  - CC resistor requirements;
  - connector pinmap evidence;
  - VBUS/GND route priority;
  - edge placement and keepout constraints;
  - optional protection companion requirements.
- Add verification manifests for positive regulator and USB-C cases.
- Add negative tests for missing capacitors, bad rail requests, missing CC
  resistors, and unresolved connector evidence.

### Acceptance

- Regulator and USB-C blocks block unsafe or incomplete requests.
- Positive cases include selected-part evidence and PCB constraints.
- Generated fragments pass existing writer correctness and block verification
  checks.

## Phase 5: I2C Sensor And Op-Amp Blocks

### Objectives

- Harden mixed-signal and bus-oriented blocks.
- Validate shared-bus and analog-bias requirements.

### Work

- Update I2C sensor rules:
  - sensor rail compatibility;
  - decoupling requirements;
  - bus pull-up ownership;
  - optional interrupt and address strap nets;
  - shared-bus duplicate pull-up detection.
- Update op-amp gain-stage rules:
  - topology selection;
  - gain resistor calculation or selection;
  - supply compatibility;
  - single-supply bias/reference handling;
  - feedback placement and local route constraints.
- Add positive manifests and negative fail-closed fixtures.

### Acceptance

- I2C and op-amp blocks expose composition constraints clearly.
- Invalid rail, pull-up, gain, and bias requests produce structured blockers.
- Verification confirms expected nets, placements, routes, and selected parts.

## Phase 6: MCU Minimal System And Companion Blocks

### Objectives

- Harden the most complex seed block.
- Add or formally block supporting crystal, reset, programming, and protection
  block families.

### Work

- Update MCU minimal rules:
  - concrete MCU requirement;
  - package-specific pinmap requirement;
  - required power pins and decoupling;
  - reset pull-up or reset circuit;
  - programming/debug interface;
  - optional crystal/oscillator pins;
  - peripheral/exported-port mapping.
- Implement minimal crystal/oscillator block if component evidence is already
  available; otherwise create explicit unsupported diagnostics and tests.
- Implement minimal reset/programming header block when supported by catalog
  metadata; otherwise create explicit unsupported diagnostics and tests.
- Implement minimal ESD/reverse-polarity protection blocks when supported by
  catalog metadata; otherwise create explicit unsupported diagnostics and tests.
- Add positive MCU manifest and negative tests for missing pinmap, missing
  decoupling, unsupported peripheral function, and missing programming path.

### Acceptance

- MCU minimal generation cannot claim readiness with placeholder MCU data.
- Companion block gaps are visible and test-covered.
- Positive MCU cases include evidence for power, reset, programming, and
  decoupling expectations.

## Phase 7: Block Verification Corpus And Golden Reports

### Objectives

- Make the expanded block library regression-testable.
- Produce durable evidence for autonomous workflows.

### Work

- Add verification manifests for all supported positive cases.
- Add negative manifests for safety-critical blockers.
- Add golden report snapshots for representative block requests.
- Ensure reports include:
  - selected components;
  - evidence;
  - ports;
  - nets;
  - placements;
  - routes;
  - constraints;
  - validation outcomes;
  - skipped KiCad checks with reason.
- Add deterministic ordering for report fields and issues.

### Acceptance

- Golden tests fail on accidental block output drift.
- Negative fixtures prove unsafe requests fail closed.
- Verification reports are useful as AI-facing evidence.

## Phase 8: Design Workflow Integration

### Objectives

- Ensure `design create` and related workflows consume expanded block rules.
- Make block readiness affect autonomous generation.

### Work

- Feed block rule outcomes into design workflow planning and result evidence.
- Block connectivity/fabrication acceptance levels when selected blocks are
  under-evidenced.
- Include block readiness, selected parts, companions, exported ports, and
  constraints in workflow output.
- Add tests for multi-block designs that combine:
  - USB-C power;
  - regulator;
  - MCU minimal;
  - I2C sensor;
  - LED indicator;
  - connector breakout.
- Ensure failed block rules stop project writing or mark output partial based on
  existing workflow policy.

### Acceptance

- AI workflow output explains why each block is ready, partial, or blocked.
- Multi-block design tests prove block composition still works.
- Unsafe block gaps prevent autonomous completion.

## Phase 9: Documentation And Examples

### Objectives

- Document the expanded block library for developers and AI workflows.
- Provide examples that demonstrate correct and blocked requests.

### Work

- Update README or block docs with:
  - supported block families;
  - verification levels;
  - request examples;
  - readiness policy;
  - common blockers.
- Add example requests for representative blocks.
- Add one combined generated design example that exercises several verified
  blocks.
- Update roadmap status after implementation.

### Acceptance

- Developers can discover supported blocks and understand readiness.
- Examples can be regenerated deterministically.
- Roadmap reflects completed and remaining Priority 2 work.

## Review And Commit Process

After each phase:

- run focused tests for touched packages;
- run `go test ./...` when changes affect shared behavior;
- run `prism review staged` with the relevant staged changes;
- address review findings or document why no code change is needed;
- commit the completed phase before starting the next phase.

## Completion Criteria

Priority 2 is complete when:

- roadmap seed block families are implemented or explicitly blocked with tested
  diagnostics;
- supported blocks have electrical rules, PCB rules, and evidence rules;
- every supported block has positive verification coverage;
- safety-critical block failures have negative tests;
- design workflow output includes block readiness evidence;
- generated schematic and PCB fragments pass current writer correctness checks;
- documentation and roadmap status are current.
