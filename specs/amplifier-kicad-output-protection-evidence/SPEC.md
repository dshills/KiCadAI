# Amplifier KiCad Output Protection Evidence Specification

## Purpose

Capture KiCad-backed evidence for the protected Class AB headphone amplifier
path without overclaiming fabrication readiness.

The AI readiness matrix now marks `amplifier.block.output_protection` as
`connectivity` and names `capture_kicad_evidence` as the next task. The
implementation already has:

- a `headphone_output_protection` block;
- Class AB headphone intent mapping that requests the protection block;
- a connectivity-level `class_ab_headphone_driver` design example;
- structured workflow and rationale summaries for headphone output protection.

The current optional KiCad-backed amplifier fixture is still
`opamp_headphone_buffer_kicad_candidate`, an older expected-fail seed that
intentionally blocks before real ERC/DRC promotion. This spec defines the next
slice: create a protected Class AB headphone amplifier evidence fixture, run it
through the generated-design promotion workflow, and record exactly what KiCadAI
can and cannot prove.

## Goals

- Add or replace an optional KiCad-backed amplifier fixture that uses the
  protected Class AB headphone path:
  - `opamp_gain_stage`;
  - `class_ab_output_stage`;
  - `headphone_output_protection`;
  - headphone output connector;
  - explicit supply and load-reference connections.
- Make promotion evidence report the protected output path directly, including:
  - block-planning summary for `headphone_output_protection`;
  - schematic electrical evidence;
  - PCB realization evidence where current writers can emit it;
  - route/contact evidence where routing is requested;
  - writer correctness and board validation evidence;
  - optional real KiCad ERC/DRC evidence when `KICADAI_KICAD_CLI` is configured.
- Keep readiness conservative:
  - `expected_fail` if current blockers remain;
  - `candidate` only if generated artifacts, validation, and optional KiCad
    gates satisfy the existing promotion policy;
  - never `pass` unless clean evidence supports it.
- Replace stale amplifier fixture blockers with current, specific blockers by
  stage, net, or validation category.
- Update docs and the readiness matrix only to the level proven by tests and
  fixture evidence.

## Non-Goals

- Do not claim a fabrication-ready headphone amplifier in this slice.
- Do not implement active fault protection, relays, mute circuits, DC servos, or
  short-circuit protection.
- Do not support speaker loads, bridge outputs, power-amplifier outputs, or
  loads outside the existing headphone envelope.
- Do not prove analog stability, oscillation margin, thermal safe operating
  area, or product safety compliance.
- Do not make real KiCad required for the default `go test ./...` suite.
- Do not globally suppress KiCad ERC/DRC findings to force promotion.
- Do not replace the broader amplifier roadmap; this work only captures the
  first protected-output KiCad evidence.

## Fixture Contract

The preferred fixture name is:

```text
examples/design/kicad-backed/class_ab_headphone_protected.json
```

with metadata:

```text
examples/design/kicad-backed/class_ab_headphone_protected.metadata.json
```

The fixture should model a low-voltage, single-ended headphone amplifier:

- a two-layer board with a deterministic prototype envelope, initially 80 mm x
  55 mm unless the fixture metadata records a reason to change it;
- op-amp voltage gain stage;
- diode-biased complementary Class AB output stage;
- `headphone_output_protection` for AC output coupling and load safety;
- input connector with signal and ground;
- headphone connector with signal and return;
- power connector or rail source with VCC and GND;
- explicit nets:
  - `AUDIO_IN`;
  - a bias-reference net, using `BIAS_REF` only when it is the exported alias
    for the op-amp gain-stage mid-rail reference rather than a conflicting
    second bias node;
  - `AMP_DRIVE`;
  - `AMP_OUT_DC_BIASED`;
  - `HP_OUT`;
  - `LOAD_REF`;
  - `VCC`;
  - `GND`.

The initial fixture is single-supply, using `VCC` and `GND` rails and a
mid-rail bias reference for the small-signal path. The protected output remains
AC-coupled; dual-rail direct-coupled amplifier variants are out of scope for
this fixture.

For this fixture, `LOAD_REF` is the headphone return reference and must be
tied to `GND` at one explicit star/reference connection. `HP_RET` is the
connector-side return port attached to the `LOAD_REF` net; it is not a separate
bridge-output, virtual-ground, or Kelvin-sense net in this fixture. Future work
may add Kelvin-return or virtual-ground variants, but this fixture uses a
single-ended grounded headphone return only.

Candidate promotion requires `LOAD_REF` and `GND` to remain distinct nets until
one explicit star point. That star/reference connection must be represented by
a net-tie schematic symbol plus matching net-tie footprint, or an equivalent
explicit tie operation, so KiCad and the promotion report can preserve evidence
of the physical return path. The preferred tie location is the output connector
return area or the main power return point selected by the layout policy, with
the location recorded in placement evidence. If the current writer collapses
them into one KiCad net before PCB realization, the fixture must remain
expected-fail with separated return-layout evidence listed as not yet proven.

The single-supply audio path must include an input/op-amp bias reference,
normally a mid-rail divider or the existing `opamp_gain_stage` bias network.
The op-amp non-inverting input must have a DC bias path through a resistor or
equivalent modeled impedance to the bias reference, and `AUDIO_IN` must be AC
coupled into that biased node. The fixture cannot be promoted beyond
expected-fail if the input node can float.

The existing `opamp_headphone_buffer_kicad_candidate` fixture may remain as a
legacy expected-fail seed, but it must not be the only amplifier KiCad-backed
fixture once this work is complete.

## Metadata Requirements

Fixture metadata must describe the current truth, not the desired endpoint.

For an initial expected-fail fixture:

- `readiness`: `expected_fail`;
- `tier`: `fabrication` or `connectivity` based on the requested validation
  gates;
- `acceptance`: the strictest currently supported acceptance value that avoids
  parse-time blockers unrelated to amplifier evidence, starting with
  `connectivity` while component selection blocks and moving to `erc-drc` only
  when schematic, PCB, and KiCad evidence are reachable;
- `require_erc`: `true` if schematic electrical promotion is expected;
- `require_drc`: `true` only when PCB layout and routing evidence are part of
  the fixture;
- `expected_stages`: the exact stages expected to run before the known blocker;
- `expected_artifacts`: `.kicadai/design-promotion.json` at minimum, with full
  generated KiCad project artifacts only when the workflow reaches project
  write;
- `known_gaps`: current blockers with stage-specific detail.

If promoted to candidate:

- `readiness`: `candidate`;
- expected artifacts include:
  - `.kicadai/transaction.json`;
  - `.kicadai/manifest.json`;
  - `.kicadai/design-promotion.json`;
  - generated `.kicad_pro`;
  - generated `.kicad_sch`;
  - generated `.kicad_pcb`;
  - preserved ERC/DRC report artifacts under `.kicadai/checks` when KiCad
    checks run.

Project-local runtime files such as `.kicad_prl` are not required promotion
artifacts because they are local KiCad UI/session state and are ignored by the
repository.
- expected stages include:
  - `block_planning`;
  - `component_selection`;
  - `schematic`;
  - `schematic_electrical`;
  - `pcb_realization`;
  - `placement`;
  - `routing` when routing is requested;
  - `project_write`;
  - `writer_correctness`;
  - `validation`;
  - `kicad_checks`;
- known gaps are warning-level or explicitly non-blocking for candidate
  readiness.

## Evidence Requirements

### Protected Output Evidence

Promotion reports must expose structured evidence that the amplifier uses the
protected headphone output path:

- `headphone_output_protection` block instance ID;
- nominal headphone load;
- AC output coupling present;
- DC-blocking capacitance;
- bleed resistor policy;
- series output resistor policy;
- connector return/reference policy;
- fault protection status;
- readiness and blockers.

The report must make it obvious whether the design is blocked because of active
fault protection, load policy, analog layout, or KiCad electrical/physical
validation.

For this fixture, the expected fault-protection status is
`omitted_by_design` for candidate promotion. `placeholder_blocked` is acceptable
only while the fixture remains expected-fail. Any wording that implies active
fault protection is complete must block candidate promotion.

### Schematic Evidence

Generated schematic evidence must prove:

- the op-amp gain stage drives the Class AB output stage;
- the Class AB output stage drives the protection block before the headphone
  connector;
- the headphone signal is AC-coupled;
- polarized output-coupling capacitors, when selected, have the positive
  terminal facing the DC-biased amplifier output and the load-side terminal
  facing `HP_OUT`; this is a KiCadAI schematic/component validation
  requirement, not something expected from stock KiCad ERC/DRC alone;
- the headphone return is tied to an explicit load reference;
- VCC and GND rails are present and labeled;
- generated labels are deterministic and do not conflict.
- any bleed resistor required by the output-protection policy is connected on
  the load side of the AC-coupling capacitor, from `HP_OUT` to the PCB-side
  `LOAD_REF` node, so the headphone output node cannot float in the generated
  model.

Schematic readability is not the primary promotion gate, but the fixture should
use the existing readability rules so the example is usable for human review.

### PCB Evidence

When the fixture reaches PCB realization, generated board evidence must prove:

- all schematic-backed symbols with assigned footprints transfer to footprints;
- footprint pads carry KiCad-native net declarations;
- the board has a valid outline;
- protected output nets retain their names through PCB realization;
- bias diodes in the Class AB output stage have placement evidence tying them
  near the complementary output devices before the fixture can be considered
  `candidate`; initial evidence should use a concrete placement rule such as
  center-to-center distance within 10 mm or an explicit shared-thermal-placement
  marker. If current placement constraints cannot express this yet, that must
  remain a named candidate blocker rather than an implicit pass.
- generated routes, if present, contact physical same-net pad anchors;
- board validation reports no unclassified disconnected pads, invalid net
  assignments, or missing outlines.

If the current placement or routing engine cannot complete the fixture, that is
acceptable only if metadata and promotion reports identify the exact blocker.

### KiCad Evidence

The default test suite must continue to skip real KiCad checks unless
`KICADAI_KICAD_CLI` is configured.

When real KiCad is available:

- KiCad CLI evidence is expected from a compatible KiCad CLI whose ERC/DRC
  reports the current check runner can parse. README-level setup may recommend
  newer KiCad versions, writer-specific docs define the active local
  file-output target, and KiCad 8 or newer may be accepted when report parsing
  is explicitly supported. If the installed CLI is outside that compatible
  target or emits an unsupported report format, the fixture must report a
  classified external-tool evidence blocker rather than silently passing.
- ERC artifacts must be captured and referenced in the promotion report.
- DRC artifacts must be captured when DRC is required.
- Findings must be classified as blocking, warning-only, or known local KiCad
  tool instability.
- A fixture may not be promoted to candidate if required KiCad evidence is
  missing or silently skipped.
- Candidate promotion for fixtures that require KiCad evidence is expected to
  happen only on local or CI runners with a compatible `KICADAI_KICAD_CLI`
  configured; ordinary default test runs may validate metadata and skip the
  external evidence.

## Acceptance Criteria

- A protected Class AB headphone KiCad-backed fixture and metadata file exist.
- Optional fixture tests include the new fixture in the promotion queue.
- The promotion report records protected output evidence in stable JSON.
- Expected-fail metadata, if still required, names current blockers rather than
  stale op-amp-buffer or parse-request blockers.
- Candidate metadata, if achieved, is backed by generated project artifacts,
  writer correctness, board validation, and configured KiCad evidence.
- The AI readiness matrix remains `connectivity` unless the captured evidence
  justifies a further readiness promotion.
- README, focused docs, and roadmap text are updated so users understand that
  the amplifier path is KiCad-evidence-backed only to the level proven.
- `go test ./...` remains independent of local KiCad.

## Risks

- The current amplifier component catalog may still be connectivity-level, so
  strict component-policy gates can block before writer evidence.
- PCB placement and routing may expose analog-layout blockers unrelated to
  output protection.
- Real KiCad ERC/DRC output can vary by installed KiCad version; tests must
  classify artifacts without making the default suite brittle.
- Existing acceptance strings may not include `fabrication-candidate`; the
  fixture must avoid parse-only blockers if the purpose is to test generated
  amplifier evidence.
- A connectivity-clean protected headphone output is not the same as active
  fault protection or safe speaker/power-amplifier output.
