# Typed-requirement live acceptance 02

Status: **preparation only; no spending authorization**. Proposed authority is one
14-case batch, at most **14 physical requests and USD 1.00**, using the previously
approved existing key. This is a new budget, not remaining allowance from v1 or
final-01; both are exhausted. Do not create an approval record without an actual
new user approval of the bound runtime, contract, cases and limits.

## Frozen input and execution boundaries

Use `gpt-4.1-mini-2025-04-14` and the qualified `typed-requirements-02` contract.
The user prompt is wrapped with application-numbered source clauses. Expected
configurations, fact requirements and scoring rubrics are evaluator-only data;
never send them to the model. No source files, native geometry, credentials or
environment contents are model input. Only the Authorization header carries the
existing key. The sole destination is `https://api.openai.com/v1/responses`.

Model, transport and accounting stay unchanged: foreground streaming, 1600 output
tokens, 24,000 request bytes, no redirects/proxies/automatic retries. Each physical
attempt permanently reserves USD 0.05, so fourteen reservations total USD 0.70.
The [official model page](https://developers.openai.com/api/docs/models/gpt-4.1-mini)
still lists the pinned snapshot, Structured Outputs, and USD 0.40/1.60 per million
input/output tokens on September 14, 2026. Report actual usage-based estimated
spend separately; unused dollars never restore consumed request slots.

Reuse the byte-identical qualified selector binary and existing native
qualification only after proving all selected compiler/dependency/template bytes
still match their qualification sources. Pin the copied runtime, source selection,
native executable, Node executable and all runner/scorer inputs. Preserve previous
runtimes and ledgers. A new LuLu rule, if needed, requires separate explicit
approval for that exact executable and api.openai.com:443; do not overwrite a
previous allowed binary, disable the firewall or send a probe.

## Success criteria: all 14 first attempts

Five useful cases cover all five family/profiles, ordinary polite/question
wording, measurement-only selection, quantity conversion, exclusions and heater
negation. Three cases require targeted clarification. Six require refusal for
combined sensors, unsupported capacitance/profile, later heater use, direct
USB/wireless demand or a physical accuracy guarantee.

Three results are separate for every case:

1. **Raw extraction:** correct source-bound shape and facts; all substantive user
   requirements retained with the correct meaning, scope and polarity; no
   invented requirements/default numeric facts. Machine fact checks are necessary
   but never sufficient. Explicit source-bound semantic review is mandatory.
2. **Application decision:** expected disposition, exact nine-field configuration
   for useful cases, or null configuration plus a genuinely targeted question or
   truthful explanation for non-design cases. A safe but unnecessary clarification
   on a useful prompt is a failure. A local correction is not a raw model pass.
3. **Artifacts:** all 14 native/manufacturing gates and a complete byte/replay match
   to the reviewed family/profile example. Non-design outcomes produce only
   `selection.json`. Never manually repair outputs. Native correctness cannot
   compensate for a missed requirement.

Keep all 14 cases in the denominator, including unattempted cases if stopped.
Report end-to-end times for all five useful attempts, including failures: median
under 60 seconds and every attempt under 120 seconds. Missing timing is unknown,
not zero. Report extraction and native times where available, request count,
input/output tokens, reserved allowance and estimated spend. Comparisons with old
batches are descriptive because wording/contracts differ; no statistical claim of
reliability or speed improvement follows from this small targeted set.

Cases are implementing-agent-authored after development, not independent holdouts
or representative user traffic. The [evaluation guidance](https://developers.openai.com/api/docs/guides/evaluation-best-practices)
informs the mix of typical, ambiguous and adversarial cases and the separation of
automatic checks from meaning review. No new model-grader/Gemini call is included.
Review authorship must be disclosed; do not call an agent review human/independent.

## Failure, continuation and evidence

Run sequentially in listed order with an exclusive process lock. Durably record
each case identity before launching its single-request child. Record terminal
process status, stdout/stderr, original prompt, selection, available response ID
and token usage, ledger snapshot and every generated file hash. Preserve errors
and unknown outcomes. The provider's current error path does not archive every
raw HTTP-stream byte; disclose that limitation rather than claiming otherwise.

Continue untouched cases after semantic misses. Stop on transport, authentication,
accounting, protocol/integrity or native-validation failure. No failed or unknown
case may be retried. A separately recorded approval may explicitly cover resuming
only untouched cases within the original frozen batch after terminal-process,
unchanged-code/evidence and remaining-budget checks. No automatic process restart
or lock removal based on a timeout observation; use authoritative process state.
Without that continuation scope, another approval is required before resuming.

No mid-batch model/context/schema/code/case changes, recovery baselines, ledger
resets, extra probes or increased caps. Source-bound final review must address
every requirement and identify specific supporting raw facts; correct automatic
scores alone do not complete acceptance. Keep PR #14 draft until complete real
acceptance and exact-head CI are verified. Firmware, fabrication and physical
bring-up remain separate work requiring their own authorization.
