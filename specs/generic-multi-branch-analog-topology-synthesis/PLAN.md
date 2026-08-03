# Generic Multi-Branch Analog Topology Synthesis Plan

Status: complete (2026-08-03)

## Phase 0 — Baseline and freeze — complete

- Record the committed 6/8 simulation result and the two current regressions.
- Freeze two independent behavior-only neutral requirements and their hashes
  before production implementation changes.
- Add strict corpus integrity and implementation-detail rejection tests.
- Refresh the roadmap's obsolete next-milestone entry.

Exit: the baseline and corpus are immutable and reproducible.

## Phase 1 — Model-aware hysteresis sizing — complete

- Derive active-stage output low/high bounds from trusted primitive-model
  evidence.
- Size the feedback ratio and reference point from the requested rising and
  falling thresholds and modeled output swing.
- Preserve deterministic catalog quantization and corner enumeration.
- Add focused equation, graph-pattern, and requirement-level tests.

Exit: the frozen hysteretic detector passes all assertions without tolerance
or requirement changes.

## Phase 2 — Generic high-side pass synthesis — complete

- Introduce terminal-role abstraction for high-side BJT and MOSFET candidates.
- Rank candidates from analysis coverage, voltage/current/thermal/SOA evidence,
  compliance, and deterministic catalog order.
- Generate bounded single and parallel variants only when justified by the
  evidence and graph budget.
- Generalize transconductance value derivation across supported terminal roles.

Exit: the frozen adjustable-current case passes its transfer, corner,
transient, and thermal assertions without a named-part rule.

## Phase 3 — Multi-branch graph repair — complete

- Add deterministic branch insertion and series split operations to the graph
  kernel.
- Expand diagnosis-local endpoint selection across the affected
  source-observation cone and semantic internal nodes.
- Add safe anonymous-node combination and preserve existing redirect and
  substitution behavior.
- Record and verify every graph delta and bounded repair search outcome.

Exit: focused tests prove add, redirect, split, combine, substitute, rejection,
deduplication, cancellation, budget, and replay behavior; a selected synthesis
result contains a passing graph-changing repair.

## Phase 4 — Frozen simulation promotion — complete

- Run the original eight-case promotion until all eight pass.
- Strengthen its acceptance floor from 6/8 to 8/8.
- Run the two neutral cases twice and compare graphs, reports, and evidence.
- Preserve existing independently frozen architecture-generalization behavior.

Exit: both corpora pass their declared gates with deterministic replay.

## Phase 5 — Physical promotion — complete

- Lower passing primitive graphs through the production schematic/PCB lane.
- Run every required installed-KiCad gate in two clean roots.
- Require clean ERC, strict DRC, complete routing/connectivity, writer
  correctness, zero normalized round-trip diffs, and identical evidence.

Exit: two clean local promotion runs prove the physical result.

## Phase 6 — Audit and delivery — complete

- Run the full local regression suite in bounded shards, compare any failure
  against the frozen baseline, and run the implementation-leakage scan.
- Update roadmap and status documentation.
- Write a requirement-by-requirement completion audit and promotion matrix.
- Stage the complete diff, run Prism, remediate findings, rerun affected tests,
  commit, and push.

Exit: all specification clauses have current authoritative evidence and the
reviewed commit is synchronized with the remote.
