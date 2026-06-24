# Timing-Sensitive Fixture Expansion Specification

## Purpose

KiCadAI now has first-pass timing-sensitive fixture support for crystal
oscillator loops: timing fixture metadata, realized evidence, timing-sensitive
placement scoring, route-length checks, load-cap proximity/symmetry evidence,
ground-return evidence, and workflow issue reporting.

The next gap is to broaden that support beyond the current two-pin crystal
path. This specification defines the next timing-sensitive fixture expansion:
canned oscillators first, then reset/programming and other timing-critical block
patterns where layout adjacency and route evidence materially affect board
quality.

## Goals

- Add a canned oscillator timing fixture with source, consumer, power,
  decoupling, enable, clock-output, and ground-return evidence.
- Extend timing evidence to support decoupling capacitor proximity and clock
  output route checks.
- Reuse the existing timing fixture metadata and workflow reporting path.
- Add fixture-specific validation findings and suggestions for oscillator,
  reset/programming, and future timing-critical paths.
- Keep placement scoring deterministic and bounded.
- Add tests that prove good fixtures pass and intentionally bad fixtures emit
  actionable timing evidence.
- Update README and ROADMAP after implementation.

## Non-Goals

- Full signal-integrity analysis.
- Oscillator startup margin analysis.
- Clock jitter, phase noise, or skew simulation.
- Datasheet-specific timing constraint extraction.
- DDR, RF, or high-speed serial bus routing correctness.
- Replacing KiCad DRC or ERC.

## Current State

Implemented timing-sensitive support includes:

- `blocks.PCBTimingFixture` metadata.
- `blocks.TimingFixtureEvidence` realization output.
- timing finding constants for source presence, consumer presence, load-cap
  presence/proximity/symmetry, clock route presence/length, and ground return.
- crystal oscillator block metadata and realized evidence.
- timing-sensitive placement candidate scoring driven by timing-relevant
  proximity rules.
- design workflow `timing_results` summaries and stage issues.

Current limits:

- Only the two-pin crystal loop is modeled as a timing fixture.
- Canned oscillator components are not represented as a block fixture.
- Decoupling proximity is not first-class timing evidence.
- Enable/pull state for oscillator enable pins is not modeled.
- Reset/programming fixtures exist structurally, but timing/adjoining path
  evidence is not yet surfaced through timing fixture results.
- There are no golden tests for oscillator-style timing fixtures.

## Fixture Scope

### Canned Oscillator Fixture

The canned oscillator fixture represents a packaged oscillator feeding a clock
consumer, usually an MCU, PHY, ADC, or connector-exported clock input.

Required roles:

- `oscillator`: packaged oscillator source.
- `clock_consumer`: receiving IC or fixture endpoint.
- `decoupling`: local supply capacitor.
- optional `enable_pull`: pull-up or pull-down component for oscillator enable.

Required nets:

- clock output net from oscillator output to consumer clock input.
- supply net.
- ground net.
- optional enable net.

Required evidence:

- oscillator source reference.
- consumer reference when available.
- source-to-consumer distance when both are present.
- clock output route length.
- local decoupling capacitor distance to oscillator.
- ground-return presence.
- enable net handling status when enable metadata exists.
- unsupported behavior notes for jitter, startup, and SI.

### Reset/Programming Timing Fixture

Reset and programming fixtures are not oscillators, but they are timing-critical
or sequencing-sensitive in many MCU boards.

Required roles:

- `reset_source`: reset switch, supervisor, or pull resistor.
- `programming_header`: SWD/JTAG/ISP header.
- `clock_consumer`: MCU or target IC.
- optional `pull`: reset or boot mode pull resistor.

Required evidence:

- reset/programming path source and target refs.
- local pull resistor proximity.
- route length for reset, clock, SWD/JTAG clock, or ISP clock nets when present.
- ground reference for programming header.
- unsupported behavior notes for protocol timing and signal integrity.

### Future Timing-Critical Fixtures

The implementation should leave extension points for:

- Ethernet PHY crystal fixtures.
- ADC sampling clock fixtures.
- USB or high-speed serial reference-clock support.
- RF oscillator fixtures.
- multi-output clock distribution fixtures.

These are explicitly out of scope for the first implementation pass unless a
small test fixture is needed to prove extensibility.

## Data Model Requirements

The existing timing metadata should be reused. New model work should be limited
to fields that cannot be expressed with the current model.

Expected additions:

- optional decoupling roles on `PCBTimingFixture`, or an equivalent generic role
  list using existing `Roles` semantics.
- optional enable/control roles for oscillator fixtures.
- finding IDs for:
  - decoupling presence;
  - decoupling proximity;
  - enable/control handling;
  - reset/programming route length;
  - programming ground reference.

The model must preserve backwards compatibility for existing crystal timing
fixtures.

## Validation Requirements

Timing evidence generation must produce structured findings for:

- missing oscillator source;
- missing clock consumer when a consumer role is declared;
- missing decoupling capacitor;
- decoupling capacitor too far from oscillator;
- missing or long clock-output route;
- missing local ground return;
- missing enable-control evidence when enable is declared required;
- reset/programming path too long when thresholds are declared.

Findings must include:

- stable finding ID;
- severity;
- message;
- refs;
- nets;
- measured value where available;
- threshold where available.

## Placement Requirements

Placement must continue using existing placement structures:

- block-derived placement groups;
- proximity rules;
- timing-sensitive candidate scoring;
- local-route mobility;
- deterministic scoring summaries.

The canned oscillator fixture should generate proximity rules that favor:

- oscillator near consumer clock input;
- decoupling capacitor near oscillator supply pin;
- enable/pull component near oscillator when present.

## Routing Requirements

The fixture expansion must produce or validate:

- local clock-output route evidence;
- local decoupling route or ground-return route evidence;
- reset/programming route evidence where applicable.

The router is not required to produce impedance-controlled or length-matched
clock routes in this phase.

## Testing Requirements

Add focused tests for:

- oscillator fixture metadata validation.
- oscillator block instantiation and PCB realization.
- decoupling distance evidence.
- clock-output route length evidence.
- missing decoupling negative case.
- route-length negative case.
- workflow issue conversion for new timing finding IDs.
- placement timing-sensitive score dimensions for oscillator proximity rules.

When practical, add deterministic golden or snapshot coverage for:

- a good canned oscillator fixture;
- a bad oscillator fixture with missing decoupling or excessive route length.

## Acceptance Criteria

- A canned oscillator timing fixture can be generated by the block system.
- Realized oscillator fixture output includes timing evidence.
- Known-good oscillator fixture evidence is satisfied.
- Known-bad oscillator fixture evidence produces structured findings.
- Design workflow surfaces oscillator timing issues as stage issues.
- Existing crystal oscillator tests continue to pass.
- Full `go test ./...` passes.
- Prism review is run before each implementation commit.

## Risks

- Oscillator footprints vary widely by package and pinout.
- Library mapping may not have a verified oscillator part yet.
- Enable pin behavior may be optional, active-high, active-low, tied, or left
  floating depending on the device.
- Strict decoupling thresholds may reject acceptable compact boards.
- KiCad DRC cannot prove oscillator timing quality.

## Open Questions

- Which oscillator package should be the first verified canned oscillator seed?
- Should oscillator fixtures be a new block or a parameterized mode of the
  existing `crystal_oscillator` family?
- Should reset/programming timing evidence be implemented in this project or as
  the following roadmap item?
- Should timing fixture evidence become part of fabrication readiness gates once
  more fixture families exist?
