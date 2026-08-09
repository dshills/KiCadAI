# Behavioral-Contract Feasibility and Realizability Gate Audit

Status: implementation complete; ready for V3 experiment handoff

Date: 2026-08-09

## Frozen boundary

The specification and plan were reviewed, committed, and pushed before
production implementation at commit `8d124d09`. `CONTRACT.sha256` binds their
exact bytes and a local contract test reproduces the checksum manifest.

The implementation does not modify any V1/V2 corpus, manifest, baseline,
selection, seal, report, checksum, or policy-v1 artifact. The legacy `Observe`
and `Evaluate` entrypoints still emit and require
`closed-loop-capability-policy-v1` evidence.

## Implemented capability

`AssessRequirementRealizability` now produces deterministic,
topology-independent findings for:

- output requirements that exceed conservative least-favorable direct external
  supply bounds and therefore require energy-domain creation;
- behavioral assertions spanning multiple distinct source outputs; and
- independent external excitation ports converging on one source output.

The voltage rules cover exact boundaries, positive and negative rails, bipolar
span, zero-crossing supplies, absolute peak voltage, output swing, and missing
supply bounds. Missing information remains unclassified. The classifier does
not subtract invented component headroom and does not treat port ratings as
energy sources.

The versioned `ObserveRealizabilityAware` entrypoint refines only a generic
terminal topology-search/no-complete-graph root. A universal candidate
diagnosis or a component, model, simulation, physical, routing, or verification
root remains authoritative. The original terminal topology code is retained as
a downstream symptom.

`EvaluateRealizabilityAware` requires policy-v2 case evidence and separates
`energy_domain_creation` from `multi_obligation_composition` clusters. The
legacy evaluator rejects policy-v2 evidence, preventing mixed-policy ranking.

## Genericity and safety

- Production code contains no corpus identity, file path, project name,
  topology family, component, coordinate, allowlist, or fixture special case.
- Classification normalizes first and rejects schema-invalid requirements.
- Findings are sorted and compacted deterministically.
- Classification cannot synthesize, promote, write, or mutate a design.
- Absence of findings is explicitly not a feasibility claim.
- Existing installed-KiCad evidence is unchanged because the gate is
  evidence-only and is not on the legacy synthesis path.

## Local verification

Passing:

- `go test ./specs/behavioral-contract-feasibility-realizability -count=1`
- focused realizability, feasibility, pre-search fail-closed, and capability
  tests in `internal/opentopologysynthesis`
- `go test ./internal/capabilityfeedback -skip
  '^TestClosedLoopFinalArtifactsAreReproducible$' -count=1`
- `go test ./internal/opentopologysynthesis -skip
  '^TestWindowedHeatingPowerPassesElectricalAndSafetyCorners$' -count=1
  -timeout 20m` (passed in 384.109 seconds)

The unfiltered open-topology package reproduced the pre-existing
`TestWindowedHeatingPowerPassesElectricalAndSafetyCorners` failure after
551.73 seconds: its only synthesized candidate measured zero for required
active current and exhausted generic repair. The classifier is not invoked by
that legacy synthesis path. This failure was already identified as incomplete
or failing broad-suite evidence in the closed-loop V1 audit and is not
attributed to this change.

The unfiltered capability-feedback package still fails closed because the V1
blind experiment intentionally did not write
`testdata/closed_loop_open_set_final/case_001.json` after held-out uplift was
zero. All other tests in that package pass.

GitHub Actions were not started or inspected, following the project's standing
local-verification instruction.

## Next experiment

V3 may now bind `closed-loop-capability-policy-v2-realizability`, a new impact
registry containing `energy_domain_creation` and
`multi_obligation_composition`, and a fresh independently authored discovery
and held-out corpus. V1 and V2 held-out material remains retired and cannot be
used as blind evidence.
