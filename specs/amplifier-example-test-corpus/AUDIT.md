# Amplifier Example Coverage Audit

Date: 2026-06-28

## Existing Assets

- `examples/06_class_ab_headphone_amp/` is a checked-in KiCad project fixture
  for a mono Class AB headphone amplifier.
- `examples/intent/amplifier_module.json` is a structured intent fixture for a
  small amplifier module.
- `examples/intent_text/headphone_amplifier_unverified.txt` is a
  natural-language fixture for an unverified Class AB headphone amplifier
  request.

## Existing Schematic Fixture

`06_class_ab_headphone_amp` includes the expected KiCad project files:

- `class_ab_headphone_amp.kicad_pro`
- `class_ab_headphone_amp.kicad_sch`
- `class_ab_headphone_amp.kicad_prl`
- `sym-lib-table`

The schematic is a checked-in fixture, not a generated design workflow output.
It should therefore be treated as a reference example and semantic regression
fixture rather than mechanically rewritten until a dedicated amplifier
generator exists.

## Expected Landmarks

The existing Class AB headphone fixture should retain at least these
amplifier landmarks:

- input connector or input net;
- headphone/output connector or output net;
- op-amp/gain-stage symbol;
- feedback network;
- diode or bias network for Class AB output bias;
- complementary output devices or explicitly modeled output stage;
- local supply/reference/ground labels;
- decoupling components.

## Current Gaps

- No amplifier-specific semantic checker exists yet.
- No checked-in Class A headphone amplifier fixture exists yet.
- No generated amplifier design request is currently promoted as a default
  runnable `design create` fixture.
- Amplifier PCB layout constraints are not yet first-class block metadata.
- Production analog correctness, thermal behavior, oscillation margin, and
  KiCad DRC-clean generated amplifier boards remain future work.

## Phase 1 Regression

Phase 1 adds smoke-level regression coverage proving that the existing
`06_class_ab_headphone_amp` project is discoverable and parseable by the
current KiCadAI project/schematic readers.
