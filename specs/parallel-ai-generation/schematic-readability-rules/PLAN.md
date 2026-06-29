# Schematic Readability Rules Plan

## Phase B1: Rule Profile Metadata

- Add exported profile/rule metadata to `internal/schematiclayout`.
- Include stable rule IDs, severity, profile ownership, and repair guidance.
- Add unit tests for profile lookup and rule inventory.

## Phase B2: Repair-Oriented Diagnostics

- Map existing readability diagnostic codes to deterministic repair guidance.
- Add small positive and negative readability fixtures for geometric/topological
  rule triggers such as left-to-right flow, rail placement, return placement,
  and diagonal-wire rejection.
- Add tests for amplifier-specific and standard diagnostics.

## Phase B3: Documentation And Workflow Alignment

- Update development docs to describe readability profiles and repair guidance.
- Add tests that ensure workflow readability summaries continue to expose the
  selected profile and diagnostic counts.
