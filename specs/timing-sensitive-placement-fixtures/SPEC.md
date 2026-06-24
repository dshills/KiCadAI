# Timing-Sensitive Placement Fixtures Specification

## Purpose

KiCadAI can now place and route basic PCB content with increasingly rich semantic placement rules. The next gap is proving that timing-sensitive circuit blocks, starting with crystals and oscillators, produce layout fragments that are not merely valid KiCad files but follow practical PCB layout conventions.

This specification defines verified timing-sensitive placement fixtures for schematic-to-PCB generation, block realization, placement scoring, routing evidence, and regression tests.

## Goals

- Add a verified crystal/oscillator placement fixture that can be generated from design intent.
- Keep timing components physically close to the IC pins they serve.
- Preserve expected symmetry and compactness for crystal load capacitors.
- Route short local clock connections with clear evidence about length, pin targets, and net assignments.
- Add placement diagnostics that explain timing-sensitive rule decisions.
- Generate deterministic golden examples suitable for regression and round-trip checks.
- Expose enough evidence for AI workflows to tell whether a generated timing block is plausible.

## Non-Goals

- Full signal-integrity simulation.
- Oscillator start-up margin calculation.
- Parasitic extraction from physical geometry.
- Manufacturer-specific crystal selection.
- Replacing KiCad ERC/DRC with a custom electrical rule engine.
- Guaranteeing RF-grade or high-frequency layout correctness.

## Current Gaps

The writer and workflow stack can create KiCad projects, schematics, PCBs, footprints, placements, routes, zones, validation summaries, and higher-level design workflow outputs. However, the following timing-sensitive gaps remain:

- Circuit blocks do not yet model crystal/oscillator placement intent as first-class layout constraints.
- Placement scoring does not have dedicated clock-source proximity and symmetry evidence.
- Routing evidence does not clearly distinguish local timing nets from ordinary signal nets.
- There are no deterministic golden fixtures proving generated clock blocks stay close to MCU clock pins.
- AI-facing workflow output cannot yet explain why a clock block is acceptable or risky.
- KiCad DRC can confirm parseability and some electrical/geometric constraints, but it cannot validate oscillator layout quality by itself.

## Fixture Scope

### Crystal Fixture

The first fixture must model a common MCU crystal circuit:

- MCU or MCU-like fixture footprint with two clock pins.
- Two-pin crystal footprint.
- Two load capacitors to ground.
- Optional damping resistor placeholder when requested by block metadata.
- Ground reference and local ground routes or vias where supported.
- Clock nets from MCU pins to crystal pins.
- Load capacitor nets attached close to the crystal pins.

### Oscillator Fixture

The second fixture should model a canned oscillator:

- Oscillator footprint with power, ground, enable, and clock output pins.
- Clock output net routed to a consuming IC clock input.
- Decoupling capacitor placed close to oscillator power pin.
- Optional enable pull-up or pull-down.

### Future Timing Fixtures

The model should leave extension points for:

- USB differential-pair clock-adjacent placement.
- Ethernet PHY crystal blocks.
- RF oscillator blocks.
- High-speed ADC clock input blocks.
- Reset and programming fixtures when they have timing or adjacency constraints.

## Placement Requirements

### Clock Source Proximity

The placement engine must support a timing-sensitive role that prefers the clock source near the consuming IC clock pins.

Required evidence:

- Consuming component reference.
- Consuming pin names or numbers.
- Clock source reference.
- Distance from source pads to consuming pads.
- Whether distance is within the configured maximum.

### Load Capacitor Symmetry

Crystal load capacitors must be placed compactly and symmetrically around the crystal where practical.

Required evidence:

- Capacitor references.
- Capacitor-to-crystal distance.
- Capacitor-to-clock-pin distance.
- Difference between the two capacitor distances.
- Whether symmetry is within tolerance.

### Local Ground Return

Load capacitors and oscillator decoupling capacitors must have an explicit ground connection.

Required evidence:

- Ground net name.
- Ground-connected pads.
- Whether the ground path is routed, zone-connected, or intentionally unresolved.
- Any missing local ground issue.

### Noise Avoidance

Timing-sensitive fixtures must avoid obvious noisy placement regions when metadata identifies high-current, switching, thermal, or connector edge regions.

Required evidence:

- Nearby noisy references or regions.
- Minimum observed distance.
- Required keepout distance.
- Pass/fail status.

## Routing Requirements

- Clock routes must be local and short.
- Crystal routes should avoid unnecessary jogs.
- Load capacitor stubs should be short and assigned to the correct crystal nets.
- Oscillator output routes may be longer than crystal routes but must still be measured.
- Routing summaries must identify clock/timing nets separately from ordinary signal nets.

The router is not required to perform impedance control for this phase. If a timing net asks for impedance constraints, the workflow must surface that as a known unsupported constraint rather than silently claiming success.

## Data Model Requirements

Timing-sensitive fixtures need metadata at both block and board levels:

- `TimingRole`: `crystal`, `oscillator`, `clock_consumer`, `load_capacitor`, `decoupling`, or `ground_return`.
- `TimingGroupID`: stable identifier tying clock source, capacitors, consumer pins, and routes together.
- `MaxSourceToConsumerDistanceMM`.
- `MaxLoadCapDistanceMM`.
- `MaxLoadCapAsymmetryMM`.
- `MinNoiseKeepoutMM`.
- `PreferredLayer`.
- `RequiredNets`.
- `EvidenceID` for validation output.

The model should reuse existing placement, routing, block, and validation structures where possible instead of creating a separate timing-only pipeline.

## Validation Requirements

Timing-sensitive validation must emit structured findings:

- `timing.fixture.present`
- `timing.clock_source.proximity`
- `timing.load_caps.proximity`
- `timing.load_caps.symmetry`
- `timing.ground_return.present`
- `timing.noise_keepout`
- `timing.clock_routes.length`
- `timing.unsupported_constraints`

Each finding must include severity, references, measured values, thresholds, and a short explanation suitable for CLI output and AI feedback.

## CLI and Workflow Requirements

The design workflow should include timing evidence in generated reports when timing fixtures are present.

Expected behavior:

- `kicadai design create` can select or realize a crystal/oscillator block when the request requires an MCU clock.
- Validation JSON includes a timing section.
- Human-readable CLI output summarizes timing-sensitive pass/fail status.
- Golden fixtures can be regenerated deterministically.

## Golden Fixtures

Add deterministic fixtures for:

- MCU with two-pin crystal and two load capacitors.
- MCU with canned oscillator and decoupling capacitor.
- A negative fixture with a crystal placed too far away.
- A negative fixture with asymmetric load capacitors.
- A negative fixture with missing local ground evidence.

Golden artifacts should include:

- `.kicad_pro`
- `.kicad_sch`
- `.kicad_pcb`
- validation JSON summary
- placement/routing evidence where available

## Acceptance Criteria

- Timing-sensitive block fixtures generate parseable KiCad projects.
- Crystal and oscillator examples include correct symbols, footprints, nets, and local placement intent.
- Validation fails known-bad timing fixtures with specific evidence.
- Validation passes known-good timing fixtures.
- Timing evidence appears in CLI/workflow output.
- Existing placement, routing, writer, round-trip, and block tests continue to pass.

## Risks

- Real-world oscillator correctness depends on datasheet and board-stackup details outside the current model.
- KiCad file validity does not imply electrical quality.
- Fixture footprints must be chosen carefully to avoid library-resolution drift.
- Overly strict thresholds may reject reasonable compact layouts on small boards.

## Open Questions

- Which MCU fixture should be the default timing consumer for goldens?
- Should thresholds be block-defined, board-profile-defined, or both?
- Should timing fixtures reserve keepout areas in the generated PCB, or only score placement?
- How much KiCad-source parity is needed for ordering of timing evidence artifacts?
