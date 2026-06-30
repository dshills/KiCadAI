# Schematic Electrical Rules Specification

Date: 2026-06-30

## Purpose

KiCadAI can now write KiCad schematics with generated connectivity evidence,
embedded seed symbol bodies, and basic power-driver policy. The next blocker is
schematic-level electrical review: an AI should not move from a generated
schematic to PCB realization merely because the file parses and wires touch
known pin anchors.

This project adds deterministic schematic electrical rules that catch common
design mistakes before PCB generation and before optional KiCad ERC. The rules
must be explicit, inspectable, and conservative. They should give an AI caller a
clear answer to: "Is this schematic electrically meaningful enough to continue,
and if not, what specific design intent is missing?"

## Current State

Implemented foundations:

- `.kicad_sch` writer, reader, validation, and round-trip preservation.
- Generated schematic connectivity inspection with issue paths.
- Generated power policy evidence: `not_required`, `requires_driver`, and
  `driven`.
- Embedded seed symbol templates and pin anchor derivation for common passives,
  LEDs, diodes, and selected power symbols.
- Schematic readability rules for generated and checked-in examples.
- Component catalog evidence for selected parts, ratings, pinmaps, and hidden
  schematic identity properties.
- Circuit block metadata for ports, local routes, PCB constraints, and
  readiness evidence.
- Optional KiCad ERC/DRC execution and report parsing.

Remaining gap:

KiCadAI does not yet have a single schematic-only rule engine that understands
required pins, labels, no-connect intent, duplicate references, power-domain
expectations, decoupling requirements, or basic value/rating sanity. Some of
these checks exist indirectly in blocks, component selection, writer checks, or
KiCad ERC output, but they are not exposed as one deterministic schematic
semantic gate for AI design workflows.

## Goals

- Add a deterministic schematic electrical rule engine above raw connectivity
  validation.
- Catch common schematic mistakes before PCB generation:
  - duplicate non-power references;
  - required generated pins with no connection;
  - intentional open pins without no-connect evidence;
  - labels that do not attach to a known anchor;
  - conflicting or malformed labels on the same net;
  - power rails without explicit source/external-driver policy;
  - missing decoupling strategy for modeled active parts;
  - impossible or suspicious values for supported passive roles;
  - selected component rating gaps visible from schematic/component evidence.
- Produce AI-facing findings with stable rule IDs, severity, object references,
  net names where known, and repair guidance.
- Integrate the rule summary into `design create`, writer/evaluate output where
  appropriate, and promotion evidence.
- Keep KiCad ERC as an external evidence layer, not the only schematic
  electrical review mechanism.

## Non-Goals

- Full analog simulation or SPICE validation.
- Full KiCad ERC reimplementation.
- Arbitrary datasheet compliance proof.
- Automatic insertion of components solely to silence warnings.
- Safe mutation of imported user-authored schematics.
- Complete cross-sheet/hierarchical connectivity modeling in the first pass.
- Guaranteeing fabrication readiness from schematic-only evidence.

## Rule Model

### Rule Categories

Each finding must include a category:

- `reference`: duplicate or malformed schematic references.
- `pin`: required, optional, hidden, or no-connect pin problems.
- `net`: label, net naming, or connectivity consistency problem.
- `power`: source, sink, rail, or external-driver policy problem.
- `decoupling`: missing or incomplete local power bypass strategy.
- `value`: invalid, missing, or suspicious component value.
- `rating`: component rating evidence missing or insufficient for known use.
- `library`: unresolved symbol/pin metadata needed to evaluate rules.
- `hierarchy`: sheet or cross-sheet limitations that block confident review.

### Severity

Findings must use the existing severity style where possible:

- `info`: evidence note, not a blocker.
- `warning`: review needed, but generation may continue at draft levels.
- `error`: invalid generated schematic for connectivity or electrical intent.
- `blocked`: required evidence is missing for the requested acceptance level.

For `fabrication-candidate`, required ERC, or AI one-shot generation requests,
warnings that affect electrical meaning should be promotable to `blocked` by
policy.

### Scope

Initial implementation should focus on generated schematics and known block
families. Imported schematics may be read and reported on, but findings must be
clearly labeled as best-effort unless preservation-safe imported-project
semantics are explicitly available.

## Required Rules

### Duplicate References

KiCadAI must detect duplicate non-power references across the generated design
scope before footprint assignment and BOM generation. Power symbols such as
`#PWRxx` and `#FLGxx` may repeat only according to KiCad conventions and must
not collide with normal component references.

Finding examples:

- `SCH_REF_DUPLICATE`
- `SCH_REF_EMPTY`
- `SCH_REF_POWER_COLLISION`

### Required Pins And No-Connect Intent

For symbols with known pin metadata or block contracts, KiCadAI must distinguish:

- connected required pins;
- connected optional pins;
- intentionally unused pins with explicit no-connect markers;
- externally provided ports represented by connector, label, sheet pin, or
  accepted external-driver policy;
- unknown pins that cannot be evaluated.

No-connect markers must only satisfy rules when they sit on a known symbol pin
anchor and the pin is declared optional or intentionally unused. KiCadAI must
not automatically add no-connect markers for required pins.

Finding examples:

- `SCH_PIN_REQUIRED_OPEN`
- `SCH_PIN_OPTIONAL_OPEN`
- `SCH_PIN_NC_MISSING`
- `SCH_PIN_NC_ON_REQUIRED`
- `SCH_PIN_METADATA_MISSING`

### Labels And Net Consistency

Labels must attach to known anchors. Repeated labels with the same text are
allowed when they intentionally join a net. Conflicting labels on a connected
net should be reported unless the model marks them as aliases. Empty labels,
case-only duplicates, or whitespace-normalized collisions should be reported.

Finding examples:

- `SCH_LABEL_FLOATING`
- `SCH_LABEL_EMPTY`
- `SCH_LABEL_CONFLICT`
- `SCH_LABEL_NORMALIZATION_COLLISION`

### Power Rail Policy

The existing generated power policy must become part of a richer power rail
report. For each known power net, the engine should identify:

- rail name;
- source evidence: regulator output, connector/external input, battery,
  `PWR_FLAG`, or unknown;
- sink evidence: active component power pins, LEDs, passives, or unknown;
- accepted external-driver policy for non-standalone modules;
- missing source or sink problems.

`PWR_FLAG` must be treated as design intent evidence, not proof of electrical
correctness. For generated modules intended to be externally powered,
`requires_driver` can pass only when the request/block contract explicitly says
the rail is externally driven.

Finding examples:

- `SCH_POWER_SOURCE_MISSING`
- `SCH_POWER_SINK_MISSING`
- `SCH_POWER_EXTERNAL_UNDECLARED`
- `SCH_POWER_FLAG_WITHOUT_RAIL`
- `SCH_POWER_METADATA_MISSING`

### Decoupling Policy

For modeled active components and blocks, KiCadAI must report decoupling
expectations at the schematic level before placement checks. The first pass
should be contract-based, not geometry-based:

- regulator input/output capacitors required by the block or component record;
- MCU and sensor supply decoupling requirements when modeled;
- oscillator supply decoupling when modeled;
- op-amp supply decoupling when the block exposes supply pins;
- explicit deferred review when the catalog lacks enough evidence.

Finding examples:

- `SCH_DECOUPLING_MISSING`
- `SCH_DECOUPLING_VALUE_MISMATCH`
- `SCH_DECOUPLING_RAIL_MISMATCH`
- `SCH_DECOUPLING_EVIDENCE_DEFERRED`

### Value And Rating Sanity

The schematic rule engine should consume available block/component evidence
without duplicating the full component-selection system. It should report
obvious local problems:

- missing value on passives where a value is required;
- invalid numeric/unit syntax for supported values;
- LED resistor absent or nonsensical when generated by supported blocks;
- capacitor voltage rating below known rail voltage when component evidence is
  present;
- resistor power rating below simple modeled requirement when available;
- catalog placeholder selected where request acceptance forbids placeholders.

Finding examples:

- `SCH_VALUE_MISSING`
- `SCH_VALUE_PARSE_FAILED`
- `SCH_VALUE_OUT_OF_POLICY`
- `SCH_RATING_INSUFFICIENT`
- `SCH_RATING_EVIDENCE_MISSING`

## Output Model

Add a schematic electrical report with this shape:

```json
{
  "status": "blocked",
  "checked_symbols": 8,
  "checked_nets": 5,
  "checked_power_rails": 2,
  "checked_required_pins": 14,
  "checked_decoupling_requirements": 3,
  "finding_count": 1,
  "findings": [
    {
      "rule_id": "SCH_POWER_SOURCE_MISSING",
      "severity": "blocked",
      "category": "power",
      "reference": "U1",
      "pin": "8",
      "net": "VCC",
      "message": "VCC has power sinks but no modeled source or accepted external driver",
      "repair": "add a regulator, connector power input, battery source, or explicit external-driver policy"
    }
  ]
}
```

Statuses:

- `not_applicable`: no schematic was available.
- `clean`: no findings.
- `warning`: only non-blocking findings.
- `blocked`: at least one blocking finding.
- `unknown`: required metadata is unavailable and the acceptance level requires
  confidence.

## Integration Points

### Go Packages

Preferred package boundary:

- `internal/kicadfiles/schematic`: low-level file model, connectivity evidence,
  and symbol/pin primitives.
- `internal/schematicrules`: new higher-level rule engine that consumes a
  schematic plus optional block/component/workflow context.
- `internal/designworkflow`: attaches generated workflow context and writes
  stage/promotion evidence.

The rule engine must be usable without KiCad installed.

### CLI And Workflow

The first implementation may expose the report through existing command output
rather than a new CLI command. Candidate integration points:

- `design create`: add a `schematic_electrical` workflow stage summary.
- `evaluate schematic`: include best-effort report for local files.
- `writer check`: include generated-project blocking findings when context is
  available.
- promotion reports: add a `schematic_electrical` gate for generated designs.

### Repair Loop

Findings should be written in a shape that future repair planning can consume,
but automatic repair is not required in this project. Suggested repair
categories:

- `add_connection`
- `add_no_connect`
- `add_power_source`
- `accept_external_power`
- `add_decoupling`
- `fix_value`
- `select_component`
- `provide_metadata`

## Testing Strategy

Default tests must not require KiCad. Required test coverage:

- duplicate references across generated root schematic;
- required pin open;
- optional pin with and without no-connect;
- floating label;
- conflicting labels on one connected net;
- power sink without source;
- explicit external power accepted;
- `PWR_FLAG` treated as intent evidence only;
- decoupling requirement missing and satisfied;
- capacitor voltage rating insufficient when evidence exists;
- unknown symbol metadata classified as `unknown` or `blocked` according to
  acceptance policy;
- workflow report serialization and promotion gate summary.

Optional KiCad-backed tests may be added only behind existing opt-in environment
variables and should preserve artifacts outside the repository unless explicitly
requested.

## Acceptance Criteria

- Generated schematic-only evaluation catches common electrical mistakes before
  PCB generation.
- `design create` exposes schematic electrical evidence in workflow output and
  promotion reports.
- Rule IDs and severities are deterministic and documented.
- Supported generated blocks can distinguish required pins from intentional
  no-connects and external ports.
- Power-source requirements are explicit and do not encourage adding
  `PWR_FLAG` as a blind workaround.
- Decoupling requirements for modeled active blocks are visible before layout.
- Default `go test ./...` passes without KiCad.

## Risks

- Schematic electrical rules can become a partial ERC clone. Keep rules focused
  on AI workflow decisions and defer broad KiCad-specific checks to KiCad ERC.
- Unknown library metadata can produce noisy findings. Fail closed only when the
  requested acceptance level requires confidence.
- Over-strict decoupling rules may reject legitimate external-module use cases.
  Allow explicit block/request contracts for externally supplied rails.
- Value/rating checks can duplicate component selection. Consume existing
  evidence and report gaps; do not fork the selector.
- Repair guidance can be unsafe if it implies arbitrary component insertion.
  Keep repair text explicit about required design intent.
