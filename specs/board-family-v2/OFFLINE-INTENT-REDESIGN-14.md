# Offline intent redesign 14: separate coverage from constraints

Date: 2026-09-18. Status: offline prototype; NOT a successful live evaluation,
production replacement, completed two-family goal, or authorization to run.

## Decision

Keep the deterministic two-family generator and engineering admission unchanged.
Test a single, explicit distinction between accounting for source text and
asserting an additional engineering requirement. The implementation is isolated
on `codex/offline-intent-redesign-14`, based on
`0528e38475401a0855d5d0f7dff9bcec9ef36c17`.

The prototype preserves the handcrafted facts and decisions for all 14 frozen
cases. It addresses an expressiveness/instruction problem behind false refusals,
but does NOT establish that a model will use the distinction faithfully. Keep it
offline pending review and any separately authorized provider experiment.

This phase made no paid API requests, changed no credentials or firewall rules,
published no PR update, and did not change the primary checkout. No new CLI
protocol, provider transport, evaluator executable, or live budget was created.

## Failure motivating the change

The closed v10 evaluation still has 4/14 complete passes and 0/5 useful native
bundles. In several failures, `other` facts restate already represented sensor,
numeric, or negative-heater requirements; other facts mistake punctuation and
politeness for requirements. Admission correctly rejects a claimed unimplemented
constraint, but the extraction should not have claimed that extra constraint.
The new compiler does not rewrite those historical facts to turn failures into
passes.

The retained local results file is
`development-13/.cache/semantic-boundaries-13-live-review-01/results.json`
under the parent `.cache/board-family-v2` directory, SHA-256
`929d1e8aac3b04cfc2e003f3317df2efb375e2f57e813c720fb079d63ecf1f43`.
This is an implementing-agent review of a known regression corpus, not an
independent reliability estimate or provider attestation.

## Contract

`internal/boardfamily/intent_coverage.go` adds a pure, offline contract named
`offline-requirement-coverage-1`. It retains application-owned original text,
clauses, quantity values, lexical mention identities, controls, and narrow fixed
states. Every residual span has one mandatory coverage record:

| Record | Meaning | Engineering effect |
| --- | --- | --- |
| `represented` | All engineering meaning is already classified in listed local mention/quantity slots | No duplicate fact |
| `non_requirement` | Politeness, formatting, background, or generic task framing | No additional fact; existing slots are never deleted |
| `constraints` | One or more additional constraints or genuine unresolved questions remain | Preserve explicit facts, source span, state, and cross-clause context |

A mixed sentence such as “Use SHT31 with galvanic isolation” must reference the
sensor AND retain isolation through `constraints`. It cannot be covered merely
by pointing to SHT31. A represented prohibition remains a prohibition in the
mention slot. A missing measurement still triggers the existing targeted
question; a genuine unsupported requirement still reaches the existing refusal.

The compiler checks exact field sets, all source inventories, clause ownership,
duplicate/unknown references, input size, JSON depth, duplicate JSON keys, numeric
eligibility, and the existing admission rules. It preserves caller bytes. An
internal lowering step reuses the unchanged checked compiler chain; it is not
replacement provider evidence. Old and new protocol bytes are mutually rejected.

Instructions are standalone rather than concatenated with the historical prompt
layers. The schema shares invariant branches through local definitions. No
automatic dropping, deduplication, repair, or regrading of `other` facts occurs.

## What the offline checks establish

- All 14 frozen scenarios produce exactly the same facts, provenance, and full
  decisions as their existing HANDCRAFTED semantic-boundary fixtures. This is
  representability and compiler parity, not 14 successful model responses.
- Known sensor/profile requests with greetings need no extra engineering fact.
- Eight mixed known/unknown constraint examples retain their exact source text.
- Required, forbidden, optional, and unresolved constraint states retain scope.
- False coverage labels cannot erase separately classified wireless, voltage,
  capacitance, or later-heater requirements.
- Malformed coverage, missing slots, invented references, cross-clause references,
  and historical versions fail closed with no partial configuration.
- Captured v10 `useful-04` and `useful-05` facts still produce their original false
  refusals. The regression fixture is JSON-normalized from recorded selection
  files, NOT exact provider bytes; original selection and response hashes are
  included. Source journals remain untouched.

Verification used Go 1.26.8 with `env -i`, no provider credentials, and dependency
networking disabled (`GOPROXY=off`, `GOSUMDB=off`). Existing provider tests use
offline placeholders and simulated transports, not paid requests.

Commands run from this worktree:

```text
go test ./internal/boardfamily ./cmd/kicadai-board-family -count=1
go test -race ./internal/boardfamily ./cmd/kicadai-board-family -run 'TestRequirementCoverage|TestOfflineCoverage' -count=1
go test ./internal/boardfamily -run '^$' -fuzz '^FuzzRequirementCoverageDecoder$' -fuzztime=10s -parallel=2
golangci-lint run ./internal/boardfamily/... ./cmd/kicadai-board-family/...
go test ./internal/boardfamily -run '^TestRequirementCoverageDesignSize$' -count=1 -v
```

The two package suites, targeted race suite, and lint pass. The 10-second decoder
fuzz run completed 258,617 executions without a failure. Native KiCad integration
tests requiring an explicit native-test environment were not enabled, and no new
native-generation success is claimed. The entire repository suite was not rerun;
the production generator and catalog have no changes in this branch.

## Size and performance: measured limitation

Across the 14 frozen prompts, serialized `{input, schema, instructions}` design
components total 135,999 bytes for this prototype versus 134,327 for v10: about
1.2% MORE, even though the instructions decrease from 3,597 to 2,839 bytes. Shared
schema definitions reduced an initial 150,833-byte prototype to the current size.

These are local JSON byte counts, not tokens, complete HTTP bodies, paid usage,
model latency, or successful-board latency. There is currently NO evidence of a
speed improvement or improved live accuracy. The hypothesis is fewer semantic
mistakes and fewer development iterations, not a demonstrated faster model.

## Review and non-promotion conditions

1. Source references prove location, not meaning. An inaccurate extractor can
   still label “Use SHT31 with galvanic isolation” `represented` and omit the
   isolation requirement. `TestRequirementCoverageReferenceIsNotSemanticProof`
   intentionally demonstrates this unsafe counterexample; its passing test status
   is NOT product acceptance. The same issue exists if unknown constraints are
   falsely called background. Do not claim universal fail-closed English parsing.
2. The model must still classify polarity, alternatives, and numeric roles
   faithfully. The prototype does not fix an incorrect profile state by fiat.
3. The schema has not been submitted to the provider. Local schema checks cannot
   establish remote acceptance or extractor quality.
4. Review and fixtures are implementing-agent authored, not independent review.

Official OpenAI documentation distinguishes schema adherence from semantic
correctness and permits nested unions with closed, required object fields:
[Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs).
That guidance informed the schema and the explicit non-promotion decision; it is
not evidence that this application understands requirements correctly.

## Bounded next gate; no automatic experiment loop

Review this one offline candidate and its known omission risk first. If selected
for integration, reuse the existing transport, journal, budget, and native gates;
do not create another parallel evaluation framework or repeated recovery loop.
Any live run still requires explicit scope and budget authorization. It must use
unaltered prompts, original acceptance criteria, raw response retention, no output
repair, and a separately recorded result. Neither unused historical dollars nor
this offline approval authorizes a new request.

Do not revise the candidate mid-batch, convert synthetic passes into live passes,
or weaken the full two-family goal. Successful useful cases must still generate
and validate complete native/manufacturing artifacts. Semantic review must still
check every original requirement and truthful refusal/question, not just decision
categories. On a failed experiment, close it and report the outcome rather than
automatically starting another version.

Frozen inputs remain unchanged:

- `typed-evaluation-02/cases-02.json`:
  `90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867`
- `evidence/examples-01.json`:
  `aa2279629d361cce7c22e2c73969c69490147f342da0e768b193ea7b26a68705`
