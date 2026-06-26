# Natural-Language Intent Adapter Implementation Plan

Date: 2026-06-26

## Phase 1: Draft Model And Package Skeleton

Create the internal adapter package and result types.

Tasks:

1. Add `internal/intentdraft`.
2. Define:
   - `Result`;
   - `ExtractionReport`;
   - `ExtractedField`;
   - `DraftAssumption`;
   - `ConfidenceSummary`;
   - `Clarification`.
3. Add `Options` for source type, source ID, acceptance override, and strict
   mode.
4. Add `Draft(text string, options Options) Result`.
5. Normalize the generated request through `intentplanner.NormalizeRequest`.
6. Validate through `intentplanner.ValidateRequest`.
7. Add basic source hashing and summary trimming.
8. Add focused tests for empty input, source hash stability, and validation
   issue propagation.

Acceptance:

- Package compiles without CLI dependencies.
- Empty input returns a blocking issue.
- Result JSON is deterministic.

Commit message:

`Add natural language draft model`

## Phase 2: Tokenization And Primitive Parsers

Build deterministic parsing helpers for the language fragments needed by the
first adapter.

Tasks:

1. Add stable text normalization:
   - lowercase matching;
   - punctuation-tolerant token lookup;
   - original byte offsets for source excerpts.
2. Parse voltage literals:
   - `3.3V`;
   - `3V3`;
   - `5 v`;
   - named domains such as `vbus`, `vcc`, `avcc`.
3. Parse current literals:
   - `100mA`;
   - `1A`.
4. Parse dimensions:
   - `50x30mm`;
   - `50 mm by 30 mm`;
   - inches converted to millimeters with deterministic rounding.
5. Parse layer count:
   - numeric;
   - `two-layer`;
   - unsupported layer counts as findings.
6. Parse values used by supported blocks:
   - oscillator frequency;
   - amplifier gain;
   - common R/C values.
7. Add unit tests for all parser helpers and byte-offset evidence.

Acceptance:

- Parsers return value, excerpt, byte offsets, confidence, and method.
- Unsupported but detected values can be reported without mutating the request.

Commit message:

`Parse natural language intent primitives`

## Phase 3: Intent Family And Requirement Extraction

Map supported prose patterns into the existing structured request schema.

Tasks:

1. Add synonym tables for known families:
   - breakout;
   - sensor node;
   - MCU minimal;
   - MCU programmer;
   - power module;
   - amplifier;
   - connector breakout;
   - LED indicator;
   - USB-C power-only;
   - protection.
2. Derive deterministic request `name`, `summary`, and `kind`.
3. Extract `PowerIntent`:
   - inputs;
   - rails;
   - `supplied_targets`;
   - aliases.
4. Extract `InterfaceIntent`:
   - I2C;
   - SPI;
   - UART;
   - GPIO;
   - programming/ISP where supported.
5. Extract `FunctionIntent`:
   - sensors;
   - MCU;
   - programming;
   - clock;
   - regulator;
   - amplifier;
   - protection.
6. Extract board constraints and acceptance defaults.
7. Record `ExtractedField` evidence for every populated structured field.
8. Add tests for representative supported phrases.

Acceptance:

- Supported seed phrases produce valid `intentplanner.Request` objects.
- Each non-default field has extraction evidence.
- Unsupported families are not silently converted to generic placeholders.

Commit message:

`Extract structured intent from prose`

## Phase 4: Confidence And Clarification Engine

Add the safety policy that decides whether drafted intent is usable.

Tasks:

1. Implement confidence aggregation.
2. Add required-field threshold checks.
3. Add conflict detection:
   - incompatible families;
   - multiple voltages without source/load relationship;
   - unsupported required interface;
   - battery input without voltage/chemistry;
   - fabrication wording without sufficient readiness proof.
4. Add clarification builders with stable IDs and suggested options.
5. Add strict-mode behavior.
6. Add tests for:
   - ambiguous power request;
   - unsupported interface;
   - conflicting families;
   - low-confidence optional preference;
   - safe inferred default with warning.

Acceptance:

- Ambiguous requests produce structured clarifications.
- Blocking clarifications prevent `intent create --text`.
- Safe warnings still allow `intent explain --text`.

Commit message:

`Add intent draft clarifications`

## Phase 5: `intent draft` CLI

Expose the adapter as a standalone command.

Tasks:

1. Add `intent draft` subcommand.
2. Support:
   - `--text`;
   - `--file`;
   - `--output`;
   - `--acceptance`;
   - `--strict`;
   - `--format json`.
3. Validate mutually exclusive `--text` and `--file`.
4. Write deterministic artifacts when output is provided:
   - `intent-source.txt`;
   - `intent-draft.json`;
   - `intent-extraction.json`;
   - `intent-clarifications.json`.
5. Print a compact JSON result to stdout.
6. Add CLI golden tests.

Acceptance:

- `kicadai intent draft --text "3.3V I2C temperature sensor breakout"` returns
  a valid structured draft.
- Output artifacts parse as JSON where applicable.
- Strict mode exits non-zero for blocking clarifications.

Commit message:

`Add intent draft CLI`

## Phase 6: Text Input For `intent explain`

Wire drafted requests into existing explanation behavior.

Tasks:

1. Add `--text` and `--file` support to `intent explain`.
2. Draft the request before planning.
3. If blocking clarifications exist, return explanation-style output with
   clarification details and do not call the planner.
4. If safe, run existing planner explanation.
5. Include extraction confidence and clarification count in the output.
6. Add CLI tests for:
   - successful text explanation;
   - blocking text explanation;
   - existing JSON-file behavior unchanged.

Acceptance:

- `intent explain --text ...` gives AI-readable reasons for selected
  requirements and clarifications.
- Unsafe prose never reaches design generation through explain/create paths.

Commit message:

`Support prose intent explanations`

## Phase 7: Text Input For `intent create`

Allow safe text-driven project generation through the existing workflow.

Tasks:

1. Add `--text` and `--file` support to `intent create`.
2. Fail closed on blocking draft clarifications.
3. Reuse existing planner and design workflow once drafting succeeds.
4. Persist draft/source artifacts under `.kicadai/` in generated projects.
5. Make generated manifests or summaries reference the draft source hash where
   appropriate.
6. Add tests for:
   - successful generated project from text;
   - refused ambiguous request;
   - artifact persistence;
   - JSON create path unchanged.

Acceptance:

- Safe text requests can generate the same class of projects as structured
  requests.
- Unsafe text requests leave no partial project unless existing create behavior
  already allows partial artifacts for blocked plans.

Commit message:

`Create designs from prose intent`

## Phase 8: Golden Fixtures And Documentation

Add examples and user-facing documentation for the new adapter.

Tasks:

1. Add `examples/intent_text/` fixtures:
   - I2C temperature sensor breakout;
   - ATmega minimal board with ISP and 16 MHz clock;
   - USB-C to 3.3V regulator module;
   - ambiguous battery sensor;
   - unsupported/unsafe headphone amplifier request.
2. Add golden expected draft/explain snippets.
3. Update `README.md`:
   - CLI examples using `kicadai`;
   - supported language coverage;
   - clarification behavior;
   - limitations.
4. Update `specs/ROADMAP.md` to mark natural-language intake foundation as
   implemented and list remaining semantic synthesis gaps.
5. Add artifact examples to docs.

Acceptance:

- Documentation matches the compiled binary command style.
- Golden fixtures demonstrate both success and fail-closed behavior.
- Roadmap status is current.

Commit message:

`Document natural language intent drafting`

## Phase 9: Review And Compatibility Sweep

Harden edge cases before considering the project complete.

Tasks:

1. Run `go test ./...`.
2. Run `prism review staged`.
3. Address high/medium findings.
4. Check that no command documentation uses `go run ./cmd/kicadai`.
5. Verify JSON-file intent fixtures and current examples still pass.
6. Confirm no network or KiCad CLI dependency was added to normal tests.

Acceptance:

- Full tests pass.
- Prism has no unresolved high/medium findings.
- Working tree is committed phase by phase.

Commit message:

`Harden natural language intent adapter`

