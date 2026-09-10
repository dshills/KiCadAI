# V23 final publication review

## Retained-evidence audit review

Prism run `0d692cf45d0d5af725768e4b4dcbfa6c` reviewed the new publication
audit and provisional documentation through configured Gemini in the separate
clean clone. The original evaluator checkout and freeze revision were untouched.
Raw review JSON SHA-256:
`4ae462e2930b5378fac2e855f8d9fb95e0ad10c59d9f954cb10356f6e8cee602`.

- `79a401309ebb3516`, sort promotion issues before hashing: not applied. This
  audit deliberately reproduces the sealed `physicalPromotionHashValue`
  projection, including ordered issues and stages. Sorting only the verifier
  would change the committed identity and reject authentic historical records.
  The producer already requires exact two-replay identity; the audit also
  compares the two retained promotion hashes. The review's reported path
  `pkg/promotion/report.go` does not exist in this repository.
- `d5ac61e47924e65a`, cache repeated small metrics reads: not applied. Semantic
  validation and exact byte-inventory authentication have separate purposes.
  This bounded 48-replay audit completes in a fraction of a second; no measured
  bottleneck justifies changing it. The reported path
  `test/e2e/inventory_test.go` likewise does not exist.

No unresolved valid findings from this review. The first completed-prefix audit
in this handoff authenticated 22 cases / 44 replays and passed every synthetic
tampering, manifest, predecessor, policy, and promotion-boundary regression.
That prefix is not a complete publication or capability-success claim.

## Final artifact review

Prism run `82b832399c3ad22a023f59020e30250f` reviewed the full 167-file staged
publication: retained artifacts, manifest pin, independent audit, native-file
verification, and final results/status/resource documentation. Raw review SHA-256:
`9b991f6345c15834c3a1fb24dbf5a6be25424c9b781641cf4796751e9b3f24b6`.

- `e07d6b3c09cf4c34`: fixed. The new publication audit no longer skips when
  `report.json` is absent. Report and inventory are mandatory locally and in
  CI now that publication is complete. The historical frozen evaluator tests
  are unchanged; the separate audit supplies this stronger final-publication
  requirement.
- `9c860c0637240d6c`: fixed. Identity-marshalling errors now preserve their
  original cause. Identity mismatch, policy, and requirement checks are unchanged.
- `9507d249cd52e7e9`: the 120 GB OOM premise does not apply. The full canonical
  synthesis stream was never written or read by this audit. The entire exact
  retained inventory is 521,456 bytes. Native-file verification covers the four
  frozen small projects, not arbitrary future boards. No unsupported performance
  claim or outcome-driven performance rewrite is introduced.
- `3c4df2975cb32ac5`: standard `go test` sets the package working directory.
  The fixed repository-relative layout is part of this versioned audit and was
  verified in both the working checkout and separate clone. No arbitrary
  externally relocated test-binary invocation is promised.
- `0020d44cbde8dc43`: the file-count check is only an initial guard, not the
  acceptance condition. Every native project's complete framed file hash must
  match its authenticated promotion record. The projection mirrors the frozen
  producer and includes child schematics; this is not an admission rule for
  unconventional new projects.

The original run is complete, and all 24 cases / 48 replays, the pinned
154-file inventory, unchanged V22 publication, and four native project byte
hashes pass authentication. Final repository-wide lint and all six V22/V23
specification/publication race-test packages pass. Review-only corrections do
not change any evaluated implementation or retained artifact.

## Final correction verification

The missing-report failure mode was exercised in the separate clone while its
V23 publication directory was absent: both required publication tests failed,
rather than skipped. Copying the exact 154-file publication into that clone
then made the complete evaluator/publication suites pass. The original evidence
was never removed, renamed, or rewritten. The corrected audit passes its full
race suite (1.634 s), including all 48 retained replays and four native projects;
the final repository-wide `make lint` reports zero issues.

After explicit user authorization for the additional Gemini review, Prism run
`c0d3d280e6e191f14dd2b2a7a89eac83` reviewed the narrow correction diff.
Raw review SHA-256:
`580503f11aef0454cf430e759a7caef3995791d29fed25678e3f653e066732dd`.

- `0780cc6b54686247`: V23 deliberately uses the unchanged V22 repair limits,
  as required by the sealed correction scope, `RUN.json`, and evaluator
  protocol. The new version identifies the solver/execution policy, not a
  search-budget change. The suggested hypothetical V23 limit substitution
  would violate the frozen contract.
- `3c5b111d8043770c`: optional message granularity, not an authentication
  defect. Every listed field remains enforced; separate named tampering tests
  exercise the rejection paths, and marshalling causes are now preserved.
  No additional refactor is needed for publication correctness.

No unresolved valid review findings. The complete publication was reviewed,
the two useful hardening corrections were verified and re-reviewed, and all
evaluated production/evaluator sources remain byte-identical to the freeze.

Unmodified raw review JSON is retained in [reviews](reviews/) under the three
`kicadai-v23-final-*-prism-*.json` names. Its hashes are recorded above; these
review records are separate from the 154-file frozen evaluation inventory.
