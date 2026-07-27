# Implementation Plan

## Phase 1: Deterministic assessment model

- Add a dependency-light generic assessment schema to
  `internal/capabilitygate`.
- Normalize and validate requirements, evidence, gaps, risks, and checkpoints.
- Derive classification exclusively from evidence linkage.
- Compute a stable SHA-256 assessment hash and add replay tests.
- Implement monotonic checkpoint reassessment.

## Phase 2: Generic evidence adapters

- Derive block-workflow requirements from registry definitions, component
  declarations, PCB realization metadata, and block verification evidence.
- Derive explicit-circuit requirements from lowered provenance, components,
  models, and validation policy.
- Derive behavioral architecture requirements from normalized requirement,
  provider selection, catalog/model hashes, and selected contracts.
- Do not inspect project names, corpus membership, or fixture paths.

## Phase 3: Pre-mutation gates

- Add explicit experimental authorization to create options and the CLI.
- Gate direct design and requirement creation before project mutation.
- Emit structured unsupported and opt-in-required diagnostics.
- Preserve the initial architecture assessment through physical generation.

## Phase 4: Lifecycle and fabrication claims

- Append deterministic checkpoints for component selection, simulation,
  routing, writer correctness, validation, KiCad checks, and fabrication.
- Enforce monotonic classification.
- Require both physical acceptance and capability eligibility before setting
  `fabrication_ready=true`.
- Ensure experimental output cannot achieve promotion `pass`.

## Phase 5: Artifacts and documentation

- Embed the assessment in workflow results, promotion reports, manifests, and
  CLI output.
- Add capability assessment to creation evidence normalization and hashing.
- Document supported, experimental, and unsupported behavior and the evidence
  promotion path.

## Phase 6: Verification

- Add unit tests for classification, evidence linkage, normalization,
  determinism, monotonicity, and malformed evidence.
- Add workflow tests for supported, experimental opt-in, unsupported refusal,
  and fabrication-ready suppression.
- Add requirement/CLI tests for supported held-out behavior and adversarial
  missing capabilities.
- Run the complete local short suite.
- Run representative installed-KiCad supported fixtures and open-world
  promotion lanes.
- Review the staged diff with Prism, resolve high/medium findings, commit, and
  push without manually running or monitoring GitHub Actions.
