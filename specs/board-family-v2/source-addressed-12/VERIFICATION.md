# Offline verification record

Date: 2026-09-17. Base commit: `a7b4d51e78940b79c9e916c82e913535e0068531`.
Worktree: `.cache/board-family-v2/development-12`, branch
`codex/source-addressed-12`. Review and test interpretation were performed by the
implementing agent, not an independent reviewer.

## Reviewed source identity

SHA-256 of the three authored Go files at verification:

```text
e12efb9b2be55fd7e7f1a7591421cb15b60b46fbfa0c92fdece549da148e7774  internal/boardfamily/intent_addressed.go
3645ff4553a05f198db4b8b8df3aa2910f68d33be2c09c37e1cece4baacc88d8  internal/boardfamily/intent_addressed_test.go
c4e3999f3736a51c2ddcfa6995ac17d5feac451448bca5f87b0d32535e827f04  cmd/kicadai-board-family/addressed_corpus_test.go
```

Unchanged frozen inputs:

```text
90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867  specs/board-family-v2/typed-evaluation-02/cases-02.json
aa2279629d361cce7c22e2c73969c69490147f342da0e768b193ea7b26a68705  specs/board-family-v2/evidence/examples-01.json
```

These local hashes identify bytes, not provider or independent attestation.

## Observed checks

Final-source checks using the installed Go 1.27.1 (`darwin/arm64`):

- Focused `SourceAddressed` tests in both affected packages: pass.
- Full tests, `-count=1 -timeout=120s`: pass (boardfamily 37.285 s;
  command 30.504 s).
- Full race tests, `-race -count=1 -timeout=180s`: pass (boardfamily
  43.682 s; command 40.530 s).
- `go vet` in both affected packages: pass.
- `FuzzSourceAddressedNeverMutatesOrPanics`, `-fuzztime=10s -parallel=2`:
  pass, 61,010 executions after 80 seed/cache baseline entries; total package
  time 11.302 s. This is bounded fuzzing, not exhaustive verification.
- Verbose synthetic corpus comparison: all 14 cases pass. Source facts and
  references, numerical assertions, expected dispositions and exact useful
  configurations are preserved. Input-component and synthetic-output byte totals
  are recorded in the README. They are not provider results.

Compatibility checks using the project's cached Go 1.26.8 (`darwin/arm64`):

- Full two-package race tests, `-race -count=1 -timeout=180s`: pass
  (boardfamily 45.205 s; command 41.648 s).
- `go vet` in both affected packages: pass.

The explicit `KICADAI_OFFLINE_NATIVE_CLI` variable was not set. Tests requiring
that opt-in skipped native KiCad execution. No new live model result, native
bundle, remote CI pass, latency target, or physical validation is claimed.

## Reproduction without credentials or downloads

Run from this worktree. This example uses the existing local module/toolchain
cache; another machine must supply its own already-provisioned cache paths.

```sh
clean_go() {
  env -i HOME=/Users/dshills \
    PATH=/Users/dshills/Development/projects/KiCadAI/.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64/bin:/opt/homebrew/bin:/usr/bin:/bin \
    GOROOT=/Users/dshills/Development/projects/KiCadAI/.cache/go/mod/golang.org/toolchain@v0.0.1-go1.26.8.darwin-arm64 \
    TMPDIR=/private/tmp \
    GOCACHE=/private/tmp/kicadai-connection-06-go-cache \
    GOMODCACHE=/Users/dshills/Development/projects/KiCadAI/.cache/go/mod \
    GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=4 \
    go "$@"
}
clean_go test ./internal/boardfamily ./cmd/kicadai-board-family -run SourceAddressed -v -count=1 -timeout=90s
clean_go test -race ./internal/boardfamily ./cmd/kicadai-board-family -count=1 -timeout=180s
clean_go vet ./internal/boardfamily ./cmd/kicadai-board-family
clean_go test ./internal/boardfamily -run '^$' -fuzz '^FuzzSourceAddressedNeverMutatesOrPanics$' -fuzztime=10s -parallel=2 -timeout=60s
```

No real API key is inherited. Existing command tests use their own fake transport
and placeholder credentials. Download and automatic toolchain lookup are off.
The prototype has no provider-dispatch integration.

## Development failures retained in context

The initial test launch used the default module-cache path and failed before
testing because the sandbox could not write a dependency lock there. It was
rerun using the existing project cache with network downloads still disabled.
The first corpus adapter did not compile because it referenced a nonexistent
`Decision.Questions` field; it was corrected to compare targeted clarification
messages and exact configurations.

A later deliberately wrong extraction test initially expected an omitted
heater-on demand to be accepted. The unchanged prompt-level engineering guard
actually rejected the explicit wording. That incorrect test expectation caused
one focused run and one race run to fail. The test now records this preserved
safeguard separately from the demonstrated semantic counterexamples; no
production safety guard was relaxed. The final-source runs above passed.

## Release boundary

This review found no reason to change the deterministic board generator,
qualification examples, default protocol, existing model, transport or source
admission rules. It deliberately does not approve release: the representation is
not integrated, provider schema acceptance is untested, state/scope errors and
unknown-requirement omission remain possible, and no new live authorization
exists. The last authenticated v8 result remains 11/14 complete and 3/5 native;
the overall goal remains active and unmet.
