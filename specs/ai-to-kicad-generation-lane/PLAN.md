# AI-to-KiCad Generation Lane Implementation Plan

Date: 2026-07-12

## Phase 1: Baseline, Specification, and Acceptance Fixture

### Objective

Capture the current prompt failure, define the complete contract, and add a
checked-in reference prompt/expected intent without changing runtime behavior.

### Likely files

- `specs/ai-to-kicad-generation-lane/SPEC.md`
- `specs/ai-to-kicad-generation-lane/PLAN.md`
- `examples/ai/usb_c_bmp280_breakout/`
- focused fixture-loading tests

### Work

1. Record the deterministic phrase-adapter baseline.
2. Add the reference prompt and normalized expected intent fixture.
3. Define expected provider, planner, workflow, and promotion artifacts.
4. Confirm current repository tests and existing optional pass fixtures.

### Tests

- fixture JSON strict-decode test;
- `go test ./internal/intentplanner ./internal/intentdraft`;
- current optional pass fixture lane where available.

### Acceptance criteria

- The baseline demonstrates the missing BMP280/protection semantics.
- The reference fixture is strict, deterministic, and limited to this design.
- No production behavior changes.

### Rollback risk

Low. Documentation and test fixtures only.

### Suggested commit

`Specify AI-to-KiCad generation lane`

## Phase 2: Intent Protection Semantics

### Objective

Extend structured intent only enough to represent the protected USB-C input
already supported by `usb_c_power`.

### Likely files

- `internal/intentplanner/model.go`
- `internal/intentplanner/map.go`
- `internal/intentplanner/model_test.go`
- `internal/intentplanner/map_test.go`
- example intent fixtures

### Work

1. Add overcurrent, transient, and bulk-capacitance strengths.
2. Normalize and validate the fields with existing strength semantics.
3. Map them into existing USB-C block parameters.
4. Emit requirement/rationale evidence.
5. Preserve old intent defaults and generated requests.

### Tests

- valid required/preferred/optional/forbidden values;
- invalid strength paths;
- required protection mapping;
- forbidden protection mapping;
- unchanged existing USB-C intent behavior;
- deterministic planner output.

### Acceptance criteria

- The reference intent generates fuse, TVS, and bulk-capacitor parameters.
- No new block family is introduced.
- Existing planner fixtures remain stable unless intentionally updated.

### Rollback risk

Medium. Intent normalization and mapping are shared; golden changes must be
reviewed for genuine contract effects.

### Suggested commit

`Represent protected USB-C intent`

## Phase 3: Provider-Neutral Contract and Recorded Provider

### Objective

Add an isolated, hermetic provider boundary and prove strict output handling
without network access.

### Likely files

- `internal/aiprovider/model.go`
- `internal/aiprovider/decode.go`
- `internal/aiprovider/recorded.go`
- `internal/aiprovider/*_test.go`
- `examples/ai/usb_c_bmp280_breakout/recorded-response.json`

Checked-in files under `examples/ai` are immutable sanitized inputs and expected
results. Runtime projects are written to caller-selected output directories or
ignored `examples/.generated` workspaces and never overwrite these fixtures.

### Work

1. Define provider interface, request/result, usage, attempt, and error types.
2. Define the `kicadai.ai.intent.v1` envelope.
3. Implement bounded strict decode and embedded strict intent decode.
4. Implement recorded provider with safe project-relative or explicit test path.
5. Add stable hashing and sanitized evidence models.

### Tests

- valid recorded response;
- unknown envelope and intent fields;
- wrong versions and trailing tokens;
- oversize, empty, and malformed output;
- deterministic replay;
- no credentials in marshaled evidence.

### Acceptance criteria

- Recorded output yields one validated normalized intent.
- Invalid output cannot reach the planner.
- Package has no CLI, writer, or network dependency.

### Rollback risk

Low to medium. New isolated package.

### Suggested commit

`Add strict AI intent provider contract`

## Phase 4: OpenAI Responses Provider

### Objective

Implement a real provider using the Responses API and strict Structured
Outputs, behind the provider interface.

### Likely files

- `internal/aiprovider/openai.go`
- `internal/aiprovider/openai_schema.go`
- `internal/aiprovider/openai_test.go`
- provider documentation references

### Work

1. Build the strict provider JSON Schema.
   - use one canonical schema builder;
   - add reflection/conformance tests that fail when Go intent fields and schema
     properties or required fields drift.
2. Build a bounded developer instruction and capability context.
3. Implement HTTP request, authorization, timeout, and context cancellation.
4. Parse completed response output and usage.
   - use stateless streaming by default;
   - support explicitly configured background polling for environments with
     short response-header limits and document its provider-storage tradeoff.
5. Classify provider failures and redact sensitive data.
   - evidence types must not contain HTTP request/response objects, headers,
     organization/project identifiers, or provider client configuration;
   - persist no HTTP headers at all, including `Set-Cookie`, rather than trying
     to maintain a sensitive-header denylist;
   - read raw response bodies through a hard 2 MiB limit;
   - classify OpenAI refusal content before structured-output decoding.
6. Add model/environment configuration while keeping the production endpoint
   fixed to the official HTTPS Responses API; inject test endpoints only through
   package-private test construction with dummy credentials.

### Tests

- request JSON against `httptest.Server`;
- authorization header presence without value exposure;
- strict schema and instructions present;
- completed output extraction;
- refusal, incomplete, multiple output, HTTP error, timeout, oversized response;
- API key absent;
- fixed production endpoint and isolated injected test endpoint.

### Acceptance criteria

- OpenAI provider returns the same validated intent contract as recorded mode.
- All tests are network-free.
- No secret appears in errors, artifacts, or snapshots.

### Rollback risk

Medium. Network boundary and external API shape, isolated behind tests.

### Suggested commit

`Add OpenAI structured intent provider`

## Phase 5: Prompt-Driven CLI Orchestration

### Objective

Expose `design create --prompt --provider` and route validated provider intent
through the existing planner and design workflow.

### Likely files

- `cmd/kicadai/main.go`
- new CLI-focused orchestration helper package if needed to keep `main.go`
  manageable
- `cmd/kicadai/main_test.go`
- CLI golden tests

### Work

1. Add `--prompt`, `--prompt-file`, `--provider`, `--model`,
   `--provider-record`, and `--max-ai-attempts` options and help.
2. Validate mutual exclusions and provider-specific configuration.
   - perform explicit cross-option checks before creating a provider because the
     shared `flag.FlagSet` does not enforce mutual exclusion.
3. Invoke provider before creating the output project.
4. Strictly validate, normalize, and plan the returned intent.
5. Reuse the existing `designworkflow.Create` and promotion code.
6. Write sanitized provider and attempt artifacts.
7. Keep structured-request and deterministic text paths compatible.

### Tests

- exact recorded-provider CLI command;
- prompt-file parity and prompt/prompt-file mutual exclusion;
- option conflict and unknown provider failures;
- malformed provider response creates no project files;
- prompt run writes provider, intent, workflow, and promotion artifacts;
- deterministic repeated runs;
- old `design create` and `intent create` paths.

### Acceptance criteria

- One command generates the reference project from a prompt.
- The recorded provider uses the exact production downstream path.
- No project is written before AI output and plan validation pass.

### Rollback risk

High. CLI orchestration is broad; isolate shared logic and run CLI regression
tests before commit.

### Suggested commit

`Add prompt-driven design creation`

## Phase 6: Bounded Provider Correction and Deterministic Repair

### Objective

Add explicit bounded correction for AI-correctable intent failures while
preserving existing deterministic repair ownership.

### Likely files

- `internal/aiprovider/retry.go`
- provider/orchestrator tests
- `cmd/kicadai` integration code
- attempt artifact models

### Work

1. Classify retryable provider decode and planner diagnostics.
2. Enforce default one and hard maximum two provider attempts.
3. Send only structured code/path/message diagnostics on correction.
4. Record every attempt and stable hashes.
5. Ensure tool/auth/rate-limit/unsupported/validation failures do not retry.
6. Leave placement/routing repair in the existing workflow.

### Tests

- malformed first response corrected on second attempt;
- second failure stops;
- no retry for authentication, timeout policy, unsupported topology, or DRC;
- attempt history deterministic and sanitized;
- design repair flags still behave as before.

### Acceptance criteria

- No unbounded loop exists.
- Retry policy is machine-readable and independently tested.
- The AI never edits route geometry or KiCad output.

### Rollback risk

Medium. Retry logic can hide failures if classification is too broad; tests must
prove the negative cases.

### Suggested commit

`Bound AI intent correction attempts`

## Phase 7: Offline End-to-End and Golden Coverage

### Objective

Prove the complete post-provider lane deterministically in the default suite.

### Likely files

- `cmd/kicadai/*_test.go`
- `internal/designworkflow/*_test.go`
- `examples/ai/usb_c_bmp280_breakout/`
- golden artifacts under package testdata

### Work

1. Run the exact reference prompt through the recorded provider.
2. Add a recorded-retry fixture that returns an invalid first response and a
   corrected second response after structured diagnostics.
3. Compare normalized intent, generated request, and transaction hashes.
4. Assert component identity, protection, regulator, pull-ups, decoupling, and
   external I2C connector evidence.
5. Assert schematic readability and internal electrical evidence.
6. Assert PCB connectivity, route completion, writer, and round-trip evidence
   available without external KiCad.
7. Add malformed/unsafe output negative tests.

### Tests

- focused provider, planner, workflow, and CLI suites;
- repeated-run equivalence;
- `go test ./...`.

### Acceptance criteria

- Default tests require neither network nor API credentials.
- Recorded mode exercises the same orchestration as live mode.
- The exact reference request is covered end to end.

### Rollback risk

Low to medium. Test and fixture additions may expose existing nondeterminism.

### Suggested commit

`Cover AI-to-KiCad lane end to end`

## Phase 8: Live Provider and KiCad-Backed Promotion

### Objective

Demonstrate the real provider and promote its normalized reference result
through actual KiCad validation.

### Likely files

- optional test metadata under `examples/ai/usb_c_bmp280_breakout/`
- optional fixture harness code
- evidence documentation

### Work

1. Run the live provider with the reference prompt and configured key.
2. Inspect normalized intent and fail closed if it leaves supported scope.
3. Run generation with required ERC, DRC, and KiCad round-trip.
4. Resolve blockers in gate order with the smallest reusable correction.
5. Capture a sanitized recorded response from the validated intent.
6. Classify checked-in optional metadata as pass only after real evidence.
7. Run existing BMP280, protected USB-C LED, and combined pass fixtures.

### Tests

- opt-in live-provider smoke test;
- opt-in target KiCad-backed promotion test;
- existing optional pass fixture group;
- full test and lint suites.

### Acceptance criteria

- Live prompt generation succeeds with a real provider.
- Recorded and live normalized intents are semantically equivalent for required
  reference features.
- Strict KiCad ERC and DRC are clean.
- Writer correctness, connectivity, route completion, and normalized
  round-trip checks pass.
- Promotion status is `pass` based on real evidence.

### Rollback risk

High. Live model variability and KiCad environment behavior require optional,
evidence-based gating; no model output is blindly checked in.

### Suggested commits

- blocker-specific commits as required;
- `Promote AI-generated BMP280 reference fixture`.

## Phase 9: Documentation and Roadmap

### Objective

Document the demonstrated command and accurately update project status.

### Likely files

- `README.md`
- `docs/ai-generation.md`
- `docs/cli-reference.md` or current CLI documentation
- `specs/ROADMAP.md`
- example README/metadata

### Work

1. Document provider setup without exposing credentials.
2. Document live and recorded commands.
3. Document artifacts, status, retry limits, and failure modes.
4. State v1 supported/unsupported scope and fabrication caveat.
5. Update roadmap completed work and the next arbitrary-prompt gap.

### Tests

- documentation command/help consistency checks;
- repository searches for stale command forms;
- link/path verification where available.

### Acceptance criteria

- A user can reproduce the reference run from documentation.
- Claims match checked-in tests and evidence.
- Remaining arbitrary-prompt limitations are explicit.

### Rollback risk

Low.

### Suggested commit

`Document AI-to-KiCad generation lane`

## Phase 10: Completion Audit

### Objective

Prove every specification requirement against current artifacts and leave a
clean repository.

### Work

1. Run all focused provider, planner, CLI, workflow, and fixture tests.
2. Run `go test ./...` and lint.
3. Run target and existing optional KiCad-backed pass fixtures.
4. Run the documented live-provider command once.
5. Verify expected evidence artifacts and zero unexpected round-trip diffs.
6. Review all staged changes with Prism before each final commit.
7. Confirm no secrets, generated workspaces, or untracked artifacts remain.
8. Map each completion criterion to direct evidence.

### Acceptance criteria

- Every item in Specification section 19 has direct evidence.
- Prism findings are fixed or explicitly proven non-actionable.
- Git worktree is clean.

### Rollback risk

Low. Verification only, except for blocker fixes discovered by the audit.
