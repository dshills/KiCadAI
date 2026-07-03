# AI-Controlled Generation Lane Specification

## 1. Purpose

Define the shortest path to useful AI-generated KiCad schematics and PCBs by
making KiCadAI a deterministic tool that an external AI agent can control.

The first usable milestone is not unconstrained "make any board" generation.
It is a narrow, validated lane where an AI can:

1. turn a user prompt into a supported structured intent request;
2. ask KiCadAI to draft, plan, and create the project;
3. inspect machine-readable evidence;
4. retry or ask a clarification when validation blocks completion.

The target outcome is a constrained breakout-style design, such as an LED,
connector/LED, or I2C sensor breakout, generated from natural language through
existing CLI commands and validated as a KiCad-native project.

In this spec, "promotion" means validating a generated design against a named
readiness gate, such as `expected_fail`, `candidate`, or `ready`, and recording
the evidence that justifies that status.

## 2. Design Principle

KiCadAI should not embed an LLM in the core generation path for this milestone.
The AI runs outside KiCadAI and treats KiCadAI as a stable command-line tool.

```text
human prompt
  -> AI agent
  -> kicadai --text ... intent draft/plan/create
  -> KiCad project files
  -> validation and promotion evidence
  -> AI retry, clarification, or final response
```

This keeps generation:

- deterministic;
- testable without network access;
- usable by any agent runtime;
- compatible with future MCP or app wrappers;
- constrained by verified blocks, components, writers, and validators.

## 3. Goals

- Provide one documented, tested AI-controlled workflow from prompt to generated
  schematic and PCB.
- Prefer already-supported design families before broadening capability:
  LED, connector/LED, and I2C breakout.
- Use existing structured intent, design workflow, block, component, placement,
  routing, validation, and report systems.
- Add only the minimum glue needed for an AI caller to decide what to run next.
- Preserve strict fail-closed behavior when the prompt maps to unsupported
  blocks, unsafe requirements, missing evidence, or validation blockers.
- Persist all request, extraction, generated project, validation, and promotion
  artifacts under the output directory.
- Return compact JSON summaries suitable for AI prompt context and automation.

## 4. Non-Goals

- No embedded LLM provider calls.
- No MCP server in this milestone.
- No arbitrary free-form schematic synthesis.
- No hidden KiCad GUI dependency.
- No claim of fabrication readiness without configured KiCad ERC/DRC and
  promotion evidence.
- No automatic web lookup, pricing, sourcing, datasheet parsing, or live
  availability checks.
- No amplifier or power-amplifier autonomous generation as the first lane.

## 5. Current Foundation

KiCadAI already has most of the lower-level pieces:

- `intent draft`, `intent plan`, `intent explain`, `intent create`, and
  `intent rationale` workflows;
- deterministic `design create` project generation from request JSON;
- reusable circuit blocks and block realization;
- catalog-backed component selection and component identity propagation;
- schematic writer, PCB writer, project writer, and readback checks;
- schematic electrical validation;
- placement, routing, and route evidence;
- board validation and writer correctness checks;
- optional KiCad CLI ERC/DRC evidence;
- repair bundle, repair apply, and post-repair evidence hooks;
- AI readiness matrix and promotion fixtures.

The missing productized lane is a stable orchestration contract that tells an
AI caller:

- which prompt families are supported;
- which command to run first;
- when to proceed from draft to create;
- how to interpret validation status;
- when to retry;
- when to ask a clarification;
- when to stop and report a blocker.

## 6. First Supported Lane

The first AI-controlled lane should support low-risk breakout-style prompts:

- "make a 3.3V I2C temperature sensor breakout";
- "make a connector breakout with power LED";
- "make a simple LED indicator board";
- "make a Qwiic-style I2C breakout with SDA, SCL, VCC, and GND";
- "make a small 2-layer board with an I2C sensor and header".

The lane should initially reject or clarify:

- high-voltage, mains, battery charging, RF, high-speed, power amplifier, and
  safety-critical requests;
- unsupported sensor families;
- unsupported connector standards;
- ambiguous power inputs;
- requests that require unverified footprints or unknown pinmaps;
- requests asking for fabrication readiness when KiCad evidence is unavailable.

## 7. Command Contract

The AI caller should be able to drive the lane with existing and lightly
extended commands.

### 7.1 Draft From Prompt

```sh
kicadai --text "make a 3.3V I2C temperature sensor breakout" \
  --output ./out/draft \
  intent draft
```

Required behavior:

- produce structured intent JSON;
- produce extraction evidence;
- produce clarifications and blockers;
- do not write KiCad project files;
- return nonzero or blocked status when strict mode is enabled and blocking
  clarifications exist.

### 7.2 Explain Or Plan

```sh
kicadai --text "make a 3.3V I2C temperature sensor breakout" \
  intent explain
```

or:

```sh
kicadai --request ./out/draft/intent-draft.json \
  --output ./out/plan \
  --overwrite \
  intent plan
```

Required behavior:

- show selected goals, blocks, assumptions, known gaps, and unsafe areas;
- preserve enough structured status for an AI caller to decide whether creation
  is allowed.

### 7.3 Create Project

```sh
kicadai --text "make a 3.3V I2C temperature sensor breakout" \
  --output ./out/sensor_breakout \
  --overwrite \
  intent create
```

Required behavior:

- run draft, planner, design workflow, validation, and report generation;
- persist source prompt, drafted request, extraction, clarifications, plan,
  design request, generated KiCad files, and validation reports;
- emit a concise final JSON status.

### 7.4 Optional Validation And Repair

```sh
kicadai --project ./out/sensor_breakout design validate
kicadai --project ./out/sensor_breakout --output ./out/repair repair export-bundle
```

Where repair is already wired into `design create`, the AI lane should document
the safe flags and expected artifacts rather than creating a competing repair
interface.

### 7.5 Exit Code Contract

When `--json` is present, the final JSON object must be the only stdout
content. Logs, progress output, and human-readable diagnostics must go to
stderr so stdout remains parseable by AI tooling. Implementations should route
internal logs and subprocess progress to stderr and reserve stdout for a single
dedicated JSON encoder write at command completion. If any dependency cannot be
kept off stdout, the lane should provide an explicit JSON-output-file option so
agents can read the machine result from disk instead of stdout. Exit codes are
automation hints, not a replacement for reading the JSON status:

- `0`: generation reached `ready` or `candidate`;
- nonzero: generation reached `blocked`, `needs_clarification`,
  `unsupported`, `tool_error`, or failed before a lane status could be
  produced.

AI agents must read the JSON `status` field to distinguish non-success states.
The CLI may later add more specific exit codes, but the first lane should
preserve compatibility with existing shell behavior.

## 8. Output Artifact Contract

For prompt-driven `intent create`, the output project should include:

```text
project/
  project.kicad_pro
  project.kicad_sch
  project.kicad_pcb
  sym-lib-table
  fp-lib-table
  .kicadai/
    manifest.json
    intent-source.txt
    intent-draft.json
    intent-extraction.json
    intent-clarifications.json
    intent-plan.json
    design-request.json
    design-promotion.json
    validation-summary.json
    rationale.json
    retry-state.json
```

`manifest.json` should contain:

- schema/version;
- normalized final lane status;
- artifact entries with project-relative path, role, stage, status, and
  optional skip reason;
- generation ownership marker proving the project was created by KiCadAI;
- source prompt hash and request hash where available;
- SHA-256 file hashes for generated files that repair apply may mutate.

When a step is skipped or blocked, `manifest.json` is the source of truth for
artifact lifecycle status. The corresponding artifact should either be absent
with a structured reason in the manifest, or present as syntactically valid
partial data while the manifest records `status: "blocked"` and stage evidence.

## 9. AI-Facing Status Model

The lane should normalize final status into one of:

- `ready`: generated project satisfies the requested acceptance level;
- `candidate`: generated project is usable for review, but warning-level
  evidence remains;
- `blocked`: generation could not safely complete;
- `needs_clarification`: prompt cannot be mapped without user input;
- `unsupported`: request is outside the verified lane;
- `tool_error`: local environment or command failure prevented validation.

Each status must include:

- stage;
- issue code;
- short AI-facing message;
- detailed human message;
- related artifact paths;
- suggested next action;
- whether retry is allowed;
- whether user clarification is required.

Allowed `stage` values are:

- `draft`;
- `plan`;
- `create`;
- `schematic`;
- `pcb`;
- `placement`;
- `routing`;
- `validation`;
- `kicad_checks`;
- `repair`;
- `manifest_generation`;
- `orchestration`;
- `internal_setup`.

## 10. First-Lane Acceptance Policy

For the first working lane, "AI-generated schematic and PCB" means:

- prompt maps to a structured supported intent without blocking
  clarifications;
- selected blocks are supported and report readiness;
- component selection uses verified or explicitly allowed experimental parts;
- schematic is written and passes internal schematic electrical checks;
- PCB is written and passes writer correctness/readback checks;
- board validation reports no blocking net/pad/connectivity issues;
- placement and routing either complete or report explicit non-fabrication
  blockers;
- optional KiCad ERC/DRC evidence is captured when requested and available;
- final JSON status accurately reflects `ready`, `candidate`, or `blocked`.

The first lane may target `candidate` before `ready` if local KiCad CLI is not
available, but it must not silently claim DRC-clean output.

## 11. Prompt Handling Rules

The natural-language adapter should stay deterministic:

- parse common phrase patterns and synonyms;
- map only known families to structured intent;
- preserve source spans and confidence;
- prefer clarification over guessing;
- default to conservative acceptance;
- never invent components, footprints, ratings, or safety claims.

The AI agent can improve prompt interpretation externally, but KiCadAI should
still validate the resulting structured request.

## 12. Retry And Repair Rules

The lane should allow AI retry only when:

- the failure is generated by KiCadAI-owned output;
- the issue has a structured repair category;
- the repair action is deterministic and bounded;
- revalidation is run after repair;
- the final status never reports success without fresh validation evidence.

Before repair apply mutates files, KiCadAI must validate that the target
contains a `.kicadai/manifest.json` ownership marker, that manifest paths are
project-relative and remain inside the project root after cleaning, and that
each changed file hash matches metadata recorded during generation. Path checks
must reject absolute paths and `..` escapes by resolving each manifest path
against the cleaned project root and verifying the resolved absolute path is
equal to the root or has the root plus path-separator as its prefix. Symlinks
must be evaluated for both the project root and resolved target path before the
prefix comparison. Imported or user-authored projects must require explicit
opt-in and remain outside this first lane.

The status JSON should include a bounded retry recommendation for external
agents. KiCadAI should report whether retry is allowed, a stable retry key for
the current blocker, and the suggested maximum automatic retry count. The first
implementation should either persist `.kicadai/retry-state.json` or accept an
explicit caller-provided retry attempt value so repeated runs are deterministic.
The retry state should include a stable blocker hash derived from stage, issue
code, path, net/reference where available, and normalized message. The first
lane should recommend at most one automatic retry for deterministic repair
apply, and require the AI caller to ask for user input or stop when the same
blocker reappears.

The lane should ask for user clarification when:

- power source or voltage is ambiguous;
- requested function maps ambiguously to multiple supported families;
- requested function maps only to unsupported families;
- fabrication readiness is requested without required evidence;
- safety-sensitive scope is detected.

## 13. Testing Requirements

Add tests for:

- prompt-to-draft golden outputs for first-lane prompts;
- clarification behavior for ambiguous/unsupported prompts;
- prompt-driven `intent create` fixture using an LED or connector/LED request;
- I2C breakout prompt fixture that is allowed to remain blocked if the current
  routing fixture is still expected-fail, but must report the exact blocker;
- artifact manifest expectations for prompt-driven output;
- final AI-facing status normalization;
- CLI documentation examples using compiled `kicadai` command syntax.

Tests must not require network access or a live LLM.

Optional KiCad CLI tests must remain gated by existing environment variables.

## 14. Documentation Requirements

Document:

- the supported first-lane prompt families;
- the external-agent control model;
- command examples;
- status meanings;
- safe retry behavior;
- current unsupported scopes;
- how to inspect `.kicadai/` artifacts;
- how this lane differs from future MCP integration.

Documentation should be clear that KiCadAI is the deterministic generator and
validator, while the AI remains the controller.

## 15. Success Criteria

This project is complete when:

- an AI or script can call one documented command with a supported prompt and
  receive a generated KiCad project plus JSON status;
- unsupported or ambiguous prompts fail closed with clarification issues;
- first-lane examples are covered by golden tests;
- the final output contract is stable enough for an AI agent to consume without
  reading source code;
- README and agent guidance point to this lane as the shortest supported path to
  AI-generated schematics and PCBs.

## 16. Next Work After This Spec

After the first lane works, the next expansion should be:

1. promote I2C breakout from expected-fail to candidate/pass;
2. add richer repair-loop orchestration;
3. expand supported prompt families;
4. add provider-specific AI examples outside the core package;
5. consider MCP only after the CLI lane is reliable.
