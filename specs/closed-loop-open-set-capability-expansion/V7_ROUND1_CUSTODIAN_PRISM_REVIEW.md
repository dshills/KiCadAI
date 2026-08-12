# V7 Round-One Custodian Prism Review

Scope: the staged, pre-observation V7 round-one public-discovery custodian and
its frozen-state verifier. Prism used the configured external Gemini provider.
No public discovery outcome or held-out material had been consumed.

## Dispositions

- Repeated-comparison JSON marshaling: `remediated`. Lineage edges now use a
  deterministic field comparator.
- Binary search over potentially unordered evidence: `remediated`. The helper
  now rejects noncanonical evidence before comparison and uses a sorted,
  zero-allocation two-pointer subset check.
- Ignored member-key errors and unknown stage ordinals: `remediated`. Both now
  fail closed with explicit errors.
- Evidence order sensitivity: `rejected_with_reproducible_evidence` after the
  local canonical-evidence check was added. The frozen selector requires every
  evidence list to be sorted, unique, and nonempty; byte order is therefore
  canonical rather than incidental.
- Exactly one successor for a removed nonselected gap:
  `rejected_with_reproducible_evidence`. V7_BASELINE_PROTOCOL, "Single round
  evaluation" step 6 and V7_SPEC_ADDENDUM section 4 require one deterministic
  successor. Zero successors silently lose lineage and multiple successors are
  ambiguous; both permanently retire V7. The custodian regression test covers
  both rejected forms.
- Claimed JSON marshaling in the final lineage sort: `not_applicable`. The
  reviewed code performs direct field and canonical-slice comparisons; no JSON
  marshal remains in that comparator.
- Repeated linear scans for exact current-gap membership: `remediated` with a
  per-case canonical identity index constructed once before prior-gap checks.
- Duplicate prior or current case IDs: `remediated`. The lineage precheck now
  rejects either collision before constructing its case index, with regression
  coverage for the overwrite case.
- JSON-derived exact gap identity: `remediated` using the policy's frozen
  length-prefixed ordered UTF-8 field encoding.
- Artifact path traversal: `remediated` with an explicit `filepath.IsLocal`
  check in addition to the preexisting exact canonical path equality check.
- Lineage identity/successor error context and temporary identity-field slice:
  `remediated` with case/gap context and direct field writes.
- JSON comparison of the two synthesis replays: `remediated` with structural
  equality. The replay structure includes the complete-export hash, and the
  observation is independently bound to that same hash, preserving the frozen
  byte-identity evidence.
- Missing selected-member map capacity: `remediated` with the known member
  count.
- Inline audit format string: `remediated` by a single named template constant.

All high and medium findings therefore have a frozen-protocol-consistent
disposition before the one-time public discovery evaluation.
