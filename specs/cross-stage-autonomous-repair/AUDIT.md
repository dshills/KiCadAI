# End-to-End Cross-Stage Autonomous Repair Audit

Verified locally on 2026-08-02 from base commit
`b6ea64c2f0b98cd749c5caba2a80e9e52960b292`.

## Result

The milestone is complete for the reviewed generation envelope. A single
versioned coordinator now sequences trusted electrical repair, transaction
repair, and generated-output regeneration. It selects the earliest blocking
stage, evaluates every authorized candidate from a restorable checkpoint,
chooses the smallest equal-outcome repair, confirms it independently, and
rolls back any unsafe, unrelated, nondeterministic, canceled, or
evidence-incomplete attempt.

The evidence schema is `kicadai.cross-stage-autonomous-repair.v1`. Reports
record normalized diagnostics and proposals, checkpoints, trials, scope and
gate changes, protected margins, budget consumption, re-entry decisions,
confirmation, stop reason, and a canonical hash.

## Frozen Corpus

The independent corpus contains one case at each required failure stage:
simulation, schematic, ERC, placement, routing, connectivity, DRC, writer,
and KiCad round trip. Its manifest SHA-256 is
`9a071ef14dcb85cdb6fece58f0f26bf9afb4898491e6c48efbc7e03604045e99`.
The freeze test checks the manifest, every member hash, the stage set, stable
IDs, and the pre-implementation base commit. Production operators contain no
corpus identity, fixture path, component reference, coordinate, allowlist, or
case-specific branch.

## Requirement Audit

| Requirement | Evidence |
| --- | --- |
| Structured attributable failures | Cross-stage diagnostics require stage, code, category, severity, evidence hash, semantic scope, key, and canonical hash. |
| Earliest-stage ordering | The mixed simulation/routing/DRC test commits sizing, placement, and routing re-entry in workflow order. |
| Smallest safe repair | Equal-outcome candidates rank by change count, normalized magnitude, earliest re-entry, and stable ID; untrusted expected-improvement estimates cannot displace a smaller repair. |
| Transactional trials | Every trial restores and verifies the exact checkpoint hash before another candidate or confirmation. |
| Unrelated-content preservation | New, removed, or changed undeclared scopes reject the candidate. Transaction tests preserve unrelated routes; generated-output tests preserve user-owned files. |
| Gate preservation | Required gates cannot disappear, become optional, or regress. New blocking gates and diagnostics reject the candidate. |
| Margin preservation | Electrical-corner, thermal, SOA, and physical margins cannot disappear, lose protection, or regress beyond policy tolerance. |
| Confirmation and rollback | Confirmation re-applies the selected proposal and compares snapshot and diagnostic hashes. Apply, re-entry, capture, diagnosis, cancellation, repetition, and mismatch paths restore the checkpoint. |
| Bounded deterministic execution | Global and per-diagnostic budgets, repeated proposal/state suppression, stable ordering, cancellation, stable stop reasons, and byte-identical report replay are covered. |
| Real electrical adapter | An independently held-out low-current converter polarity failure uses causal repair, re-enters synthesis, preserves critical margins, and replays byte-identically. |
| Real physical adapter | Transaction-backed placement, routing, connectivity, and DRC failures use the normal planner/executor boundary and preserve unrelated transaction content. |
| Real output adapter | Schematic, ERC, writer, and round-trip drift regenerate only authoritative generated files and preserve user-owned project files. |

## Local Verification

- `go test ./internal/repair ./internal/repairloop ./internal/opentopologysynthesis -count=1 -timeout=5m`: pass.
- `go vet ./internal/repair ./internal/repairloop ./internal/opentopologysynthesis`: pass.
- Function-level capability-report authority check: pass.
- Installed KiCad 10.0.3 protected LED and protected I2C promotion fixtures,
  two consecutive runs: pass with clean ERC, strict DRC, complete routing and
  connectivity, writer correctness, and zero round-trip differences.
- The protected LED transaction snapshot was updated to the deterministic
  endpoint-crossing repair hash; the protected I2C evidence remained passing.

A repository-wide `go test ./... -count=1` preservation run also exposed two
existing simulation-grounded composition failures in the base commit:
`current_sense_protection` and `mixed_function_control_power`. Reproduction
against an archive of the unchanged base commit gives the same failures. The
strict transient solver now correctly rejects their formerly accepted
unpowered negative glitch as a non-response. This milestone does not weaken
that safety check or claim those unrelated cases as passing; control-polarity
and state-sequencing semantics are the next repair target.

## Boundary

This is autonomous repair across existing trusted operators, not unrestricted
electronics invention. Unsupported diagnostics, absent evidence, unknown
operators, unreviewed component/model capability, exhausted budgets, and
unsafe candidates still fail closed.

The repository `make lint` wrapper completed `go vet` but its optional
golangci-lint/staticcheck phase reproduced four baseline `SA4006` findings in
untouched placement/routing files. They are not attributed to this milestone
and remain visible rather than being folded into an unrelated change.
