# Proposed next step: a frozen live interface evaluation

Status: **proposal only; not authorized, not frozen and not executed**.
Date: September 11, 2026. This is a new interface-reliability campaign, not a
recovery or final phase of the practical-board protocol-v2 campaign.

## Goal and decision

Determine whether the repaired AI-facing v3 contract produces faithful,
compiler-ready requirements and correct structured refusal/clarification flows
from fresh ordinary-language requests. Measure both initial success and success
after the existing single bounded correction. A pass permits planning a new
end-to-end board milestone; it does not establish a board-generation success
rate or change stable support.

## Proposed corpus and acceptance

Author eight small new briefs and expected semantic clauses independently of
the practical-board corpus and the new synthetic test values. Before the first
call, review the exact briefs, predetermined outcomes and any clarification
answers against the installed capability snapshot and publish their hashes.
These are public-frozen tests, not a statistically representative benchmark.

| Group | Cases | Predetermined requirement |
| --- | --- | --- |
| Ready | 4 | Fully specified low-voltage v3 interface briefs covering external endpoints, composed signal bindings, participant endpoints and bounded operating cases; all needed generation/verification capabilities must be in the frozen snapshot |
| Refusal | 2 | One request requiring unavailable trusted verification/certification; one explicitly requiring later-version control/event semantics outside the advertised v3 boundary |
| Clarification | 2 | One missing quantitative behavior bound; one missing operating/load choice; each has one exact prewritten answer bundle and a fully specified expected contract afterward |

Ready acceptance requires correct terminal status, strict decode, no blocking
compiler issue and a source-bound clause audit: no missing material facts,
invented supplied facts, hidden circuit implementation, unsupported capability
claims or substituted engineering goal. Checking that references exist is
necessary but not sufficient; retain a manual faithfulness audit.

Refusal acceptance requires a structured capability gap matching the frozen
reason and evidence requirement, complete source coverage and no executable
requirement. Unrelated refusal or a question that changes the requested goal
does not pass. Clarification acceptance requires exactly the needed unresolved
facts, valid uncertainty ownership and no executable requirement before answers;
the bound follow-up must retain original facts and become ready after the frozen
answer. Wrong/tampered bindings must fail in offline replay controls.

The proposed gate is **4/4 ready, 2/2 refusal and 2/2 complete clarification**
within their allowed correction paths, with zero unsafe accepted output, no
evidence mismatch and no budget violation. Report first-attempt results
separately. Missing, interrupted and infrastructure-failed cases remain in the
denominator. Do not turn this small sample into a general reliability claim.

## Provider and resource limits to approve

- Reuse the existing OpenAI key only after explicit live approval; never print,
  commit or upload it. No Gemini review or other provider is included.
- Request the previously recorded model identifier `gpt-5.6-sol`, explicitly
  pinned rather than inherited from environment defaults. Verify its current
  availability, schema support and official pricing before the freeze. This
  proposal does not assert current service availability or prices. If unavailable,
  stop for a model/protocol decision rather than substituting a model.
- Responses, strict behavioral-intent schema, validated installed capability
  context, streaming on, background off, `store=false`, no tools/file uploads,
  no sampling/reasoning override.
- At most **20 generation POSTs**: eight initial calls plus at most one correction
  each (16); two clarification-answer calls plus at most one correction each (4).
  Corrections use the existing bounded diagnostics, at most eight issues. An
  electrical or physical failure is not a reason for another model request.
- At most **USD 25 estimated/reserved new spend**, independent of all historical
  campaign budgets. This is a proposed ceiling, not an allocation or promise that
  all calls fit. Record actual billed cost separately if independently available.
- Reserve conservatively before every dispatch using current frozen official
  input/output rates, one input token per encoded request byte plus 4,096 overhead
  tokens, and all 16,384 output tokens. Release a reservation only when valid
  complete usage is available. Keep unknown/rejected-call reservations charged
  against the ceiling; do not infer zero spend from absent accepted usage.
- Maximum output 16,384 tokens; maximum encoded request 131,072 bytes; stream
  limit 2,097,152 bytes. No increase to these limits. Validate the complete request
  before dispatch, including the expanded schema and capability context.
- One in-flight generation request, five-minute evaluator transport deadline,
  20-minute case ceiling, 90-minute campaign ceiling, sampled process-tree RSS
  ceiling 16 GiB and raw evidence ceiling 1 GiB. The five-minute override must be
  explicit in the separately frozen evaluator; production's default is unchanged.
- No automatic retries for uncertain transport completion, no extra probes,
  recovery baselines, correction cycles or mid-campaign code/corpus changes.
  Before a request, stop if its maximum allowed duration or reservation cannot
  fit within the remaining campaign resources. Record all unrun cases.

The maximum request count is a limit, not a requirement to spend all requests.
Compiler-invalid proposals may use their single correction; transport/schema
configuration failure or an integrity/budget violation stops the campaign. Report
the failure and request a new decision rather than silently starting over.

## Freeze and evidence preconditions

Before live execution, the new evaluator/journal must be implemented and tested
offline, and the exact new corpus/answers must be reviewed. This offline milestone
has not built that new live harness or frozen its corpus. Preparation belongs to
the proposed next scope.

Seal a clean source commit and evaluator binary/build identity; exact corpus,
expected clauses/answers, provider schema/context, capability snapshot and policy
hashes; official pricing snapshot; all request/time/memory/disk caps; acceptance
rules and complete command line. Confirm the final encoded requests fit without
silently trimming source facts. A mismatch blocks dispatch.

Use a new raw evidence root and journal. Retain pre-dispatch reservations, exact
redacted requests, response IDs, complete bounded raw streams where available,
transport status, accepted and rejected proposal artifacts, compiler issues,
bound-answer records, usage/reservations, resource measurements and semantic
audits. Oversized streams may need a separately bounded capture path; capture is
not production acceptance and must not bypass the parser cap. Preserve the
failure and every captured byte with a completeness/truncation indicator.

Authenticate a terminal campaign against file hashes and the dispatch journal;
scan credentials locally before publication. Keep all old corpus, journal,
freeze, results and evidence untouched. New output must be labeled interface
evidence, never substituted into protocol-v2 results.

## Stop and handoff

Publish the entire eight-case outcome table and source-bound audit, including
failures and unrun cases. Do not tune against this corpus and rerun it within the
same authorization. If the interface gate fails, diagnose it and propose a new
bounded decision. If it passes, separately scope electrical synthesis, schematic
readability, placement, routing, native validation and deterministic replay for
fresh practical boards. No board manufacture, release, merge or stable-support
expansion is part of this proposal.
