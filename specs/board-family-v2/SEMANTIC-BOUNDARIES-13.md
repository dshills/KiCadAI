# Offline semantic-boundary checkpoint

Status: **offline prototype, not live acceptance or a finished goal**.

This checkpoint reduces the model's responsibility for a small set of
unambiguous configuration instructions. It does not run another evaluation,
change the public selector, or revise any historical score.

## Identity and scope

- Base commit: `c07ef8a3b9cae06a96d4916df8109ced8484eaa5`.
- Isolated branch: `codex/semantic-boundaries-13`, development-13 worktree.
- New pure contract: `10-semantic-boundaries-offline`.
- No provider route, CLI selector, journal identity, ledger, paid request,
  firewall mutation, PR publication, or merge is added.
- Primary checkout and the evaluated development-12 source remain unchanged.
- Frozen corpus SHA-256:
  `90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867`.

The credential-safety workflow kept all verification children key-free, using
`env -i`, cached Go dependencies, `GOPROXY=off`, and `GOSUMDB=off`. The existing
key was not read into a test or used for a request.

## Implemented behavior

`internal/boardfamily/intent_boundaries.go` adds application-owned recognition
of complete, narrowly defined clauses:

1. Reviewed defaults fill only unspecified values. They are not a demand for
   unsupported hardware and must not overwrite explicit electrical values.
2. Keeping stated profiles/values unchanged reuses admission's existing
   no-substitution rule. An actual electrical violation reaches its engineering
   explanation rather than being masked by a generic unsupported-other fact.
3. Clear “not needed” statements retain the distinction between non-requirement
   and prohibition. Explicit heater-off statements preserve their local scope.
4. Pull-up-only exclusions cannot become demands or prohibitions for whole-board
   power or battery operation in the recognized complete-clause grammar.

Controls require empty additional-fact arrays for their *complete* clauses.
Constrained mention states have source-specific schema enums and matching
decoder checks. A contradictory model response is rejected with no generated
configuration; its bytes are never repaired. Source clauses, quantities, known
mention identities and every unconstrained fallback remain unchanged.

Detected quotation, example, conditional, exception and other scope cues disable
these local rules for the request. This is deliberately conservative and is
not a general proof of English discourse meaning.

## Verification

With Go 1.26.8 and cached dependencies, the following checks passed locally:

- Focused tests: 11 top-level test/fuzz-seed groups plus 83 subtests; zero
  failures. These are synthetic contract/grammar/admission checks, not 94
  independent samples and not model success-rate evidence.
- Race tests for `internal/boardfamily`, `cmd/kicadai-board-family`, and
  `internal/aiprovider`: 53.176 s, 48.949 s, and 2.645 s respectively.
- `go vet` for those three packages.
- Repository-wide `golangci-lint run ./...`: zero issues.
- Decoder fuzzing with two workers and a 10-second requested duration: 219,545
  mutation executions, passing in 11.319 s including shutdown. These mutations
  are not independent semantic examples. Checks include schema/decoder
  agreement, no caller-byte mutation, and no configuration from invalid evidence.

Focused command, with the same key-free/cached environment:

```sh
go test -race ./internal/boardfamily -run SemanticBoundary -count=1
go test -race ./internal/boardfamily ./cmd/kicadai-board-family ./internal/aiprovider -count=1
go test ./internal/boardfamily -run '^$' -fuzz '^FuzzSemanticBoundaryDecoder$' -fuzztime=10s -parallel=2
go vet ./internal/boardfamily ./cmd/kicadai-board-family ./internal/aiprovider
golangci-lint run ./...
```

Three tests use unchanged tracked prompts (`useful-03`, `useful-05`,
`refuse-02`) with **handcrafted new-contract responses**. They demonstrate
representability/admission of the two supported configurations and the real
SHT31-standard 70 pF refusal reason. They are not model replays, evidence of
improved extraction, replacement provider responses, or updated live results.

No native board generator, routing, manufacturing exporter or qualified family
is modified. Native qualification was not rerun for this unintegrated prototype.
Code review here was performed by the implementing agent, not an independent
reviewer.

## Preserved live result

The completed v9 result remains **7/14 complete passes and 2/5 useful native
bundles**. This checkpoint does not promote a case or re-interpret old bytes.
Historical v9 decoders reject the new version and the new decoder rejects v9
version bytes. Final checksum spot-checks matched the previously sealed values:

- `results.json`: `c10ce95eb807f93e33c576f4aee025692af58f4793a239f3fafc3ac06bf268a0`
- `qa.json`: `bd851a7adb85cc19242e146c25d5f9da51a44d45b4d736fd9b00f359328488ab`
- Approved development-12 evaluator:
  `b9d3a89ec13c91185fd41b0ee448f61d82e3b68dc018a8f78347faffe79833a1`

The saved LuLu rule still belongs only to that unchanged development-12
executable. It is not authorization for a new binary or a new request batch.

## Remaining work before live consideration

- Compound default wording remains on the existing extraction path, including
  “use your standard profile, 70 pF ... and the reviewed ... limits.” Do not
  remove a whole clause merely because it contains “defaults.” A successor must
  preserve exact source spans and all residual requirements.
- Distinguish “standard electrical defaults” from selecting the named standard
  profile; this checkpoint does not constrain that lexical ambiguity.
- Preserve complete no-adapter/no-substitution scope across multiple demands.
- Unknown or ambiguous negation is still the model's responsibility. Finite
  grammar checks do not establish general semantic completeness.
- Keep new negative/contrast fixtures separate from the frozen evaluation and
  explicitly label all synthetic evidence. Integrate only after review of the
  remaining boundaries, with isolated protocol and journal identities.
- Any future live attempt requires a new explicitly approved payload, request
  count, spending cap, and applicable network authority. Existing unused money
  and the consumed v9 request allowance grant no additional requests.

The design treats schema validity as a structural property, not semantic
acceptance, consistent with the official guidance on
[Structured Outputs mistakes](https://developers.openai.com/api/docs/guides/structured-outputs#handling-mistakes).
