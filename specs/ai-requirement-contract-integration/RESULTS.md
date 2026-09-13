# Offline AI-to-requirements integration results

Date: September 11, 2026. Scope: the separately approved offline integration
milestone, starting from `5cd36155bb412798edc8c6c05874ecfbf69dc316` on
`codex/ai-requirement-contract-integration`.

## Outcome

The AI-facing v3 contract is now more consistent with its compiler, and the
independent synthetic ready, refusal and bound-clarification workflows pass.
Malformed local shapes, invalid semantic references and corrupted provider
streams retain fail-closed handling. No live provider evaluation or new board
generation was performed. This is an offline integration result, not a measured
improvement to the practical-board success rate.

## Implemented changes

1. **Version and schema alignment.** The advertised v3 boundary now rejects other
   requirement versions even when invoked directly. Later-version control,
   transition and event fields are constrained to null/empty forms in provider
   output. Schema emission uses fresh copies of the compiler's registered
   vocabulary, exclusive endpoint-binding forms, numeric limits, relation/value
   forms, canonical metric/analysis/unit pairs and operating-axis/unit pairs.
   All object fields remain required and additional properties remain forbidden.
2. **Source and blocker consistency.** Coverage dispositions and reference kinds
   are coupled in the schema. Clarification and uncertainty identities, ownership
   and resolution rules are described in provider context. Capability identifiers
   cannot be prose. Compilation still determines reference existence, complete
   source accounting, valid bound ordering and exclusive terminal outcomes.
3. **Provider-response integrity.** Stream parsing handles complete SSE frames,
   checks event/status consistency, rejects duplicate terminal or post-terminal
   data, validates contiguous sequence numbers when supplied, and checks streamed
   output text against the terminal output. Completed responses with negative,
   inconsistent or over-limit usage are rejected. Raw stream error details are
   not exposed through the new error path.

The provider byte/token caps, background policy, request timeout and retry policy
were not increased. The 16,384-token stream limit remains 2,097,152 bytes.
Compiler engineering validation and the stable-support boundary were not
relaxed. The shared architecture validator still supports its existing versions;
only the already advertised behavioral-intent v3 boundary is pinned explicitly.

## Verification

All checks below ran locally with synthetic transports or read-only retained
evidence. Provider credentials and the live-test switch were removed from Go
test and lint processes. The existing credential was used only by the historical
evidence authenticator's local exact-secret scan; it was not printed or sent.

| Check | Observed result |
| --- | --- |
| Five affected packages, uncached short race tests | Passed: architecturesearch, behavioralintent, aiprovider, practicalboardeval, cmd/kicadai |
| Final additional local-form and contract regressions, race enabled | Passed |
| Scoped golangci-lint with writable workspace cache | 0 issues |
| Strict schema structural budget | 80,081 bytes; 944 properties; 690 enum values; 11,698 counted string characters; envelope depth 9 |
| Ready/refusal/clarification synthetic wire → strict decode → compile | All three expected outcomes, no blocking issues |
| Bound clarification answer and replay protection | Ready after valid answer; four source/capability/prior-artifact hash substitutions rejected |
| Invalid local shapes | 20 mutations rejected by both schema checks and compiler |
| Compiler-only invalidity | Unknown endpoint, reversed bounds and incomplete source coverage rejected despite schema conformance |
| Direct version boundary | Versions 1, 2, 4, 5 and 6 rejected at behavioral-intent v3 entry |
| Registered local forms | All three endpoint forms, all directions, seven constraint relations, 23 metric triples and nine operating axes accepted by the emitted local schemas |
| Invalid synthetic streams | 12 cases rejected without accepted intent/usage or hidden request retry |
| Valid stream framing | Numbered events, comments/DONE, CRLF, multiline data and EOF final frame accepted with usage preserved |
| Opaque metadata and usage | Small metadata accepted; oversized metadata rejected at unchanged cap; four inconsistent usage records rejected |
| Historical evidence | 472 files / 101,237,999 bytes matched the published inventory; all three prior inventories reverified |
| Publication/audits/archive | 16 cases, 108 clauses, retained audit bindings and local archive hash verified |
| Historical source/frozen inputs | No diff in practical-board specs/evaluator; all 25 frozen files and sealed evaluator identity reverified |
| Whitespace integrity | `git diff --check` passed |

The independent schema checker in tests implements only the emitted subset and
fails on unknown keywords. It is not a general JSON Schema implementation or an
API-side schema acceptance test. The structural limits and supported constructs
were checked against the [official Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs).

The earlier complete repository CI belongs to the baseline publication, not this
new local implementation. GitHub reported that baseline PR #11 was merged with
successful checks at head `5cd36155bb412798edc8c6c05874ecfbf69dc316` on this date.
No fresh full-repository coverage, new native KiCad replay, independent external
review or CI result is claimed for this implementation.

### Reproduction

Run from the repository root. Tests deliberately disable live credentials and
the explicit live switch. Equivalent local cache paths may be used.

```sh
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY -u GOOGLE_API_KEY \
  -u KICADAI_OPENAI_LIVE_TEST GOTOOLCHAIN=go1.26.8 GOENV=off GOWORK=off \
  GOFLAGS= GOEXPERIMENT= CGO_ENABLED=1 GOMAXPROCS=4 \
  GOCACHE="$PWD/.cache/go/build" GOMODCACHE="$PWD/.cache/go/mod" \
  go test -short -race -p=1 -count=1 -timeout=5m \
  ./internal/architecturesearch ./internal/behavioralintent \
  ./internal/aiprovider ./internal/practicalboardeval ./cmd/kicadai

env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY -u GOOGLE_API_KEY \
  -u KICADAI_OPENAI_LIVE_TEST GOLANGCI_LINT_CACHE="$PWD/.cache/golangci-lint" \
  GOTOOLCHAIN=go1.26.8 GOENV=off GOWORK=off GOFLAGS= GOEXPERIMENT= \
  CGO_ENABLED=1 GOMAXPROCS=4 GOCACHE="$PWD/.cache/go/build" \
  GOMODCACHE="$PWD/.cache/go/mod" golangci-lint run --timeout=5m \
  ./internal/architecturesearch ./internal/behavioralintent ./internal/aiprovider

node specs/practical-sensor-controller-boards/publication-v2/authenticate.mjs
node specs/practical-sensor-controller-boards/publication-v2/verify-publication.mjs --verify-local-archive
node specs/practical-sensor-controller-boards/publication-v2/verify-audits.mjs
git diff --check
```

Authentication requires the existing local raw roots, sealed historical binary,
archive and credential for a local exact-match scan. It makes no provider calls
and does not rewrite inventory or result files. It authenticates historical
execution, not the new implementation's ability to reproduce model output.

## Remaining limitations and decision

- Schema conformance does not prove that a proposal faithfully represents the
  prompt, that installed capabilities can synthesize it, or that its electrical
  and physical design is correct.
- All newly successful contracts were authored offline. The live first-attempt,
  bounded-correction, refusal and clarification success rates remain unmeasured.
- Legacy streams with no sequence numbers remain accepted when otherwise valid;
  sequence integrity is enforced for numbered streams. Streams without text
  deltas rely on terminal-response validation. These are intentional compatibility
  boundaries, not claims of exhaustive transport authentication.
- Missing/all-zero usage remains compatible with the existing decoder; it is not
  proof of zero provider cost. Rejected oversized/incomplete responses may incur
  provider cost even when no accepted usage is returned.
- The protocol-v2 result remains **0/8 complete boards**, **1/4 valid refusals**,
  **0/2 complete clarifications** and **0/2 paraphrases**. No retained failure has
  been relabeled, repaired or promoted. Fresh/paired growth metrics remain null.

The offline work is ready for user review. The next step is the separately
authorized [live interface evaluation proposal](LIVE-PROTOCOL-PROPOSAL.md), not a
rerun of the frozen practical-board campaign. No new live budget, publication,
merge, release or fabrication authorization is inferred.
