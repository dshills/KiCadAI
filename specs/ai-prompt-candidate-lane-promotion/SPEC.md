# AI Prompt Candidate Lane Promotion Specification

Date: 2026-07-03

## 1. Objective

Promote the first prompt-driven AI generation lane from "instrumented but may
block" to "candidate-ready for a supported simple board."

The target user-facing command is:

```sh
kicadai --text "make a simple LED indicator board" --output ./out/ai_led --overwrite intent create
```

The target outcome is a generated KiCad project that is:

- deterministic from the prompt;
- fail-closed for unsafe or ambiguous requests;
- complete enough to open in KiCad;
- backed by workflow, validation, promotion, and AI-status artifacts;
- classified as `candidate` or better when the local evidence supports it.

This work is the shortest path from the current toolchain to useful
AI-generated schematic and PCB output. It does not attempt broad natural
language design synthesis. It proves one narrow, trustworthy lane end to end.

## 2. Background

KiCadAI now has mature foundations for:

- prompt drafting and deterministic intent planning;
- verified LED, connector, and I2C-oriented block families;
- schematic and PCB writers;
- schematic readability;
- footprint assignment and pad/copper net propagation;
- PCB placement and routing evidence;
- writer correctness, board validation, optional KiCad ERC/DRC, promotion
  reports, repair bundles, and AI-status summaries.

The documentation still describes prompt-driven `intent create` as a first-lane
workflow that may stop at a precise placement blocker. Meanwhile lower-level
LED and connector/LED KiCad-backed design fixtures have candidate-level
evidence. The gap is the user-facing prompt lane: an AI agent should be able to
ask for a simple LED indicator board and receive a candidate-ready generated
project or a precise fail-closed status.

## 3. Primary Prompt Lane

Primary prompt:

```text
make a simple LED indicator board
```

Expected drafted intent:

- request kind: breakout or equivalent simple board;
- one low-voltage LED indicator function;
- explicit or default low-voltage supply, currently 3.3 V when safe;
- one connector or input interface suitable for power/control;
- no mains/high-voltage assumptions;
- deterministic project basename such as `led_indicator`.

Expected generated design:

- root KiCad project, schematic, and PCB files;
- readable schematic with signal/power flow;
- LED, resistor, power/return symbols, and connector where applicable;
- footprints with resolvable library IDs and pad-net assignments;
- simple PCB outline;
- placed footprints inside the usable board area;
- routed or locally connected required LED nets where modeled;
- `.kicadai/` artifacts for manifest, workflow result, validation summary,
  promotion report, and retry state when relevant.

## 4. Goals

- Make the LED prompt lane reach `candidate` or `ready` without manual request
  editing.
- Keep unsafe prompts fail-closed before generation.
- Preserve JSON output shape used by AI agents:
  - CLI stdout exposes `data.ai_status.status`;
  - `.kicadai/validation-summary.json` exposes root `status`;
  - retryable blockers include stable category and artifact fields.
- Ensure prompt-generated output uses the same validation and promotion gates as
  structured `design create` requests.
- Persist enough artifacts for an AI agent to inspect, retry, repair, or report
  a blocker without reading KiCad internals.
- Update README, agent docs, and roadmap language to match the actual prompt
  lane status.

## 5. Non-Goals

- Do not claim unconstrained autonomous board generation.
- Do not make all prompt drafts candidate-ready.
- Do not require live LLM calls inside KiCadAI.
- Do not require a local KiCad installation for default tests.
- Do not suppress real validation, ERC, DRC, or routing blockers to force a
  promotion.
- Do not make generated LED output fabrication-ready unless the existing
  fabrication readiness gates independently support that status.
- Do not replace the broader I2C, regulator, amplifier, or route-tree
  promotion projects.

## 6. Required Behavior

### 6.1 Prompt Drafting

The prompt drafter must convert the primary LED prompt into a structured intent
request without asking unnecessary clarification questions.

It must:

- infer a low-voltage indicator use case;
- apply only safe defaults;
- expose all assumptions in planner artifacts;
- avoid hidden defaults for mains, battery chemistry, current capability, or
  production readiness;
- return `needs_clarification` or `unsupported` for prompts outside the
  supported lane.

Terminal drafting outcomes such as `needs_clarification`, `unsupported`, and
unsafe high-voltage refusals must set `retry_allowed: false` and
`max_automatic_retry_attempts: 0`. An agent must not automatically retry the
same prompt without new user input or a materially changed request.
`unsupported` is reserved for architectural, safety, or unsupported-capability
refusals during drafting/planning. Technical design failures after a supported
prompt enters the workflow, such as placement, routing, validation, or KiCad
check failures, must use the repair evidence pipeline instead.

### 6.2 Planning And Block Selection

The planner must map the drafted intent to supported blocks with stable
metadata.

Required evidence:

- selected block IDs;
- selected component IDs for every generated schematic/PCB component, or an
  explicit non-blocking rationale for any virtual/power symbol that cannot have
  a catalog component record;
- selected component roles;
- applied LED resistor calculation or explicit fallback value;
- voltage-domain assumptions;
- connector/interface assumptions;
- known gaps and warning-level limitations.

Candidate readiness must not retain generic component-selection warnings such
as `block component has no component_id or component_query` for physical
connector, resistor, LED, or other PCB-mounted components. A component may be
identified either by a concrete catalog component ID or by an explicit generic
component identity that includes library ID, footprint ID, value, role, and
pinmap evidence. Missing identity evidence must be reported as a true blocker.

### 6.3 Schematic Generation

The generated schematic must satisfy the existing readability and electrical
rules for simple examples.

Required properties:

- components are not stacked or visually cramped;
- signal/control flow is left to right;
- VCC/high rails are above return/ground rails;
- wires are orthogonal;
- exported labels do not conflict;
- symbol properties include component identity evidence where selected;
- schematic electrical validation has no blocking findings.

### 6.4 PCB Generation

The generated PCB must satisfy the current candidate-level board contract for a
simple LED board.

Required properties:

- board outline is present and valid;
- every generated footprint is inside the usable board area;
- pad nets and copper nets are KiCad-native and survive readback;
- required LED nets are connected according to modeled route/local-route
  evidence;
- no disconnected required pads;
- no invalid net assignments;
- routing/placement summaries are present.

### 6.5 AI Status And Repair Evidence

If the lane succeeds, status must be `candidate` or `ready`.

If it blocks, status must include:

- blocker stage;
- issue code;
- retryability;
- repair category;
- artifact paths;
- no generated command arrays that an agent might execute blindly.

The lane must not report a generic failure when a precise placement, routing,
validation, or KiCad-check issue is known.

All AI-status artifact paths must be lexically normalized and validated before
they are written. The implementation must convert both `/` and `\` separators
to a standard internal POSIX-style representation before cleaning and traversal
checks, regardless of host operating system. After normalization, artifact paths
must pass an allow-list of ASCII letters, digits, `.`, `_`, `-`, and `/`; reject
absolute paths, empty paths, `..` traversal after cleaning, doubled separators,
and paths whose resolved location leaves the generated output directory or a
documented generated-output subdirectory. Invalid paths must reject and fail the
workflow with a blocking issue. Unsafe paths must not be silently omitted.

The safe retry pattern is:

- retry only when `retry_allowed` is true;
- stop at `max_automatic_retry_attempts`;
- persist and check the current attempt count in `.kicadai/retry-state.json`
  before starting another automatic attempt;
- use `issue_code`, `repair_category`, and referenced artifacts as planning
  inputs;
- rerun validation after each repair;
- ask the user or stop when the next status is not retryable.

The default automatic retry budget for this lane is one attempt for a
repairable generated-project blocker. This value is an internal policy exposed
through `max_automatic_retry_attempts`; changing it requires an explicit spec
update or CLI policy option.

If `.kicadai/retry-state.json` cannot be read, written, or updated when retry
state is required, the workflow must report a terminal tool/artifact error with
`retry_allowed: false` rather than allowing an automatic retry loop.

Candidate status may include only allow-listed warning evidence. For this lane,
the allow-list is limited to optional KiCad tool-skipped evidence, explicitly
classified non-blocking KiCad warnings, and documented generic component
identity warnings that include complete library/footprint/value/pinmap
evidence. Any unclassified warning from writer correctness, schematic
electrical validation, board validation, promotion gates, artifact path
validation, or required physical connectivity must keep the lane below
candidate.

### 6.6 KiCad-Backed Evidence

Default tests must not require KiCad. When `KICADAI_KICAD_CLI` is configured,
the prompt lane should be testable against real KiCad ERC/DRC through the
existing optional policy.

Candidate readiness can tolerate warning-only KiCad evidence that is already
classified by promotion policy. True ERC/DRC errors must keep the lane below
candidate and expose the blocker.

## 7. Acceptance Criteria

The primary prompt lane is promoted when:

- `kicadai --text "make a simple LED indicator board" --output <dir> --overwrite intent create`
  exits successfully;
- CLI JSON reports `data.ai_status.status` as `candidate` or `ready`;
- generated `.kicadai/validation-summary.json` reports root `status` as
  `candidate` or `ready`;
- generated `.kicadai/design-promotion.json` exists and its achieved readiness
  matches or exceeds the declared prompt-lane expectation;
- generated KiCad project, schematic, and PCB files exist;
- all physical components have component identity evidence or an explicit
  blocker;
- writer correctness and board validation have no blocking findings;
- schematic electrical validation has no blocking findings;
- footprint placement is inside the usable board area;
- required LED connectivity has physical pad/copper evidence where modeled;
- unsafe mains/high-voltage prompts still fail closed before project write;
- focused and full Go tests pass.

## 8. Regression Fixtures

Add or update regression coverage for:

- successful LED prompt lane;
- unsafe mains/high-voltage LED prompt that fails closed;
- ambiguous prompt that requests clarification rather than guessing;
- stale placement blocker regression, if the old failure can be represented as
  a focused unit fixture;
- artifact-shape regression for AI-status and promotion outputs.

Optional KiCad-backed tests should be gated by environment variables and should
not change default CI behavior.

## 9. Documentation Updates

Update documentation only after implementation proves the new status.

Required updates:

- README AI-controlled generation lane;
- `docs/intent-planning.md`;
- `docs/kicadai-agent-skill.md`;
- `specs/ROADMAP.md`.

Documentation must distinguish:

- candidate-ready simple LED prompt generation;
- broader first-lane prompts that may still block;
- expected-fail I2C/amplifier promotion work;
- fabrication readiness, which remains stricter than candidate generation.

## 10. Risks

- A prompt lane may pass by bypassing validation instead of satisfying it.
- Documentation may overclaim general AI generation.
- Candidate status may hide warning-only KiCad evidence that should be visible.
- Repair-loop fields may encourage unsafe agent execution if command strings are
  emitted.
- A fixture may become dependent on a local KiCad installation.

Mitigation:

- keep all existing gates active;
- require promotion report evidence;
- keep optional KiCad tests opt-in;
- assert fail-closed unsafe prompts;
- expose artifact paths and categories rather than shell command arrays;
- validate AI-facing artifact paths so they cannot escape the generated output
  root.

## 11. Success Boundary

After this work, KiCadAI should honestly support this statement:

> An AI agent can use `kicadai intent create` to generate a simple low-voltage
> LED indicator KiCad project from text, inspect structured status and
> promotion evidence, and receive a candidate-ready result when the supported
> lane succeeds.

It should not yet claim:

> An AI agent can generate arbitrary schematics and PCBs without human
> intervention.
