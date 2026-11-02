# Source-addressed v9: experimental command integration

Checkpoint date: 2026-09-17. Follows prototype commit `7d8e2488` on the isolated
`codex/source-addressed-12` branch. **Offline integration passes; live semantic
acceptance is unproven and the overall two-family goal remains incomplete.**

## Changes

The same board-family command now has an explicit `source-addressed-v9`
experimental mode. The default stays `typed-v2`. Family capabilities, source
clauses/quantities, deterministic generation, board qualification, fixtures,
prompts and acceptance thresholds are unchanged.

The new adapter reuses the existing one-attempt transport, immutable evidence
capture, fail-closed accounting and local replay. It has separate identities:

| Purpose | Identity |
| --- | --- |
| Admission | `9-source-addressed-evidence-experimental` |
| Request revision | `source-addressed-request-12` |
| JSON schema | `board_family_source_addressed_requirements_v9` |
| Journal | `source-addressed-evidence-journal-1` |
| Audit | `source-addressed-journal-audit-1` |
| Accounting | `source-addressed-v9-gpt-4.1-full-standard-2026-09-17` |

The pinned model remains `gpt-4.1-2025-04-14`, with 1,600 maximum output tokens,
16,000 maximum request bytes and the existing 50,000-microdollar reservation per
attempt. Current standard rates remain $2/$8 per million input/output tokens;
the unchanged conservative estimator is below the reservation at the configured
request/output bounds. This is an accounting limit, not permission to spend.
[Official model documentation](https://developers.openai.com/api/docs/models/gpt-4.1)
was checked on the checkpoint date.

`--export-live-contract` exports the exact input, schema and extraction context
without credentials, a ledger or a request. Its metadata explicitly states that
export does not dispatch or authorize anything. `--inspect-source-addressed-journal`
replays only local bytes. Generation requires an explicit prompt, separate budget
policy and ledger, and a new private journal outside the output directory. No
existing protocol journal or ledger is accepted as v9 evidence or funds.

The source schema/instructions did not change from the prototype. The integration
moves contract export into the provider adapter and replaces the obsolete
"not-integrated" metadata; it does not erase the prototype's limitations.
[Official structured-output guidance](https://developers.openai.com/api/docs/guides/structured-outputs#handling-mistakes)
informs the separation between schema validity and semantic correctness. Wrong
state/scope, incorrect context-only classification and omitted unfamiliar
requirements remain possible and remain demonstrated by regression tests.

## Verification performed

All Go checks used the cached **Go 1.26.8 darwin/arm64** toolchain, a clean
environment, `GOPROXY=off`, `GOSUMDB=off`, and `GOTOOLCHAIN=local`. Real credentials
were excluded. Provider tests use only injected in-memory transports and clearly
marked placeholder credentials/responses.

- Focused source-addressed suite: both packages pass. It includes all 14 original
  synthetic case decisions/configurations, CLI export/inspection and failure
  gates, no-retry checks, pre-dispatch rejection, uncertain/model-mismatch
  history halting, bidirectional ledger isolation, request/cost limits and nine
  journal-tampering scenarios.
- Exact provider-wire construction for all 14 prompts: pass. Bodies range from
  5,496 to 8,605 bytes, under the 16,000-byte cap. These are actual serialized
  requests delivered only to fake transports, not live dispatches.
- Full race tests for `internal/boardfamily`, `cmd/kicadai-board-family` and
  `internal/aiprovider`: pass, respectively 50.319 s, 47.955 s and 2.147 s.
- `go vet ./...`: pass. Repository Go-format check: pass. Installed
  golangci-lint 2.13.2 on the three affected/adjacent packages: zero issues.
- All 14 existing v8 production journals replay with the unchanged v8 inspector
  and are rejected by the new v9 inspector. This preserves their historical
  outcomes; it does not turn v8 failures into v9 successes.
- Command build, credential-free standalone contract export and standalone
  replay of all five new synthetic native journals: pass. Each replay joins one
  synthetic ledger entry and eight immutable journal files.

There were **zero new live API calls**. No provider acceptance test, external
model review, current-branch remote CI result or independent review is claimed.
Review was performed by the implementing agent. The only failed launch in this
integration checkpoint was a missing parent `.cache` directory before the native
test started; after creating the parent, one native test run completed normally.

## Real native output from synthetic extraction

The explicit native test injected the pre-existing hand-authored extraction
fixtures, then ran the real command/generator and **KiCad 10.0.3**. It completed
all five useful cases in 22.89 s of test-body time (23.145 s package time):

| Case | Family/profile | Validation gates | Raw export files checked |
| --- | --- | ---: | ---: |
| useful-01 | BMP280 / standard | 14/14 | 19 |
| useful-02 | BMP280 / fast | 14/14 | 19 |
| useful-03 | BMP280 / low_current | 14/14 | 19 |
| useful-04 | SHT31 / standard | 14/14 | 19 |
| useful-05 | SHT31 / fast | 14/14 | 19 |

Every schematic, PCB and project file hash matches the reviewed native example.
All 19 manufacturing-manifest entries per case were independently re-hashed from
the raw files, and each manifest's PCB hash matches its validated board. This
includes the export set and its logs/reports, not 19 Gerber layers. Gates include
electrical/BOM/connectivity checks, native parsing, ERC, strict DRC/parity,
round-trips, previews, manufacturing exports and input immutability. No generated
file was manually repaired. Generator-only synthetic corpus tests also retain
the existing full generated-file comparisons against the reviewed examples.

Native executable SHA-256:
`cc5433d3b41421a4cba065f291aeeb1c5fa69c7a10305a3dc28854f4e6d69534`.
Raw artifacts are retained locally under:
`.cache/source-addressed-12-integration-01/source-addressed-native/`.
They are labeled synthetic provider evidence. Raw exports contain native
timestamps; no byte-identity claim is made between those exports and old exports.
These timings exclude a real model call and are not live end-to-end latency.

The corpus and reviewed-example manifests remain byte-identical:

```text
90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867  typed-evaluation-02/cases-02.json
aa2279629d361cce7c22e2c73969c69490147f342da0e768b193ea7b26a68705  evidence/examples-01.json
```

## Reproduce the checks

Use the clean, pinned `clean_go` shell function in [VERIFICATION.md](VERIFICATION.md),
then run:

```sh
clean_go test ./internal/boardfamily ./cmd/kicadai-board-family -run SourceAddressed -count=1 -timeout=90s
clean_go test -race ./internal/boardfamily ./cmd/kicadai-board-family ./internal/aiprovider -count=1 -timeout=180s
clean_go vet ./...
```

For native checks, set `KICADAI_OFFLINE_NATIVE_CLI` explicitly to the verified
KiCad executable in the clean environment and run only
`TestSourceAddressedCommandNative`. To retain artifacts, also set
`KICADAI_PARTITIONED_REVIEW_ROOT` to a new absolute directory; an existing native
evidence child is refused, not overwritten. For read-only historical replay,
set `KICADAI_ADDRESSED_V8_BASELINE` to the old batch directory and run
`TestSourceAddressedHistoricalV8JournalIsolation`. No API key is needed for either.

## Remaining release work and authority

The most recent authenticated live result remains **v8: 11/14 complete and 3/5
native bundles**. Synthetic v9 results do not replace it. The original goal still
requires all evaluated requirements, appropriate targeted clarification/refusal,
all five useful bundles and complete-board validation without manual repair.

This checkpoint has not been pushed, published to the PR, or merged, and the
primary checkout and evaluated worktree are unchanged. No firewall rule was
created or modified. Remote CI and release-preflight binding remain to be done
before proposing a new small live trial. Its payload/budget need new explicit
approval; the completed v8 batch has no request slots left. No fabrication,
assembly or bench claim/authorization is implied.
