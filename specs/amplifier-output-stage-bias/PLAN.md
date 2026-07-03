# Amplifier Output Stage And Bias Block Plan

## Phase 1: Catalog Metadata Contract

Goal: make amplifier output-device requirements explicit and validated.

Tasks:

- Add or extend component metadata structures for amplifier output devices.
- Model device class, polarity, output role, ratings, package, footprint,
  symbol, pinmap evidence, thermal status, SOA status, and companion
  requirements.
- Update catalog validation to reject incomplete fabrication-capable output
  devices.
- Keep existing unverified power-output placeholders blocked by default.
- Add unit tests for valid low-current BJT seed records and blocked
  high-power/output placeholders.

Validation:

- `go test ./internal/components`
- `go test ./...`
- `prism review staged`

Commit:

- `Add amplifier output device metadata validation`

## Phase 2: Complementary Output Pair Selection

Goal: select safe, deterministic output-device pairs for supported headphone
Class AB scenarios.

Tasks:

- Add selection logic for complementary NPN/PNP output pairs.
- Enforce requested supply/load/current limits against modeled ratings.
- Return structured rejected-candidate diagnostics for polarity, pinmap,
  thermal, SOA, and rating failures.
- Block speaker/high-power output requests until stronger evidence exists.
- Add tests for successful headphone pair selection and blocked unsupported
  power-output requests.

Validation:

- `go test ./internal/components`
- `go test ./internal/designworkflow`
- `go test ./...`
- `prism review staged`

Commit:

- `Select amplifier output device pairs`

## Phase 3: Class AB Bias Block Contract

Goal: define the reusable bias-network block independently of PCB promotion.

Tasks:

- Add a Class AB bias-network block manifest or block definition.
- Define required ports for driver output, upper/lower output-device drive,
  rails, amplifier output, and reference/ground.
- Model initial `diode_string` topology and leave `vbe_multiplier` as an
  explicit future variant or blocked option.
- Add block inventory and block validation coverage.
- Add readiness metadata tying the bias block to output-device dependency
  evidence.

Validation:

- `go test ./internal/blocks`
- `go test ./internal/blocks/verification`
- `go test ./internal/aireadiness`
- `prism review staged`

Commit:

- `Add Class AB bias block contract`

## Phase 4: Schematic Realization

Goal: generate a human-readable schematic fragment for the supported Class AB
headphone output stage.

Tasks:

- Add design operations for the output pair and diode bias block.
- Apply schematic readability rules: left-to-right signal flow, upper/lower
  rail placement, output pair symmetry, and non-overlapping labels/wires.
- Label driver, bias, output, rail, and load nodes consistently.
- Preserve fail-closed diagnostics for missing SOA, thermal, or PCB evidence.
- Add schematic operation and generated-design fixture tests.

Validation:

- `go test ./internal/schematic`
- `go test ./internal/schematiclayout`
- `go test ./internal/designworkflow`
- `go test ./...`
- `prism review staged`

Commit:

- `Realize Class AB amplifier schematic output stage`

## Phase 5: Workflow Diagnostics And Rationale

Goal: make AI-facing output explainable and conservative.

Tasks:

- Surface selected output-device identities, ratings, pinmap evidence, and
  rejected alternatives in workflow output.
- Add rationale sections for amplifier output-stage assumptions and blockers.
- Ensure unsupported load/power requests report structured blocked issues.
- Update example intent/design fixtures only where they remain honest about
  current readiness.
- Add regression tests for rationale and blocked diagnostics.

Validation:

- `go test ./internal/rationale`
- `go test ./internal/intentplanner`
- `go test ./internal/designworkflow`
- `go test ./...`
- `prism review staged`

Commit:

- `Explain amplifier output stage readiness`

## Phase 6: Readiness And Documentation Update

Goal: advance readiness records only to the level supported by evidence.

Tasks:

- Update `data/ai-readiness/matrix/amplifier.json` for output-device and bias
  network readiness.
- Update docs for supported Class AB headphone scope and unsupported
  power-amplifier cases.
- Document remaining blockers for KiCad-backed amplifier promotion: output
  protection, stability/feedback layout, thermal/current layout, and real
  ERC/DRC evidence.
- Update roadmap status with completed catalog/block work and remaining
  dependency graph.

Validation:

- `go test ./internal/aireadiness`
- `go test ./...`
- `git diff --check`
- `prism review staged`

Commit:

- `Document amplifier output stage readiness`

## Completion Criteria

- A supported headphone Class AB output-stage path can be selected and realized
  at schematic/connectivity level.
- Unsupported speaker or high-power amplifier requests still fail closed.
- The AI readiness graph accurately reflects what is implemented and what still
  blocks KiCad-backed amplifier fixture promotion.
