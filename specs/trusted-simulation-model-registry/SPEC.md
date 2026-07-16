# Catalog-Backed Trusted Simulation Model Registry

## Status

Implemented and acceptance-tested.

## Problem

The generic-circuit workflow previously recognized one graph-level regulator
contract and evaluated it from a workflow-local map. That behavior was
deterministic, but it was neither catalog-backed nor extensible: compatibility,
limits, and artifact structure were coupled to the regulator workflow stage.

## Requirements

1. A provider may select only a model ID registered by KiCadAI, bind declared
   component instance IDs to named model roles, provide finite bounded scalar
   operating inputs, and declare bounds for metrics emitted by that model.
2. Provider input has no model text, model file, include, command, expression,
   code, path, or executable field. Strict schema decoding rejects additions.
3. Every bound component must resolve through the immutable component catalog.
   Its catalog record must explicitly declare compatibility with the selected
   trusted model. Catalog model IDs, parameter names, values, family
   compatibility, and duplicates are validated and unknown models fail closed.
4. Resolution snapshots the registry version/hash, catalog ID/hash, catalog
   component ID/family, component value where required, and catalog model
   parameters into a deterministic plan. Workflow execution evaluates only that
   resolved plan and rejects a stale or malformed registry snapshot.
5. The initial trusted registry contains:
   - `linear_regulator_ideal_v1`, parameterized by catalog output voltage,
     minimum headroom, and maximum load current;
   - `resistor_divider_dc_v1`, using catalog-validated resistor instance values;
   - `rc_lowpass_ac_v1`, using catalog-validated resistor/capacitor values.
6. The normalized report uses schema
   `kicadai.trusted-simulation-report.v1`, records all trust/provenance fields,
   inputs, measurements, assertion results, and an explicit status, and is
   byte-reproducible under recorded replay.
7. Unsupported models, missing catalog claims, incompatible families, missing
   roles/parameters/values, invalid scalar ranges, stale plans, operating-limit
   violations, and assertion failures block with actionable diagnostics.

## Held-Out Acceptance Fixture

`generic_filtered_divider_hierarchy` is a `generic-circuit-v1` recorded fixture,
not a block-family request. It composes input/output headers, two catalog
resistors, and a DC-open filter capacitor as an explicit graph; selects the
trusted divider model; and requests automatic hierarchy with no more than two
components per generated child sheet.

The optional real-KiCad acceptance must prove all of the following in one run:

- catalog/model resolution and electrical validation pass;
- trusted simulation passes and emits provenance-complete evidence;
- routing is complete and connectivity is clean;
- KiCad ERC and strict DRC are clean;
- writer correctness passes;
- strict root, every child, and PCB round-trip diffs are zero;
- at least two child schematics prove automatic hierarchy partitioning;
- replay from the generated sanitized provider artifact produces an identical
  KiCad file set, byte-identical root/children/PCB/project files, and a
  byte-identical simulation report;
- declared and achieved promotion readiness are both `pass` and match.

The existing generic RC fixture must carry real KiCad-backed
`rc_lowpass_ac_v1` pass evidence. Existing protected LED, protected I2C sensor,
and hierarchical BMP280 pass fixtures remain regression requirements.

## Non-Goals and Prohibitions

- No fixture-specific model, topology detector, coordinate, allowlist, schema,
  block family, exception path, or readiness override.
- No provider-supplied SPICE, behavioral source, shared library, executable,
  include, or model file.
- No claim of parasitic, tolerance-distribution, transient, thermal, stability,
  noise, distortion, or fabrication signoff from these ideal analytic models.
