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
- JSON is the default output for agent-consumed commands. Use `--format json`
  only when you need to make that default explicit.
- Prefer structured request files over freeform text when project generation is
  expected to succeed.
- Treat `kicad-cli` checks as optional unless the request or acceptance policy
  requires ERC/DRC evidence.
- Treat amplifier simulation as optional unless the fixture or workflow
  explicitly requires a `simulation` promotion gate. Missing simulator
  configuration should be reported as skipped/not-supported, not guessed. There
  is not yet a stable CLI flag or environment variable for external simulator
  execution; current simulation runners are Go-level/test-harness integrations.
- Treat KiCad-backed fixtures as an opt-in evidence tier. Do not claim a
  fixture passes unless `KICADAI_KICAD_CLI` was configured, required KiCad
  report artifacts were produced, and `.kicadai/design-promotion.json` records
  `"status": "pass"`. Several checked-in fixtures now reach that tier;
  metadata alone is not pass evidence.
- Preserve request JSON, generated artifacts, `.kicadai/` metadata, and
  validation outputs with any final answer or handoff.
- Only mutate generated projects unless the command explicitly supports safe
  imported-project behavior.

## Capability Boundaries

Current strong paths:

- direct KiCad project, schematic, and PCB file writing;
- structured intent planning and `intent create`;
- strict provider-backed generation for bounded references and explicit
  `generic-circuit-v1` graphs;
- trusted-catalog resolution of provider component/function/pin intent before
  any project write;
- deterministic schematic layout, PCB placement, routing, and promotion
  evidence for the checked-in generic reference fixtures;
- bounded deterministic placement/routing correction for generated
  `generic-circuit-v1` requests, with protected-invariant and retry evidence;
- generic multi-unit schematic lowering with one physical component,
  footprint, and BOM identity, proven by the LM358 reference;
- verified circuit block generation for supported block families;
- component selection from the local catalog;
- writer correctness checks;
- connectivity-first board validation;
- optional KiCad ERC/DRC checks through `kicad-cli`;
- opt-in Class AB headphone amplifier simulation artifacts and promotion gates
  for the narrow low-voltage headphone slice;
- deterministic repair planning and generated-project repair apply;
- design rationale reports.

Current weak or blocked paths:

- arbitrary natural-language-to-board synthesis outside catalog-resolvable,
  supported graph, placement, and routing capabilities;
- arbitrary imported project mutation;
- speaker, bridge, mains, or power-amplifier generation claims without SOA,
  thermal, active fault protection, simulation, KiCad ERC/DRC, and physical-rule
  evidence;
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
kicadai --request request.json --output ./out/plan --overwrite intent plan
kicadai --request request.json --output ./out/project --overwrite intent create
kicadai writer check ./out/project
kicadai validate board ./out/project
kicadai --target ./out/project intent rationale
```

Provider-backed reference sequence:

```sh
kicadai --prompt-file examples/ai/usb_c_bmp280_breakout/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/usb_c_bmp280_breakout/recorded-response.json \
  --output ./out/ai_usb_c_bmp280 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-kicad-roundtrip --strict-diffs \
  design create
# Continue autonomously only when AI status is ready and promotion is pass.
kicadai writer check ./out/ai_usb_c_bmp280
kicadai validate board ./out/ai_usb_c_bmp280
```

The protected USB-C BMP280 and protected USB-C LED fixtures are the two
provider-backed production references. Their strict KiCad-backed lanes should return
`data.ai_status.status: "ready"` and `.kicadai/design-promotion.json` status
`pass`. Do not describe that as fabrication-ready. For a live provider run,
set `OPENAI_API_KEY` and replace the two recorded-provider flags with
`--provider openai`. Keep the recorded provider for deterministic CI and
reproduction.

For a catalog-resolved topology that is not one of those bounded profiles, use
`--ai-profile generic-circuit-v1`. The provider then emits a strict circuit
graph; KiCadAI, not the provider, resolves components, functions, pins, pads,
symbols, footprints, placement, and routes. Use
`examples/ai/generic_rc_filter/`,
`examples/ai/generic_usb_c_led_indicator_protected/`,
`examples/ai/generic_usb_c_bmp280_breakout/`, and
`examples/ai/generic_lmv321_ac_gain_stage/`,
`examples/ai/generic_dual_lmv321_signal_conditioner/`, and
`examples/ai/generic_lm358_buffered_signal_conditioner/` as passing references.
The LMV321 and LM358 references prove structure and configured KiCad checks,
while analog performance remains review-required.
Never translate a generic failure into invented component IDs, library IDs,
pin names, or coordinates.

### Generic Multi-Unit Contract

Model a multi-unit package as one physical graph component. Its `units` array
declares logical schematic units, while endpoints and layout relationships use
explicit unit qualifiers. For the verified LM358 record:

- `A` and `B` are independent amplifier units;
- required unit `P` owns the shared supply pins;
- all units retain one reference, one catalog identity, one SOIC-8 footprint,
  and one BOM identity.

Do not create one physical component per logical unit. Do not add unit
qualifiers to ordinary single-unit parts. Use relative qualifiers such as
`near_unit` only when the target component declares that unit. Stop on unknown
or duplicate units, missing required power units, conflicting shared-pin nets,
duplicate package footprints, or ambiguous unit-to-pad mappings. Do not repair
those failures by duplicating symbols or footprints.

The graph representation uses separate fields rather than a combined `U1.A`
string. For example:

```json
{
  "components": [
    {
      "id": "amplifier",
      "reference": "U1",
      "units": [
        {"id": "A", "role": "reference_buffer"},
        {"id": "B", "role": "gain_stage"},
        {"id": "P", "role": "power"}
      ]
    }
  ],
  "nets": [
    {
      "name": "GAIN_OUT",
      "endpoints": [
        {
          "component": "amplifier",
          "unit": "B",
          "selector_kind": "function",
          "selector": "OUT"
        }
      ]
    }
  ],
  "schematic_layout": {
    "placements": [
      {
        "component": "feedback",
        "near": "amplifier",
        "near_unit": "B"
      }
    ]
  }
}
```

Recorded LM358 verification:

```sh
kicadai --prompt-file examples/ai/generic_lm358_buffered_signal_conditioner/prompt.txt \
  --provider recorded \
  --provider-record examples/ai/generic_lm358_buffered_signal_conditioner/recorded-response.json \
  --ai-profile generic-circuit-v1 \
  --catalog-dir ./data/components \
  --symbols-root /path/to/kicad-symbols \
  --footprints-root /path/to/kicad-footprints \
  --output ./out/ai_generic_lm358 --overwrite \
  --kicad-cli /path/to/kicad-cli \
  --require-erc --require-drc --require-kicad-roundtrip \
  --strict-diffs --strict-unrouted \
  design create
```

For a live run, reuse the same prompt and trusted-library arguments, replace
the recorded-provider flags with `--provider openai --ai-background`, and add
`--max-ai-attempts 2`. Change `--output` to `./out/live_generic_lm358` so the
semantic comparison below reads the generated live graph. Load
`OPENAI_API_KEY` from the environment; never put it in a command, request,
fixture, log, or evidence artifact. Live provider timeouts before graph
generation do not invalidate recorded evidence. Replay a saved graph or rerun
the optional live lane; never infer a graph from a timeout.

Use the profile output-token default first: 8,192 for bounded reference
profiles and 32,768 for `generic-circuit-v1`. If the CLI returns
`AI_PROVIDER_INCOMPLETE` at `provider.max_output_tokens`, inspect its reported
limit and usage, then perform at most one intentional retry with the suggested
bounded `--ai-max-output-tokens` value. Never add a blind retry loop. The CLI
flag overrides `KICADAI_AI_MAX_OUTPUT_TOKENS`; both are constrained to
1,024-65,536 tokens and can increase provider cost and latency.

To compare a saved live LM358 graph with the checked-in critical projection
without another provider request:

```sh
KICADAI_OPENAI_LIVE_TEST=1 \
KICADAI_OPENAI_LM358_LIVE_GRAPH=./out/live_generic_lm358/.kicadai/circuit-graph.json \
  go test ./internal/aiprovider -run '^TestOpenAILiveGenericLM358Graph$' -count=1 -v
```

For the LED reference, replace both BMP280 fixture paths with
`examples/ai/usb_c_led_indicator_protected/` and use a separate output path.
Do not rewrite an unsupported or composite request to force either profile;
preserve the structured fail-closed diagnostic.

`candidate` remains a valid generated artifact for manual inspection, but it is
not permission for an agent to claim the requested KiCad-backed gates passed.

After provider-backed `design create` or deterministic `intent create`, inspect
`data.ai_status` from stdout or
`.kicadai/validation-summary.json`:

- `ready`: continue to writer/board checks and optional ERC/DRC.
- `candidate`: review warnings and skipped evidence before claiming success.
- `blocked`: inspect `stage`, `issue_code`, `message`, and artifacts.
  `ai_status` summarizes the primary blocker; inspect `issues[]`,
  `.kicadai/workflow-result.json`, and `.kicadai/validation-summary.json` for
  the full issue list. If `retry_allowed` is true, compare `retry_key` against
  `.kicadai/retry-state.json` or your own local state before retrying. Do not
  repeat the same automatic repair when the retry key and current attempt count
  show that the same failure state already consumed its retry budget. The file
  `.kicadai/retry-state.json` stores `current_automatic_retry_attempt`, and
  the budget field is `max_automatic_retry_attempts`. KiCadAI writes this file
  during provider-backed `design create` and `intent create`;
  when the same output directory and
  retry key are reused, KiCadAI detects the existing file and increments the
  current attempt count. An external retry loop may keep additional state, but
  should not hand-edit KiCadAI's generated retry-state file as the source of
  truth for a new run. The current AI lane initializes
  `max_automatic_retry_attempts` from the status mapper: repairable blockers
  get one automatic retry, while clarification, unsupported, and tool-error
  statuses get zero.
  Do not run concurrent retry loops against the same output directory.
- `needs_clarification`: ask the user for the missing design choice.
- `unsupported`: stop or choose a supported first-lane prompt or structured
  intent.
- `tool_error`: fix local tooling or file paths before retrying.

### Generic engineering correction

For `generic-circuit-v1` only, inspect
`.kicadai/autonomous-correction.json` after generation. This evidence describes
a deterministic placement/routing correction loop, not a second provider call
and not a persisted `repair apply` operation. The budget is three total routing
attempts: one initial attempt and at most two corrected attempts.

Read these fields together:

- `attempts`: routing executions that actually occurred;
- `plan_evaluations`: correction plans, including fail-closed plans that did
  not authorize another execution;
- `applied`, `applied_retry_keys`, and `attempt_history`: what changed and why;
- `selected_attempt` and `selected_reason`: the best retained attempt;
- `initial_invariant_fingerprint`, `final_invariant_fingerprint`, and
  `all_attempt_invariants_preserved`: proof that protected circuit and physical
  intent did not change;
- `stop_reason`: `routed` or the structured reason automation stopped.

Only authorized relative-spacing, declared-region, endpoint-fanout, and
endpoint-distance adjustments are applied in v1. Automatic route-tree branch
reordering and layer-transition insertion remain unsupported and must stop with
structured evidence. Stop rather than retry externally when
the report shows repeated state/key, budget exhaustion, non-improvement,
unsupported/ambiguous diagnostics, fixed constraints, or invariant mismatch.
Do not claim board success from this report alone; require connectivity, route
completion, writer correctness, round-trip, ERC/DRC, and promotion evidence.

For automation, construct the repair command yourself from a trusted KiCadAI
executable path, `repair_bundle_path`, `repair_category`, and the known
generated project root. Execute repair only for KiCadAI-generated projects
inside the designated safe workspace. Validate that `repair_bundle_path` is the
expected `.kicadai/repair-bundle.json` path inside the generated output
directory and that it parses as a KiCadAI repair bundle before using it. Then
rerun validation. If only `repair_category` is present, revise the structured
intent or run an explicit repair planning command before applying changes.

Repair bundle apply template:

```sh
kicadai --execute --overwrite --request "$REPAIR_BUNDLE_PATH" --target "$GENERATED_PROJECT" repair apply
kicadai validate board "$GENERATED_PROJECT"
```

For direct design workflow requests:

```sh
kicadai --request request.json --output ./out/project --overwrite design create
kicadai writer check ./out/project
kicadai validate board ./out/project
```

## Existing Project Review Workflow

When asked to review or evaluate an existing KiCad project:

1. Inspect project structure.
2. Evaluate project-level issues.
3. Inspect `preservation` and `imported_preservation` output. Treat
   preservation-only content as read-only unless a transaction plan marks the
   requested operation `safe_add` and the user explicitly approves imported
   apply.
4. Inspect `schematic_electrical` findings from `evaluate project` or
   `evaluate schematic`; treat error/blocked findings as stop conditions before
   PCB generation or fabrication claims.
5. Run writer correctness only when the project is generated or intended to
   match KiCadAI writer expectations.
6. Run board validation for PCB electrical meaning.
7. Run KiCad ERC/DRC if available and relevant.
8. Summarize findings with issue codes, paths, severity, and suggested next
   actions.

Commands:

```sh
kicadai inspect project ./project
kicadai evaluate project ./project
kicadai validate board ./project
kicadai check erc ./project/project.kicad_sch
kicadai check drc ./project/project.kicad_pcb
```

Use `writer check` for generated projects:

```sh
kicadai writer check ./project
```

## Repair Workflow

Repair is deterministic and guarded. Planning is read-only. Persisted apply is
only for generated projects with provenance and requires explicit execution
flags.

Plan from stage issues:

```sh
kicadai --request stage-issues.json repair plan
```

Apply an existing generated repair bundle to a generated project:

```sh
kicadai --execute --overwrite \
  --target ./out/project \
  --request ./out/project/.kicadai/repair-bundle.json \
  repair apply
```

After repair apply, rerun validation:

```sh
kicadai writer check ./out/project
kicadai validate board ./out/project
```

Never report a repair as complete without post-repair validation evidence.

## Component And Library Evidence Workflow

Before claiming a selected component is safe, inspect catalog and resolver
evidence:

```sh
kicadai component validate
kicadai --source-dir ./data/component-sources component validate
kicadai --source-dir ./data/component-sources component coverage
kicadai component show resistor.generic.0805
kicadai component find --family resistor --package 0805 --value-kind resistance --value 10k
kicadai --request examples/components/select_concrete_resistor.json component select
kicadai pinmap validate ./out/project
```

For generated intent workflows, inspect:

- `component_selection` stage output;
- `schematic_electrical` stage status and findings;
- component IDs and variants;
- rejected candidates;
- missing or insufficient ratings;
- resolver/pinmap evidence;
- procurement evidence when a local source snapshot is supplied;
- placeholder or inferred-confidence warnings.

Generated schematic symbols carry hidden component identity properties when
selection evidence exists. Before claiming a generated part identity, inspect
the schematic properties and compare them with
`.kicadai/workflow-result.json`.

Important property names:

- `KiCadAI Component ID`
- `KiCadAI Variant ID`
- `KiCadAI Component Role`
- `KiCadAI Block ID`
- `Manufacturer`
- `MPN`
- `Component Class`
- `Component Confidence`
- `Component Source`
- `Lifecycle Status`
- `Availability Status`
- `Pinmap ID`

Recommended evidence check:

```sh
kicadai --request examples/design/led_indicator.json --output ./out/led_indicator --overwrite design create
kicadai inspect schematic ./out/led_indicator/led_indicator.kicad_sch
kicadai export bom ./out/led_indicator
```

Treat conflicts between schematic properties, workflow component-selection
evidence, and BOM/fabrication output as blockers for imported projects and as
warnings that require explanation for generated projects. Do not infer
manufacturer or MPN values when these properties are absent.

The checked-in catalog includes a small verified alternatives slice for a 10
kOhm 0805 resistor, 100 nF 0805 capacitor, green 0805 LED, and 1x04 2.54 mm
header. Connectivity and stronger selection should prefer concrete records
when they satisfy the request; draft and structural workflows may still use
generic fallback records. Inspect `alternative_coverage` in `component coverage`
when judging catalog breadth.

For a supported concrete I2C sensor, set `i2c_sensor.params.sensor_component_id`
to exactly one of:

- `sensor.bosch.bme280.lga8`;
- `sensor.bosch.bmp280.lga8`;
- `sensor.sensirion.sht31_dis.dfn8`.

Use `0x76`/`0x77` for the Bosch profiles and `0x44`/`0x45` for SHT31-DIS.
Inspect the generated `component_selection` evidence and schematic identity
properties before claiming a concrete part was used. Do not invent IDs, infer
pin roles, request SPI mode, or substitute a generic topology after a concrete
selection fails. The checked-in intent examples
`sensor_bmp280_breakout.json` and `sensor_sht31_breakout.json` demonstrate the
supported handoff.

Stop if a fabrication-candidate request lacks verified component evidence.
When lifecycle or availability matters, provide `--source-dir` and inspect the
selected `procurement` object. Treat source evidence as a local snapshot only;
do not claim live stock, price, lead time, or distributor approval.

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
and exact capacitor part-number selection are still catalog expansion work. This
slice is not fabrication-ready regulator stability proof:
`component select`, `design create`, workflow summaries, and rationale output
now expose structured `regulator_evidence` and `capacitor_evidence`, but
fabrication-candidate selection blocks until ESR-window, MLCC DC-bias,
effective-capacitance, and thermal review evidence is proven or explicitly
not applicable. For any LDO, verify the exact selected part is stable with the
generated output capacitors or choose a catalog record that models the required
output-capacitor ESR.

Inspect component hint enforcement before claiming the generated layout honors
catalog guidance. `workflow-result.json` placement and routing stages can
contain `component_hints` plus `component_hint_summary`; `design-rationale.json`
also emits component hint evidence records. Treat `enforced` and
`satisfied_by_block` as useful workflow evidence. Treat `failed`, `skipped`,
and `unsupported` as repair or review inputs. Treat `pending` as recognized
guidance that has not been consumed by the current stage. These records are not
a fabrication-proof substitute for KiCad ERC/DRC, thermal checks,
output-capacitor stability proof, or impedance/clearance analysis.

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
kicadai writer check ./out/project
kicadai validate board ./out/project
```

When ERC/DRC evidence is required:

```sh
kicadai check erc ./out/project/project.kicad_sch
kicadai check drc ./out/project/project.kicad_pcb
```

Success requires:

- command result reports OK/success;
- no blocking issues;
- generated schematic connectivity evidence is present and clean for generated
  projects that emit schematic files;
- generated schematic power policy is `not_required`, intentionally `driven`,
  or explicitly accepted as externally driven for a non-standalone module;
- generated files exist;
- validation artifacts exist when requested;
- rationale or plan explains assumptions and gaps;
- any skipped optional KiCad checks are clearly labeled optional;
- `pass` promotion claims include clean required KiCad ERC/DRC evidence when
  those checks are requested.

For provider-backed generic circuits, also inspect
`.kicadai/circuit-graph.json`, `.kicadai/circuit-resolution.json`,
`.kicadai/design-request.json`, `.kicadai/autonomous-correction.json`,
`.kicadai/workflow-result.json`, and `.kicadai/design-promotion.json`. For
multi-unit parts, verify that logical
units remain distinct in the schematic while the PCB and BOM contain exactly
one physical package. A live graph is equivalent to a recording only when the
fixture's semantic-projection test passes; textual or ordering similarity is
not sufficient.

To run all checked-in provider fixtures through the optional KiCad-backed
promotion lane:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols \
KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints \
  go test ./cmd/kicadai -run '^TestAIProviderOptionalKiCadPromotion$' -count=1 -v
```

Generation commands accept library and KiCad paths as CLI flags. The optional
Go promotion harness uses the corresponding environment variables because it
runs multiple checked-in fixtures in one test process.

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
- command output says imported project mutation is blocked;
- transaction planning reports a preservation operation review with
  `mutability: "unsafe"`.

Do not work around blockers by editing KiCad files manually unless the user
explicitly asks for low-level writer changes.

## Parallel Workstreams

Before splitting work across agents or worktrees, inspect
`data/ai-readiness/matrix/*.json` and the coverage output for
`parallel_group` and `depends_on` metadata.

Current workstream groups are:

- `fixture_promotion`: KiCad-backed fixture promotion and validation evidence.
- `catalog_block_expansion`: verified component, footprint, pinmap, and block
  catalog work.
- `engine_hardening`: placement, routing, layout, and validation engine
  hardening.
- `intent_ai_ux`: intent planner and AI workflow behavior.
- `documentation`: user and agent documentation.

Treat `depends_on` as a prerequisite edge. Parallel work can begin before every
dependency is finished, but do not claim a dependent record is `verified` unless
all dependencies are also `verified`. Cross-group dependencies mean the streams
need coordination before final promotion.

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
For amplifier work, distinguish simulation evidence from fabrication evidence:
a passing `.kicadai/amplifier-simulation.json` can support a narrow
headphone-slice promotion gate, but it does not prove speaker or
power-amplifier safety. For LMV321 and LM358 references, clean ERC, DRC,
routing, writer, and round-trip evidence does not establish stability,
gain-bandwidth, output swing, input common-mode range, output drive, noise,
distortion, or load compatibility. Preserve their `review_required` evidence.

## Common Safe Commands

```sh
kicadai --help
kicadai config
kicadai ping
kicadai capabilities
kicadai component validate
kicadai block list
kicadai inspect project ./project
kicadai evaluate project ./project
kicadai writer check ./project
kicadai validate board ./project
kicadai --request request.json intent plan
kicadai --request request.json --output ./out/project --overwrite intent create
kicadai --target ./out/project intent rationale
```

## Preferred References

- [README](../README.md) for quick start and repository map.
- [Docs Index](README.md) for subsystem documentation.
- [Intent Planning And AI Workflow](intent-planning.md) for structured intent
  behavior.
- [AI Generation](ai-generation.md) for bounded and generic provider commands,
  multi-unit semantics, fixture evidence, and current limits.
- [Validation And Analysis](validation-and-analysis.md) for validation command
  details.
- [Libraries And Components](libraries-and-components.md) for catalog, pinmap,
  and resolver evidence.
- [Validation Repair Loop](validation-repair.md) for repair behavior.
