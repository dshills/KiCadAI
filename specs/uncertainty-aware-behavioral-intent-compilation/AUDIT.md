# Uncertainty-Aware Behavioral Intent Compilation Audit

## Result

The milestone satisfies the specification. Ordinary-language circuit requests
enter through a strict AI proposal boundary, are compiled into behavioral
requirements or one stable blocker outcome, and only become executable after
deterministic architecture-search and hash-bound closed-loop evidence pass.
The compiler never accepts provider-selected topology, parts, nets, placement,
routing, solver controls, or validation evidence.

## Requirement Traceability

| Specification clause | Authoritative implementation | Test and corpus evidence |
| --- | --- | --- |
| Public compilation contract and fail-closed statuses | `internal/behavioralintent/model.go`, `compile.go` | `TestCompileReadyRequirementIsDeterministicAndEnforcesFullAcceptance`, clarification, unsupported, invalid, and missing-capability tests |
| Compiler-owned source statements and two-way coverage | `internal/behavioralintent/source.go`, `compile.go` | `TestPrepareSourceOwnsStableStatementBoundaries`, `TestCompileFailsClosedOnMissingCoverageAndDuplicateClarificationPath`, adversarial blocker mutation tests |
| Behavioral-only v3 scope and authoritative semantic validation | `internal/behavioralintent/schema.go`, `internal/architecturesearch/validate.go` | `TestProposalSchemaIsStrictFullyRequiredAndV3Only`, `TestSemanticRegistriesRemainValidatorAuthoritative` |
| Minimal clarification ownership | `internal/behavioralintent/compile.go` | `TestCompileAmbiguousPromptRequestsOneTargetedClarification`, `TestCompileCoalescesClarificationsAndReverseCoversBlockers` |
| Stable capability gaps | `internal/behavioralintent/compile.go`, `search.go`, `closed_loop.go` | unsupported compiler test, search ambiguity test, closed-loop mismatch tests, held-out unsupported cases |
| Strict provider and installed-capability boundary | `internal/behavioralintent/decode.go`, `provider_context.go`, `internal/aiprovider/reference_profiles.go` | strict decode/schema tests, provider-context fail-closed tests, registry-derived capability tests |
| Exact-source follow-up binding | `internal/behavioralintent/follow_up.go` | exact-evidence binding test; unrelated, missing, invented, circular, and unknown-field mutation tests |
| Architecture-search qualification | `internal/behavioralintent/search.go`, `internal/architecturesearch/semantic_capabilities.go` | selected-architecture and ambiguity tests; deterministic registry snapshot tests |
| Hash-bound closed-loop qualification | `internal/behavioralintent/closed_loop.go` | passing-evidence and mismatched-hash tests |
| CLI orchestration, bounded retries, evidence persistence, and replay | `cmd/kicadai/behavioral_intent.go` | recorded clarification, bound follow-up, retry-policy, ready-v3, persisted search/closed-loop evidence, and byte-identical two-run replay tests |
| Frozen held-out behavioral corpus | `internal/behavioralintent/testdata/held_out_corpus/manifest.json` | `TestFrozenHeldOutBehavioralIntentCorpus`: 24 SHA-256-pinned prompts, 12 paraphrase groups, six domains, 12 ready, four clarification, eight unsupported |
| Supported end-to-end promotion | `internal/compositionlowering/promotion_test.go` | offline and optional installed-KiCad held-out promotion tests cover search, simulation, corners, lowering, routing, connectivity, writer correctness, strict ERC/DRC, zero-diff round trip, and deterministic project replay |
| No production identity shortcuts | generic compiler/search/evidence code above | held-out test rejects topology/part/net/layout language; full repository tests preserve existing open-set, amplifier, MCU, sensor, power/protection, USB-C, routing, writer, and KiCad lanes |

## Fresh Verification

The following commands passed from the completed working tree on 2026-07-21:

```text
go test ./internal/behavioralintent ./internal/architecturesearch ./internal/aiprovider ./cmd/kicadai
go test ./...
```

The full suite passed with exit code 0; `internal/compositionlowering` completed
in 597.042 seconds and `internal/designworkflow` completed in 48.557 seconds.

The supported held-out corpus also passed its real installed-KiCad lane:

```text
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
go test ./internal/compositionlowering \
  -run '^TestFrozenBehavioralIntentHeldOutReadyCorpusOptionalKiCadPromotion$' \
  -count=1 -v
```

All six unique supported behavioral contracts passed. The installed-KiCad run
completed in 244.49 seconds.

The protected USB-C regressions were then reproduced after the final compiler
changes:

```text
go test ./internal/designworkflow \
  -run '^TestDesignExamplesOptionalKiCadBackedTier$/(usb_c_led_indicator_protected|usb_c_i2c_sensor_3v3_protected)$' \
  -count=1 -v
```

Both cases passed installed KiCad in 11.38 seconds. Their promotion helper
requires clean ERC, strict DRC, connectivity, complete routing, writer
correctness, and zero normalized round-trip differences.

`git diff --check` also passed. Prism staged-diff review, exact-commit push, and
GitHub Actions verification are release controls performed after this audit is
staged; their results are reported with the delivered commit.
