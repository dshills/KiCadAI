# Practical AI-generated sensor/controller boards

Date: 2026-09-10
Status: acceptance frozen by the companion `freeze.json`; zero corpus executions
or live requests occurred before sealing these bytes.

## Objective and authority

Demonstrate useful ordinary-language requests reaching electrically justified,
readable, fully routed native KiCad projects without manual circuit or layout
repair. This is a new experiment, not a continuation or re-scoring of V18–V23.
The user supplied the milestone and explicitly authorized reuse of the existing
OpenAI API key for testing on 2026-09-10. Credentials stay in the environment;
they must never appear in evidence, shell arguments, commits, or reports.

The production baseline is merged main
`87c411b7a13bdeb0efc6ff36b35e9a69c4e2706d`. Evaluator and documentation additions
do not constitute production capability growth. Historical evidence is immutable.

## Bounded product scope

Low-voltage, low-energy controller boards with reviewed power, sensor/analog,
bus/interface, clock/programming, indicator and small-load support. Firmware,
RF performance, high-speed links, mains, battery charging, high-energy loads,
safety-critical use and fabrication authorization are outside scope. The eight
positive cases combine at least three substantive electrical functions each.
Requirements specify external behavior and genuine bounds, not parts, topology,
pins, internal wiring, layout coordinates, or routes.

The corpus has eight positive boards, four predetermined refusals, two targeted
clarifications, and two positive paraphrases. Paraphrases and answered
clarifications do not increase the eight-board denominator. Cases P07 and P08
are reserved against implementation tuning. They are **public frozen**, not
independently authored or blind: the same agent prepares and inspects the set.
Both must pass final acceptance. Baseline outcomes for them are recorded, but
their individual details may not motivate production changes.

## Freeze and evaluation protocol

Before the first corpus execution, commit and SHA-256 seal this specification,
the corpus, feasibility review, evaluator source/tests, commands and toolchain
inventory. A separate freeze record binds the production commit, evaluator
commit, catalog/model/library identities and every acceptance file. Preparation
tests use synthetic evaluator inputs only, never the candidate corpus.
`QUALITY.md` specifies evidence grain, missing-data treatment and the sole
library-identity normalization (collection timestamp only). `environment.json`
pins the installed tools, baseline snapshot and unchanged closed-loop policy.
The supervisor checks these identities before dispatching any provider request.

Build executable workers with
`-ldflags '-X kicadai/internal/practicalboardeval.FreezeSHA256=<freeze-file-sha256>'`.
The worker rejects an empty or mismatched embedded freeze identity before any
provider call. An ordinary development build is live-disabled. Retain binary
SHA-256 and `go version -m` output separately for baseline and final builds.

Run one baseline campaign and one final campaign. Preserve every attempt,
partial artifact, error and timeout. No case replacement, denominator change,
budget increase or final re-run to improve results. Infrastructure interruption
is recorded; uncertain requests are not silently retried. Authentication means
checked source/input/output identities and replay, not a claim of third-party
certification or independent authorship.

The baseline and final use identical evaluator bytes and acceptance rules.
Baseline accepted structured requirements are replayed on the final engine as a
separate paired control. A claimed uplift must pass this same-input comparison,
as well as the fresh final live workflow, so provider randomness alone cannot
be credited as an engine improvement. A baseline compilation failure can count
as compiler uplift only with the same retained provider proposal, plus a passing
fresh final live attempt; edited or newly authored answers cannot manufacture
that uplift.

Before production edits, publish a baseline-driven scope decision naming at most
three reusable fixes, independent regression tests and why two complete uplifts
are credible. If fewer than two suitable failures exist, or the remaining work
requires a materially different capability scope, stop for a scope decision.

## Live provider controls

- Endpoint: OpenAI Responses API through the existing `aiprovider` implementation.
- Explicit model: `gpt-5.6-sol`; do not rely on mutable environment defaults.
- Profile: `behavioral-intent-v1`, strict JSON schema, installed capability context.
- Streaming on; background off; `store=false`; no tools or external file uploads.
- Maximum output: 16,384 tokens per request. No sampling/reasoning override:
  preserve the existing client's omitted fields and record the exact payload.
- One initial attempt and at most one diagnostic correction for invalid
  structured output per prompt. A correct refusal/clarification is terminal;
  electrical/layout failures do not trigger another model request. Transport,
  authentication, quota and timeout failures receive no automatic retry.
- Per campaign: 14 original prompts, P01/P05 paraphrases, and at most one
  prewritten clarification-answer turn for each ambiguous prompt: 18 logical
  inputs, at most 36 POST requests. Both campaigns together: at most 72 POSTs.
- Maximum encoded request body: 131,072 bytes. Conservative reservation before
  dispatch uses one input token per request byte plus 4,096 overhead tokens,
  and the full output-token cap. Reconcile to returned usage after completion;
  missing usage retains the reservation. Total reserved/reconciled test spend
  must remain at or below USD 50. No additional requests outside the manifest.
- Baseline, final and same-input downstream replay are reported separately.
  Record first-attempt and bounded-correction outcomes, returned model/response
  identity, token usage, finish reason, elapsed time and all candidate JSON.

The documented model supports Responses and Structured Outputs. Schema validity
does not establish faithful requirements or electrical correctness; both require
independent acceptance checks. Sources checked 2026-09-10:
[model and pricing](https://developers.openai.com/api/docs/models/gpt-5.6-sol),
[Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs).
The reservation rates are USD 4/M input and USD 20/M output; cache discounts are
ignored. If documented/account billing differs before freeze, revise this draft
before any calls; after freeze, stop rather than silently changing the model.

## Resource and retention controls

Run cases serially, with `GOMAXPROCS=4`, no overlapping promotion campaigns.
Each case gets a 20-minute wall-clock cap including both downstream replays,
and a sampled 16 GiB aggregate process-tree RSS cap. Each campaign gets two
hours wall time; the shared campaign tree is capped at 10 GiB retained evidence.
The same caps apply to paired
controls. A cap hit is a failure, never an omitted case. Record sampling interval
and peak measured RSS; do not present sampled peaks as exact instantaneous maxima.
Build/test/review costs are recorded separately from per-case generation costs.

No original V18–V23 experiment is rerun. Existing reference fixtures may be used
for pre-freeze evaluator plumbing checks and required regression tests; they
are never counted as new corpus cases or capability gains. Retain new evidence under a dedicated
campaign directory; fail if a destination already exists. Secrets and HTTP
authorization headers are excluded at capture time. Save prompts, capability
context, schema, provider proposals, compilation, search/rejections, selected
component/model provenance, electrical reports, workflow stages, native files,
previews, normalized replay identities, resource metrics, and a file inventory.
Do not prune failed attempts to meet the storage cap; stop the campaign instead.

## Acceptance gates

Gate order is requirement interpretation, component/model qualification,
electrical analyses, schematic/readability, placement, routing/connectivity,
native validation/writer, deterministic replay. A missing or skipped required
gate is not a pass. Report the first failed gate and all observed subsequent
failures; downstream stages not reached are explicitly `not_run`.

1. Every numbered acceptance clause in the corpus must be preserved in the
   accepted requirement and traced to a check or explicit limitation. Provider
   coverage assertions alone are insufficient. A read-only semantic audit may
   grade evidence, but must not edit the proposal or implementation output.
2. All selected parts, pins, footprints, ratings and applicable models must be
   supported by reviewed provenance. No placeholder substitutions.
3. Required voltage, load, timing, supply/tolerance/temperature corners, startup,
   partial-power/fault conditions and applicable thermal limits must pass. The
   report retains assertion ID, observation, condition, actual and bound where
   available; unavailable measurements are marked unavailable, never inferred.
4. Readability rubric: every functional group is identifiable; all external
   interfaces and power/return nets have unambiguous labels; references/values
   are legible at normal sheet scale; symbols and labels do not collide; wires
   have clear junction semantics; cross-sheet links resolve; support components
   can be associated with their parent function. Every item must pass, with
   inspectable rendered sheet evidence. Machine checks plus read-only visual
   review are required. A self-review is disclosed as such.
5. No required connection may remain unrouted. Placement, board dimensions,
   clearance, and footprint constraints must pass without manual adjustments.
6. Installed KiCad ERC, strict DRC, connectivity, writer correctness and zero
   normalized round-trip differences must all pass. Synthetic libraries or
   disabled KiCad checks cannot qualify.
7. Two clean downstream runs from the same accepted structured requirement
   repeat architecture selection, electrical synthesis and physical creation.
   Compare normalized evidence and generated project identity. The exact
   normalization is: replace the known output root (including its canonical
   macOS path alias); replace only random numeric suffixes of native temporary
   directories under `project/.kicadai/checks` and `project/.kicadai/roundtrip`;
   replace only nonnegative `duration_ms` values of the `kicad_checks` stage's
   ERC/DRC command summaries; use the existing round-trip byte normalizer for
   native project files. Include every generated hierarchical sheet, board,
   project file, local symbol/footprint and library table, not just root files.
   Never normalize away measured values, failures, part identities or geometry.
8. No human or agent manually repairs evaluated components, nets, placement or
   routing. Generic production changes are allowed only in the scoped
   implementation phase, followed by the single frozen final evaluation.

Human assistance is accounted separately: authoring/feasibility review, provided
clarification answers, credential approval, read-only acceptance review, and
manual implementation repair. Record counts, available elapsed measurements and
unknown human-active minutes honestly. Agent execution time is not human time.
Zero manual repair does not mean zero engineering review.

The evaluator calls a compiler-ready contract `compiled-requirement.json`, not
an independently accepted design. Machine-complete results remain
`complete_candidate` until every frozen acceptance clause and the visual rubric
have been graded. The acceptance audit is independent of provider assertions;
it is a disclosed self-review, not a claim of a second independent reviewer.

## Success and stopping rule

Success requires at least 6/8 complete positives, at least two materially
different paired baseline failures becoming complete passes, preservation of
every baseline pass, P07 and P08 complete passes, all four correct refusals,
both targeted clarifications without invented assumptions, zero manual repair,
all frozen budgets respected and the required existing regression suites green.
Paraphrase consistency is reported separately and cannot hide a primary failure.

Publish specification/plan/corpus, authenticated baseline/final comparison,
native projects/previews where generated, rationale/provenance, limitations,
per-case provider/resource/assistance metrics, reproduction instructions,
inventory, review dispositions and a reviewed PR. A negative final result is
published honestly with exact blockers; it does not authorize another correction
cycle. No merge, release, fabrication approval or stable-support admission.

## Pinned reproduction and verification commands

Run from the repository root with `GOTOOLCHAIN=go1.26.8`, `GOENV=off`,
`GOWORK=off`, empty `GOFLAGS`/`GOEXPERIMENT`, `CGO_ENABLED=1`, `GOMAXPROCS=4`,
`GOCACHE=$PWD/.cache/go/build`, `GOMODCACHE=$PWD/.cache/go/mod`, and
`GOLANGCI_LINT_CACHE=$PWD/.cache/golangci-lint`. Native commands require the
CLI, symbols and footprints paths recorded in `environment.json` in
`KICADAI_KICAD_CLI`, `KICADAI_SYMBOLS_ROOT`, `KICADAI_FOOTPRINTS_ROOT`.
No other capability/model/catalog/library overrides are permitted.

```sh
go test -race ./internal/practicalboardeval ./cmd/practical-board-eval -count=1
node --test specs/practical-sensor-controller-boards/evidence-utils.test.mjs
node --check specs/practical-sensor-controller-boards/run-campaign.mjs
go test -short -p=1 -count=1 -timeout=15m ./...
golangci-lint run ./cmd/... ./internal/...
```

The final native regression command is
`go test ./internal/practicalboardeval -run '^TestEvaluatorOptionalNativePlumbingControl$' -count=1 -v -timeout=11m`
with `KICADAI_PRACTICAL_EVAL_SELFTEST=1` and a fresh
`KICADAI_PRACTICAL_EVAL_SELFTEST_OUTPUT` outside historical evidence. It covers
the existing regulated MCU/sensor subsystem twice, including power, simulation,
schematic, routing and native writer/round-trip checks. Any additional regression
needed for a scoped fix must be named in the pre-implementation scope decision;
it cannot replace or weaken these required checks.

Build with the embedded freeze hash as above into a new binary path. The single
shared campaign root is `/tmp/kicadai-practical-sensor-controller-public-1`.
Invoke `node specs/practical-sensor-controller-boards/run-campaign.mjs` with four
arguments: absolute worker binary, absolute repository root, that campaign root,
and `baseline`, `final`, or `paired`. Preserve all phase directories and the
shared request journal. Never repeat a completed or interrupted live phase.
