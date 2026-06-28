# Amplifier Example And Test Corpus Specification

Date: 2026-06-28

## Summary

KiCadAI is intentionally generic, but amplifier designs are a valuable
domain-specific proving ground for schematic semantics, PCB constraints,
component evidence, routing evidence, and fabrication-readiness checks.

This specification defines an amplifier example and test corpus focused on
Class A and Class AB headphone and power amplifiers. The goal is not to make
KiCadAI an analog-design expert immediately. The goal is to add realistic
amplifier fixtures that exercise the writer, validators, circuit-block model,
and future AI design workflow in a domain the project owner actually builds.

## Current Foundations

The repository already contains useful amplifier-related starting points:

- `examples/06_class_ab_headphone_amp/`: checked-in KiCad schematic fixture for
  an op-amp gain stage with diode-biased Class AB headphone output.
- `examples/intent/amplifier_module.json`: structured intent fixture for a
  small amplifier module.
- `examples/intent_text/headphone_amplifier_unverified.txt`: natural-language
  draft input for a Class AB headphone amplifier.
- Existing schematic/project/PCB writers, readers, round-trip checks, and
  writer-correctness tests.
- Existing component catalog foundations for passives, op-amp, connectors,
  LEDs, diodes, regulators, and related evidence.
- Existing placement/routing summaries, contact evidence, and validation
  feedback.

Implementation status as of 2026-06-28:

- `examples/06_class_ab_headphone_amp/`,
  `examples/09_class_a_headphone_amp/`, and
  `examples/10_opamp_buffer_headphone_amp/` are checked-in schematic fixtures
  with amplifier semantic coverage.
- `internal/amplifiers` contains schematic landmark checks and PCB
  constraint/routing evidence checks.
- `examples/intent/` and `examples/intent_text/` include Class A, Class AB, and
  low-voltage power-amplifier intent fixtures that fail closed or expose known
  gaps.
- `examples/design/amplifier/opamp_headphone_buffer.json` is the first draft
  generated amplifier design request.
- `examples/design/kicad-backed/opamp_headphone_buffer_kicad_candidate.*`
  documents the optional KiCad-backed fabrication-candidate `expected_fail`
  path.

These assets are not production analog-design proof. They are fixtures that
prevent regressions and make unsupported amplifier gaps explicit.

This project should expand those foundations with amplifier-specific examples
and tests without forcing unsupported production-layout claims.

## Problem

The current examples mostly cover digital, sensor, LED, regulator, and generic
block workflows. They do not yet stress analog amplifier concerns such as:

- feedback networks and closed-loop gain;
- input coupling and biasing;
- split-rail versus single-supply operation;
- virtual ground or mid-rail references;
- Class A standing-current load/bias behavior;
- Class AB output bias and crossover-control networks;
- complementary output devices and matched/paired placement intent;
- output zobel/snubber/load stability networks;
- local decoupling around active devices;
- high-current output routing;
- thermal spacing and heat-sink regions;
- grounding topology and input/output separation.

Without amplifier-specific fixtures, the project can regress in ways that are
invisible to LED/sensor tests. A schematic may parse, but omit a feedback
return. A PCB may write, but place output devices poorly or route input near
output/high-current paths. A generated intent may say "headphone amplifier"
without selecting the required topology or reporting that the needed topology is
unsupported.

## Goals

- Add checked-in amplifier examples ranging from simple to complex.
- Add KiCad-independent tests that verify amplifier schematic/project
  structure and semantic requirements.
- Add structured design/intent fixtures for amplifier workflows.
- Add amplifier-specific semantic validators or test helpers for:
  - input/output connectors;
  - power rails;
  - feedback networks;
  - bias/reference nets;
  - output stage components;
  - decoupling;
  - protection/stability networks where modeled.
- Define amplifier circuit-block families and explicit gaps.
- Add PCB-placement/routing expectations for amplifier blocks without claiming
  production DRC-clean status before evidence exists.
- Keep default tests deterministic and independent of local KiCad.
- Use optional KiCad-backed fixtures only when local `kicad-cli` evidence is
  configured.

## Non-Goals

- Do not promise analog correctness from the first implementation.
- Do not design a high-power mains-connected amplifier supply.
- Do not add unsafe or uncertified protection claims.
- Do not require live supplier, pricing, or SPICE simulation data.
- Do not promote generated amplifier PCBs to fabrication-ready unless writer,
  board validation, optional KiCad ERC/DRC, and fabrication checks support that
  claim.
- Do not replace human analog review for bias stability, oscillation margin,
  thermal safety, or listening/load testing.

## Example Corpus

### Schematic Fixture Examples

The checked-in KiCad example set should grow from the existing
`06_class_ab_headphone_amp` into a named amplifier corpus:

- `09_class_a_headphone_amp`
  - single-ended Class A headphone amplifier;
  - input coupling/bias network;
  - active gain device or op-amp/VAS driver;
  - Class A output load or current source;
  - output coupling or split-rail output policy;
  - local decoupling and clear signal labels.
- `10_opamp_buffer_headphone_amp`
  - op-amp gain stage plus discrete emitter/source follower buffer;
  - feedback from output node where appropriate;
  - output resistor and optional zobel/stability network;
  - input/output connectors.
- `11_class_ab_power_amp_skeleton`
  - low-voltage safe educational Class AB power amplifier skeleton;
  - differential/input stage or op-amp/VAS abstraction;
  - Vbe multiplier or diode bias spreader;
  - complementary output devices;
  - emitter/source resistors;
  - feedback network;
  - speaker/load output connector;
  - supply decoupling.

The existing `06_class_ab_headphone_amp` should remain, but tests should begin
asserting that it contains the required amplifier landmarks.

### Design Workflow Fixtures

Add structured design requests under `examples/design/` or
`examples/design/kicad-backed/` once block support exists:

- `class_a_headphone_amp.json`
- `class_ab_headphone_amp.json`
- `opamp_buffer_headphone_amp.json`
- `class_ab_power_amp_skeleton.json`

Early fixtures may be `expected_fail` or blocked if no safe generator exists.
That is acceptable if the failure is explicit and tested.

### Intent Fixtures

Add or expand intent examples:

- "Class A headphone amplifier with 3.5 mm input/output and 9 V supply."
- "Class AB headphone amplifier with op-amp gain stage and discrete buffer."
- "Small low-voltage Class AB power amplifier skeleton for 8 ohm load."

Intent handling should either map to supported structured requests or return
precise known gaps such as unsupported output stage, insufficient component
evidence, missing thermal constraints, or no verified layout proof.

## Amplifier Semantic Checks

Amplifier tests should verify more than file parseability. Each fixture should
declare expected landmarks and tests should assert them.

### Common Checks

- Project contains `.kicad_pro`, `.kicad_sch`, `.kicad_prl`, and
  `sym-lib-table` where applicable.
- Schematic parses through the project reader.
- Required references or roles exist.
- Required net labels exist:
  - input;
  - output;
  - ground or reference;
  - positive rail;
  - negative rail or virtual ground where applicable;
  - feedback;
  - bias.
- No duplicate references.
- Expected values are present for key passives.
- Power pins and decoupling components are present near active stages in the
  modeled metadata.

### Headphone Amplifier Checks

- Input connector or jack exists.
- Output/headphone connector or jack exists.
- Output has a current-limiting/isolation resistor where topology requires it.
- Headphone load is not directly tied to a DC bias node unless coupling or
  split-rail policy is explicit.
- Class AB stages include bias diodes, Vbe multiplier, or explicit unsupported
  gap.
- Class A stages include standing-current load/current source/resistor evidence.

### Power Amplifier Checks

- Output device pair or bridge/push-pull structure is declared.
- Emitter/source resistors exist when modeled.
- Feedback network returns from the output-side node, not only the driver node.
- Zobel/snubber/stability network exists or is explicitly omitted with a
  tested rationale.
- Output connector/load is separated from small-signal input in placement
  intent.
- High-current output and supply nets carry width/current intent.
- Thermal/heat-sink placement constraints exist for output devices.

## Circuit Block Model

Add amplifier-oriented block definitions gradually. Initial blocks may be
structural and `expected_fail` for PCB realization until verified parts and layout
proof mature.

Recommended block families:

- `amplifier_input_stage`
  - input connector, coupling capacitor, bias resistor/reference.
- `opamp_gain_stage`
  - op-amp, feedback resistor, gain resistor, rail/reference handling.
- `class_a_output_stage`
  - active device, load/current source, output coupling policy.
- `class_ab_output_stage`
  - complementary devices, bias spreader, emitter/source resistors,
    output isolation resistor.
- `headphone_output_connector`
  - jack/connector, optional series resistors, ground/reference policy.
- `speaker_output_connector`
  - speaker/load connector, high-current net policy.
- `stability_network`
  - zobel/snubber/output RC network.
- `amplifier_power_entry`
  - low-voltage DC input, split rail or virtual ground, decoupling.

Each block should expose:

- exported ports;
- required nets;
- required components or explicit gaps;
- schematic landmark metadata;
- placement constraints;
- routing constraints;
- verification manifests.

## Component And Library Requirements

The first implementation may use generic symbols where existing fixtures
already do, but generated design workflows should eventually prefer verified
parts:

- op-amp packages with supply range and output-drive evidence;
- small-signal BJTs or MOSFETs for output buffers;
- complementary power BJTs/MOSFETs for power output stages;
- diodes or Vbe multiplier transistor for bias;
- emitter/source resistors with power rating;
- output resistors and zobel resistors/capacitors with rating evidence;
- input/output jacks/connectors;
- decoupling capacitors with voltage rating.

Any unverified active part must be reported as a known gap for generated
designs and must not be treated as fabrication-ready.

## PCB And Layout Expectations

Amplifier PCB realization should be introduced conservatively.

### Placement Expectations

- Keep input connector and small-signal stage away from output connector and
  high-current output path.
- Place feedback components close to the gain stage and route feedback from the
  correct output-side node.
- Place output transistor pairs symmetrically where applicable.
- Place emitter/source resistors near output devices.
- Place local decoupling near op-amp and output stage supply pins.
- Reserve thermal regions or heat-sink edge/courtyard intent for output
  devices.
- Keep star-ground or reference-return intent explicit when modeled.

### Routing Expectations

- Mark output and supply nets with high-current or width policy where
  applicable.
- Mark input nets as sensitive/analog and prefer short, quiet routing.
- Keep feedback loop short and avoid routing it near high-current output paths.
- Report partial/blocked routing when contact proof, clearance, or DRC evidence
  is incomplete.

## Validation Requirements

Default tests should verify:

- checked-in amplifier examples parse;
- required project files exist;
- schematic semantic landmarks exist;
- structured intent fixtures either map to supported blocks or fail closed with
  explicit known gaps;
- generated amplifier design requests do not silently produce unsupported
  fabrication-ready claims.

Optional KiCad-backed tests should verify:

- examples load through KiCad CLI when configured;
- generated amplifier fixtures record ERC/DRC status or `expected_fail` evidence;
- any candidate/pass promotion is backed by artifacts.

## Acceptance Criteria

- Existing `06_class_ab_headphone_amp` receives semantic regression coverage.
- At least two additional amplifier schematic fixtures are checked in and
  covered by tests.
- Amplifier intent examples fail closed or map to supported structured design
  requests with explicit rationale.
- Amplifier-specific checks can detect missing feedback, missing output
  connector, missing bias/reference, and missing decoupling in representative
  fixtures.
- Circuit-block inventory reports amplifier block support and gaps.
- Documentation explains which amplifier examples are schematic-only,
  generated, optional KiCad-backed, candidate, or `expected_fail`.
- Normal `go test ./...` remains independent of local KiCad.

## Current Completion Notes

The initial corpus phases are implemented through optional KiCad-backed
metadata. The next amplifier-specific milestone should be a verified headphone
output/protection block that realizes a real output DC-blocking or split-rail
policy, output isolation/protection, load-drive component evidence, and local
PCB constraints. Until that exists, generated amplifier designs must remain
draft or `expected_fail` evidence fixtures.

## Open Questions

- Should the first generated amplifier block use an op-amp-based headphone
  amplifier, because the catalog already has op-amp support, or a discrete
  Class A topology, because it better matches the desired domain?
- Should amplifier semantic checks live in `internal/designworkflow`, a new
  `internal/amplifiers` package, or generic schematic semantics?
- How much amplifier-specific electrical checking belongs in KiCadAI before
  external SPICE simulation or domain review becomes necessary?
- Which verified output devices and packages should be added first?
- Should power-amplifier examples stay low-voltage educational fixtures until
  stronger thermal/current/fabrication checks exist?
