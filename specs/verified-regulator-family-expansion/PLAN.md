# Verified Regulator Family Expansion Implementation Plan

Date: 2026-06-26

## Implementation Rules

- Commit each phase independently after `prism review staged`.
- Keep default tests hermetic; no network, distributor API, or live KiCad
  dependency.
- Treat local KiCad library inspection as audit input only. Any symbol,
  footprint, or pinmap facts needed by tests must be committed as catalog data,
  built-in pinmaps, or static fixtures.
- Tests that need footprint geometry or pad counts must use committed static
  fixtures, built-in pinmaps, or catalog pad mappings instead of reading a
  developer's KiCad library checkout.
- Prefer small catalog additions with verified symbol, footprint, and pinmap
  evidence over broad placeholder coverage.
- Do not mark fabrication-candidate paths ready unless thermal, stability, and
  capacitor derating evidence is either modeled or explicitly blocked.
- Use existing component catalog, block component role, intent planner, workflow
  evidence, and rationale patterns.
- Preserve deterministic JSON ordering and stable issue codes.
- Run focused tests after each phase and `go test ./...` before final closeout.

## Phase 1: Regulator Evidence Audit

Objective: choose the smallest safe regulator expansion slice.

Tasks:

- Inspect available KiCad symbols and footprints for likely LDO candidates in
  configured local libraries and built-in pinmap fixtures.
- Audit existing regulator, capacitor, and power block records.
- Choose one or two concrete regulator candidates that can be verified locally.
- Record which candidates are blocked by missing symbol, footprint, or pinmap
  evidence.
- Define the thermal/stability evidence fields required for this slice.
- Write an audit note under this spec directory if useful.

Review:

- Confirm the selected part or parts can be represented without placeholder
  confidence.
- Confirm any unmodeled thermal, ESR, or capacitor derating behavior is
  documented as a blocker or warning.
- `prism review staged`

Commit:

- Commit with a message like `Plan verified regulator family expansion`.

## Phase 2: Catalog Records And Validation

Objective: add verified regulator/capacitor evidence to the component catalog.

Tasks:

- Add selected regulator records under `data/components/`.
- Include output-voltage, input-voltage, output-current, dropout, package,
  temperature, companion, placement, routing, thermal, and stability evidence.
- Include EN voltage metadata and output-capacitor ESR/stability review
  metadata when the selected regulator exposes those requirements.
- Include pin semantic metadata that distinguishes ordinary `NC` pins from
  manufacturer do-not-connect/internal-connection pins.
- Add or update capacitor policy metadata only where existing schema supports
  it; otherwise add minimal schema support with tests.
- Add pinmap fixtures or built-in evidence for new active regulator records.
- Add catalog validation tests for:
  - required regulator function pins;
  - optional regulator function pins such as `EN` and `NC`;
  - package pad mappings;
  - required ratings and values;
  - companion capacitor requirements;
  - thermal/stability review metadata.

Review:

- `go test ./internal/components ./internal/pinmap`
- `prism review staged`

Commit:

- Commit with a message like `Add verified regulator family catalog records`.

## Phase 3: Selection Policy And Diagnostics

Objective: make regulator selection deterministic, rating-aware, and
diagnostic-rich.

Tasks:

- Add selection tests for:
  - matching 3.3 V regulator requests;
  - unsupported output voltage;
  - insufficient input voltage;
  - insufficient headroom where
    `input_voltage < output_voltage + dropout_voltage + margin`;
  - insufficient output current;
  - package preference pass/fail;
  - fabrication-candidate requests with missing thermal/stability proof.
- Add first-order thermal estimation tests using
  `P = ((Vin - Vout) * Iout) + (Vin * Ignd)` when ground-current metadata is
  available, with a missing-ground-current warning otherwise, and a
  conservative package threshold where modeled.
- Extend selection diagnostics if current rejected-candidate reasons are too
  generic for regulator failures.
- Ensure selected evidence includes thermal/stability review rules.
- Ensure capacitor voltage policy distinguishes minimum automated class from
  fabrication-candidate recommended derating where modeled.

Review:

- `go test ./internal/components ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Harden regulator selection policy`.

## Phase 4: Voltage Regulator Block Variant Support

Objective: connect regulator family selection to the `voltage_regulator` block.

Tasks:

- Make block component-role queries derive output voltage, input voltage,
  output current, package preference, and capacitor policy from block params.
- Add regulator pin-role behavior for five-pin LDOs:
  - tie `EN` to VIN by default unless an explicit supported enable control is
    provided, but only after validating VIN is within the modeled `EN` maximum
    recommended range using `input_voltage + 0.5 V` when no request-specific
    transient maximum is known;
  - emit explicit KiCad schematic no-connect flags for `NC` pins or block if
    the schematic model cannot represent them safely;
  - distinguish generic `NC` pins from manufacturer do-not-connect/internal
    connection pins and block unsafe generated connections;
  - block `ADJ` and `BYP` variants until their companion networks are modeled.
- Add block validation for unsupported or contradictory regulator params.
- Add tests for:
  - default 3.3 V path;
  - alternate supported regulator variant if added;
  - enable tied to VIN for AP2112K-style regulators;
  - explicit KiCad no-connect flag handling for AP2112K-style `NC` pins;
  - unsupported voltage blocking;
  - optional power LED roles staying conditional;
  - selected role evidence in block/component selection output.
- Keep PCB realization deterministic and do not broaden layout claims beyond
  current evidence.

Review:

- `go test ./internal/blocks ./internal/designworkflow`
- `prism review staged`

Commit:

- Commit with a message like `Add regulator block variant selection`.

## Phase 5: Intent Planner And Rationale Evidence

Objective: allow structured power intent to select regulator variants without
guessing.

Tasks:

- Map `power.inputs[]`, `power.rails[].voltage`, and
  `power.rails[].current_ma` into regulator selection requirements.
- Add synthesis evidence for:
  - input and output voltage;
  - current requirement;
  - dropout/headroom;
  - capacitor minimum voltage class;
  - recommended derating gap for fabrication candidates;
  - thermal review formula or requirement;
  - stability review requirement.
- Add planner tests for supported, unsupported, ambiguous, and
  fabrication-candidate cases.
- Update rationale generation if regulator warnings are not already surfaced
  clearly.

Review:

- `go test ./internal/intentplanner ./internal/rationale`
- `prism review staged`

Commit:

- Commit with a message like `Map regulator variants into intent planning`.

## Phase 6: Workflow Fixtures And Generated Evidence

Objective: prove the expanded regulator path works through generated projects.

Tasks:

- Add or update `examples/intent/` fixtures for regulator-backed requests.
- Add CLI or workflow tests asserting generated artifacts contain:
  - selected regulator ID and package;
  - selected capacitor roles;
  - rating requirements;
  - thermal/stability limitations;
  - rejected candidates when relevant.
- Confirm connectivity-oriented requests fail closed when required regulator
  evidence is missing.
- Add explicit fixtures for missing regulator catalog evidence and insufficient
  regulator headroom so safety behavior is tested end to end.
- Confirm fabrication-candidate requests do not silently pass with generic
  capacitor or unproven thermal evidence.

Review:

- `go test ./internal/designworkflow ./cmd/kicadai`
- `prism review staged`

Commit:

- Commit with a message like `Add regulator workflow evidence fixtures`.

## Phase 7: Documentation, Agent Skill, And Roadmap Updates

Objective: keep user and agent guidance aligned with the expanded regulator
coverage.

Tasks:

- Update:
  - `README.md` if status changes;
  - `docs/component-intelligence.md`;
  - `docs/circuit-blocks.md`;
  - `docs/intent-planning.md`;
  - `docs/kicadai-agent-skill.md`;
  - `specs/ROADMAP.md`.
- Document:
  - supported regulator variants;
  - known unsupported variants;
  - capacitor voltage policy;
  - thermal and stability limits;
  - fabrication-candidate blockers.
- Keep all examples using the compiled `kicadai` binary.

Review:

- Local markdown link check.
- `rg -n "go run ./cmd/kicadai|go run" README.md docs`
- `prism review staged`

Commit:

- Commit with a message like `Document regulator family expansion`.

## Phase 8: Final Compatibility Sweep

Objective: ensure the expansion does not regress existing generation behavior.

Tasks:

- Run `go test ./...`.
- Run focused component, block, intent, and workflow tests.
- Verify generated JSON ordering and issue paths are deterministic.
- Verify default tests do not require KiCad, network, or external catalog data.
- Inspect `git status --short` for generated-file noise.

Review:

- `go test ./...`
- `prism review staged` if any final files changed.

Commit:

- Commit final fixes, if any, with a message like `Harden regulator family expansion`.

## Completion Criteria

- The catalog includes at least one additional verified regulator candidate or
  a documented audit explaining why no candidate can be safely promoted yet.
- Regulator selection handles voltage, current, dropout, package, and
  fabrication-evidence policy deterministically.
- The `voltage_regulator` block can request supported regulator variants and
  rejects unsupported variants cleanly.
- Structured intent persists regulator, capacitor, thermal, and stability
  evidence in generated artifacts.
- Docs and agent skill guidance make the remaining power-design limits explicit.
- Full tests pass.
