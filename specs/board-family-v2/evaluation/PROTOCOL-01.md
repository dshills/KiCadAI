# Final two-family live acceptance protocol

Status: cases and payload contract are frozen before any v2 live call. **Spending is not yet authorized.** Proposed authority: at most **14 physical requests and USD 1.00 total**, using the existing key, for this one batch only. This document does not grant that authority. Record the user's approval separately before execution; do not alter this protocol or cases to reinterpret results.

## Scope and payload

Use the existing `gpt-4.1-mini-2025-04-14` snapshot and OpenAI Responses transport, with the exported capability context/schema and each original prompt from `cases-01.json`. No source files, native geometry, PDF files, secrets or environment contents are model input. The key is used only for the HTTPS Authorization header. The destination remains `https://api.openai.com/v1/responses`; automatic retries, redirects, proxies and background calls remain disabled. Native subprocesses must not receive provider credentials.

The [official model page](https://developers.openai.com/api/docs/models/gpt-4.1-mini) lists this snapshot, Responses/Structured Outputs support, and standard prices of USD 0.40/million input tokens and USD 1.60/million output tokens, checked September 13, 2026. Keep the existing 24,000-byte request bound, 1,600-token output cap and USD 0.05 conservative reservation per physical attempt. Fourteen reservations total USD 0.70, below the proposed USD 1.00 ceiling. Report actual provider usage and estimated spend separately; unknown outcomes retain their full reservations.

Before the first request, implement and offline-test a **separate v2 goal-scoped budget policy/ledger** enforcing these smaller caps. Do not reset, relabel or extend `.cache/board-family-v1/live-ledger.json`, whose 41-request allowance is exhausted. Do not mistake the existing legacy CLI's 41/$10 constants for new authority. Pin the final executed binary and relevant source hashes after the budget plumbing is ready, and verify its exported payload still matches the frozen contract. This is a preflight obligation, not permission to send a probe.

## Cases and acceptance

The batch has five supported cases covering every family/profile, three targeted clarifications and six refusals. Supported prompts also exercise explicit exclusions and pull-up-current versus whole-board-power distinctions. Negative cases cover combined sensing, excessive loading, a wrong-family profile, conflicting heater instructions, unsupported power/wireless and unsubstantiated accuracy.

Acceptance requires **14/14 correct first attempts**:

- Supported: exact nine-field configuration, all original requirements retained, all 14 native/export gates pass, complete output bundle, no manual repair. Match native/BOM/library bytes to the corresponding offline example; compare preview/manufacturing content with the existing narrow timestamp policy. Report total wall time, including interpretation and validation, for all five attempts; target median below 60 seconds and each below 120 seconds. Report any timeout/failure without dropping it from the denominator.
- Clarify: null configuration, no native design/manufacturing output, and a question that actually resolves the ambiguity named in the case. Asking an unrelated generic question does not pass.
- Unsupported: null configuration, no native design/manufacturing output, and a truthful explanation of the unmet requirement(s). Silently substituting a supported alternative does not pass.

Check both the raw provider decision and admitted application decision. A local guardrail correction must be disclosed separately from model correctness; do not label a corrected model miss as a correct raw selection. Clauses must preserve the entire original prompt. All 14 cases remain in the planned denominator even if the batch stops early.

This is implementing-agent-authored targeted acceptance, not an independent holdout, a statistical reliability estimate, or evidence of arbitrary circuit synthesis. No firmware, fabrication, assembly or bench performance is tested.

## Execution and failure policy

One reserved physical request per case, sequentially, in the listed order. Record request index, response ID, usage, raw response, admitted selection, configuration, validation/export identities and all failures. Protect the batch with an exclusive process lock and durable accounting before network dispatch. Resume only never-attempted cases after verifying the previous process is terminal; an unknown request outcome is already an attempt, not permission to retry.

Continue collecting remaining cases after a semantic miss, preserving that failure. Stop on transport, authentication, usage/accounting, protocol or native validation failure. Do not rerun failed cases, tune prompts/schema/model mid-batch, add recovery baselines, change firewall rules or exceed either cap without new authority. Final reporting must distinguish complete pass, complete failure and incomplete/blocked evidence. Earlier v1 trials and all offline v2 attempts remain unchanged.
