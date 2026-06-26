# KiCadAI Agent Skill

Use this skill when an AI agent needs to generate, inspect, validate, repair, or
explain KiCad projects through KiCadAI.

This is an operational contract for agents. It is intentionally prescriptive:
prefer deterministic CLI workflows, consume JSON output, validate before
claiming success, and stop when evidence is missing.

## Purpose

KiCadAI helps agents work with KiCad projects without driving the KiCad GUI.
The safe write path is direct KiCad file generation through the `kicadai` CLI
and Go packages. Live KiCad IPC is useful for probing a running KiCad instance,
but current live API write support is limited.

Use KiCadAI for:

- creating generated KiCad projects from structured intent;
- inspecting and evaluating existing KiCad projects;
- validating generated schematics and PCBs;
- checking component catalog, footprint, symbol, and pinmap evidence;
- producing rationale reports that explain planner decisions and blockers;
- planning or applying safe repairs to generated projects.

Do not use KiCadAI to claim arbitrary fabrication-ready autonomous design unless
the workflow evidence explicitly supports that claim.

## Preconditions

- Use the compiled `kicadai` binary. Do not document or suggest source-tree
  invocation patterns for normal agent workflows.
- Prefer `--json` for all agent-consumed commands.
- Prefer structured request files over freeform text when project generation is
  expected to succeed.
- Treat `kicad-cli` checks as optional unless the request or acceptance policy
  requires ERC/DRC evidence.
- Preserve request JSON, generated artifacts, `.kicadai/` metadata, and
  validation outputs with any final answer or handoff.
- Only mutate generated projects unless the command explicitly supports safe
  imported-project behavior.

## Capability Boundaries

Current strong paths:

- direct KiCad project, schematic, and PCB file writing;
- structured intent planning and `intent create`;
- verified circuit block generation for supported block families;
- component selection from the local catalog;
- writer correctness checks;
- connectivity-first board validation;
- optional KiCad ERC/DRC checks through `kicad-cli`;
- deterministic repair planning and generated-project repair apply;
- design rationale reports.

Current weak or blocked paths:

- arbitrary natural-language-to-board synthesis;
- arbitrary imported project mutation;
- live schematic/PCB mutation through KiCad IPC;
- production layout quality for large or unfamiliar boards without KiCad-backed
  validation evidence;
- component selection beyond the verified or policy-allowed local catalog;
- unsupported MCU alternate functions, arbitrary GPIO assignment, and safe
  external-clock topology generation.

## Default Agent Workflow

When asked to create a project from intent:

1. Convert the request into a structured intent JSON file when possible.
2. Run `intent plan` and inspect status, issues, known gaps, selected blocks,
   generated request, and synthesis trace.
3. If the plan is ready or acceptable with explicit known gaps, run
   `intent create` or `design create`.
4. Run writer and board validation on the generated project.
5. Run ERC/DRC checks when KiCad CLI is available and the requested acceptance
   level requires it.
6. Produce or inspect the rationale report.
7. Report success only when blocking issues are absent and required evidence is
   present.

Minimum command sequence:

```sh
kicadai --json --request request.json --output ./out/plan --overwrite intent plan
kicadai --json --request request.json --output ./out/project --overwrite intent create
kicadai --json writer check ./out/project
kicadai --json validate board ./out/project
kicadai --json --target ./out/project intent rationale
```

For direct design workflow requests:

```sh
kicadai --json --request request.json --output ./out/project --overwrite design create
kicadai --json writer check ./out/project
kicadai --json validate board ./out/project
```

## Existing Project Review Workflow

When asked to review or evaluate an existing KiCad project:

1. Inspect project structure.
2. Evaluate project-level issues.
3. Run writer correctness only when the project is generated or intended to
   match KiCadAI writer expectations.
4. Run board validation for PCB electrical meaning.
5. Run KiCad ERC/DRC if available and relevant.
6. Summarize findings with issue codes, paths, severity, and suggested next
   actions.

Commands:

```sh
kicadai --json inspect project ./project
kicadai --json evaluate project ./project
kicadai --json validate board ./project
kicadai --json check erc ./project/project.kicad_sch
kicadai --json check drc ./project/project.kicad_pcb
```

Use `writer check` for generated projects:

```sh
kicadai --json writer check ./project
```

## Repair Workflow

Repair is deterministic and guarded. Planning is read-only. Persisted apply is
only for generated projects with provenance and requires explicit execution
flags.

Plan from stage issues:

```sh
kicadai --json --request stage-issues.json repair plan
```

Apply an existing generated repair bundle to a generated project:

```sh
kicadai --json --execute --overwrite \
  --target ./out/project \
  --request ./out/project/.kicadai/repair-bundle.json \
  repair apply
```

After repair apply, rerun validation:

```sh
kicadai --json writer check ./out/project
kicadai --json validate board ./out/project
```

Never report a repair as complete without post-repair validation evidence.

## Component And Library Evidence Workflow

Before claiming a selected component is safe, inspect catalog and resolver
evidence:

```sh
kicadai --json component validate
kicadai --json component show resistor.generic.0805
kicadai --json component find --family resistor --package 0805 --value-kind resistance --value 10k
kicadai --json pinmap validate ./out/project
```

For generated intent workflows, inspect:

- `component_selection` stage output;
- component IDs and variants;
- rejected candidates;
- missing or insufficient ratings;
- resolver/pinmap evidence;
- placeholder or inferred-confidence warnings.

Stop if a fabrication-candidate request lacks verified component evidence.

For regulator-backed power rails, include `power.rails[].current_ma` whenever
the expected load is known. Then inspect
`.kicadai/generated-request.json` for `component_policy.overrides` and
`.kicadai/workflow-result.json` for the selected `component_selection` evidence.
The supported verified slice currently covers fixed 3.3 V AMS1117-style
SOT-223 and AP2112K SOT-23-5 LDO paths with ceramic input and output
capacitors. The AP2112K path is limited to 3.3 V rails from inputs at or below
6 V and at or below 150 mA for automatic planner selection; generated
connectivity ties `EN` to VIN and emits
an explicit KiCad no-connect marker for the NC pin. Broader regulator families
and exact capacitor part-number selection are still catalog expansion work. Do
not treat this slice as regulator stability proof: ESR, MLCC DC-bias derating,
thermal dissipation, and transient response still require part-specific
evidence or human review. For any LDO, verify the exact selected part is stable
with the generated ceramic output capacitors or choose a catalog record that
models the required output-capacitor ESR.

## Intent Planning Guidance

Prefer structured intent fields:

- `power.inputs[]`;
- `power.rails[]`;
- `functions[]`;
- `interfaces[]`;
- `constraints`;
- `manufacturing`;
- target, bus, and supply metadata when multiple choices exist.

Inspect these plan fields:

- `status`;
- `issues[]`;
- `known_gaps[]`;
- `selected_blocks[]`;
- `connections[]`;
- `generated_request`;
- `synthesis.decisions[]`;
- `synthesis.evidence[]`;
- `synthesis.constraints[]`;
- `synthesis.calculations[]`;
- `synthesis.gaps[]`.

Calculation statuses mean:

- `applied`: a supported generated block parameter was safely written.
- `deferred`: the value is evidence or a requirement, but the block should not
  be mutated directly yet.
- `blocked`: the calculation is physically invalid or unsafe to continue.

Known supported calculated value application:

- LED resistor values into `led_indicator.params.resistor_value`;
- I2C pull-up policy into `i2c_sensor.params.pullup_value` when pull-ups are
  block-owned;
- crystal load capacitors into
  `crystal_oscillator.params.load_capacitor_value`.
- regulator output-current and capacitor-voltage requirements into generated
  component policy overrides for the verified linear-regulator path.

Known requirement-only calculations:

- regulator headroom, except invalid headroom blocks planning;
- op-amp gain, unless a future block exposes safe feedback resistor mutation.

## Validation Requirements

For generated projects, run at least:

```sh
kicadai --json writer check ./out/project
kicadai --json validate board ./out/project
```

When ERC/DRC evidence is required:

```sh
kicadai --json check erc ./out/project/project.kicad_sch
kicadai --json check drc ./out/project/project.kicad_pcb
```

Success requires:

- command result reports OK/success;
- no blocking issues;
- generated files exist;
- validation artifacts exist when requested;
- rationale or plan explains assumptions and gaps;
- any skipped optional KiCad checks are clearly labeled optional.

## Stop Conditions

Stop and report blockers when:

- `status` is `blocked`;
- any issue has blocking/error severity that affects the requested goal;
- required component, footprint, symbol, or pinmap evidence is missing;
- calculated values are `blocked`;
- required ERC/DRC checks cannot run;
- generated board validation reports disconnected pads, missing outlines,
  unrouted required nets, invalid net assignments, or zone problems;
- request requires unsupported topology, arbitrary GPIO assignment, unknown MCU
  alternate function mapping, or external-clock generation;
- command output says imported project mutation is blocked.

Do not work around blockers by editing KiCad files manually unless the user
explicitly asks for low-level writer changes.

## Reporting Contract

When returning results to a user or another agent, include:

- command(s) run;
- output project path;
- generated artifacts of interest;
- validation status;
- blocking issues and known gaps;
- optional checks skipped and why;
- next recommended action.

Avoid saying "fabrication ready" unless fabrication readiness, writer
correctness, board validation, and required ERC/DRC evidence all support it.

## Common Safe Commands

```sh
kicadai --json --help
kicadai --json config
kicadai --json ping
kicadai --json capabilities
kicadai --json component validate
kicadai --json block list
kicadai --json inspect project ./project
kicadai --json evaluate project ./project
kicadai --json writer check ./project
kicadai --json validate board ./project
kicadai --json --request request.json intent plan
kicadai --json --request request.json --output ./out/project --overwrite intent create
kicadai --json --target ./out/project intent rationale
```

## Preferred References

- [README](../README.md) for quick start and repository map.
- [Docs Index](README.md) for subsystem documentation.
- [Intent Planning And AI Workflow](intent-planning.md) for structured intent
  behavior.
- [Validation And Analysis](validation-and-analysis.md) for validation command
  details.
- [Libraries And Components](libraries-and-components.md) for catalog, pinmap,
  and resolver evidence.
- [Validation Repair Loop](validation-repair.md) for repair behavior.
