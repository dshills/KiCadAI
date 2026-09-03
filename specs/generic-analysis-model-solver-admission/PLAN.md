# Generic Analysis, Model, and Solver Admission Plan

## Phase 0 — Evidence and boundary freeze

1. Record the clean starting commit and authenticate the V18 selection, V19
   retirement, corpus, evaluator, and report artifacts.
2. Publish a public-only nine-case failure taxonomy by exact typed atom.
3. Add contract tests proving all historical V19 files and source-bound
   manifests remain unchanged.

Exit: the successor boundary is explicit and no historical artifact changed.

## Phase 1 — Typed admission model

1. Add the bounded admission request, source, solver, model selection,
   diagnostic, evidence, and policy types.
2. Implement canonical normalization, strict validation, stable JSON hashing,
   deep cloning, and deterministic ordering.
3. Add unit/property tests for order permutations, duplicate identities,
   unknown fields, malformed hashes, size limits, and tampering.

Exit: admission artifacts are reproducible and fail closed independently of
the synthesis engine.

## Phase 2 — Trusted source resolver

1. Adapt the embedded provenance registry and reviewed component overlays into
   explicit source descriptors.
2. Merge sources deterministically and reject non-identical collisions.
3. Retain source, record, model-definition, and parameter digests in every
   selection.
4. Test bundled, project-overlay, configured-overlay, missing, conflicting,
   malformed, and reordered sources.

Exit: no selected model lacks exact source and content provenance.

## Phase 3 — Analysis planner and solver registry

1. Derive canonical required analyses from behavior-only requirements.
2. Validate analysis shape before topology search.
3. Add the immutable solver registry and environment availability filter.
4. Prove model/workflow/solver compatibility without accepting provider solver
   controls.
5. Test each successful analysis and every required refusal category.

Exit: an invalid or unavailable analysis fails before synthesis with a stable
typed diagnostic.

## Phase 4 — Candidate model admission

1. Resolve each connected graph component to exactly one reviewed primitive
   model for the requested analysis.
2. Validate required parameters and applicability bounds.
3. Replace message-substring diagnostic classification at the admission
   boundary with typed causes.
4. Attach canonical admission evidence to opted-in simulation attempts.

Exit: simulation starts only with a complete, unambiguous model-and-solver
decision.

## Phase 5 — Production and successor integration

1. Run requirement admission in `open-topology create` before search.
2. Add a version-isolated successor constructor and evaluator; leave V18 and
   retired V19 byte-identical.
3. Add public fixtures covering admission and every fail-closed category.
4. Preserve existing CLI behavior when no new configuration is supplied,
   except for more precise early refusal evidence.

Exit: production uses admission and historical constructors remain sealed.

## Phase 6 — Evaluation

1. Run focused synthetic and public-only selected-case tests during
   implementation.
2. Commit the evaluator boundary before the bounded cohort run.
3. Execute all 24 immutable public cases twice in manifest order.
4. Publish aggregate outcomes, typed transitions, preservation results, and
   promotion evidence without replacing V19 artifacts.
5. If the improvement or preservation gate fails, publish a fail-closed audit
   and do not claim the capability.

Exit: the selected family materially improves and every preservation gate
passes, or the successor is explicitly refused.

## Phase 7 — Regression, documentation, and delivery

1. Run focused packages after each implementation phase.
2. Run lint, race-short, authenticated coverage, deterministic replay, corpus,
   seals, writer, release reproducibility, and installed-KiCad promotion gates.
3. Update architecture, CLI, diagnostics, support boundary, project status,
   roadmap, and changelog documentation.
4. Stage the complete diff, run Prism, remediate valid findings, rerun affected
   gates, commit, push, and open a pull request.

Exit: clean tree, no unresolved valid Prism findings, green local evidence,
and a reviewable pull request.
