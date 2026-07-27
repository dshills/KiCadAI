# Dynamic Electrothermal And Control-Loop Synthesis Plan

## Objective

Generate, select, repair, verify, and physically realize previously unsupported
feedback and power-control circuits using derived dynamic stability,
electrothermal, event, protection, and transient-SOA evidence.

Status: Complete.

## Phase 1: Specification, Corpus Freeze, And Baseline

- Publish the additive V5 behavior/event and dynamic-evidence contract.
- Freeze six behavior-only requirements spanning the mandated circuit classes.
- Pin manifest membership and fixture bytes with SHA-256.
- Add an independent strict mirror decoder and neutrality/coverage tests.
- Record the untouched V4 rejection and missing-evidence baseline.

Acceptance: corpus bytes are immutable, all required dynamic domains and events
are covered, no implementation hints are present, and the baseline proves the
new evidence is unavailable.

## Phase 2: Catalog Model Contract And Primitives

- Add reviewed claim contracts for inductors, bounded switches/MOSFETs,
  controlled stages, thermal RC networks, transient SOA, and protection
  elements required by the corpus.
- Validate units, ranges, finite work bounds, source/revision/hash, review
  status, analysis applicability, temperature range, and unique selection.
- Extend trusted graph translation and solvers without accepting provider
  equations, matrices, code, or solver controls.
- Add focused positive, mutation, ambiguity, and provenance tests.

Acceptance: every dynamic primitive resolves uniquely from reviewed catalog
evidence, model hashes are deterministic, and incomplete or untrusted claims
fail before evaluation.

## Phase 3: Loop Discovery And Stability

- Derive control influence and canonical feedback loops from resolved
  connectivity.
- Select a DC-preserving injection boundary and reject ambiguous loops.
- Calculate deterministic return-ratio samples, crossover, phase margin, gain
  margin, and peaking.
- Cover nested/multiple loops and all declared operating corners.
- Connect stability assertions to behavioral requirements and model evidence.

Acceptance: synthetic and corpus-level tests prove derived loop identity,
correct margins, deterministic interpolation, corner coverage, and stable
fail-closed diagnostics.

## Phase 4: Electrothermal Transient And Event Evaluation

- Add V5 event decode, normalization, validation, and analysis planning.
- Calculate per-device instantaneous power on deterministic transient grids.
- Integrate finite thermal RC networks and supported coupled temperature
  feedback.
- Evaluate temperature, transient SOA, peak stress, protection response, and
  recovery for every applicable event/corner.
- Enforce convergence and dynamic-work budgets.

Acceptance: known analytical cases, unsafe mutations, and reordered events
prove deterministic trajectories, peaks, margins, coverage, and terminal
diagnostics.

## Phase 5: Dynamic Architecture Search And Repair

- Feed stability, electrothermal, SOA, and protection outcomes into complete
  candidate selection.
- Rank passing candidates by worst critical dynamic margin.
- Retain rejection evidence for statically valid but dynamically unsafe
  alternatives.
- Add bounded generic repair variables and immutable safety guards.
- Record canonical diagnoses, trials, applied changes, state hashes, replay,
  and final rationale.

Acceptance: at least two frozen cases reject a static favorite and select a
safe alternative; repair cases improve dynamic evidence without changing
requirements or relying on fixture identity.

## Phase 6: Lowering, Traceability, And Physical Promotion

- Propagate dynamic requirement, model, loop, event, corner, diagnosis, repair,
  and selection identities through composition lowering and transaction
  evidence.
- Derive applicable feedback, high-current, thermal, protection, and
  sensitive-node physical constraints.
- Promote every positive fixture through routing, connectivity, writer, ERC,
  DRC, and round-trip gates.
- Add the reordered negative corpus and preserved regression matrix.

Acceptance: every dynamic claim traces to generated KiCad objects and evidence;
all six positives pass locally and all negatives fail with stable expected
codes.

## Phase 7: Reproducible Local Promotion

- Add a versioned dynamic promotion matrix and local Make target.
- Run every scenario twice from clean isolated roots with the locked KiCad
  toolchain.
- Verify content-addressed bundles and normalized equality.
- Run the existing full local promotion matrices and regression suites.

Acceptance: all local bundles verify independently, repeat byte-for-byte under
the declared normalization contract, and preserve every existing promotion.

## Phase 8: Audit, Prism, Commit, And Push

- Publish the final capability report and requirement-by-requirement audit.
- Review the complete staged diff with Prism.
- Resolve every high and medium finding and rerun affected/full local gates.
- Commit and push the clean final candidate.

Acceptance: local evidence proves every specification item, Prism is clear of
high/medium findings, and the pushed tree is clean. Automatic GitHub workflows
are not awaited; a later user-reported failure reopens the goal.
