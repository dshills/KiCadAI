# Simulation-Grounded Closed-Loop Circuit Synthesis Plan

Status: complete. See [AUDIT.md](AUDIT.md) for the requirement-to-evidence map
and fresh verification commands.

## Phase 1 — Specification, Gap Audit, And Corpus Freeze

- Publish the public behavioral contract, trust boundary, loop semantics,
  promotion gates, and prohibitions.
- Record a current-state gap matrix against the existing MNA, tolerance,
  architecture-search, lowering, and workflow implementation.
- Freeze ten behavior-only v3 requirements and a SHA-256 manifest before
  corpus-driven production implementation.
- Add independent strict-decoding, neutrality, representative-coverage,
  membership, and hash tests.

Acceptance: the frozen-corpus test passes while production supports no v3
behavioral fields, proving the corpus preceded implementation.

## Phase 2 — V3 Behavioral Requirements

- Add strict v3 decoding without changing v1/v2 replay bytes.
- Add typed behavioral requirements, semantic observations, analysis classes,
  operating cases, and simulation acceptance gates.
- Normalize units and ordering; reject ambiguity, duplicate coverage, unknown
  metrics/targets/corners, unsafe omissions, and unbounded workloads.
- Add deterministic natural-language coverage evidence at the provider intent
  boundary.

Acceptance: schema/property tests and independent mutation tests prove strict,
order-neutral, fail-closed behavior and backward compatibility.

## Phase 3 — Trusted Analysis Planning And Model Provenance

- Extend catalog simulation claims with reviewed source, revision, immutable
  hash, applicability, and operating-domain evidence.
- Bind semantic requirement targets to canonical graph nodes/devices only
  after catalog resolution and lowering.
- Add registered noise, stability, startup, distortion, and thermal analysis
  plans while preserving existing DC/AC/nonlinear/transient behavior.
- Expand corner planning to explicit supply, load, temperature, tolerance, and
  model axes with bounded deterministic work accounting.

Acceptance: every required analysis either produces a canonical trusted plan
or a blocking actionable diagnostic; untrusted/inapplicable models never run.

## Phase 4 — Deterministic Closed Loop

- Evaluate all retained architecture candidates in fingerprint order.
- Normalize assertion margins and select passing candidates using the public
  deterministic policy.
- Classify failures into registered diagnoses and plan bounded architecture,
  variant, preferred-value, bias/gain/filter/compensation/protection repairs.
- Re-resolve and rerun all analyses after every repair; detect repeated states,
  non-improvement, ambiguity, unsupported diagnoses, and exhausted budgets.
- Emit a provenance-complete, byte-stable closed-loop report and replay
  artifact.

Acceptance: generic synthetic tests prove selection, successful repair, full
reevaluation, all stop reasons, safety precedence, budget enforcement,
order-independence, and byte-identical replay.

## Phase 5 — Workflow And Artifact Integration

- Integrate the loop between architecture realization and final physical
  promotion without exposing diagnostics to the AI provider.
- Persist behavioral requirements, simulation plans/reports, model decisions,
  diagnoses, repairs, final rationale, and budget consumption.
- Make required simulation a hard promotion gate; `not_applicable`, skipped,
  partial, or stale evidence cannot promote.
- Preserve downstream catalog, lowering, writer, route, connectivity,
  round-trip, ERC, and DRC authority.

Acceptance: end-to-end recorded fixtures replay byte-identically and deliberate
simulation failures prevent project promotion.

## Phase 6 — Held-Out Behavioral Promotion

- Run all ten frozen requirements through architecture search, simulation,
  deterministic repair where needed, and the complete physical workflow.
- Require applicable DC, AC, noise, stability, transient, startup, distortion,
  thermal, and all declared corner assertions.
- Demonstrate the required Class-A and Class-AB evidence without specialized
  fixture logic.
- Add independent fail-closed mutations for missing model trust, corner
  failures, ambiguity, unsupported analyses, unsafe repair, and exhausted work.

Acceptance: all ten pass offline and installed-KiCad gates; mutations fail for
the intended reason.

## Phase 7 — Completion Audit And Delivery

- Run focused packages, full tests, formatting/lint, deterministic replay,
  corpus promotion, protected USB regressions, and all installed-KiCad gates.
- Update the roadmap, project status, AI-readiness matrix, and user-facing
  simulation documentation with measured coverage and explicit boundaries.
- Publish an audit mapping every specification requirement to authoritative
  current evidence.
- Review the complete staged diff with Prism, resolve findings, commit, push,
  and verify GitHub Actions for the exact commit.

Acceptance: every specification clause has direct evidence, the worktree is
clean, and the exact pushed commit is green.
