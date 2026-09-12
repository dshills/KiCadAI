# Frozen interface evaluation v1

This specification implements the separately approved proposal at parent commit
`906cf2ab16998bfb3347a89e3382e7b9eedec2bc`. Authorization is recorded in
[AUTHORIZATION.md](AUTHORIZATION.md). All live execution follows the freeze;
preparation and fake-transport tests make no provider calls.

## Objective and denominator

Measure faithful AI-to-v3-requirement translation, structured refusal and bound
clarification completion. The denominator is exactly I01–I04 (four ready),
R01–R02 (two refusal), and C01–C02 (two clarification), in that order.
The exact ordinary-language prompts, fixed answer bundles and withheld semantic
clauses are in [corpus.json](corpus.json). No practical-board case or output is
an input. These are independently authored, public-frozen interface cases, not
blind tests or complete-board cases.

The gate is 4/4 ready, 2/2 valid refusals and 2/2 complete clarifications, with
zero unsafe accepted output, missing evidence or resource violations. Report
first-attempt results separately from bounded-correction results. Every unrun,
interrupted, configuration-failed or transport-failed case stays in its original
denominator. Candidates are not passes until all frozen clauses have a
source-bound semantic audit.

## Admission and acceptance

The evaluator sends only the frozen prompt, the production installed-capability
context, production schema, and at most eight compiler diagnostics on the single
correction. Titles, expected statuses, acceptance clauses, required-capability
lists and prewritten answers are withheld from initial provider requests.

Every accepted proposal must strict-decode and pass the unchanged production
behavioral compiler. A ready result needs an executable v3 requirement.
A refusal needs valid gap identifiers, reasons and evidence requirements, no
executable requirement, and faithful preservation of the refused goal.
A clarification needs no executable requirement, a minimal question, correct
uncertainty ownership and no guesses that replace missing facts.

Before any clarification answer is sent, the evaluator pauses for local semantic
review. A reviewer checks the complete original prompt and returned question,
paths, uncertainties and coverage against the fixed missing-fact clauses.
Approve only when the frozen answer addresses all and only the requested unknown
facts; reject a question that changes known facts, adds choices or substitutes a
different goal. Write one create-only `answer-review.json` containing case ID,
the exact selected-artifact SHA-256, approve/deny, reviewer identity and rationale.
Review time counts toward the existing case and campaign ceilings.

An approved answer is copied verbatim from the corpus and bound with the
production source/capability/prior-proposal/prior-compilation hash mechanism and
the exact clarification/uncertainty IDs. No free-form new answer is permitted.
The follow-up must be ready, retain every original fact and incorporate the
answer without recurring answered identities. Offline tampered-binding controls
must reject. Denied review is a failed clarification case, not a reason for a
different answer or another model attempt.

Final audits bind every numbered clause to exact retained files, JSON paths or
prose fields and their hashes; mark pass, fail or not_run with explanation.
They check semantic faithfulness, canonical quantities, endpoints, operating
ranges, complete coverage, forbidden additions and appropriate terminal outcome.
Correct prose without valid structured state is not a workflow pass.

## Frozen provider and resource policy

- Exact request model `gpt-5.6-sol`, Responses, strict behavioral-intent-v1,
  streaming on, background off, store false, no tools/uploads or sampling/reasoning
  override. Production defaults are not changed.
- Maximum 16,384 output tokens; encoded request 131,072 bytes; accepted production
  stream 2,097,152 bytes. Raw capture separately retains at most 8 MiB plus one
  overflow-detection byte, with completeness/truncation and redaction indicators.
  Raw capture never bypasses production acceptance.
- At most 20 generation POSTs: eight initial legs each with at most one compiler
  correction; two bound-answer legs each with at most one compiler correction.
  No diagnostic correction for a wrong but compiler-valid semantic outcome.
- At most USD 25 estimated/reserved new spend. Integer microdollar accounting
  uses [pricing.json](pricing.json): conservatively 5 per million input tokens
  (including documented cache-write premium) and 20 per million output tokens.
  Before dispatch reserve one input token per actual request byte plus 4,096,
  and all output tokens. The largest reservation is USD 1.003520; even twenty
  unreleased maximum reservations total USD 20.070400.
- Only complete accepted and internally consistent usage may replace its
  reservation. Cost all input tokens at the conservative rate; do not assume
  cache discounts. Unknown, incomplete, refused or rejected provider usage
  retains its reservation. Over-reservation or malformed accounting stops.
  Actual billing remains separately unknown; this is not an account-wide limit.
- One in-flight request; five-minute transport deadline; 20-minute case ceiling;
  90-minute campaign ceiling. Before dispatch require at least five minutes
  remaining in the effective case/campaign deadline.
- Sample process-tree RSS and evidence size every 500 ms, plus before dispatch:
  RSS 16 GiB, evidence 1 GiB. Sampling failure stops the campaign. Sampled peak
  does not prove instantaneous peak. Every live evidence write enforces the disk
  ceiling; reserve 32 MiB evidence headroom before each dispatch.
- Only direct TLS POSTs to api.openai.com/v1/responses; no ambient proxy,
  redirect, extra probe, model substitution or uncertain transport retry.
  Authorization headers are never retained. Credential/credential-shaped
  redaction is recorded; any necessary response redaction stops acceptance.
- Any provider transport, authentication, timeout, rate-limit, configuration,
  envelope/schema, incomplete-stream, parser-integrity, capture, freeze, resource
  or accounting error stops the entire campaign. Compiler-invalid proposals may
  use their one correction; semantic failures consume no extra calls.
- There is one fixed evidence root, created exclusively. An existing root blocks
  execution, whether its campaign completed or was interrupted. There is no
  resume, reset, retry, recovery or repeat flag.

## Freeze and source binding

The source commit containing evaluator, corpus, authorization, policy, pricing,
feasibility and offline preflight is committed before `freeze.json` is created.
The freeze lists exact file byte counts and SHA-256 hashes, including capabilities,
production envelope schema, captured fake requests and preflight metadata.

The sealed binary is built from a clean post-freeze commit with the freeze SHA
injected at link time. Runtime requires matching build metadata/current clean
HEAD, the reviewed source commit with only the freeze addition, matching frozen
file bytes, exact binary/evidence paths and fresh installed capability equality.
The source/freeze check repeats before every generation request.

Before any live request, verify the sealed binary SHA and build identity and
confirm that its restricted firewall access is already allowed or separately
approved. No network probe is used for account access; the first scheduled case
establishes access. A failure is retained and does not authorize recovery.

Live evidence root: `/tmp/kicadai-ai-requirement-interface-v1`.
Sealed binary: `/tmp/kicadai-ai-requirement-eval-v1`.
The command is exactly the sealed binary with `run --live --repo` and this
repository's absolute root. No output-root override is accepted.

## Evidence and reporting

Retain frozen inputs, source/binary/build identities, create-only reservations,
exact header-free requests, bounded raw responses and metadata, accepted intent,
every compiler result, correction diagnostics, selected artifacts, bound answers
and local admission reviews. Retain terminal case outcomes, not-run entries,
sampled resources and an outer file inventory. Preserve incomplete/failed evidence
without repair or promotion. Generate final semantic audits outside the terminal
raw tree; authenticate their links against its immutable inventory.

Offline compiler replay compares every typed result value, sorting only complete
diagnostic objects because existing validation traverses Go maps. It preserves
duplicate issues and every field, and does not reorder other arrays. Original
compilation bytes and the actual correction-diagnostic order remain immutable
and separately hash-authenticated. This is semantic compiler-result replay, not
a claim that production diagnostic ordering is byte-deterministic.

Report the complete table, first-attempt and final interface outcomes, live request
count, conservative cost/reservations, failure stage and evidence completeness.
Do not change the practical-board baseline, any prior journal, the stable support
boundary, or the original six-board/two-uplift goal. A passing interface gate
permits a separately scoped board evaluation; a failing gate calls for a new
decision, not tuning and rerunning this corpus. No merge, release, fabrication
or additional provider review is authorized by this run.
