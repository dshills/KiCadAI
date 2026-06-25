# Intent Planner And AI Orchestration Implementation Plan

## Phase 1: Intent Model And Strict Decode

### Objective

Add the structured intent schema and validation model without changing existing
`design create` behavior.

### Implementation Steps

1. Add an intent planner package:
   - preferred path: `internal/intentplanner`;
   - keep CLI-specific code in `cmd/kicadai`.
2. Define request types:
   - schema version;
   - top-level intent request;
   - board intent;
   - power input and rail intent;
   - interface intent;
   - function intent;
   - protection intent;
   - manufacturing intent;
   - constraint intent;
   - requirement strength enum.
3. Implement strict JSON decode:
   - size limit;
   - `DisallowUnknownFields`;
   - trailing token rejection;
   - stable issue paths.
4. Implement normalization:
   - project name;
   - default board layers;
   - default acceptance;
   - default requirement strengths;
   - sorted collections where applicable.
5. Implement validation:
   - required name or safe name fallback;
   - supported kind;
   - positive board dimensions when provided;
   - voltage/current parseability where required;
   - supported interface/function/protection kinds;
   - requirement strength validity.
6. Add tests for:
   - valid minimal request;
   - unknown field rejection;
   - invalid dimensions;
   - unsupported kind;
   - invalid strength;
   - deterministic normalization.

### Review Checklist

- Intent decode does not silently accept misspelled fields.
- Validation issues use AI-editable input paths.
- No CLI or workflow behavior changes yet.

### Suggested Commit

`Add intent planner request model`

## Phase 2: Plan Result Model And Deterministic Artifacts

### Objective

Create the plan output contract and artifact writer before adding block mapping.

### Implementation Steps

1. Define plan result types:
   - schema constant `kicadai.intent.plan.v1`;
   - status enum;
   - score;
   - normalized intent summary;
   - generated request pointer;
   - requirement records;
   - selected block records;
   - component policy records;
   - assumptions;
   - clarifications;
   - known gaps;
   - issues;
   - artifacts.
2. Implement status calculation:
   - blocking issues -> `blocked`;
   - clarification issues -> `needs_clarification`;
   - warnings/omitted preferred requirements -> `partial`;
   - no issues -> `ready`.
3. Implement deterministic JSON marshal:
   - sorted issues;
   - sorted selected blocks;
   - sorted assumptions/clarifications/gaps;
   - stable empty array behavior where useful.
4. Add artifact writing helpers:
   - `intent-plan.json`;
   - `generated-request.json` when available;
   - inside-output path checks;
   - overwrite policy.
5. Add tests for:
   - stable JSON snapshots;
   - status calculation;
   - artifact path normalization;
   - overwrite blocking;
   - no generated request when blocked.

### Review Checklist

- The plan can be inspected independently of KiCad project generation.
- Artifacts are deterministic and project-relative.
- Output writing is explicit and safe.

### Suggested Commit

`Add intent plan result artifacts`

## Phase 3: Block Mapping Rules

### Objective

Translate supported intent functions, interfaces, power, and protection
requirements into `designworkflow.BlockInstanceSpec` and connection specs.

### Implementation Steps

1. Add a rule-based planner:
   - input: normalized intent request;
   - output: plan result plus generated `designworkflow.Request`.
2. Implement supported mappings:
   - indicator -> `led_indicator`;
   - connector/gpio/power breakout -> `connector_breakout`;
   - USB-C input -> `usb_c_power`;
   - rail conversion -> `voltage_regulator`;
   - I2C sensor -> `i2c_sensor`;
   - MCU -> `mcu_minimal`;
   - clock option -> `crystal_oscillator` or `canned_oscillator`;
   - reset/programming -> `reset_programming`;
   - amplifier -> `opamp_gain_stage`;
   - ESD preference -> `esd_protection`;
   - reverse-polarity preference -> `reverse_polarity_protection`.
3. Generate stable block instance IDs:
   - derive from requirement kind and ordinal;
   - preserve user IDs when valid;
   - avoid collisions deterministically.
4. Generate basic connection specs:
   - power input to regulator;
   - regulator rail to downstream blocks;
   - I2C bus to connector/sensor/MCU where modeled;
   - protection blocks at edge-facing endpoints where supported.
5. Record selected block rationale:
   - satisfied requirement IDs;
   - parameters;
   - readiness and known gaps from block metadata;
   - omitted preferred requirement reasons.
6. Add tests for each supported mapping and at least:
   - unsupported function blocks;
   - required protection unsupported by topology;
   - stable IDs under reordered input.

### Review Checklist

- No unsupported topology is guessed silently.
- Generated block IDs are stable and readable.
- Required features block when unmappable.

### Suggested Commit

`Map intent requirements to design blocks`

## Phase 4: Component Policy And Constraint Derivation

### Objective

Convert intent-level constraints into component policy, board constraints,
validation settings, and routing retry policy.

### Implementation Steps

1. Map acceptance:
   - default to structural;
   - connectivity and higher require stricter component acceptance;
   - ERC/DRC and fabrication candidate set validation expectations.
2. Derive component policy:
   - minimum confidence;
   - placeholder allowance;
   - package preferences;
   - required ratings from voltage/current/tolerance fields;
   - fabrication-candidate preference for concrete MPN-backed parts.
3. Derive board and manufacturing settings:
   - width/height/layers;
   - edge clearance;
   - mounting-hole preference where supported;
   - manufacturer profile.
4. Derive validation settings:
   - `StrictUnrouted`;
   - `StrictZones`;
   - `RequireERC`;
   - `RequireDRC`;
   - `SkipRouting` for draft/structural requests when requested.
5. Derive routing retry policy:
   - disabled by default;
   - opt-in max attempts;
   - optional DRC policy.
6. Add tests for:
   - draft vs connectivity component policy;
   - fabrication-candidate policy;
   - package preference propagation;
   - rating propagation;
   - validation flag derivation.

### Review Checklist

- Higher acceptance cannot quietly allow unsafe placeholders.
- Intent constraints become visible in generated request JSON.
- Manufacturing profile is carried as far as the current workflow supports.

### Suggested Commit

`Derive design policies from intent`

## Phase 5: CLI Plan And Explain Commands

### Objective

Expose planning without writing KiCad project files.

### Implementation Steps

1. Add `intent` command family to `cmd/kicadai`.
2. Add `intent plan`:
   - requires `--json`;
   - reads `--request`;
   - optional `--output`;
   - optional `--overwrite`;
   - returns `reports.Result`.
3. Add `intent explain`:
   - same planner path;
   - no writes unless output is provided;
   - includes rationale, assumptions, clarifications, and gaps.
4. Wire artifact writing for `--output`.
5. Add CLI help text.
6. Add tests:
   - successful JSON plan;
   - blocked unsupported intent;
   - artifact writing;
   - overwrite protection;
   - command requires `--json`.

### Review Checklist

- Planning is usable before generation.
- CLI output carries issues and artifacts consistently with other commands.
- Existing commands remain unchanged.

### Suggested Commit

`Expose intent plan CLI`

## Phase 6: Intent Create Orchestration

### Objective

Run `design create` from a validated planner output and return combined evidence.

### Implementation Steps

1. Add `intent create` CLI command.
2. Implement orchestration:
   - decode and plan intent;
   - stop if plan status is blocked or needs clarification;
   - pass generated request to `designworkflow.Create`;
   - merge planner and workflow artifacts;
   - merge issues without losing stage context.
3. Write project-local artifacts:
   - `.kicadai/intent-plan.json`;
   - `.kicadai/generated-request.json`.
4. Add result shape:
   - plan summary;
   - workflow result;
   - generated request artifact path;
   - acceptance status.
5. Add tests:
   - minimal LED intent creates a project;
   - unsupported intent stops before project write;
   - plan artifacts are written;
   - generated request validates through `designworkflow.ValidateRequest`;
   - downstream workflow blockers remain visible.

### Review Checklist

- No KiCad files are written after blocked planning.
- Generated request is reproducible and inspectable.
- Workflow acceptance is not overclaimed.

### Suggested Commit

`Add intent create orchestration`

## Phase 7: Golden Intent Fixtures

### Objective

Add representative intent fixtures that prove the planner can cover the seed
design families.

### Implementation Steps

1. Add fixtures under `internal/intentplanner/testdata` or
   `examples/intent`.
2. Include at least:
   - LED indicator;
   - connector breakout;
   - USB-C power regulator;
   - I2C sensor breakout;
   - MCU minimal with reset/programming;
   - op-amp gain stage;
   - protection-enabled edge connector.
3. Add golden plan snapshots:
   - selected blocks;
   - generated request summary;
   - assumptions/gaps;
   - issue counts.
4. Add CLI golden tests for at least one `intent plan` and one `intent create`
   fixture.
5. Keep fixtures hermetic and KiCad-independent by default.

### Review Checklist

- Fixtures demonstrate breadth without pretending unsupported variants work.
- Golden snapshots are stable and focused on meaningful fields.
- Test names make the supported intent surface obvious.

### Suggested Commit

`Add intent planner golden fixtures`

## Phase 8: Documentation And Roadmap Update

### Objective

Document the new planner surface and update the roadmap status.

### Implementation Steps

1. Update `README.md`:
   - explain `intent plan`, `intent explain`, and `intent create`;
   - show request examples;
   - explain deterministic planning versus natural-language interpretation;
   - document blockers, clarifications, assumptions, and generated request
     artifacts.
2. Update `specs/ROADMAP.md`:
   - move Priority 9 foundation forward;
   - keep natural-language LLM parsing and MCP explicitly future work;
   - note remaining supported-intent breadth gaps.
3. Add example request files under `examples/intent` if not done in Phase 7.
4. Run:
   - `gofmt` on changed Go files;
   - `go test ./...`;
   - Prism review on staged changes.

### Review Checklist

- Documentation does not imply arbitrary natural-language generation.
- Examples use `kicadai`, not `go run`.
- Roadmap reflects implemented and remaining work accurately.

### Suggested Commit

`Document intent planner workflow`

## Completion Criteria

The project is complete when:

- strict intent requests can be decoded, validated, normalized, and planned;
- supported seed intents produce deterministic `designworkflow.Request` values;
- unsupported or ambiguous intent returns actionable issues before project
  writes;
- `intent plan` and `intent explain` expose machine-readable rationale;
- `intent create` runs existing `design create` from the generated request;
- plan and generated-request artifacts are deterministic;
- representative golden fixtures cover the seed intent families;
- README and roadmap document the feature and limits;
- `go test ./...` passes;
- Prism review is clean or findings are addressed before commits.

