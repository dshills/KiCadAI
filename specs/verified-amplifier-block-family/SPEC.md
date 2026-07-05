# Verified Amplifier Block Family Specification

## Purpose

Build a verified amplifier block family that lets KiCadAI generate credible
amplifier schematics and PCB fragments from AI intent while failing closed for
unsafe, unsupported, or unverified power-amplifier requests.

The first supported target is a low-voltage educational headphone amplifier.
The long-term target is a composable amplifier library that can support input
buffers, gain stages, bias networks, Class AB output stages, output protection,
supply decoupling, speaker/headphone outputs, KiCad ERC/DRC promotion, and
eventually simulation-backed validation.

This is not a claim that AI-generated power amplifiers are fabrication-ready.
Fabrication readiness requires real KiCad ERC/DRC evidence, verified component
ratings, thermal/current evidence, and simulation evidence where appropriate.

## Existing Foundation

The project already has:

- `opamp_gain_stage` with a narrow LMV321-style non-inverting topology;
- `dc_blocking_capacitor`;
- `headphone_output_protection`;
- seeded MMBT3904/MMBT3906 amplifier output-device records;
- amplifier AI readiness records in `data/ai-readiness/matrix/amplifier.json`;
- amplifier examples and schematic readability rules;
- KiCad-backed promotion fixtures, including an amplifier fixture that remains
  `expected_fail`;
- component selection, pinmap, footprint assignment, placement, routing,
  validation, fabrication readiness, and promotion-report infrastructure.

The current gaps are mostly analog correctness, richer block contracts,
schematic/PCB layout quality, KiCad-backed evidence, and simulation.

## Supported Design Tiers

### Tier 0: Logic And Metadata

The system can describe amplifier requirements, reject unsupported requests,
and emit rationale without generating a complete board.

### Tier 1: Schematic Connectivity

The system can generate parseable, readable, ERC-oriented schematic fragments
for supported headphone amplifier blocks. Structural and schematic-rule tests
must pass, but real KiCad ERC may still be optional or expected-fail.

### Tier 2: KiCad Candidate

The system can generate a complete KiCad project with schematic and PCB
fragments for the supported headphone amplifier lane. Real KiCad ERC must pass
or be warning-only under explicit policy. KiCad DRC may remain candidate-level
with documented warnings.

### Tier 3: Simulation Candidate

The system can generate a SPICE/simulation netlist or project sidecar for a
supported amplifier slice and prove basic gain, bias, output swing, and
stability/compensation checks.

### Tier 4: Fabrication Candidate

The system can generate a board with clean real KiCad ERC/DRC, verified
component ratings, pinmaps, thermal/current evidence, package metadata, and
explicit unsupported-risk boundaries. This tier is not in scope for the first
implementation wave.

## Block Family

### Input Buffer

Purpose: isolate the input source, provide biasing, optional coupling, and
defined input impedance.

Required capabilities:

- exported ports: `IN`, `OUT`, `VCC`, `VEE` or `GND`, optional `BIAS_REF`;
- selectable topology:
  - passive AC-coupled input bias network;
  - op-amp voltage follower;
  - optional RF/input stopper resistor;
- parameters:
  - input impedance target;
  - coupling capacitor value or low-frequency cutoff;
  - supply mode;
  - source impedance assumption;
- evidence:
  - input impedance calculation;
  - high-pass cutoff calculation when AC-coupled;
  - op-amp input common-mode and supply compatibility for active buffers;
  - no floating input/output pins.

### Gain Stage

Purpose: provide controlled voltage gain with stable feedback.

Required capabilities:

- exported ports: `IN`, `OUT`, `VCC`, `VEE` or `GND`, optional `BIAS_REF`;
- supported first topology: non-inverting op-amp stage;
- future topologies:
  - inverting op-amp;
  - discrete BJT/FET small-signal stage;
- parameters:
  - voltage gain;
  - feedback resistor family/package;
  - bandwidth or compensation policy;
  - supply mode;
- evidence:
  - resistor ratio calculation;
  - op-amp gain-bandwidth margin warning or blocker;
  - output drive/load compatibility;
  - feedback components placed close to op-amp pins;
  - readable feedback-loop schematic layout.

### Bias Network

Purpose: generate Class AB output bias safely and visibly.

Required capabilities:

- exported ports: `DRIVER_OUT`, `BIAS_N`, `BIAS_P`, `AMP_OUT`, `VCC`, `VEE`;
- first topology: diode-string bias for headphone/low-current output;
- future topology: Vbe multiplier with thermal coupling;
- parameters:
  - diode count;
  - target quiescent current;
  - emitter resistor value;
  - thermal coupling policy;
- evidence:
  - fail-closed status when quiescent current cannot be calculated;
  - diode/transistor thermal-coupling placement hint;
  - clear unsupported marker for speaker/power-output bias without SOA proof.

### Class AB Output Pair

Purpose: drive headphone or speaker loads through complementary emitter/source
followers.

Required capabilities:

- exported ports: `DRIVER_OUT`, `VCC`, `VEE`, `AMP_OUT`, `LOAD_REF`;
- supported first topology:
  - low-voltage complementary BJT emitter follower;
  - MMBT3904/MMBT3906 or verified low-current alternatives;
  - headphone load only;
- future topologies:
  - power BJT pair;
  - MOSFET source follower;
  - bridged output;
- parameters:
  - load impedance;
  - rail voltage;
  - maximum output current;
  - emitter/source resistor values;
  - device pair preference;
- evidence:
  - output current estimate;
  - dissipation estimate;
  - package thermal warning/blocker;
  - SOA blocker unless verified;
  - matched NPN/PNP or N/P device selection;
  - local high-current route-width constraints.

### Output Protection

Purpose: prevent unsafe DC or fault behavior at the load connector.

Required capabilities:

- exported ports: `AMP_OUT`, `HP_OUT` or `SPK_OUT`, `LOAD_REF`, `LOAD_RET`;
- supported first topology:
  - headphone DC-blocking capacitor;
  - load/reference diagnostics;
  - optional output bleeder/load resistor;
- future topologies:
  - relay muting;
  - DC detect;
  - current limiting;
  - speaker fusing/PTC;
  - Zobel/snubber network;
- evidence:
  - high-pass cutoff calculation;
  - capacitor voltage/ripple/current warning;
  - load return policy;
  - fail-closed for speaker output without DC fault protection.

### Supply Decoupling

Purpose: provide local rail stability for active amplifier blocks.

Required capabilities:

- exported ports: `VCC`, `VEE` or `GND`, optional `BIAS_REF`;
- components:
  - local ceramic decoupling;
  - optional bulk capacitors;
  - optional rail splitter/virtual ground for single-supply headphone designs;
- evidence:
  - every active device has nearby decoupling;
  - rail polarity and voltage rating checks;
  - local placement and route evidence;
  - return-current placement hints.

### Speaker/Headphone Output

Purpose: expose the load interface with correct connector, return, protection,
and mechanical placement constraints.

Required capabilities:

- exported ports:
  - headphone: `HP_L`, `HP_R`, `HP_RET` or mono `HP_OUT`, `HP_RET`;
  - speaker: `SPK_OUT`, `SPK_RET`;
- supported first connector:
  - mono/stereo headphone connector or generic header;
- evidence:
  - edge-facing connector placement;
  - load impedance requirement;
  - current/thermal blockers for speaker load;
  - no silent short between output and return;
  - board-edge anchor binding where applicable.

## Composition Requirements

The amplifier family must support the following deterministic chains:

1. `input_buffer -> gain_stage -> output_pair -> output_protection -> load`
2. `input_buffer -> gain_stage -> bias_network -> output_pair`
3. `supply_decoupling` attached to every active stage.

Composition must preserve named nets:

- signal nets: `AUDIO_IN`, `BUFFER_OUT`, `GAIN_OUT`, `DRIVER_OUT`, `AMP_OUT`,
  `HP_OUT`, `SPK_OUT`;
- rail nets: `VCC`, `VEE`, `GND`, `BIAS_REF`, `LOAD_REF`, `LOAD_RET`;
- bias nets: `BIAS_N`, `BIAS_P`, `UPPER_BASE`, `LOWER_BASE`,
  `UPPER_EMITTER`, `LOWER_EMITTER`.

The planner must reject ambiguous amplifier requests when it cannot determine:

- headphone versus speaker load;
- single-supply versus dual-supply rails;
- load impedance;
- output power/current target;
- whether fabrication readiness is requested.

## Schematic Requirements

Generated amplifier schematics must be human-readable:

- signal flow left-to-right;
- higher voltage rails above the signal path;
- lower rails/ground toward the bottom;
- feedback loops above or around the gain stage;
- bias network near the output devices but not overlapping them;
- decoupling near active-device supply pins;
- output protection and load connector to the right of the output stage;
- all required no-connects explicit;
- no conflicting labels on a connected segment;
- no off-grid label stubs, wire endpoints, or pin anchors.

## PCB Requirements

Generated amplifier PCB fragments must expose placement and routing intent:

- input and output connectors separated;
- high-current output loops short and wide;
- feedback path away from output-current loop where possible;
- bias/thermal coupling constraints between bias device and output pair;
- decoupling capacitors close to active-device supply pins;
- star/quiet-ground or load-return policy recorded as evidence;
- output connector edge-facing where requested;
- high-current route-width constraints on output and rail nets;
- keepout/thermal regions for output devices where modeled;
- explicit blockers for unmodeled heatsinks, chassis, speaker current, or
  mains/line-voltage designs.

## Component And Library Requirements

Every promoted amplifier block must use component records with:

- KiCad symbol ID;
- footprint ID;
- verified or explicitly blocked pinmap;
- package and thermal metadata where relevant;
- voltage/current/power ratings when used in a calculation;
- lifecycle/procurement status where fabrication readiness is requested;
- role-specific evidence:
  - op-amp drive/stability;
  - output transistor SOA/thermal;
  - coupling capacitor voltage and impedance;
  - protection device fault rating.

Generic placeholders are allowed only below candidate readiness and must emit
warnings or blockers in AI-facing rationale.

## Validation Requirements

### Structural Validation

- block manifests validate;
- transactions validate;
- schematic rules detect no label conflicts or accidental disconnected pins for
  supported Tier 1 cases;
- local route and entry-anchor evidence exists for PCB-capable blocks.

### KiCad ERC/DRC

- every block entering KiCad corpus must have optional fake-runner tests and
  opt-in real `kicad-cli` coverage;
- KiCad-backed design fixtures must record declared and achieved readiness in
  `.kicadai/design-promotion.json`;
- expected failures must include exact known gaps;
- no fixture may silently pass or silently skip required ERC/DRC evidence.

### Simulation

Simulation is a later validation tier. The first simulation target should be a
small-signal headphone chain:

- AC gain within tolerance;
- high-pass cutoff within tolerance;
- output DC blocked at load;
- quiescent operating point available;
- output swing/load current within modeled limits;
- basic stability/phase-margin evidence when the selected simulator supports it.

The project may initially emit simulator-ready netlists and mark simulation as
`not_run`, then promote to `candidate` only after automated checks exist.

## AI Workflow Requirements

AI-facing outputs must:

- explain chosen topology and rejected alternatives;
- list assumptions: rail voltage, load impedance, gain, target output power,
  supply type, headphone/speaker use;
- expose blockers before writing files when the request is unsafe or
  underspecified;
- never claim fabrication readiness without real KiCad and component evidence;
- recommend the next missing evidence item.

Example supported prompt:

> Build a mono 5 V headphone amplifier with 2x gain, 32 ohm load, AC-coupled
> output, and a generic 3.5 mm output connector.

Example fail-closed prompt:

> Build a 100 W Class AB speaker amplifier.

The second request must produce a blocked rationale until power-device SOA,
thermal, protection, supply, and simulation evidence exist.

## Acceptance Criteria

- The block inventory lists the full amplifier family with implemented,
  planned, and blocked status.
- Input buffer, gain stage, bias network, Class AB output pair, output
  protection, supply decoupling, and headphone/speaker output have explicit
  contracts.
- At least one low-voltage headphone amplifier composition reaches Tier 1
  schematic connectivity.
- At least one KiCad-backed amplifier fixture records exact expected-fail
  blockers.
- Unsupported power-amplifier requests fail closed with actionable rationale.
- Documentation tells AI agents what is supported, what is blocked, and what
  evidence is needed for promotion.

