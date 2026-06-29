# KiCad-Backed Promotion Closeout Plan

## Phase A1: Promotion Action Model

- Add `PromotionNextAction` to the promotion report schema.
- Derive deterministic actions from failed/warn/skipped/not-run required gates.
- Add unit tests for issue-linked and artifact-linked actions.

## Phase A2: Fixture Metadata Progression Policy

- Add tests that validate optional KiCad-backed metadata readiness rules.
- Keep default tests independent of KiCad CLI.
- Document the policy through failing messages that explain the required
  promotion evidence.

## Phase A3: Fixture Readiness Summary

- Add a small readiness summary helper for optional design fixture metadata.
- Cover grouping/order with tests.
- Update development docs with the fixture promotion workflow.

