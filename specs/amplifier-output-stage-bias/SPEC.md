# Amplifier Output Stage And Bias Block Spec

## Purpose

KiCadAI can already model amplifier examples and fail closed when amplifier
generation lacks verified component and layout evidence. The next blocker in
the AI readiness graph is the `catalog_block_expansion` path for:

- `amplifier.component.output_transistor`;
- `amplifier.block.bias_network`.

This specification defines the first implementation slice for verified
headphone-amplifier output devices and Class AB bias networks. The goal is not
to make all amplifier designs fabrication-ready. The goal is to replace
placeholder output-stage behavior with catalog-backed parts, deterministic
block metadata, and tests that prove generated Class AB headphone amplifier
designs fail closed or advance for concrete reasons.

## Scope

- Add verified or explicitly blocked amplifier output-device catalog records.
- Add enough output-device metadata for Class A/Class AB headphone use:
  package, pinmap, polarity/device kind, current, voltage, power, thermal, and
  safe-operating-area notes.
- Add a reusable Class AB bias-network block contract.
- Connect the bias block to catalog-backed diode or transistor choices where
  supported.
- Emit schematic-level design operations for the bias network and complementary
  output stage.
- Preserve conservative PCB/layout status until thermal/current constraints and
  KiCad evidence are available.
- Update AI readiness records and docs only when tests provide supporting
  evidence.

## Non-Goals

- Do not claim power-amplifier or speaker-load fabrication readiness.
- Do not support mains-connected supplies.
- Do not infer transistor SOA from incomplete data.
- Do not promote KiCad-backed amplifier fixtures to `pass` in this slice.
- Do not hand-write vendor claims without checked-in source/evidence metadata.
- Do not replace analog design review; encode bounded, conservative defaults
  and blockers.

## Target Use Case

The first supported use case is a low-voltage, educational Class AB headphone
amplifier output stage:

- external low-voltage supply, such as 5 V to 12 V;
- headphone load, not speaker power output;
- op-amp voltage gain stage or buffer driving a complementary emitter follower;
- diode-biased or Vbe-multiplier-style bias network represented explicitly;
- DC blocking and load/protection handled by existing or separate output
  protection records;
- generated schematic is readable left-to-right and marks unverified PCB/layout
  work clearly.

## Data Model Requirements

### Output Device Records

Each output-device catalog record must include enough structured metadata to
support deterministic selection and safe blocking:

- device class: `bjt` or future `mosfet`;
- polarity/channel: `npn`, `pnp`, `n_channel`, or `p_channel`;
- intended role: `headphone_output`, `bias`, `small_signal_driver`, or
  `blocked_power_output`;
- package and footprint ID;
- symbol ID;
- pinmap evidence for package pins to schematic pins;
- collector/emitter/base or drain/source/gate role mapping;
- maximum voltage rating;
- maximum continuous current rating;
- package power-dissipation rating or explicit unknown blocker;
- thermal notes or thermal-resistance source where modeled;
- SOA evidence status: `not_required`, `documented`, or `blocked`;
- companion requirements, such as emitter resistors, bias diodes, or heatsink
  review.

If any required field is missing for a candidate fabrication-capable part, the
part must be rejected with a structured reason. Placeholder records may remain
in the catalog only if they are blocked by default and cannot be selected for
fabrication-candidate workflows.

### Bias Network Records

The Class AB bias block must expose structured fields for:

- topology: `diode_string` initially; `vbe_multiplier` may be added later;
- diode/transistor count and polarity;
- target output-stage family, such as complementary BJT emitter follower;
- quiescent-bias intent, represented as qualitative `low`, `nominal`, or
  `review_required` until calculation support exists;
- temperature-coupling requirement;
- required ports: driver input, upper-base drive, lower-base drive, output,
  positive rail, negative/ground rail;
- generated schematic landmarks for AI/human review;
- blockers for unverified thermal coupling or quiescent-current calculation.

The bias block may reach `connectivity` before it reaches `candidate`.
`candidate` requires catalog-backed devices and schematic connectivity evidence.
`verified` requires dependency records to be verified and KiCad-backed evidence.

## Component Selection Behavior

Selection must be deterministic and fail closed.

- If a request asks for a headphone Class AB output stage, the planner may
  select supported complementary low-current BJT seeds when voltage/current
  requirements are within their modeled ratings.
- If a request asks for speaker, high-power, high-voltage, or unbounded load
  output, selection must return a structured blocked issue.
- If complementary polarity cannot be satisfied by verified or policy-allowed
  records, the output stage must not be generated as fabrication-capable.
- If a part lacks pinmap evidence, it may appear in blocked diagnostics but
  must not be selected into generated KiCad symbols.
- If SOA or thermal evidence is missing, generated output may remain draft or
  connectivity-only, but not fabrication-ready.

## Schematic Generation Requirements

Generated Class AB headphone schematic fragments must:

- place signal flow left-to-right;
- place higher-voltage rails above lower-voltage rails;
- show complementary output devices as an upper/lower pair;
- place the bias network between driver and output-device bases/gates;
- include explicit labels for driver output, bias nodes, amplifier output, rail
  nodes, and load/output connector;
- avoid overlapping labels, pins, and wires according to schematic readability
  rules;
- keep DC-blocking/protection separate unless requested by a protection block.

The schematic generator must not hide unsupported safety gaps in comments only.
Blocking issues must be represented in workflow diagnostics and rationale
artifacts.

## PCB/Layout Requirements

This slice does not need to make amplifier PCBs DRC-clean, but it must prepare
the handoff:

- output-device records must expose placement and routing hints for thermal and
  high-current review;
- emitted blocks must mark output current paths and rail paths with roles that
  downstream placement/routing can consume;
- missing thermal/current layout proof must remain a blocker in AI readiness
  records and validation reports;
- generated PCB fragments may be skipped or draft-only when required footprint
  and thermal evidence is incomplete.

## Validation Requirements

Tests must cover:

- catalog validation for required output-device metadata;
- blocked placeholder behavior for unverified power output devices;
- deterministic complementary output-pair selection for supported headphone
  scenarios;
- blocked selection for unsupported speaker/high-power scenarios;
- bias-network block manifest/schema validation;
- generated schematic operations containing expected amplifier landmarks;
- design workflow diagnostics for unverified SOA, thermal, or layout evidence;
- AI readiness dependency behavior for `amplifier.component.output_transistor`
  and `amplifier.block.bias_network`.

Default tests must not require KiCad or network access. Optional KiCad-backed
tests may be added only as opt-in evidence fixtures.

## Documentation Requirements

Documentation must explain:

- supported amplifier output-stage scope;
- unsupported power-amplifier and speaker-load cases;
- what “connectivity”, “candidate”, and “verified” mean for amplifier records;
- what evidence remains before KiCad-backed amplifier promotion can proceed;
- how agents should interpret blocked amplifier output-stage diagnostics.

## Acceptance Criteria

- `go test ./...` passes.
- Output-device catalog records are validated with structured amplifier
  metadata.
- The first Class AB headphone output-stage selection path is deterministic and
  tested.
- Unsupported output-stage requests fail closed with actionable diagnostics.
- A Class AB bias-network block contract exists and is covered by tests.
- AI readiness records for output transistor and bias network are advanced only
  as far as the implemented evidence supports.
