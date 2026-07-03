# Amplifier Output Protection And Load Safety Spec

## Purpose

The previous amplifier slice made the low-voltage Class AB headphone output
stage connectivity-capable: KiCadAI can select the seeded MMBT3904/MMBT3906
pair, realize a diode-biased complementary output stage, add a DC-blocking
capacitor, and explain why the result is still not fabrication-ready.

The next blocker in the AI readiness graph is:

- `amplifier.block.output_protection`

That record is now `connectivity`, but it still blocks KiCad-backed amplifier
promotion because load safety, bleed policy, connector constraints, fault
protection, and validation evidence are incomplete. This spec defines the next
implementation slice: a conservative headphone output-protection and load-safety
model that can be generated, diagnosed, and tested without pretending to solve
power-amplifier protection.

The goal is to let AI-generated headphone amplifier designs move from "has an
output coupling capacitor" toward "has explicit, reviewable load-safety
assumptions." It must still fail closed for speaker loads, high-power outputs,
unknown headphones, missing return paths, missing bleed/load policy, or
unverified fault-protection claims.

## Scope

- Add a reusable headphone output-protection/load-safety block contract.
- Model the minimum supported low-voltage headphone output network:
  - AC output coupling through a DC-blocking capacitor;
  - post-capacitor load/output connector;
  - optional or required bleed/reference resistor policy;
  - optional series output resistor policy for damping/current limiting;
  - explicit load impedance evidence for 16 ohm, 32 ohm, and 64 ohm headphone
    classes;
  - optional placeholder fault-protection metadata that remains blocked until
    verified components and rules exist.
- Connect the protection block to the existing Class AB headphone driver fixture
  and design workflow diagnostics.
- Extend AI-facing block summaries so agents can distinguish:
  - missing DC blocking;
  - present DC blocking but missing bleed/load safety evidence;
  - supported headphone-load assumptions;
  - blocked speaker/power-amplifier requests.
- Update readiness records and docs only to the level supported by tests.

## Non-Goals

- Do not claim speaker protection or power-amplifier output readiness.
- Do not support relays, crowbar circuits, DC-servo protection, muting FETs, or
  active headphone protection in this slice.
- Do not claim IEC, UL, product-safety, or hearing-safety compliance.
- Do not calculate real headphone SPL, acoustic safety, or transducer damage
  limits.
- Do not infer safe operating area from incomplete component metadata.
- Do not promote the KiCad-backed amplifier fixture to `candidate` or `pass`
  unless separate KiCad evidence is produced by later work.
- Do not add remote datasheet or distributor dependencies.

## Supported Design Envelope

This slice supports only educational, low-voltage headphone outputs:

- single-ended headphone output;
- low-voltage supply, normally 5 V to 12 V;
- load classes represented as nominal impedance: 16 ohm, 32 ohm, or 64 ohm;
- Class AB emitter-follower output stage or op-amp buffer source;
- AC-coupled output with a DC-blocking capacitor between amplifier output and
  headphone connector;
- connector return tied to the selected load reference or analog ground;
- no bridge-tied load, no differential headphone drive, no speaker load.

When the request asks for a speaker, unbounded load, high-power output, bridge
output, or load impedance below the supported headphone range, the workflow must
surface a blocking issue rather than generating a protection network that looks
safe.

## Data Model Requirements

### Headphone Load Safety Metadata

The protection/load-safety block must expose structured metadata for:

- `load_kind`: initially `headphone`;
- `nominal_load_ohms`: supported values are 16, 32, and 64 unless an explicit
  review-only mode is added;
- `coupling`: `ac_coupled_required`, `ac_coupled_present`, or
  `dual_rail_direct_review_required`;
- `dc_blocking_capacitance`: value inherited from, or emitted to, the
  `dc_blocking_capacitor` block;
- `bleed_resistor_ohms`: optional/recommended value and whether it is required;
- `series_resistor_ohms`: optional/recommended value and whether it is required;
- `connector_return_policy`: `load_ref`, `analog_ground`, or blocked unknown;
- `fault_protection_status`: `not_modeled`, `placeholder_blocked`, or future
  `connectivity`;
- `readiness`: `draft`, `connectivity`, `candidate`, or `blocked`;
- `known_limits`: human/AI-readable limitations that must appear in rationale
  output.

The metadata must not hide safety limits in prose only. Important state must be
machine-readable so AI callers can branch on it.

### New Or Extended Block Contract

The implementation may choose either of these conservative shapes:

1. A new block, `headphone_output_protection`, that composes or references
   existing `dc_blocking_capacitor` behavior.
2. An extended `dc_blocking_capacitor` block plus a separate
   `headphone_load_safety` block.

The preferred shape is a new `headphone_output_protection` block because it
keeps electrical intent explicit:

- `AMP_OUT`: input from amplifier output or buffer output before AC coupling;
- `HP_OUT`: AC-coupled output to headphone connector signal;
- `LOAD_RET`: connector return/load reference;
- `LOAD_REF`: analog ground or load reference;
- optional `FAULT_REF`: reserved future protection reference.

The block should emit schematic operations for:

- DC-blocking capacitor;
- optional bleed/reference resistor where policy requires it;
- optional series output resistor where policy requires it;
- output connector signal/return labels;
- clear node labels: `AMP_OUT_DC_BIASED`, `HP_OUT`, `LOAD_REF`.

If the block cannot emit all required elements with current writer primitives,
it must still declare the missing operations as known gaps and keep readiness at
`draft` or `connectivity`, not `candidate`.

## Component Requirements

This slice may reuse existing concrete passive seeds if available. If catalog
coverage is insufficient, it must remain conservative:

- DC-blocking capacitor:
  - must have voltage rating evidence or a blocking warning when missing;
  - polarized capacitor orientation must be deterministic for single-supply
    outputs;
  - capacitance defaults may remain educational but must be visible.
- Bleed/load resistor:
  - must use resistor component policy compatible with low-power load-reference
    behavior;
  - default value must be documented and test-covered;
  - absence must produce a diagnostic when the block policy requires it.
- Series output resistor:
  - may be optional in the first implementation;
  - if emitted, must have a clearly named purpose and route role;
  - must not be described as complete short-circuit protection.
- Fault protection:
  - may appear only as a blocked placeholder or known gap in this slice.

## Workflow Behavior

### Planner And Block Diagnostics

The design workflow must summarize headphone output-protection state for each
supported amplifier output path:

- whether AC output coupling through a DC-blocking capacitor is present;
- whether the load connector has both signal and return/reference;
- whether the nominal load class is supported;
- whether bleed/reference policy is satisfied or intentionally skipped;
- whether series output resistance is present or intentionally skipped;
- whether fault protection is not modeled and remains a blocker for higher
  readiness;
- why the design is `connectivity`, `blocked`, or still `draft`.

Diagnostics must be stable JSON values suitable for AI agents. Free-text notes
are allowed, but the core state must be structured.

### Intent Planner

For supported Class AB headphone amplifier intent, the intent planner should be
able to request the protection/load-safety block explicitly once this work is
implemented. If the natural-language or structured intent asks for:

- "headphone output",
- "3.5 mm headphone jack",
- "32 ohm headphones",
- "AC-coupled headphone output",

then the generated design request should include the protection block where the
current mapper has enough context. If context is missing, the plan should ask
for clarification or emit a known gap instead of silently omitting protection.

Power-amplifier, speaker, bridge-output, or unknown load intent must remain
blocked.

### Rationale Output

Rationale artifacts should explain:

- the selected load class;
- why AC coupling is required for single-supply headphone outputs;
- what DC-blocking capacitor policy was applied;
- whether bleed/reference and series output resistor policies were applied;
- why fault protection remains incomplete;
- what evidence is still needed before KiCad-backed amplifier promotion.

## Schematic Readability Requirements

Generated headphone output-protection fragments must follow the established
schematic readability rules:

- signal flows left to right;
- amplifier output is left of the coupling/protection network;
- output connector is to the right;
- lower-voltage/reference symbols are below signal path;
- labels do not overlap components or wires;
- capacitor polarity is visually clear when a polarized capacitor is emitted;
- load/reference return is not hidden behind a generic unlabeled wire.

## PCB/Layout Requirements

This slice does not need to produce DRC-clean amplifier PCBs. It must, however,
prepare useful downstream evidence:

- output path nets should be role-tagged for routing/current review;
- connector signal and return anchors should be discoverable;
- optional series resistor and DC-blocking capacitor should be placed between
  amplifier output and connector in schematic and PCB realization order;
- fault-protection placeholders must not create misleading DRC-clean claims;
- missing high-current/thermal proof must remain a readiness blocker.

## Validation Requirements

Default tests must not require KiCad or network access.

Tests must cover:

- block manifest/model validation for headphone output protection;
- supported 16/32/64 ohm headphone load classes;
- blocked load classes for speakers, unknown loads, bridge outputs, or
  unsupported low impedance;
- generated schematic operations and landmarks for capacitor, connector,
  optional bleed resistor, optional series resistor, and labels;
- deterministic polarity/orientation behavior for single-supply output
  coupling;
- diagnostics for missing DC blocking, missing return/reference, missing bleed
  policy, and unmodeled fault protection;
- integration with the Class AB headphone driver design request;
- AI readiness behavior for `amplifier.block.output_protection`.

Optional tests may add KiCad-backed evidence later, but this spec does not
require real `kicad-cli` execution.

## Documentation Requirements

Docs must explain:

- supported headphone-only scope;
- why AC coupling through a DC-blocking capacitor is required for single-supply
  headphone outputs;
- what bleed/reference and series output resistors mean in this project;
- what is not protected: shorts, overloads, speaker outputs, acoustic safety,
  bridge outputs, product safety, and power-amplifier faults;
- how AI agents should interpret `connectivity` versus `candidate` for
  amplifier output protection.

## Acceptance Criteria

- `go test ./...` passes.
- A supported headphone output-protection/load-safety block contract exists.
- Supported Class AB headphone design requests can include explicit output
  protection/load-safety state.
- Unsupported loads fail closed with structured diagnostics.
- Workflow summaries expose machine-readable output-protection state.
- The AI readiness matrix is advanced only as far as implemented evidence
  supports.
- Documentation makes clear that this is not speaker or power-amplifier
  protection.
