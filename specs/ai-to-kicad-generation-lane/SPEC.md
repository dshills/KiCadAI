# AI-to-KiCad Generation Lane Specification

Date: 2026-07-12

## 1. Purpose

KiCadAI already has deterministic layers for structured intent planning,
component and block selection, schematic generation, PCB realization, placement,
routing, validation, repair evidence, writer correctness, round-trip checking,
and KiCad-backed promotion. It also has a deterministic phrase extractor for a
small natural-language subset. What it does not have is a production boundary
where an AI provider can translate an arbitrary user request into the strict
structured intent contract and then hand control to those deterministic layers.

This milestone adds that boundary and proves it with one reference request:

> Create a protected USB-C powered BMP280 I2C breakout with a 3.3 V regulator,
> pull-ups, decoupling, and an external I2C connector.

The result must be a complete KiCad project accompanied by machine-readable
evidence that the design passed every required schematic, PCB, writer, and
KiCad validation gate.

## 2. Design Stance

The model is an interpreter, not the electrical or file-format authority.

1. The provider converts natural language into `intentplanner.Request` JSON.
2. KiCadAI strictly decodes, validates, and normalizes that JSON.
3. The deterministic intent planner selects supported blocks and verified
   components and creates a `designworkflow.Request`.
4. The existing design workflow generates and validates the project.
5. Bounded repair may retry only explicitly classified deterministic repairs.

The model must never emit KiCad S-expressions, transactions, route geometry, or
validation results. Provider output is untrusted input even when constrained by
provider-side Structured Outputs.

## 3. Goals

- Add a provider-neutral Go interface for natural-language intent generation.
- Add an OpenAI Responses API provider using strict Structured Outputs.
- Add a deterministic recorded provider for offline tests and reproducibility.
- Strictly decode and validate provider output before project writes.
- Expose one prompt-driven `kicadai design create` command.
- Preserve provider request/response metadata without persisting credentials.
- Route validated intent through the existing planner and workflow.
- Make automatic retries bounded, classified, and observable.
- Prove the reference request as a reproducible KiCad-backed pass.
- Preserve all existing pass fixtures and the default offline test suite.

## 4. Non-Goals

- No conversational UI or MCP server.
- No direct AI generation of KiCad syntax or transaction operations.
- No general arbitrary-circuit synthesis guarantee.
- No new unrelated block or component families.
- No online provider call in default tests.
- No self-modifying prompt loop or unbounded model retries.
- No suppression of ERC, DRC, connectivity, or writer findings.
- No claim of fabrication readiness beyond evidence actually produced.

## 5. Current Baseline and Gaps

### 5.1 Existing reusable systems

- `internal/intentdraft` performs deterministic phrase extraction.
- `internal/intentplanner` strictly decodes intent and generates a design
  request with planning evidence.
- `internal/schematicir` validates and normalizes schematic design/layout IR.
- `internal/designworkflow` owns project generation and validation stages.
- `internal/repair` classifies and applies bounded deterministic repairs.
- `cmd/kicadai` exposes intent and design workflows and writes AI-lane status.
- Optional KiCad-backed fixtures prove BMP280, protected USB-C LED, and the
  combined protected USB-C/I2C/3.3 V design independently.

### 5.2 Demonstrated prompt gap

The current deterministic draft of the reference prompt loses or misstates
material intent:

- classifies the design as `power_module`;
- does not select the BMP280 component identity;
- assigns 3.3 V to the USB-C input instead of distinguishing 5 V input and
  regulated 3.3 V output;
- does not represent fuse, TVS, or bulk-capacitor requirements;
- does not capture explicit pull-up, decoupling, or external connector intent.

### 5.3 Missing integration surfaces

- no AI provider abstraction;
- no strict provider response envelope;
- no provider-side JSON Schema;
- no `--prompt`/`--provider` design command;
- no persisted provider evidence;
- no recorded-provider parity test;
- no live-provider acceptance test;
- no orchestration-level retry budget for provider correction versus
  deterministic design repair;
- insufficient intent fields for protected USB-C input requirements.

## 6. Architecture

```text
natural-language prompt
  -> IntentProvider.Generate
  -> strict provider envelope decode
  -> strict intentplanner.Request decode
  -> semantic validation and normalization
  -> intentplanner.Plan
  -> designworkflow.Create
  -> bounded deterministic repair
  -> promotion/evidence artifacts
  -> pass | candidate | fail
```

The provider package must not import CLI code or KiCad writers. The CLI composes
provider generation with the existing planner and workflow.

## 7. Provider Contract

### 7.1 Interface

The internal contract should be equivalent to:

```go
type Provider interface {
    Name() string
    GenerateIntent(context.Context, GenerateRequest) (GenerateResult, error)
}

type GenerateRequest struct {
    Prompt        string
    SchemaVersion string
    Attempt       int
    Diagnostics   []Diagnostic
}

type GenerateResult struct {
    Provider       string
    Model          string
    ResponseID     string
    IntentJSON     json.RawMessage // extracted inner intent object, not envelope
    Usage          Usage
    FinishReason   string
    Recorded       bool
}

type Diagnostic struct {
    Code    string `json:"code"`
    Path    string `json:"path,omitempty"`
    Message string `json:"message"`
}

type Usage struct {
    InputTokens  int `json:"input_tokens,omitempty"`
    OutputTokens int `json:"output_tokens,omitempty"`
    TotalTokens  int `json:"total_tokens,omitempty"`
}
```

Provider errors must distinguish configuration, transport, authentication,
rate-limit, timeout, refusal, incomplete response, malformed response, and
schema failure. Errors must never include credentials or raw authorization
headers.

Envelope decoding belongs to each provider implementation because wire response
shapes are provider-specific. A provider returns only the extracted inner intent
JSON plus normalized metadata. The shared post-provider orchestrator then
strictly decodes and validates that inner JSON as `intentplanner.Request`.

### 7.2 Providers

- `openai`: real OpenAI Responses API integration.
- `recorded`: checked-in deterministic response selected by a stable fixture
  name or explicit record path.

Unknown providers fail before output directories are created. No silent
fallback from `openai` to `recorded` is allowed.

### 7.3 OpenAI API behavior

The OpenAI provider must:

- read `OPENAI_API_KEY` from the environment;
- use the hard-coded official HTTPS Responses API endpoint in production;
- expose no CLI or environment base-URL override in v1; HTTP contract tests may
  inject a package-private endpoint/client with a dummy key so real credentials
  can never be redirected;
- use the Responses API;
- send the user request as data, with fixed developer instructions defining the
  supported intent contract;
- request strict `json_schema` output through `text.format`;
- set a finite HTTP timeout and output token limit;
- use stateless streaming by default so long generation does not depend on a
  response-header timeout;
- support explicit background polling for constrained network paths through
  `KICADAI_AI_BACKGROUND=true`; this sets `background:true` and `store:true`,
  and documentation must disclose that OpenAI temporarily stores the response
  to make retrieval possible;
- reject incomplete, refused, empty, multi-payload, or non-message output;
- inspect response content items for an OpenAI `refusal` item/string before
  attempting to decode structured output and classify it as a provider refusal;
- extract exactly one `output_text` payload;
- read the HTTP response through `io.LimitReader` with a hard 2 MiB body limit
  and reject a body that exceeds it;
- never log or persist the API key;
- default to a documented model that supports Structured Outputs, while
  allowing `--model` or `KICADAI_AI_MODEL` override.

## 8. Structured Output Contract

### 8.1 Provider envelope

The model output is one object:

```json
{
  "schema": "kicadai.ai.intent.v1",
  "intent": {
    "version": "0.1.0",
    "name": "usb_c_bmp280_breakout",
    "summary": "Protected USB-C powered BMP280 I2C breakout",
    "kind": "breakout",
    "acceptance": "erc-drc",
    "auto_schematic_layout": true,
    "board": {"layers": 2},
    "power": {
      "inputs": [{"kind": "usb_c", "voltage": "5V", "strength": "required"}],
      "rails": [{"name": "VCC", "voltage": "3.3V", "strength": "required"}]
    },
    "interfaces": [{"kind": "i2c", "voltage": "3.3V", "connector": "external", "strength": "required"}],
    "functions": [{
      "kind": "sensor",
      "family": "i2c_sensor",
      "strength": "required",
      "params": {
        "sensor_component_id": "sensor.bosch.bmp280.lga8",
        "i2c_address": "0x76",
        "supply_voltage": "3.3V",
        "include_pullups": true,
        "include_decoupling": true
      }
    }],
    "protection": {
      "overcurrent": "required",
      "transient": "required",
      "bulk_capacitance": "required"
    }
  }
}
```

### 8.2 Strictness

- Provider-side JSON Schema sets `additionalProperties: false` recursively.
- Every object property required by Structured Outputs is listed in `required`;
  optional semantics use nullable values or zero/empty values where necessary.
- KiCadAI then applies `json.Decoder.DisallowUnknownFields` independently.
- The envelope schema version must match exactly.
- The embedded intent version must be supported.
- Trailing JSON tokens are rejected.
- Provider output size is bounded.
- Validation issues use stable paths rooted at `provider.intent`.

Provider-side schema adherence is defense in depth, not a replacement for Go
validation.

All strength fields use the existing `intentplanner.Strength` enum and accept
exactly `required`, `preferred`, `optional`, or `forbidden`. Empty values are
accepted only where the existing normalization contract supplies a documented
default. Provider Structured Outputs should emit the explicit value.

Voltage strings use the existing intent-planner voltage parser. Provider output
must use `<decimal>V` form such as `5V`, `5.0V`, or `3.3V`. The defensive Go
decoder accepts lowercase `v` and whitespace around the complete value, then
normalizes it before semantic validation; the provider schema still requests a
canonical uppercase `V`. Values with other units, ranges, or unparseable text
fail closed.

The `text.format` shape is specifically the Responses API Structured Outputs
contract. `response_format` is the corresponding Chat Completions shape and is
not used by this provider. The implementation follows the official Responses
API Structured Outputs guide at
`https://developers.openai.com/api/docs/guides/structured-outputs` and locks the
wire shape in HTTP contract tests.

The schema implementation must have one canonical builder and parity tests that
compare its properties, required fields, and nested object strictness against
the Go intent model. A Go model change that is not reflected in the provider
schema must fail tests rather than drift into runtime provider errors.

Exact envelope-version matching is intentional in v1. Recorded responses are
execution inputs, not archival documents; accepting a nominally compatible but
unverified envelope would weaken the fail-closed boundary. A future migration
must add an explicit decoder for each accepted version. The embedded
`intent.version` follows the same existing `intentplanner` policy: v1 accepts
exactly `0.1.0`; a new intent version requires an explicit decoder/migration and
fixture update rather than automatic minor-version acceptance.

## 9. Intent Extensions for the Reference Design

`ProtectionIntent` must add only semantics required by this fixture:

- `overcurrent`: fuse or equivalent input overcurrent protection;
- `transient`: TVS/transient clamp requirement;
- `bulk_capacitance`: protected input bulk capacitance requirement.

The planner maps these strengths into existing `usb_c_power` parameters:

- required/preferred overcurrent -> `include_fuse`;
- required/preferred transient -> `include_tvs`;
- required/preferred bulk capacitance -> `include_bulk_capacitor`.

Optional and forbidden map to false in v1; optional means the fixture does not
require inclusion and no heuristic is introduced in this milestone. A required
feature that the selected block cannot provide is a planning blocker. This
milestone does not create a generalized protection taxonomy beyond these
existing capabilities.

The BMP280 identity must remain a concrete catalog ID in sensor function
parameters. The planner and component pipeline remain responsible for checking
that the ID, pinmap, symbol, and LGA-8 footprint are verified.

## 10. CLI Contract

The primary command is:

```sh
kicadai design create \
  --prompt "Create a protected USB-C powered BMP280 I2C breakout with a 3.3 V regulator, pull-ups, decoupling, and an external I2C connector" \
  --provider openai \
  --output ./out/bmp280-breakout \
  --require-erc \
  --require-drc \
  --require-kicad-roundtrip
```

Global flags must remain before the command under the current CLI parser. The
implementation may also accept `intent create --prompt` as a compatibility
alias, but documentation uses `design create`.

New options:

- `--prompt`: natural-language request;
- `--prompt-file`: UTF-8 natural-language request file, avoiding shell history
  and process-list exposure for sensitive prompts;
- `--provider`: `openai` or `recorded`;
- `--model`: provider model override;
- `--provider-record`: recorded-provider fixture path or ID;
- `--max-ai-attempts`: total provider generations, default 1, hard maximum 2.

`--prompt` and `--prompt-file` are mutually exclusive with each other and with
`--request`, `--text`, and `--file`. `--provider` is required with either
provider prompt source; provider options without one are invalid. The output
directory is not created until provider output and the intent plan are free of
blockers.

The current CLI uses a shared standard-library `flag.FlagSet`, so these
cross-option rules must be checked explicitly before provider construction; flag
registration alone is not sufficient.

## 11. Prompt and Trust Boundary

The fixed developer instruction must state:

- produce only the requested structured intent;
- use only enumerated supported intent kinds and fields;
- never invent KiCad symbols, footprints, component IDs, block IDs, pins, nets,
  route geometry, or validation evidence;
- use the verified component IDs supplied in bounded catalog context;
- represent ambiguity as a structured diagnostic rather than guessing.

For v1, provider context includes a small deterministic capability summary for
the reference design, including the verified BMP280 component ID and supported
protection semantics. It must not include the entire library corpus.

Prompt text is treated as untrusted data. Instructions embedded in the user's
request cannot alter the output schema, system policy, or validation gates.

Supported enum values come from the current Go source of truth:

- intent kinds: `breakout`, `power_module`, `sensor_node`, `mcu_minimal`,
  `amplifier`, and `custom_structured` from `internal/intentplanner/model.go`;
- this reference fixture uses function kind `sensor` and family `i2c_sensor`;
- the provider capability context exposes only the narrower values needed by
  this milestone even though the deterministic planner supports more.

## 12. Validation and Fail-Closed Behavior

Validation occurs before project writes in this order:

1. CLI option validation.
2. Provider configuration validation.
3. Provider response status/refusal/incomplete validation and provider-owned
   envelope strict decode.
4. Shared inner-intent strict decode and schema version validation.
5. Intent semantic validation and normalization.
6. Planner blocker/clarification validation.
7. Generated design-request validation.
8. Existing generation and evidence gates.

Unknown components, component IDs, interfaces, protection semantics, pins,
nets, footprints, topology, voltage relationships, or constraint values block
generation. Provider claims cannot mark a component verified or a gate passed.

## 13. Bounded Repair

There are two distinct retry classes.

### 13.1 Provider correction

- Default: one provider attempt.
- Maximum: two attempts.
- A second attempt is permitted only for malformed envelope, strict schema
  failure, or a planner diagnostic explicitly classified as AI-correctable.
- The retry includes structured diagnostic codes and paths, not arbitrary logs.
- At most eight diagnostics are sent, ordered by blocking severity, stable code,
  and path; individual messages are length-bounded. Additional findings remain
  in local evidence but do not expand provider context.
- Authentication, rate limit, tool, unsupported topology, and validation-gate
  failures are not automatically resubmitted to the provider.

### 13.2 Deterministic design repair

Existing placement/routing/validation repair remains authoritative. The AI does
not propose route coordinates or edit generated KiCad files. Existing repair
budgets and regression checks remain unchanged.

Each attempt is recorded with index, provider, model, response ID, status,
diagnostic codes, and input/output hashes. Raw provider reasoning is never
requested or stored.

The initial AI-correctable code allowlist is intentionally small:

- `ai_output_json_invalid`;
- `ai_output_schema_invalid`;
- `ai_intent_field_invalid` for a supported field with an invalid enum, unit,
  range, or missing required value;
- `ai_intent_component_unknown` when the supplied capability context contains a
  verified alternative and the model selected a different identifier.

An `ai_intent_component_unknown` diagnostic includes a bounded list of verified
component IDs already present in the original capability context. Retry input
never adds unverified catalog candidates or asks the model to guess another ID.

Unknown topology, unsupported functions/interfaces, authentication, transport,
rate limiting, refusal, incomplete output, and any post-plan generation or
validation finding are not AI-correctable in v1. Retry eligibility is an
explicit code allowlist, never inferred from message text or severity alone.

## 14. Evidence and Artifacts

Successful prompt runs write provider artifacts after validation establishes a
safe output workspace. Provider configuration, transport, refusal, malformed
response, strict-decode, and pre-plan failures return structured stdout
diagnostics and create no output directory; they never persist raw failed
responses. This preserves the fail-before-project-writes guarantee.

- `.kicadai/ai-request.json`: sanitized provider/model/schema/attempt metadata
  and prompt hash;
- `.kicadai/ai-response.json`: response metadata, intent payload, usage, and
  hashes; no credentials or hidden reasoning;
- `.kicadai/ai-attempts.json`: bounded-attempt history;
- `.kicadai/intent-plan.json`;
- `.kicadai/generated-request.json`;
- existing transaction, manifest, workflow, promotion, ERC, DRC, writer,
  route-completion, electrical, and round-trip artifacts.

The normalized structured intent is the reproducibility boundary. A recorded
provider can replay it through the identical post-provider pipeline.

Evidence models are purpose-built value objects. They must never embed or
marshal `http.Request`, `http.Response`, headers, transports, or provider client
configuration. `Authorization`, API keys, organization/project identifiers,
and any provider-specific credential-bearing metadata are excluded rather than
redacted after serialization. No HTTP headers are persisted, including response
headers such as `Set-Cookie`; evidence fields use an explicit allowlist of
non-HTTP scalar values.

The prompt source may contain sensitive design information. Provider metadata
artifacts contain only its hash. The provider-backed `design create --prompt`
lane must not write `intent-source.txt` or any other plaintext prompt artifact;
its normalized intent is the reproducibility boundary. Existing deterministic
`intent create --text` behavior remains outside this provider lane. KiCadAI does
not attempt unreliable PII detection.

## 15. Promotion Classification

Provider success alone never advances readiness. `pass` requires all current
promotion gates:

- strict intent and generated-request validation;
- schematic electrical validation;
- readable schematic evidence;
- internal PCB connectivity;
- required-net route completion;
- writer correctness;
- clean KiCad ERC;
- clean strict-severity KiCad DRC;
- zero unexpected normalized schematic and PCB round-trip differences;
- verified BMP280 identity, pinmap, symbol, footprint, and pad-net evidence;
- required USB-C protection and regulator evidence.

Missing KiCad tooling remains environment-gated in default tests and cannot be
reported as a real pass.

## 16. Testing Strategy

### 16.1 Unit tests

- provider registry and option validation;
- strict envelope decode and size limits;
- malformed, refused, incomplete, and multi-output response handling;
- OpenAI request shape and response parsing with `httptest.Server`;
- authorization redaction and error classification;
- recorded provider determinism;
- protection-intent validation and mapping;
- retry eligibility and hard attempt limits.

### 16.2 Integration and CLI tests

- prompt -> recorded provider -> strict intent -> plan -> design workflow;
- malformed provider output writes no project files;
- provider selection and mutually exclusive flags;
- deterministic normalized intent and generated request;
- expected provider evidence artifacts;
- exact reference prompt golden result.

### 16.3 Optional live and KiCad-backed tests

- live OpenAI provider smoke test is opt-in through an environment variable;
- KiCad-backed reference test is opt-in and requires an explicit KiCad CLI;
- the recorded response drives the same downstream workflow used by live runs;
- existing BMP280, protected USB-C LED, and combined fixture pass tests run as
  regressions before completion.

## 17. Security, Privacy, and Reliability

- Never persist or print `OPENAI_API_KEY`.
- Default OpenAI requests use `store:false`; opt-in background mode requires
  temporary provider-side storage and is unsuitable for Zero Data Retention
  requirements.
- Do not include local file contents beyond the bounded capability context.
- Do not send generated KiCad files to the provider.
- Use context cancellation, finite HTTP timeouts, response-size limits, and
  bounded attempts.
- Sanitize provider errors before reports.
- Persist prompt content only where current intent-source behavior already does;
  otherwise prefer hash plus an explicit local artifact.
- Keep default tests hermetic and network-free.

## 18. Documentation Requirements

Documentation must include:

- provider setup and environment variables;
- one-command real-provider example;
- recorded/offline example;
- exact output and evidence paths;
- fail-closed behavior and retry limits;
- supported reference topology;
- distinction between electrical pass and fabrication readiness;
- troubleshooting for credentials, rate limits, provider schema errors, KiCad
  CLI availability, and failed validation gates.

## 19. Completion Criteria

The milestone is complete only when:

1. A real configured provider converts the reference prompt into valid intent.
2. The recorded provider exercises the identical downstream path offline.
3. Malformed or unsafe provider output fails before project writes.
4. The reference design is a reproducible KiCad-backed pass.
5. All required evidence artifacts are present and internally consistent.
6. Repeated recorded runs produce equivalent normalized intent and design
   requests.
7. Existing pass fixtures remain pass.
8. `go test ./...` and lint pass.
9. Prism has reviewed staged implementation changes before each commit.
10. The worktree is clean.

The real-provider check is a manual/opt-in milestone acceptance run because it
depends on network and provider availability. Hermetic CI proves the same
post-provider path with the recorded provider; a temporary live-provider outage
must not make the default repository test suite nondeterministic.

## 20. Explicit v1 Limitations

- The AI-facing schema supports the existing intent planner, not arbitrary
  circuit netlists.
- The proven live prompt is limited to the reference BMP280 topology.
- Provider output may still require user clarification for unsupported or
  ambiguous requests.
- Recorded-provider reproducibility proves the deterministic post-provider lane;
  it does not guarantee identical output from a changing live model.
- Fabrication release still requires the project's fabrication-specific gates
  and any documented human review not covered by `erc-drc` acceptance.

## 21. Two-Profile Provider Extension

The next bounded extension adds one second proven profile: a protected
USB-C-powered LED indicator. It reuses block instances `usb_power` and
`indicator`, whose existing variants are `usb_c_power` and `led_indicator`,
respectively. It does not introduce a general electronics schema.

### 21.1 Reference profiles

The provider boundary exposes an immutable profile descriptor containing:

- stable profile ID and Structured Output schema name;
- bounded capability context;
- a fresh strict JSON Schema for the profile;
- fixture/evidence identity used only after semantic selection.

The initial profile registry contains exactly:

- protected USB-C BMP280 I2C breakout;
- protected USB-C LED indicator.

### 21.2 Deterministic selection

Selection occurs before any provider request and examines normalized prompt
semantics only. It must not inspect output paths, project names, fixture paths,
recorded-response filenames, or model output. The bounded classifier recognizes
the required USB-C context plus one functional anchor (`BMP280` or `LED
indicator`). Prompts matching neither profile are rejected as unsupported.
Prompts matching both are rejected as an unsupported composition. No provider
call or output directory creation occurs after selection failure.

Normalization applies Unicode lowercase mapping, treats characters outside the
Unicode letter and number categories as token separators, and collapses
whitespace. USB-C context matches the adjacent token sequence `usb`, `c` or the
single token `usbc`. The BMP280 anchor is the
token `bmp280`. The LED anchor is the token `led` or plural `leds`. A profile
requires USB-C context and exactly its own
anchor; synonyms and implicit topology inference are intentionally unsupported
in this bounded phase.

The checked-in BMP280 reference prompt already contains explicit `USB-C`
context, so this requirement preserves the existing promoted provider command
and tests. Compatibility does not extend to previously unpromoted BMP280-only
phrasing that omitted the required power topology.

Rejecting a prompt that combines both anchors is an explicit limitation of this
two-profile milestone. Multi-profile circuit composition requires a separate
reviewed contract and is not inferred here.

This software rejection behavior is unrelated to physical protection behavior:
the generated board's fuse must open the overcurrent path rather than conduct
through a fault.

This classifier is a safety dispatch layer, not a natural-language circuit
parser. Adding another topology requires another reviewed profile and promotion
fixture.

### 21.3 LED profile contract

The strict LED profile fixes the proven deterministic design requirements:

- 50 mm by 30 mm, two-layer board;
- 5 V USB-C power-only input, including the `usb_c_power` variant's mandatory
  5.1 kohm CC1 and CC2 pull-downs;
- required fuse, TVS, and bulk capacitance enabled as subcomponents of the
  `usb_power` block instance;
- reverse-polarity protection remains disabled to match the promoted fixture;
- one active-high indicator function;
- 5 mA LED current and 2.0 V forward-voltage assumption for the promoted
  indicator LED; color-specific LED selection is outside this profile;
- a pinned 600 ohm current-limiting resistor result, derived from
  `(5.0 V - 2.0 V) / 0.005 A`; standard-series part selection is outside this
  fixture-equivalence milestone and must map the value to an orderable part
  before fabrication release;
- automatic readable schematic layout;
- ERC/DRC acceptance.

The generated critical projection must contain only the `usb_power` and
`indicator` block instances. Fuse, TVS, bulk capacitor, CC pull-downs, and LED
resistor are verified through their containing block's strict parameters and
generated component evidence. GND and `VCC_5v` connections must match the
checked-in deterministic fixture. Equivalence is a typed exact comparison of board width,
height, and layer count; block kind, variant, parameters, and enabled optional
components; connection net names and endpoints; layout mode; and acceptance
policy. Project name, summary, provider prose, and diagnostic metadata are
excluded. `VCC_5v` and GND are explicit schema outputs and the existing promoted
fixture's deterministic canonical net names; anonymous generated net names are
not compared as strings. Their endpoint grouping is still validated inside the
verified block projection and generated electrical evidence. Explicit
inter-block connections are canonicalized by their schema-pinned well-known net
name (`GND` or `VCC_5v`) and sorted `(block instance name, port name)` endpoint
sets. Internal anonymous nets are canonicalized only by their sorted endpoint
sets. Their source ordering is not electrically meaningful. Pinned numeric inputs are schema-constrained and normalized before
comparison, then compared exactly rather than accepting arbitrary model drift.

### 21.4 Validation and promotion

Recorded and live responses use the same selected profile and post-provider
pipeline. Default tests validate both profile fixtures without credentials or
network access. Optional tests require explicit OpenAI and KiCad environments.
`pass` still requires real ERC, DRC, connectivity, route completion, writer
correctness, and strict round-trip evidence. The existing BMP280 provider lane
and deterministic protected USB-C LED fixture remain regression gates.

### 21.5 Extension completion criteria

1. Semantic profile selection is deterministic and fail-closed.
2. Unsupported and ambiguous prompts make no provider request and write no
   project.
3. Recorded LED generation reaches AI `ready` and promotion `pass`.
4. Live and recorded LED critical projections are semantically equivalent.
5. Live LED generation reaches AI `ready` and promotion `pass`.
6. Existing BMP280 live/recorded and protected LED deterministic gates remain
   clean.
7. Default tests, lint, Prism review, documentation, and worktree cleanliness
   meet the original milestone requirements.
