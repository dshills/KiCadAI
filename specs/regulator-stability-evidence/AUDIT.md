# Regulator Stability Evidence Audit

Date: 2026-07-02

## Current Regulator Records

### `regulator.linear.ams1117_3v3.sot223`

- Fixed 3.3 V SOT-223 AMS1117-compatible regulator.
- Carries verified symbol, footprint, pinmap, input voltage, output current,
  and output voltage metadata.
- Requires input and output capacitor companions.
- Carries a thermal derating rule.
- Does not currently model output-capacitor ESR or regulator stability
  requirements in structured data.

Implication: this record may remain useful for structural and connectivity
generation, but fabrication-candidate output must be blocked until
part-specific output-capacitor ESR/stability evidence exists.

### `regulator.linear.ap2112k_3v3.sot23_5`

- Fixed 3.3 V SOT-23-5 AP2112K regulator.
- Carries verified symbol, footprint, pinmap, input voltage, output current,
  dropout/headroom, enable voltage, EN handling, and NC handling metadata.
- Requires input and output capacitor companions.
- Carries thermal, enable-voltage, and capacitor-stability derating rules.
- Does not currently model ceramic-stable status or MLCC effective
  capacitance/DC-bias evidence as structured data.

Implication: this record can be preferred for small 3.3 V generated rails and
connectivity-oriented output, but fabrication-candidate output must remain
blocked until the selected ceramic capacitors carry concrete derating evidence.

## Current Capacitor Records

### Generic Ceramic Capacitors

Generic `capacitor.ceramic.*` records provide nominal capacitance, voltage,
symbol, package, and passive-rule confidence. They do not identify dielectric,
MPN, ESR, DC-bias, or effective-capacitance evidence.

Implication: they are not sufficient for fabrication-candidate regulator
stability proof.

### Murata 100 nF Records

Curated Murata 100 nF records provide concrete MPNs and verified pinmaps. Some
records already include prose derating notes. They do not currently expose
structured dielectric, DC-bias, effective-capacitance, or ESR evidence fields.

Implication: these records can support decoupling identity and connectivity,
but should not be treated as complete regulator stability evidence without
structured derating proof.

### Murata 10 uF Record

`capacitor.ceramic.murata_grm21br61a106ke19l.0805` provides a concrete 10 uF
10 V X5R 0805 capacitor with a DC-bias derating warning. It does not currently
encode structured effective-capacitance evidence.

Implication: it is a good candidate for AP2112K connectivity output-capacitor
selection, but fabrication-candidate output must still require review of
effective capacitance at the applied rail voltage.

## Minimal Schema Shape

The first implementation should add two optional component-record evidence
objects:

- `regulator_evidence`
- `capacitor_evidence`

`regulator_evidence` should include:

- output-capacitor stability kind;
- required output capacitance;
- accepted dielectric families where known;
- ESR min/max window when required;
- fabrication-candidate blocker flag and note.

`capacitor_evidence` should include:

- dielectric;
- nominal capacitance;
- voltage rating;
- DC-bias review status;
- effective-capacitance review status;
- ESR evidence status;
- fabrication-candidate blocker flag and note.

The schema should be intentionally conservative: missing fields are allowed for
older/generic records, but malformed fields should fail catalog validation once
an evidence object is present.

## Phase 1 Test Targets

- Invalid regulator stability kind is rejected.
- ESR minimum greater than ESR maximum is rejected.
- Regulator stability evidence requiring capacitance but omitting capacitance
  is rejected.
- Invalid capacitor voltage/capacitance evidence is rejected.
- Generic capacitor records with fabrication-proof claims are rejected.
