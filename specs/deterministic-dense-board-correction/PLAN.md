# Deterministic Dense-Board Correction Plan

## Objective

Implement operation-correlated, affected-net-only placement and routing
correction for dense unseen boards while preserving circuit intent and every
unrelated route operation.

## Phase 1: Baseline And Contracts

- Record current correction, route-tree, layer-transition, transaction, and
  promotion behavior.
- Define operation correlation, mutation scope, preservation, ranking, and
  fail-closed contracts.
- Add the milestone to the roadmap without changing existing claims.

Acceptance:

- The specification accounts for every goal requirement.
- The design extends existing subsystems and adds no fixture identity logic.

## Phase 2: Operation Correlation

- Add stable route-operation traces for in-memory routing results.
- Carry exact operation identities and indexes into normalized diagnostics.
- Resolve by exact identity/path first and exact diagnostic net membership
  second.
- Reject ambiguous or incomplete routing scopes.
- Include operation scope in deduplication and retry keys.

Tests:

- exact identity, path, net, ambiguity, unknown identity, non-route, reorder,
  and deterministic serialization cases.

Acceptance:

- Every authorized routing action proves its current route-operation scope.

## Phase 3: Routing-Only Planning And Application

- Authorize branch reorder and layer-transition actions only with proven scope.
- Separate placement application from routing application.
- Add route-preservation fingerprints.
- Enforce invariant, budget, repeated-key, and current-slice checks.

Tests:

- pure planning, no input mutation, stale scope, repeated key, protected route,
  and illegal action rejection.

Acceptance:

- A routing-only plan reaches no placer and cannot mutate an unrelated net.

## Phase 4: Selective Affected-Net Replacement

- Partition the current route operation slice.
- Convert non-affected routes to fixed net-aware obstacles.
- Route only affected nets with unchanged resolved rules.
- Splice replacements deterministically.
- Re-run connectivity, contact, clearance, via, and route completion checks.
- Reject candidates that alter non-affected canonical operation bytes.

Tests:

- two-net crossing plus decoy net;
- no affected operation;
- fixed/protected local route;
- deterministic replay and canonical-byte preservation.

Acceptance:

- Recovery changes only the explicitly affected nets.

## Phase 5: Branch-Order Correction

- Expose the existing route-tree branch candidates to the correction action.
- Evaluate the closed deterministic alternate-order sequence.
- Rank using blockers, endpoint proof, graph completion, failed branches,
  route cost, and canonical key.
- Persist selected-order evidence.

Tests:

- failures-first recovery;
- failures-last recovery;
- equal-score canonical tie;
- renamed refs/nets;
- all-candidates-fail stop.

Acceptance:

- A held-out blocked tree recovers without placement or rule changes.

## Phase 6: Layer-Transition Correction

- Reuse same-net junction detection and legal via policy.
- Insert a missing transition only at a proven junction.
- Fall back to selective affected-net layer-aware rebuild.
- Validate pad-hole, via, clearance, contact, and connectivity invariants.

Tests:

- legal insertion;
- already-connected no-op;
- via limit, disallowed layer, keepout, via-in-pad, and foreign-net conflict
  rejection;
- deterministic selective rebuild fallback.

Acceptance:

- A held-out missing transition recovers and illegal transitions fail closed.

## Phase 7: Placement/Endpoint Integration

- Preserve existing bounded placement actions.
- Derive reroute expansion mechanically from moved-component pad nets.
- Prove fixed components, required regions, keepouts, and protected local
  routes remain unchanged.
- Never translate routing-only evidence into placement motion.

Tests:

- one movable/one fixed endpoint;
- all-fixed stop;
- moved component with multiple connected affected nets;
- unrelated component and copper preservation.

Acceptance:

- Endpoint recovery moves only eligible components and reroutes only
  mechanically affected nets.

## Phase 8: Held-Out And Adversarial Corpus

- Add identity-neutral crossing, branch-order, transition, endpoint, fail-closed,
  and decoy-net fixtures.
- Add a three-or-more-net held-out board with unrelated identities and geometry.
- Assert renamed-input equivalence and byte-identical replay.

Acceptance:

- Production behavior is independent of fixture names and coordinates.

## Phase 9: Local Promotion

- Run focused suites after each implementation phase.
- Run `go test ./...`.
- Run configured local KiCad-backed fixtures and promotion lanes.
- Require clean ERC, strict DRC, connectivity, route completion, writer
  correctness, and zero round-trip differences.
- Confirm every existing promoted domain remains passing.

Acceptance:

- All gates in the specification have current local evidence.

## Phase 10: Completion Audit

- Map each specification requirement to authoritative current evidence.
- Inspect staged scope for fixture-specific production logic.
- Review the staged diff with Prism only with current user authorization for
  the configured external provider.
- Stage the completed milestone without committing unless requested.

Acceptance:

- No requirement is supported only by intent, indirect evidence, or a narrower
  test than its claimed scope.
