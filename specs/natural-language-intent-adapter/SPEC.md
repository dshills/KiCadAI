# Natural-Language Intent Adapter Specification

Date: 2026-06-26

## 1. Summary

Build a CLI-first adapter that turns a user's natural-language board request
into the existing structured `intentplanner.Request` schema, with source
attribution, confidence, assumptions, and clarification blockers.

This project does not add an MCP server or a live LLM dependency. It creates the
deterministic local intake layer that an AI caller can use before `intent plan`,
`intent explain`, or `intent create`.

The first supported workflow is:

```text
user text
  -> deterministic phrase extraction and normalization
  -> structured intent draft
  -> confidence and source attribution
  -> clarification/blocking issues when unsafe
  -> intent plan/create when safe
```

## 2. Goals

- Accept plain English requests through the CLI.
- Produce the current structured intent JSON schema without bypassing existing
  planner validation.
- Record where each extracted field came from in the original text.
- Assign confidence to extracted dimensions so AI callers can decide whether to
  proceed, ask a clarification question, or block.
- Convert ambiguity into structured clarification issues rather than guessed
  design decisions.
- Reuse the existing intent planner, design workflow, validation, repair,
  fabrication, and artifact conventions.
- Keep the adapter deterministic and testable without network access.

## 3. Non-Goals

- No MCP server.
- No live LLM provider integration.
- No arbitrary freeform circuit synthesis beyond known patterns.
- No automatic datasheet lookup or web search.
- No direct KiCad file generation from unvalidated prose.
- No attempt to parse every possible circuit request in the first version.

## 4. User-Facing CLI

Add a new command family:

```text
kicadai intent draft --text "make a 3.3V I2C temperature sensor breakout"
kicadai intent draft --file request.txt --output ./out
kicadai intent explain --text "..."
kicadai intent create --text "..." --output ./out/project
```

Existing `intent plan`, `intent explain`, and `intent create` JSON-file
workflows must continue to work unchanged.

### 4.1 `intent draft`

`intent draft` converts text into structured intent JSON and a companion
extraction report.

Inputs:

- `--text`: request text.
- `--file`: path to a UTF-8 text file.
- `--output`: optional output directory.
- `--acceptance`: optional default acceptance override.
- `--format`: `json`, with room for future `text`.
- `--strict`: fail if any required clarification is present.

Outputs:

- stdout structured response.
- when `--output` is set:
  - `intent-draft.json`: normalized structured request;
  - `intent-extraction.json`: extraction evidence;
  - `intent-clarifications.json`: clarification prompts and blockers.

The command must not create KiCad project files.

### 4.2 `intent explain --text`

When `--text` is provided, `intent explain` must:

1. run the draft adapter;
2. if the draft has blocking clarification issues, return those issues and stop;
3. otherwise run the existing planner explanation on the drafted request.

### 4.3 `intent create --text`

When `--text` is provided, `intent create` must:

1. run the draft adapter;
2. fail closed on blocking clarification issues;
3. run the existing intent planner;
4. fail closed on blocking planner issues;
5. run the existing design create workflow.

The generated project must persist the same `.kicadai/` artifacts produced by
JSON-driven `intent create`, plus:

- `.kicadai/intent-source.txt`;
- `.kicadai/intent-draft.json`;
- `.kicadai/intent-extraction.json`;
- `.kicadai/intent-clarifications.json`.

## 5. Data Model

Add an internal package, likely `internal/intentdraft`, that returns a structured
result without importing CLI concerns.

### 5.1 Draft Result

```go
type Result struct {
    Request        intentplanner.Request `json:"request"`
    Extraction     ExtractionReport      `json:"extraction"`
    Clarifications []Clarification       `json:"clarifications,omitempty"`
    Issues         []reports.Issue       `json:"issues,omitempty"`
}
```

`Request` must be normalized with existing intent planner normalization before
being returned.

### 5.2 Extraction Report

```go
type ExtractionReport struct {
    SourceID    string            `json:"source_id"`
    SourceType  string            `json:"source_type"` // "text" or "file"
    SourceHash  string            `json:"source_hash"`
    Summary     string            `json:"summary,omitempty"`
    Fields      []ExtractedField  `json:"fields,omitempty"`
    Assumptions []DraftAssumption `json:"assumptions,omitempty"`
    Confidence  ConfidenceSummary `json:"confidence"`
}
```

### 5.3 Extracted Field

```go
type ExtractedField struct {
    Path       string   `json:"path"`
    Value      any      `json:"value,omitempty"`
    SourceText string   `json:"source_text,omitempty"`
    StartByte  int      `json:"start_byte,omitempty"`
    EndByte    int      `json:"end_byte,omitempty"`
    Confidence float64  `json:"confidence"`
    Method     string   `json:"method"`
    Notes      []string `json:"notes,omitempty"`
}
```

Rules:

- `Path` must use the structured request path, for example
  `power.rails[0].voltage`.
- `SourceText` must be a short excerpt from the input.
- Byte offsets are best effort but must be stable for ASCII/UTF-8 input.
- Confidence is `0.0` to `1.0`.
- `Method` values should start with deterministic names such as
  `keyword`, `regex`, `synonym`, `default`, or `inferred`.

### 5.4 Clarification

```go
type Clarification struct {
    ID          string           `json:"id"`
    Path        string           `json:"path,omitempty"`
    Severity    string           `json:"severity"` // "blocking" or "warning"
    Question    string           `json:"question"`
    Options     []string         `json:"options,omitempty"`
    Evidence    []ExtractedField `json:"evidence,omitempty"`
    Suggestion  string           `json:"suggestion,omitempty"`
}
```

Clarification IDs must be stable enough for golden tests.

Examples:

- `intent.board.size_missing`
- `intent.power.voltage_ambiguous`
- `intent.interface.kind_unsupported`
- `intent.function.family_ambiguous`
- `intent.acceptance.fabrication_requested_without_evidence`

## 6. Supported Initial Language Coverage

The first version should intentionally cover the design families KiCadAI can
already plan:

- sensor breakout;
- MCU minimal system;
- MCU programmer;
- power module;
- amplifier module;
- connector breakout;
- LED indicator;
- USB-C power-only;
- I2C sensor node;
- protection requests for ESD and reverse polarity where current block support
  exists.

## 7. Extraction Rules

### 7.1 Project Name And Summary

The adapter should derive:

- `name` from explicit phrases such as "named X", "called X", or from the
  strongest detected family;
- `summary` from the original text, trimmed and length-limited.

If no clean name exists, generate a deterministic slug from detected family and
key features, for example `i2c_sensor_breakout`.

### 7.2 Intent Kind

Map phrases to `intentplanner.IntentKind`:

- "breakout", "breakout board", "adapter board" -> `breakout`
- "sensor", "sensor node", "temperature sensor" -> `sensor_node`
- "mcu", "microcontroller", "minimal system", "arduino-like" ->
  `mcu_minimal`
- "programmer", "ISP", "UART programming header" -> `mcu_minimal` plus
  programming interface/support functions
- "power supply", "regulator", "buck", "LDO" -> `power_module`
- "amplifier", "op amp", "gain stage" -> `amplifier`

If multiple families are present and compatible, choose the broader kind and
emit selected functions/interfaces. If incompatible, emit a blocking
clarification.

### 7.3 Board Constraints

Recognize:

- dimensions: `50x30mm`, `50 mm by 30 mm`, `2 inch x 1 inch`;
- layer count: `2 layer`, `two-layer`, `4 layer`;
- mounting holes: "mounting holes", "no mounting holes";
- fabrication intent: "fab ready", "manufacturable", "order from fab".

Only supported layer counts from existing validation are allowed. Unsupported
layer counts must become blocking issues, not silently downgraded.

### 7.4 Power

Recognize:

- USB-C, USB, barrel jack, external input, battery;
- voltage literals: `5V`, `3.3 V`, `1.8V`, `12 V`;
- current literals: `100mA`, `1A`;
- rails: "make 3.3V from 5V", "needs 5V and 3V3";
- named domains: `VCC`, `VBUS`, `3V3`, `AVCC`.

Use `PowerRailIntent.SuppliedTargets` as the canonical output field. The legacy
`supplies` alias should not be emitted by the adapter.

Ambiguity examples:

- "low power" without voltage/current evidence -> warning or blocking depending
  on whether a concrete rail is required.
- "battery powered" without chemistry or voltage -> blocking when no other
  input voltage exists.

### 7.5 Interfaces

Recognize:

- I2C, SPI, UART, GPIO, analog input, USB-C, programming, ISP;
- connector hints such as "header", "JST", "screw terminal", "USB-C";
- bus names such as "I2C bus 1", "sensor bus", "STEMMA/Qwiic".

Only emit supported interface kinds into the structured request. Unsupported
interfaces should produce clarification or blocker records with the original
phrase preserved.

### 7.6 Functions

Recognize:

- `sensor` with family hints: temperature, humidity, pressure, generic I2C;
- `mcu` with family hints: ATmega, RP2040, STM32, ESP32;
- `programming` with ISP/UART mode;
- `clock` with crystal/canned oscillator frequency hints;
- `led_indicator`;
- `regulator`;
- `amplifier`;
- protection functions.

When family support is not implemented, emit an explicit known gap or blocking
clarification before the request reaches `intent create`.

### 7.7 Ratings And Values

Recognize common numeric values and route them into `params` only when the
current planner/block supports that parameter:

- resistor/capacitor-like values such as `10k`, `100nF`;
- oscillator frequency such as `16 MHz`;
- amplifier gain such as `gain of 10`;
- regulator current such as `500mA`.

If a value is detected but no supported target exists, keep it in extraction
evidence and emit a warning rather than dropping it silently.

## 8. Confidence Policy

The adapter must produce confidence at field and summary level.

Recommended initial policy:

- exact keyword/regex matches with unambiguous schema mapping: `0.9`;
- synonym matches: `0.75`;
- inferred defaults: `0.55`;
- unsupported but detected phrases: `0.4`;
- conflicting phrases: `0.2`.

Blocking thresholds:

- required field below `0.6` and no safe default -> clarification.
- conflicting required fields -> blocking clarification.
- unsupported required family/interface -> blocking issue.

Warnings:

- optional preference below threshold;
- value detected but currently unused;
- fabrication phrasing without enough current proof to guarantee readiness.

## 9. Clarification Policy

The adapter must ask for clarification when:

- a required voltage is missing for a power-dependent design;
- a requested interface is unsupported or conflicts with the selected block;
- multiple incompatible design families are requested;
- multiple target components are implied but not distinguishable;
- fabrication readiness is requested but acceptance/proof gates cannot satisfy
  the request;
- user text asks for removal/editing of an existing project, which belongs to a
  different imported-project preservation workflow.

Clarifications should be machine-actionable. Each should include:

- stable ID;
- path where the ambiguity would map;
- short question;
- constrained options where possible;
- evidence excerpts.

## 10. Planner Integration

The adapter must not duplicate the planner's safety gates. It should:

- produce the structured request;
- call existing `intentplanner.NormalizeRequest`;
- call existing `intentplanner.ValidateRequest`;
- expose validation issues in the draft result;
- allow `intent explain/create --text` to reuse the existing planner path.

The planner remains the source of truth for:

- block selection;
- semantic target resolution;
- voltage-domain compatibility;
- unsupported topology reporting;
- design workflow request generation.

## 11. Artifact Format

When output is requested, write:

```text
intent-source.txt
intent-draft.json
intent-extraction.json
intent-clarifications.json
```

For `intent create --text`, copy or write these under generated
`.kicadai/` alongside existing planner artifacts.

All JSON must be deterministic:

- stable key order through Go encoder behavior;
- sorted maps/slices where necessary;
- stable generated names and IDs;
- no timestamps in golden-tested artifacts.

## 12. Testing

Required tests:

- unit tests for phrase extraction;
- unit tests for voltage/current/dimension parsing;
- unit tests for confidence and clarification thresholds;
- golden CLI tests for `intent draft --text`;
- golden CLI tests for `intent explain --text` blocking and successful paths;
- golden CLI tests for `intent create --text` refusal on unsafe ambiguity;
- regression tests proving JSON-file `intent plan/explain/create` behavior is
  unchanged.

Golden fixture families:

- "3.3V I2C temperature sensor breakout";
- "ATmega minimal board with ISP header and 16 MHz crystal";
- "USB-C 5V to 3.3V regulator module";
- "class AB headphone amplifier" should warn or block if current support cannot
  produce a safe verified design;
- "battery powered sensor" should request battery/voltage clarification.

## 13. Acceptance Criteria

- `kicadai intent draft --text ...` produces a valid structured request for
  supported seed phrases.
- Draft output includes extraction evidence, confidence, and any
  clarifications.
- Ambiguous or unsupported requests fail closed before project generation.
- `intent explain --text` and `intent create --text` reuse existing planner
  behavior after successful drafting.
- Generated projects from text persist source and draft artifacts.
- Existing JSON request workflows remain backward compatible.
- `go test ./...` passes without KiCad or network access.

## 14. Open Questions

- Should future LLM adapters write into this draft result shape directly, or
  should they produce only candidate structured requests that this deterministic
  adapter audits?
- Should clarification responses be accepted through a follow-up JSON patch
  command, or should callers regenerate a new text request?
- Should field confidence eventually feed design workflow acceptance levels, or
  remain an intake-only signal?

