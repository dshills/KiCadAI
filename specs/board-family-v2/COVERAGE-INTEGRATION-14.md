# Requirement coverage: command integration 14

2026-09-20. Experimental, offline-tested integration; no live acceptance.

## Outcome

The coverage candidate now uses the existing `kicadai-board-family` command,
transport, journal, budget accounting, generator, and native validation pipeline.
The default selector remains unchanged. No new evaluator or recovery framework
was introduced. No paid request, firewall change, PR publication, or merge was
performed in this phase; the primary checkout and closed v10 evidence were left
unchanged.

- `--intent-protocol requirement-coverage-v11` selects the candidate explicitly.
- `--export-live-contract FILE` with a prompt exports its actual non-secret
  request components without a key, ledger, or network request.
- `--inspect-requirement-coverage-journal DIR` replays retained local evidence
  without a key or network request.
- Prompt generation requires a fresh evidence directory and a separately
  approved policy/ledger. Flags and files do not themselves authorize spending.

The offline prototype version `offline-requirement-coverage-1` remains distinct
from wire version `11-requirement-coverage-experimental`. Neither decoder accepts
the other's bytes. No historical provider response is converted into a passing
candidate response.

## Verification

All commands used a credential-free environment, the cached Go 1.26.8 toolchain,
and disabled dependency networking. Provider-facing tests inject in-memory
transports and placeholder credentials; they do not contact OpenAI.

| Check | Result |
| --- | --- |
| Both affected package suites, uncached | Pass; boardfamily 85.0%, command 81.2% statement coverage |
| Repository-wide compile check (`go test ./... -run '^$'`) | Pass; this is not a repository-wide test run |
| Targeted new/prototype and v10-regression race tests | Pass |
| Lint of both affected packages | 0 issues |
| Original 14-case synthetic command test | 14/14 expected decisions and preserved raw extraction |
| Failure injection | Invalid extraction, malformed output, refusal, server error, missing model, broken stream, token limit, generation and validation failures fail without retry |
| Preflight/accounting | Missing prerequisites, oversize input, unknown/disputed outcomes, request/cost caps and cross-protocol ledger reuse rejected |
| Journal integrity | Tampering rejected; historical inspectors cannot accept candidate journals |
| Exact 14 request bodies, synthetic transport | 7,902–12,391 bytes, below unchanged 16,000-byte cap |
| Real KiCad native handoff with synthetic extraction | All five supported configurations pass 14/14 gates and match reviewed native-file hashes |

The five native runs exercised BMP280 standard/fast/low_current and SHT31
standard/fast through the same command. Each produced the native project,
schematic/PCB previews, BOM, validation, and manufacturing exports. The existing
source-owned qualification and engineering checks were not weakened or replaced.
No generated output was manually repaired.

Retained artifacts (relative to this worktree):
`.cache/coverage-14-integration-01/requirement-coverage-native/useful-01` through
`useful-05`. Each contains its synthetic evidence journal, ledger, and `output`
bundle. `output/validation.json` records the real KiCad 10.0.3 checks;
`output/manufacturing/manifest.json` records the export inventory. These are
synthetic-extraction native results, NOT five live AI successes.

Reproduction, with provider credentials removed from the process environment:

```text
go test ./internal/boardfamily ./cmd/kicadai-board-family -count=1
go test -race ./internal/boardfamily ./cmd/kicadai-board-family -run 'TestCoverage|TestRequirementCoverage|TestOfflineCoverage|TestSemanticBoundaryProvider|TestSemanticBoundaryLedger' -count=1
golangci-lint run ./internal/boardfamily/... ./cmd/kicadai-board-family/...
go test ./internal/boardfamily -run '^TestCoverageProviderCorpusWireAndJournal$' -count=1 -v
```

To rerun the real native test, set `KICADAI_OFFLINE_NATIVE_CLI` to KiCad 10.0.3 and
run `go test ./cmd/kicadai-board-family -run '^TestCoverageCommandNative$' -count=1
-v`. An optional, new `KICADAI_PARTITIONED_REVIEW_ROOT` retains artifacts. The test
refuses to overwrite an existing evidence directory.

## Model/accounting and source guidance

The model remains pinned to `gpt-4.1-2025-04-14`, output cap 1,600 tokens, one
attempt, no retry. A separate accounting profile prevents reuse of any historical
ledger. Conservative uncached rates remain $2/$8 per million input/output tokens,
rechecked against the [official GPT-4.1 page](https://developers.openai.com/api/docs/models/gpt-4.1).
No new live budget was approved or consumed.

The [official Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs)
informed the closed schema and the distinction between schema compliance and
semantic fidelity. No schema change here proves that a model classified every
requirement correctly; remote schema acceptance is also still untested.

## Completion boundary

The goal is NOT complete. The known omission counterexample and other limitations
in [the prototype review](OFFLINE-INTENT-REDESIGN-14.md) remain. Live accuracy and
end-to-end live speed have not improved by evidence. The closed v10 result remains
4/14 complete cases and 0/5 useful native bundles.

PR #14 was observed open/draft at commit
`0528e38475401a0855d5d0f7dff9bcec9ef36c17`, with all 33 reported checks successful.
Those remote checks do not cover this unpublished candidate. Publication to that
draft PR, if authorized, must obtain checks on the exact new head.

Before a live evaluation, freeze the candidate and request components, reuse the
existing qualification and collection/acceptance machinery, and obtain fresh
payload/budget authorization. Keep the original 14 prompts and full success
criteria. Retain all raw outputs, do not repair them, do not retry or start an
automatic successor on failure, and report the single evaluation honestly.
Physical bring-up remains a separately authorized milestone.
