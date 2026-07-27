# Capability-Aware, Confidence-Gated Circuit Generation

## Objective

KiCadAI must decide whether a requested design is inside its reproducibly
validated capability envelope before it mutates a project or claims that the
result is fabrication-ready.

The decision is a machine-readable assessment with one of three
classifications:

- `supported`: every required capability is linked to verified, reproducible
  evidence and the request may proceed automatically;
- `experimental`: the request is structurally representable but at least one
  required capability is supported only by inferred or provisional evidence;
- `unsupported`: at least one required capability is missing, contradicted, or
  failed.

Classification is independent from user authorization. Experimental opt-in
permits generation to proceed, but never upgrades evidence and never permits a
fabrication-ready claim.

## Capability Requirements

An assessment must identify normalized requirements in these generic
categories:

- electrical domains;
- architecture or function capabilities;
- component and package identities;
- simulation and behavioral models;
- physical realization constraints;
- verification obligations.

Requirements must be derived from normalized requests, registered block and
provider metadata, selected architecture contracts, component records, model
provenance, and requested validation policy. Circuit names, fixture paths, and
hand-authored coordinate patterns must not participate in classification.

## Evidence Contract

Every positive capability requirement must reference one or more evidence
records. Evidence records have a stable ID, kind, status, source, digest, and
generation stage.

Evidence status is:

- `verified`: reproducible evidence is present;
- `inferred`: the capability is plausible but not fully promoted;
- `missing`: required evidence is absent;
- `failed`: evidence was evaluated and contradicted the capability claim.

A `supported` assessment requires at least one verified evidence record for
every required capability. Verified evidence without a stable source and
digest is invalid.

Assessments and their hashes must be deterministic. Equivalent normalized
inputs and registries must serialize to identical bytes.

## Gate Behavior

Before project mutation:

1. `supported` proceeds automatically.
2. `experimental` stops with a structured opt-in diagnostic unless explicit
   experimental authorization is present.
3. `unsupported` always stops with structured missing-capability diagnostics.

Experimental authorization is recorded in the assessment. It does not change
the classification and does not permit `fabrication_ready=true`.

## Lifecycle Reassessment

The assessment carries ordered checkpoints for:

- architecture or block selection;
- component resolution;
- simulation/model verification when applicable;
- placement and routing;
- writer correctness;
- structural validation;
- KiCad ERC/DRC;
- fabrication readiness.

Checkpoint classification is monotonic: later evidence may preserve or reduce
confidence, but it may not silently upgrade an earlier classification.
Blocking evidence changes the assessment to `unsupported`. Skipped required
evidence prevents a fabrication-ready claim.

## Result and Artifact Contract

The final assessment, checkpoints, evidence, gaps, risks, authorization state,
and deterministic hash must appear in:

- the workflow result;
- the promotion report when promotion is evaluated;
- the creation manifest;
- structured CLI success and failure output.

Reports must keep inferred evidence visibly distinct from verified evidence.
The workflow acceptance result may set `fabrication_ready=true` only when both
the physical workflow and the capability assessment independently permit it.

## Compatibility and Safety

- Existing promoted design families must classify as `supported` from their
  generic registered evidence.
- Held-out requirements composed entirely from supported capabilities must
  remain supported.
- Missing providers, components, models, physical realization evidence, or
  requested verification capabilities must fail closed.
- Experimental output must remain inspectable and testable, but cannot be
  represented as fabrication-ready or promotion `pass`.
- No fixture-specific allowlists, coordinate checks, request-name checks,
  schema forks, component exceptions, or circuit-family conditionals are
  permitted.

## Acceptance

The implementation is complete only when:

1. promoted and held-out supported corpora classify as supported;
2. adversarial missing-capability requests are refused deterministically;
3. experimental requests require opt-in and remain non-fabrication-ready;
4. assessment replay is byte-identical;
5. workflow, promotion, manifest, and CLI artifacts expose the assessment;
6. lifecycle classification never improves after a lower-confidence
   checkpoint;
7. the complete local short suite passes;
8. representative installed-KiCad fixtures pass ERC, strict DRC,
   connectivity, route completion, writer correctness, and zero normalized
   round-trip diffs;
9. staged changes receive Prism review before commit and push.
