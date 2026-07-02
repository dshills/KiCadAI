# Regulator Stability Evidence Implementation Plan

Date: 2026-07-02

## Implementation Rules

- Implement phases in order.
- Commit each phase independently after validation and `prism review staged`.
- Keep default tests hermetic. Do not depend on network access, distributor
  APIs, or live KiCad.
- Use existing component catalog, selection, block, workflow, rationale, and
  report patterns where possible.
- Keep the first implementation conservative. Missing stability evidence should
  block fabrication-candidate readiness rather than infer safety.
- Preserve structural/connectivity behavior for existing generated regulator
  examples unless a real safety bug is exposed.

## Phase 1: Evidence Audit And Schema Shape

Objective: define the minimal structured evidence needed for regulator
stability checks.

Tasks:

1. Audit current regulator, capacitor, derating, and selection metadata.
2. Identify which facts already exist for AMS1117, AP2112K, Murata 100 nF, and
   Murata 10 uF records.
3. Decide the smallest schema extension that can represent:
   - regulator output-capacitor stability kind;
   - ESR window requirement or ceramic-stable status;
   - capacitor dielectric;
   - DC-bias/effective-capacitance review status;
   - fabrication-candidate blocker status.
4. Add an audit note under `specs/regulator-stability-evidence/AUDIT.md`.
5. Add schema/model tests that describe expected validation behavior before
   implementation changes.

Validation:

- `go test ./internal/components`
- `prism review staged`

Commit:

- `Document regulator stability evidence audit`

## Phase 2: Catalog Metadata And Validation

Objective: encode regulator/capacitor stability evidence in catalog records and
validate malformed records.

Tasks:

1. Add structured stability metadata to AMS1117 and AP2112K records.
2. Add structured dielectric/DC-bias/effective-capacitance metadata to existing
   concrete Murata capacitor records.
3. Keep generic capacitor records explicitly insufficient for
   fabrication-candidate regulator stability proof.
4. Add validation for:
   - invalid stability kind;
   - ESR min greater than ESR max;
   - missing required capacitance when stability evidence requires it;
   - concrete capacitor records with impossible voltage/capacitance metadata;
   - fabrication-candidate proof fields on generic records.
5. Update coverage/golden outputs if the report exposes new evidence fields.

Validation:

- `go test ./internal/components`
- `go test ./cmd/kicadai`
- `prism review staged`

Commit:

- `Add regulator stability catalog metadata`

## Phase 3: Selection Diagnostics And Acceptance Gates

Objective: make regulator/capacitor stability evidence affect selection
outcomes.

Tasks:

1. Add issue codes or structured issue paths for:
   - missing regulator stability proof;
   - incompatible output capacitor ESR/dielectric;
   - missing MLCC DC-bias/effective-capacitance evidence;
   - missing thermal evidence.
2. Add selection checks for regulator output-capacitor compatibility.
3. Preserve connectivity-oriented pass/warn behavior where current tests
   expect generated regulator paths to work.
4. Block fabrication-candidate selection when:
   - AMS1117 output capacitor ESR compatibility is not proven;
   - AP2112K selected ceramic capacitor lacks required DC-bias/effective
     capacitance evidence;
   - thermal review remains unmodeled.
5. Add tests for AP2112K and AMS1117 at structural, connectivity, and
   fabrication-candidate acceptance levels.

Validation:

- `go test ./internal/components`
- `go test ./cmd/kicadai`
- `prism review staged`

Commit:

- `Gate regulator stability in component selection`

## Phase 4: Voltage Regulator Block Evidence Propagation

Objective: carry regulator stability evidence through block and workflow
outputs.

Tasks:

1. Add regulator stability/effective-capacitance review summaries to
   `voltage_regulator` block evidence.
2. Ensure component-selection stage output includes selected regulator and
   capacitor compatibility status.
3. Ensure generated request and workflow artifacts expose:
   - selected regulator ID;
   - input/output capacitor IDs;
   - stability status;
   - remaining review blockers.
4. Preserve AP2112K EN tied-to-VIN behavior and explicit NC handling.
5. Add workflow tests for:
   - AP2112K connectivity path with review evidence;
   - AMS1117 fabrication-candidate block on missing ESR proof;
   - generated schematic identity properties still present.

Validation:

- `go test ./internal/blocks ./internal/designworkflow`
- `go test ./cmd/kicadai`
- `prism review staged`

Commit:

- `Propagate regulator stability workflow evidence`

## Phase 5: CLI, Rationale, And Agent-Facing Output

Objective: make the new evidence visible to users and AI agents.

Tasks:

1. Update `component select` JSON output if needed to expose stability blockers.
2. Update `component coverage` summaries if a regulator stability coverage
   subsection is useful.
3. Update rationale generation so regulator stability blockers are explained in
   plain language.
4. Add CLI tests for:
   - AP2112K selection evidence;
   - AMS1117 fabrication-candidate blocking;
   - missing MLCC derating proof;
   - source snapshot limitations.
5. Ensure errors remain structured and deterministic.

Validation:

- `go test ./internal/rationale ./cmd/kicadai`
- `go test ./...`
- `prism review staged`

Commit:

- `Expose regulator stability evidence in CLI`

## Phase 6: Documentation And Roadmap Update

Objective: close the task with accurate docs and project status.

Tasks:

1. Update `docs/component-intelligence.md`.
2. Update `docs/libraries-and-components.md`.
3. Update `docs/circuit-blocks.md` if block behavior changed.
4. Update `docs/kicadai-agent-skill.md` so agents do not overclaim regulator
   readiness.
5. Update `specs/ROADMAP.md` Priority 1 status and remaining work.
6. Run full tests.

Validation:

- `go test ./...`
- `prism review staged`

Commit:

- `Document regulator stability evidence`

## Completion Criteria

- Existing regulator workflows remain usable for structural/connectivity
  generation.
- Fabrication-candidate regulator claims are blocked without regulator-specific
  stability, ESR, thermal, and MLCC derating evidence.
- AI-facing workflow and CLI output explains regulator stability gaps without
  relying on prose-only warnings.
- Documentation clearly distinguishes nominal component selection from
  regulator stability proof.
