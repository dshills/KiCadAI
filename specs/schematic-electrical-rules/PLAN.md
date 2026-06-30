# Schematic Electrical Rules Implementation Plan

Date: 2026-06-30

## Phase 1: Define Rule Report Model And Package Boundary

Objective: create the stable in-process report API without changing workflow
behavior yet.

Deliverables:

- Add `internal/schematicrules` with report, status, severity, category, and
  finding types.
- Define rule ID constants for the first rule families.
- Add an options/context type that can carry acceptance level, generated vs
  imported scope, block contracts, component evidence, accepted external rails,
  and symbol metadata availability.
- Add helper methods for status aggregation and deterministic finding sorting.

Tests:

- Status aggregation: clean, warning, blocked, unknown.
- Deterministic finding ordering by severity, category, rule ID, reference,
  net, pin, and message.
- JSON serialization stays stable for AI callers.

Verification:

- `go test ./internal/schematicrules`
- `go test ./internal/kicadfiles/schematic`

Commit: `Add schematic electrical rule report model`

## Phase 2: Implement Reference, Label, And No-Connect Rules

Objective: cover pure schematic-file checks that do not require component
catalog or block metadata.

Deliverables:

- Detect duplicate non-power references in generated schematic scope.
- Detect empty or malformed references where the low-level writer does not
  already block them.
- Detect floating labels by consuming generated connectivity anchors.
- Detect empty labels and whitespace/case normalization collisions.
- Detect no-connect markers not placed on known symbol pin anchors.
- Detect no-connect markers placed on required pins only when required-pin
  context is available; otherwise report metadata missing at most.

Tests:

- Duplicate root references.
- Duplicate power references are ignored according to KiCad-style prefixes.
- Floating label produces `SCH_LABEL_FLOATING`.
- Same connected net with conflicting labels produces `SCH_LABEL_CONFLICT`.
- No-connect marker away from a pin produces `SCH_PIN_NC_MISSING` or
  equivalent configured rule.

Verification:

- `go test ./internal/schematicrules ./internal/kicadfiles/schematic`

Commit: `Add basic schematic electrical file rules`

## Phase 3: Add Required Pin And External Port Policy

Objective: connect block/design intent to schematic pin checks.

Deliverables:

- Define required/optional/external/no-connect pin intent inputs in the rule
  context without forcing a broad block-model refactor.
- Map existing generated block ports and design API pin anchors into rule
  context where feasible.
- Detect required known pins with no connection.
- Accept external ports only when represented by connector, label, sheet pin,
  or explicit external policy.
- Treat unknown pin metadata as `unknown` for draft workflows and `blocked` for
  ERC-required/fabrication-candidate workflows.

Tests:

- Required pin open blocks.
- Optional pin open warns or passes according to context.
- Optional pin with no-connect passes.
- Required pin with no-connect blocks.
- External pin with accepted external policy passes.
- Unknown metadata produces policy-dependent status.

Verification:

- `go test ./internal/schematicrules ./internal/blocks ./internal/designworkflow`

Commit: `Validate schematic required pin intent`

## Phase 4: Add Power Rail Rule Engine

Objective: replace the flat generated power policy with per-rail evidence for
AI decisions.

Deliverables:

- Build a per-rail power summary from labels, power symbols, known power pins,
  block contracts, regulator/connector source evidence, and `PWR_FLAG` intent.
- Detect rails with sinks and no source or accepted external-driver policy.
- Detect rails with sources and no sinks as warnings.
- Detect `PWR_FLAG` without a meaningful rail attachment.
- Preserve the existing generated power policy fields by adapting them from the
  richer report where needed.
- Ensure `PWR_FLAG` remains intent evidence, not proof of KiCad ERC success.

Tests:

- VCC sink without source blocks.
- Connector or regulator source satisfies rail source.
- Explicit external module rail passes with evidence.
- `PWR_FLAG` attached to a rail marks intent but does not suppress missing
  metadata warnings.
- `PWR_FLAG` floating blocks.

Verification:

- `go test ./internal/schematicrules ./internal/kicadfiles/schematic ./internal/designworkflow`

Commit: `Add schematic power rail rules`

## Phase 5: Add Decoupling And Value/Rating Sanity Rules

Objective: surface early electrical-quality evidence for active blocks and
selected components.

Deliverables:

- Introduce rule-context inputs for block/component decoupling requirements.
- Detect missing decoupling capacitors for modeled MCU, sensor, regulator,
  oscillator, and op-amp supply pins where the block already declares the
  requirement.
- Validate modeled capacitor roles against expected rails.
- Consume existing component evidence for value/rating checks instead of
  duplicating selector logic.
- Add value parsing for the small supported passive set needed by current
  blocks.
- Report missing or insufficient voltage/current/power rating evidence where a
  known requirement exists.

Tests:

- Regulator input/output capacitor requirements pass/fail.
- MCU/sensor decoupling requirement pass/fail.
- Op-amp supply decoupling deferred when unsupported by block context.
- Capacitor voltage rating below rail voltage blocks for fabrication-candidate.
- Missing passive value reports warning/error by policy.

Verification:

- `go test ./internal/schematicrules ./internal/blocks ./internal/components ./internal/designworkflow`

Commit: `Add schematic decoupling and rating rules`

## Phase 6: Integrate With Design Workflow And Promotion Reports

Objective: make the report visible to AI callers during generation.

Deliverables:

- Run schematic electrical rules in `design create` after schematic generation
  and before PCB realization decisions that depend on schematic correctness.
- Add a `schematic_electrical` workflow stage with compact summary and findings.
- Add promotion report gate and issue-code mapping for schematic electrical
  findings.
- Ensure draft workflows can continue with warnings while
  fabrication-candidate, required ERC, and one-shot autonomous acceptance
  requests block on critical findings.
- Persist report artifacts under `.kicadai/` only where existing workflow
  artifact conventions support it.

Tests:

- `design create` result includes schematic electrical summary.
- Blocking schematic electrical finding prevents inappropriate promotion.
- Draft acceptance keeps non-blocking findings visible.
- Promotion report records gate status, issue codes, and repair guidance.

Verification:

- `go test ./internal/designworkflow ./cmd/kicadai`

Commit: `Integrate schematic electrical rules into design workflow`

## Phase 7: Add CLI/Evaluate Surfacing

Objective: expose best-effort schematic electrical evaluation outside
`design create`.

Deliverables:

- Add schematic electrical report to `evaluate schematic` when enough local
  evidence exists.
- Add best-effort report to `evaluate project` for generated projects.
- Keep imported-project findings clearly labeled as best-effort unless
  generated metadata is present.
- Update CLI golden tests for stable JSON shape.

Tests:

- `evaluate schematic` includes `schematic_electrical` report.
- Generated project evaluation includes report and generated status.
- Imported schematic without context reports unknown metadata rather than false
  confidence.
- CLI golden output remains deterministic.

Verification:

- `go test ./cmd/kicadai ./internal/evaluate ./internal/schematicrules`
  adjusted to existing package names.

Commit: `Expose schematic electrical rules in evaluation`

## Phase 8: Documentation And Agent Guidance

Objective: make the new gate understandable for users and AI agents.

Deliverables:

- Update `docs/validation-and-analysis.md` with report fields, rule IDs, and
  command examples.
- Update `docs/kicadai-agent-skill.md` with success and stop conditions.
- Update `README.md` status paragraph only if needed.
- Add troubleshooting examples for required pin, power source, decoupling, and
  rating findings.
- Update `specs/ROADMAP.md` after implementation to move this item from
  remaining work to current foundation.

Tests:

- Documentation commands use `kicadai`, not `go run`.
- Any JSON examples match the implemented report shape.

Verification:

- `go test ./...`

Commit: `Document schematic electrical rules`

## Prism And Commit Process

For each phase:

1. Implement the phase.
2. Run targeted tests listed for the phase.
3. Stage only relevant files.
4. Run `prism review staged`.
5. Fix findings or document false positives in the final turn summary.
6. Commit before moving to the next phase.

## Completion Criteria

The project is complete when generated schematic workflows produce a deterministic
schematic electrical report, common schematic mistakes are caught before PCB
generation, promotion reports include schematic electrical evidence, and default
tests pass without KiCad.
