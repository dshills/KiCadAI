# Verified Amplifier Block Family Implementation Plan

## Phase 1: Inventory And Readiness Baseline

Goal: make the amplifier family visible as a coherent roadmap-backed block
family before adding behavior.

Tasks:

- Add or update inventory entries for:
  - `amplifier_input_buffer`;
  - `amplifier_gain_stage`;
  - `amplifier_bias_network`;
  - `class_ab_output_pair`;
  - `amplifier_output_protection`;
  - `amplifier_supply_decoupling`;
  - `headphone_output_connector`;
  - `speaker_output_connector`.
- Map each inventory entry to readiness:
  - implemented;
  - planned;
  - blocked;
  - experimental;
  - reference-needed.
- Link each entry to AI readiness records and known blockers.
- Add tests proving the inventory reports the family and does not imply
  fabrication readiness.
- Update docs to describe supported headphone scope and blocked power-amplifier
  scope.

Acceptance:

- `block list` or equivalent inventory output includes every amplifier family
  entry.
- Unsupported speaker/power amplifier entries are visible as blocked, not
  missing.
- Tests pass:

  ```sh
  go test ./internal/blocks ./internal/aireadiness -run 'Amplifier|Inventory|Readiness' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Add amplifier block family inventory`.

## Phase 2: Component Evidence Model

Goal: represent amplifier-specific component evidence in structured data.

Tasks:

- Extend or reuse component metadata for:
  - op-amp drive/load/stability evidence;
  - output transistor role, package, current, power, SOA, and thermal evidence;
  - bias diode role and thermal-coupling evidence;
  - coupling capacitor voltage and impedance evidence;
  - output connector load policy.
- Validate seeded LMV321, MMBT3904, and MMBT3906 records against the new model.
- Keep power-output transistor records blocked until SOA/thermal evidence is
  modeled.
- Add rejected-candidate diagnostics for unsupported speaker-current or
  high-power output requests.

Acceptance:

- Low-current headphone parts can be selected with warnings where appropriate.
- Power amplifier output devices fail closed.
- Tests pass:

  ```sh
  go test ./internal/components ./internal/componentprops ./internal/designworkflow -run 'Amplifier|Output|Thermal|SOA|Selection' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Model amplifier component evidence`.

## Phase 3: Input Buffer And Gain Stage Contracts

Goal: formalize the front-end amplifier blocks.

Tasks:

- Add `amplifier_input_buffer` block contract:
  - passive AC-coupled bias network first;
  - op-amp follower optional if supported by existing op-amp template.
- Extend `opamp_gain_stage` or add `amplifier_gain_stage` alias/contract:
  - requested gain;
  - resistor-ratio calculation;
  - supply mode;
  - feedback placement rules;
  - output drive/load compatibility warnings.
- Add schematic operations with readable left-to-right layout metadata.
- Add PCB realization metadata for feedback proximity and decoupling needs.
- Add block verification manifests.

Acceptance:

- A simple input buffer plus gain stage generates a valid transaction.
- Invalid gain, supply, or missing load assumptions block with clear issues.
- Tests pass:

  ```sh
  go test ./internal/blocks ./internal/designworkflow ./internal/schematicrules -run 'InputBuffer|GainStage|OpAmp|Amplifier' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Add amplifier input and gain stage contracts`.

## Phase 4: Bias Network Block

Goal: add a reusable Class AB bias-network block that is explicit about what it
does and does not prove.

Tasks:

- Add `amplifier_bias_network` or `class_ab_bias_network` block definition.
- Support diode-string bias for low-voltage headphone use.
- Expose ports:
  - `DRIVER_OUT`;
  - `BIAS_N`;
  - `BIAS_P`;
  - `AMP_OUT`;
  - `VCC`;
  - `VEE` or `GND`.
- Add params:
  - diode count;
  - emitter resistor value;
  - target quiescent current;
  - thermal coupling policy.
- Fail closed when quiescent-current evidence is requested but unsupported.
- Add placement hints for diode/output-device thermal proximity.

Acceptance:

- The block can emit a readable schematic fragment for supported headphone use.
- Requests for Vbe multiplier, speaker output, or verified quiescent current
  block until implemented.
- Tests pass:

  ```sh
  go test ./internal/blocks ./internal/designworkflow -run 'Bias|ClassAB|Quiescent|Thermal' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Add Class AB bias network block`.

## Phase 5: Class AB Output Pair Block

Goal: generate a constrained complementary output pair for headphone use.

Tasks:

- Add `class_ab_output_pair` block definition or extend existing output-stage
  code to match the new family contract.
- Select deterministic NPN/PNP seed devices for low-current headphone use.
- Emit:
  - complementary output symbols;
  - emitter resistors;
  - driver/bias/output nets;
  - rail connections;
  - output/load reference connection.
- Add component evidence checks:
  - load impedance;
  - rail voltage;
  - output-current estimate;
  - dissipation estimate;
  - SOA/thermal warning or blocker.
- Add PCB realization metadata:
  - output pair proximity;
  - emitter resistor proximity;
  - rail/output route-width constraints;
  - thermal/current layout hints.

Acceptance:

- Supported headphone output pair generates schematic and PCB fragment
  operations.
- Speaker/high-power requests block with SOA/thermal/protection requirements.
- Tests pass:

  ```sh
  go test ./internal/blocks ./internal/components ./internal/designworkflow -run 'ClassAB|OutputPair|Headphone|Speaker|SOA' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Realize Class AB headphone output pair`.

## Phase 6: Output Protection And Load Connectors

Goal: make the load interface explicit and safe.

Tasks:

- Extend `headphone_output_protection` to align with the family contract.
- Add or formalize `headphone_output_connector` and
  `speaker_output_connector` entries.
- Support headphone AC coupling and load return policy.
- Add calculations:
  - output high-pass cutoff;
  - coupling capacitor voltage warning;
  - load impedance check.
- Block speaker output unless DC fault protection and current/thermal evidence
  exist.
- Add edge-facing connector placement and board-edge anchor evidence where
  applicable.

Acceptance:

- Headphone output chain can be composed with output protection and connector.
- Speaker output remains fail-closed with actionable blockers.
- Tests pass:

  ```sh
  go test ./internal/blocks ./internal/designworkflow ./internal/placement -run 'Headphone|Speaker|Protection|OutputConnector|Load' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Add amplifier output protection connectors`.

## Phase 7: Supply Decoupling Block

Goal: provide local supply evidence for amplifier active devices.

Tasks:

- Add `amplifier_supply_decoupling` block or shared helper.
- Attach decoupling requirements to:
  - op-amp gain/input stages;
  - bias/output stages where applicable;
  - rail splitter or virtual-ground future extension.
- Add schematic and PCB realization:
  - ceramic decoupling;
  - optional bulk capacitors;
  - local-route evidence;
  - rail polarity and voltage rating checks.
- Add validation that active amplifier chains cannot claim candidate readiness
  without required decoupling evidence.

Acceptance:

- Active amplifier blocks report nearby decoupling needs and evidence.
- Missing decoupling blocks candidate/fabrication claims.
- Tests pass:

  ```sh
  go test ./internal/blocks ./internal/designworkflow ./internal/boardvalidation -run 'Amplifier|Decoupling|Supply|Rail' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Add amplifier supply decoupling evidence`.

## Phase 8: Headphone Amplifier Composition

Goal: compose the first complete supported amplifier chain.

Tasks:

- Add deterministic composition for:
  - input buffer;
  - gain stage;
  - bias network;
  - Class AB output pair;
  - output protection;
  - supply decoupling;
  - headphone output connector.
- Add or update AI planner mapping for prompts such as:
  - mono 5 V headphone amplifier;
  - 2x gain;
  - 32 ohm headphone load;
  - AC-coupled output.
- Add rationale output for topology, assumptions, calculations, and blockers.
- Add design example request and metadata.

Acceptance:

- A constrained headphone amplifier prompt generates schematic and PCB project
  artifacts.
- The generated project records explicit readiness and known gaps.
- Unsupported requests ask for clarification or block before file generation.
- Tests pass:

  ```sh
  go test ./internal/intentplanner ./internal/designworkflow ./cmd/kicadai -run 'Amplifier|Headphone|Intent|DesignExamples' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Compose headphone amplifier design lane`.

## Phase 9: Schematic Readability And ERC Closeout

Goal: make the generated amplifier schematic readable and structurally clean.

Tasks:

- Apply schematic layout rules:
  - signal left-to-right;
  - rails top/bottom;
  - feedback above gain stage;
  - bias/output pair grouped clearly;
  - protection/load on the right;
  - decoupling near rails.
- Add strict schematic readability checks for the generated headphone amplifier.
- Fix label aliases, no-connects, pin endpoints, and off-grid stubs.
- Add KiCad-backed ERC expected-fail or candidate fixture depending on real
  evidence.

Acceptance:

- Structural schematic checks pass.
- Readability checks pass for supported amplifier fixture.
- Real KiCad ERC findings are either clean/warning-only or recorded as exact
  expected-fail blockers.
- Tests pass:

  ```sh
  go test ./internal/schematiclayout ./internal/schematicrules ./internal/designworkflow -run 'Amplifier|Readability|ERC|KiCad' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Close amplifier schematic readability and ERC gaps`.

## Phase 10: PCB Placement And Routing Evidence

Goal: make the amplifier PCB electrically meaningful, not merely parseable.

Status: Complete. The Class AB headphone fixture now proves clean local route
contacts plus partial route-tree/contact evidence while preserving the remaining
power/return completion blockers for later closeout work.

Tasks:

- Add placement constraints:
  - input/output separation;
  - output pair proximity;
  - bias thermal proximity;
  - decoupling proximity;
  - edge-facing output connector;
  - quiet-ground/load-return policy.
- Add route constraints:
  - high-current rail/output widths;
  - feedback path constraints;
  - local output loop evidence;
  - load return/contact graph evidence.
- Add validation that required amplifier nets are routed or explicitly blocked.
- Add repair hints for high-current route failures and poor placement.

Acceptance:

- The headphone amplifier PCB emits route/contact evidence for required nets.
- DRC blockers are precise and actionable when present.
- Tests pass:

  ```sh
  go test ./internal/placement ./internal/routing ./internal/designworkflow -run 'Amplifier|Placement|Routing|Contact|DRC' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Add amplifier PCB placement routing evidence`.

## Phase 11: KiCad-Backed Promotion

Goal: promote the first amplifier fixture through real KiCad evidence as far as
the output supports.

Status: Complete. A KiCad-backed Class AB headphone driver fixture now records
the current expected-fail promotion point: component-selection gaps for 1210
capacitors and 0.47-ohm emitter resistors block full workflow promotion before
real ERC/DRC can run.

Tasks:

- Add KiCad-backed fixture metadata for the supported headphone amplifier lane.
- Add fake-runner tests for:
  - clean ERC/DRC;
  - ERC blocker;
  - DRC blocker;
  - missing KiCad CLI.
- Add opt-in real KiCad tests gated by environment variables.
- Ensure `.kicadai/design-promotion.json` records declared and achieved
  readiness.
- Promote from `expected_fail` to `candidate` only when real blockers are gone.

Acceptance:

- Fixture readiness matches promotion-report behavior.
- No silent pass or silent skip is allowed.
- Tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'Amplifier|Promotion|KiCad|DesignExamples' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Add KiCad-backed amplifier promotion fixture`.

## Phase 12: Simulation Netlist Foundation

Goal: prepare simulator-backed amplifier validation without making it a default
dependency.

Tasks:

- Define a simulator-neutral amplifier validation model:
  - operating point;
  - AC gain;
  - high-pass cutoff;
  - output swing/current;
  - optional stability margin.
- Add netlist or sidecar export for supported amplifier blocks.
- Add deterministic tests using checked-in expected netlist fragments.
- Add `simulation_status` to amplifier rationale:
  - `not_supported`;
  - `not_run`;
  - `blocked`;
  - `candidate`;
  - `pass`.
- Keep external simulator execution opt-in.

Acceptance:

- Supported amplifier chain can emit simulation-ready artifacts.
- Simulation is clearly marked `not_run` unless executed.
- Tests pass without requiring an installed simulator:

  ```sh
  go test ./internal/amplifiers ./internal/designworkflow -run 'Simulation|SPICE|Amplifier' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Add amplifier simulation artifact foundation`.

## Phase 13: Simulation-Backed Promotion

Goal: use simulation evidence to promote supported amplifier designs beyond
static structural checks.

Tasks:

- Add optional simulator runner abstraction.
- Support at least one local simulator when configured.
- Validate:
  - gain within tolerance;
  - high-pass cutoff within tolerance;
  - output DC blocked at load;
  - quiescent point exists;
  - load current within modeled limits.
- Record raw and normalized simulation artifacts under `.kicadai/`.
- Feed simulation failures into repair/rationale output.

Acceptance:

- Simulation checks are opt-in and skipped cleanly when unavailable.
- A known-good headphone amplifier simulation fixture passes.
- A deliberately bad fixture fails with actionable diagnostics.
- Tests pass:

  ```sh
  go test ./internal/amplifiers ./internal/designworkflow ./cmd/kicadai -run 'Simulation|Amplifier|Promotion' -count=1
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Validate amplifier designs with simulation evidence`.

## Phase 14: Documentation And AI Skill Update

Goal: make amplifier generation safe for AI agents to use.

Tasks:

- Update:
  - `README.md`;
  - `docs/circuit-blocks.md`;
  - `docs/intent-planning.md`;
  - `docs/kicadai-agent-skill.md`;
  - `docs/ai-readiness.md`;
  - `specs/ROADMAP.md`.
- Document:
  - supported headphone amplifier prompts;
  - blocked power-amplifier prompts;
  - required evidence for candidate/pass/fabrication claims;
  - how to interpret simulation and KiCad evidence;
  - exact commands for generation, validation, and promotion.
- Update examples if generated outputs change.

Acceptance:

- Docs tell users and AI agents what is safe to ask for.
- No doc claims power-amplifier fabrication readiness.
- Tests pass:

  ```sh
  go test ./...
  ```

Review and commit:

- Run `prism review staged`.
- Commit: `Document verified amplifier block family`.

## Phase 15: Full Regression

Goal: prove the new amplifier family did not regress existing generation lanes.

Tasks:

- Run:

  ```sh
  go test ./...
  ```

- Run targeted optional KiCad-backed tests when `kicad-cli` is available.
- Check generated examples that touch amplifier blocks.
- Check `git status --short`.
- Run Prism on staged changes if any remain.

Acceptance:

- Full Go test suite passes.
- Optional KiCad-backed amplifier fixture status is documented and defensible.
- Worktree is clean after final commit.

Review and commit:

- Commit only if Phase 15 produces changes:

  ```text
  Finalize verified amplifier block family regression
  ```
