# Evidence-Driven Capability Expansion Audit

Date: 2026-07-28

## Result

The milestone is implemented. Unsupported assessments remain immutable and
fail-closed, while the new pipeline can produce deterministic generic
expansion proposals, experimental candidate registries, review-ready evidence
bundles, and explicitly approved supported-registry updates.

No production decision uses a fixture name, coordinate, allowlist, component
exception, or circuit-family branch. Promoted architecture providers advertise
only reviewed electrical contracts stored in their package; a request cannot
manufacture the bounds used to satisfy itself.

## Acceptance evidence

| Requirement | Evidence |
| --- | --- |
| Three electrically different unsupported requests produce accurate deterministic plans | `TestPlansThreeUnsupportedDomainsDeterministically` covers analog architecture, power-model, and digital-routing gaps and compares stable bytes. |
| Source-backed component and model proposals remain quarantined | `TestComponentAndModelEvidenceRemainQuarantinedUntilPromotion` verifies required source kinds, canonical artifact payload hashes, experimental status, and failed premature promotion. |
| Two reusable capabilities move from unsupported to supported | `TestTwoCapabilitiesPromoteAndServeFreshHeldOutSearches` promotes `precision_buffering` and `low_side_current_sensing` through candidate, complete bundle, exact-hash approval, and explicit mutation. |
| Fresh held-out requests consume promoted capabilities | The same test builds a new typed provider registry and selects both promoted providers for fresh behavior-only requirements. |
| Conflicting, incomplete, irrelevant, or fabricated evidence fails closed | `TestEvidenceIngestionFailsClosed` covers digest mismatch, irrelevant claims, conflicting source identity, and missing required source kind while preserving the original unsupported assessment. |
| Experimental packages never become fabrication-ready | Candidate status is fixed to `experimental`; incomplete bundles remain experimental; `Promote` rejects them even with approval. |
| Promotion is review- and mutation-gated | Registry mutation requires a complete `review_ready` bundle, approval bound to its exact hash, a non-conflicting registry, and `execute=true`. |
| Runtime uses the reviewed registry only when requested | `requirement create --capability-registry` combines reviewed declarative providers with the built-in catalog provider; the default path is unchanged. |
| CLI artifacts are strict and reproducible | `TestCapabilityExpansionPlanCLIIsDeterministic` proves identical plan-file bytes; `TestCapabilityExpansionPlanCLIFailsClosedOnUnknownInput` rejects unknown JSON. |

## Local verification

The repository’s GitHub Actions workflows were not manually invoked. All
verification was run locally:

```text
make GO_TEST_FLAGS=-short test
PASS

make lint
PASS: gofmt, go vet, and golangci-lint (0 issues)

make COVER_TEST_FLAGS=-short coverage-check
PASS: 81.4% generated-code-excluded coverage, threshold 75.0%

KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
go test -v ./internal/designworkflow \
  -run '^TestDesignExamplesOptionalKiCadBackedTier/(usb_c_led_indicator_protected|usb_c_i2c_sensor_3v3_protected|esp32_wroom_32e_minimal_pass)$' \
  -count=1 -timeout 20m
PASS: all three installed-KiCad fixtures

KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
go test -v ./internal/compositionlowering \
  -run '^TestOpenWorldCapabilityPromotionCorpusOptionalKiCadPromotion$' \
  -count=1 -timeout 20m
PASS: all five open-world installed-KiCad cases
```

The installed KiCad version was `10.0.3`.
