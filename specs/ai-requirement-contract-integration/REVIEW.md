# Local implementation review

Date: September 11, 2026. Reviewer: the implementing Codex task; this is a local
self-review with regression evidence, not an independent or Gemini review.
Scope: the changes from baseline `5cd36155bb412798edc8c6c05874ecfbf69dc316` in
architecturesearch provider vocabulary, behavioralintent contract/schema/context,
aiprovider parsing and the new synthetic tests. Historical publication files
are outside the implementation diff.

## Disposition

No unresolved blocking finding was identified in this local review. The offline
implementation may be reviewed for integration, subject to normal repository CI
and any independently requested review. Live API-side schema acceptance and
practical-board capability remain unverified and are not release claims.

## Review checks and corrections

- **Version mismatch:** reflection originally exposed later-version fields while
  the provider advertised v3. Those fields now have non-populated wire forms;
  direct compilation also rejects non-v3 schema/version pairs. No invalid legacy
  proposal is silently upgraded or normalized into a new contract version.
- **Registry drift/mutation:** the provider consumes clones of compiler tables,
  including constraint relations. A regression mutates every returned table and
  confirms that validator vocabulary remains unchanged.
- **Semantic precision:** binding alternatives cannot combine selectors; coverage
  alternatives cannot use mismatched reference kinds. Numeric/unit/relation
  constraints are paired. The board dimension lower bound was checked against
  the actual compiler floor of 0.01 mm, not merely positive values.
- **Schema overconstraint:** positive tests exercise the seven relation forms,
  all binding forms/directions, metric triples and operating axes. Complete
  ready/refusal/clarification examples traverse schema, strict decode and compile.
  Cross-reference and source-accounting failures still reach and fail compilation.
- **Clarification provenance:** the completed example retains the original supply
  range and records the added cutoff facts in a bound answer; prior source and
  compilation hashes are not replaced. Tampered answer bindings are rejected.
- **Stream integrity:** final text must equal received text deltas; duplicate or
  contradictory completion and broken numbered sequences cannot yield accepted
  output. SSE comments, CRLF, multiline data and an EOF final frame are retained.
  The existing large-stream fixture was corrected to put its synthetic padding
  in reasoning deltas instead of contradictory output-text deltas.
- **Boundaries and privacy:** caps and retries are unchanged. The synthetic
  oversized metadata case still rejects, usage inconsistencies reject, and raw
  error-message content does not escape the new error path. No keys are stored in
  tests or docs; test keys are explicit synthetic literals.
- **Evidence separation:** all frozen files, raw inventories, sealed evaluator
  identity and archive hash reverified. No practical-board case or frozen tool
  changed. Previous negative results remain the comparison record.

## Explicit residual risks

1. The emitted schema is larger (80,081 bytes before provider wrapping). Tests
   reserve headroom under the previous 128-KiB evaluation request cap and the
   documented structure limits, but a full real request must be measured before
   any future dispatch. Do not raise a limit if capability/source context does
   not fit; stop and request a protocol decision.
2. The local schema checker is independent of production emission but authored
   in this task and limited to this subset. Only a separately authorized API run
   can establish service acceptance for the explicitly selected model.
3. Global output-delta concatenation is appropriate to the existing single-intent
   output contract. This is not an implementation for arbitrary interleaved tool
   output or multiple intent messages. Existing decoder restrictions still apply.
4. Numbering-free legacy streams and missing usage remain compatibility cases.
   No claim of authenticated billing or fully measured cost is made.
5. Schema/prompt alignment does not supply missing engineering capabilities,
   qualify components, route a board or establish deterministic native replay.
   The original complete-board goal remains unmet.

The API credential and documentation skills influenced the workflow by keeping
tests credential-free and checking the strict schema subset against official
documentation. No additional model-based review was requested or run under this
offline approval.
