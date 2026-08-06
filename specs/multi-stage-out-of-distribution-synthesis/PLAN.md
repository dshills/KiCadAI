# Implementation Plan

Implementation status: phases 0 and 1 complete. The corpus was independently
frozen at `a5effe06`; the two-run public-CLI baseline is checksum-bound in
`BASELINE_REPORT.json`. No production behavior has changed. Phase 2 is next.

## Phase 0 — Freeze The Independent Corpus

- Freeze nine positive and four adversarial behavior-only requirements against
  the pre-implementation commit.
- Record per-file checksums, functional-behavior coverage, required analyses,
  expected fail-closed outcomes, and complete acceptance gates.
- Add integrity tests that reject implementation leakage and corpus mutation.
- Review and commit the freeze independently from production changes.

## Phase 1 — Public-CLI Baseline And Gap Clustering

- Execute every frozen case twice through the public CLI from clean output
  directories.
- Record deterministic terminal-stage, diagnostics, artifact emission, and
  replay evidence.
- Cluster failures by reusable missing relationship, analysis, safety,
  diagnosis, lowering, or physical capability.

Measured result: all 26 invocations were byte-identical by case and emitted no
project files. Three valid cases lack a complete multi-stage decision graph;
six valid cases reach complete graphs but exhaust evaluation without repair;
and all four adversarial cases fail closed without the required precise safety
or capability classification. These are the three primary implementation
clusters, in that order.

## Phase 2 — Generic Multi-Stage Architecture Composition

- Add only relationship-derived composition operators justified by baseline
  clusters.
- Carry typed obligations and constraints across stage boundaries.
- Retain materially distinct candidates and auditable rejection evidence.

## Phase 3 — Values, Models, And Multi-Stage Analysis

- Extend reviewed provenance and analysis coverage only where frozen behavior
  requires it.
- Propagate values, operating corners, dynamic constraints, and power loss
  across the whole candidate.
- Require deterministic convergence, thermal, and SOA evidence.

## Phase 4 — Diagnosis And Cross-Stage Repair

- Map failed evidence to generic diagnoses and bounded repair actions.
- Re-enter the earliest invalidated stage and rerun all affected gates.
- Keep safety terminal and preserve complete repair provenance.

## Phase 5 — Readable Physical Synthesis

- Lower functional stages into conventional left-to-right schematic flow with
  visible feedback, control, protection, supply, and return paths.
- Derive placement and routing constraints from graph roles and electrical
  stress without fixture identities or coordinates.
- Correct access, branch ordering, congestion, and layer transitions
  deterministically.

## Phase 6 — Promotion And Preservation

- Prove all nine valid designs and four frozen fail-closed outcomes twice
  through the public CLI with installed KiCad.
- Require simulation, corners, thermal/SOA, readability, route completion,
  connectivity, writer correctness, ERC, strict DRC, zero round-trip
  differences, and replay equality.
- Run every existing promotion corpus and local preservation suite.

## Phase 7 — Audit And Closeout

- Record requirement-by-requirement evidence and update project documentation.
- Run the complete local validation envelope.
- Stage the final diff, run Prism, remediate actionable findings, commit, and
  push.
