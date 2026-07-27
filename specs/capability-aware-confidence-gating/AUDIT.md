# Capability-Aware Confidence Gating Completion Audit

Date: 2026-07-27

## Outcome

KiCadAI now evaluates normalized creation requests before project mutation and
classifies them as `supported`, `experimental`, or `unsupported` from
deterministic, evidence-linked requirements. Experimental generation requires
explicit opt-in and cannot receive fabrication-ready or promotion-pass status.
Unsupported requests return structured diagnostics without writing a project.

The assessment records domain, architecture, component/model, physical, and
verification requirements; verified, inferred, missing, and failed evidence;
gaps; risks; monotonic stage checkpoints; fabrication eligibility; and a stable
SHA-256 assessment hash. Unlinked inference must be explicitly marked advisory;
required inferred evidence always yields an experimental classification.
Workflow, promotion, and manifest artifacts embed the same validated
assessment.

## Generic implementation

- `internal/capabilitygate` owns the dependency-light assessment schema,
  normalization, validation, classification, stable serialization, hashing,
  and monotonic reassessment.
- Architecture assessments derive from normalized typed requirements,
  deterministic search selections, catalog hashes, provider evidence, physical
  bounds, and the requested verification contract.
- Design workflow assessments derive from explicit-circuit provenance or
  registered block definitions and built-in verification evidence.
- The gate executes before planning, writing, or output-directory mutation.
- No fixture names, fixture coordinates, circuit-family allowlists, special
  request schemas, or block-family branches were added.

## Regression and adversarial evidence

Automated tests prove:

- promoted block and open-world corpus cases classify as supported;
- compositionally supported held-out requests complete the offline workflow;
- an unknown block is refused as unsupported before filesystem mutation;
- an unpromoted block is experimental and requires explicit opt-in;
- experimental results cannot retain fabrication-ready status or promotion
  `pass`;
- downstream failed evidence downgrades a supported assessment;
- verified evidence requires a stable source and SHA-256 digest;
- identical inputs with reordered registries/evidence serialize byte
  identically;
- manifests round-trip the assessment and hash.

## Local verification

Complete repository short suite:

```text
make GO_TEST_FLAGS=-short test
PASS: all Go packages
```

Promoted open-world offline workflow:

```text
go test ./internal/compositionlowering \
  -run '^TestOpenWorldCapabilityPromotionCorpusPassesOfflineWorkflow$' \
  -count=1 -timeout=20m -v
PASS: 5/5
```

Representative installed-KiCad fixtures:

```text
go test ./internal/designworkflow \
  -run '^TestDesignExamplesOptionalKiCadBackedTier$/(usb_c_led_indicator_protected|usb_c_i2c_sensor_3v3_protected|esp32_wroom_32e_minimal_pass)$' \
  -count=1 -timeout=20m -v
PASS: 3/3
```

Installed-KiCad open-world promotion:

```text
go test ./internal/compositionlowering \
  -run '^TestOpenWorldCapabilityPromotionCorpusOptionalKiCadPromotion$' \
  -count=1 -timeout=20m -v
PASS: 5/5
```

The installed-KiCad lanes require clean ERC, strict DRC, complete required-net
routing and connectivity, writer correctness, and zero normalized schematic
and PCB round-trip differences.

## Documentation

The README, project status, AI generation guide, CLI reference, documentation
index, roadmap, and the new
[capability gate guide](../../docs/capability-gating.md) explain the envelope,
experimental authorization, deterministic refusal, artifact contract,
monotonic evidence lifecycle, and the process for promoting generic
capabilities.
