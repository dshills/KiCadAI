# Transistor Tester Component Catalog Plan

## Phase 1: Evidence and schema fit

- Inspect component confidence, package, pinmap, and acceptance semantics.
- Confirm exact symbols and footprints in the installed KiCad libraries.
- Gather manufacturer-primary pinout, package, rating, and lifecycle sources.
- Separate exact bare-part evidence from unknown breakout-board evidence.

## Phase 2: Exact parts

- Add ADC, DAC, display, and driver family definitions where absent.
- Add exact G5V-1 DC5, ULN2803A, ADS1115IDGSR, MCP4725A0T-E/CH,
  BD139-16, and CD74HC4053E records.
- Add human-verified symbol-to-footprint pinmaps.
- Preserve part-specific caveats: relay case marking, ULN clamp common,
  MCP4725 lifecycle, and BD139 SOA/thermal review.

## Phase 3: Fail-closed modules and fixture connector

- Add draft-only ESP32, ADS1115, MCP4725, and SSD1306 module records.
- Mark logical connector packages as non-mechanical surrogates.
- Record unverified header order, pull-ups, addresses, dimensions, and mounting
  details as evidence obligations.
- Add a neutral structural 1x03 transistor test connector.

## Phase 4: Regression evidence

- Test exact function-to-pin and function-to-pad mappings.
- Test ULN2803A channel pairing and relay contact semantics.
- Test draft-only acceptance for provisional modules.
- Test structural-only acceptance and neutral roles for the test connector.
- Regenerate the deterministic checked-in coverage golden.

## Phase 5: Documentation and local verification

- Update component intelligence, project status, AI readiness, and roadmap
  documentation.
- Run local unit, lint, coverage, library-resolution, protected-board, writer,
  and round-trip checks.
- Record the exact commands and results in `AUDIT.md`.

## Phase 6: Review and delivery

- Stage only the scoped catalog, test, spec, and documentation changes.
- Run Prism against the staged diff and resolve actionable findings.
- Re-run affected local checks after review changes.
- Commit and push the reviewed branch.

## Physical evidence follow-up

Promotion of provisional modules requires clear photos or purchasing records
for both sides of each actual board, readable header silkscreen, and mechanical
measurements. The relay requires a readable case marking. The final test socket
requires its exact part identity or a dimensioned drawing.
