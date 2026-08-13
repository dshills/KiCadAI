# V8 Discovery Baseline Prism Review

Prism reviewed the exact staged generation-zero discovery baseline through the
configured Gemini provider. The final review reported two high, five medium,
and one low finding. None identifies a defect in the baseline publication.

## Dispositions

- **Missing `frontier.json` and `report.json` (high): false positive.** Both
  files are staged, and their whole-file SHA-256 values reproduce the final two
  entries of `CHECKSUMS.sha256`.
- **Unsafe disabled-state current (high): expected nonpass evidence.** The cited
  case is classified `unsupported`, never `pass`, and has a nonempty immutable
  obligation-anchored root frontier. Section 2 of `V8_BASELINE_PROTOCOL.md`
  requires preserving such failed evidence under one of the four outcome
  buckets; changing synthesis before freezing the baseline would contaminate
  generation zero.
- **Audit/checksum hash mismatch (medium): distinct commitments.** The audit
  records the canonical objects' internal self-hashes. `CHECKSUMS.sha256`
  records the hashes of the complete serialized files, which include those
  self-hash fields. Both layers reproduce and the structural baseline test
  verifies their relationship.
- **Sweep setup, nonconvergence, and unity-gain results (medium): expected gap
  observations.** The cited cases are respectively nonpassing with complete
  typed, obligation-anchored `simulation` frontiers. Sections 2–4 of
  `V8_BASELINE_PROTOCOL.md` require these results to be frozen and ranked before
  any generic repair is selected. They are candidate inputs to the capability
  feedback loop, not baseline-publication defects.
- **Null per-attempt diagnoses (medium): no loss of the required root
  frontier.** Candidate inventories include bounded attempts that can exhaust
  before producing an additional diagnosis. Each affected case still has
  normalized terminal diagnostics and a nonempty anchored root frontier, which
  are the required causal outputs under section 3 of the baseline protocol.
- **Floating-point representation (low): retained deterministic evidence.** The
  complete synthesis exports are byte-identical across the two required runs.
  Post-hoc rounding would rewrite evaluated evidence and is therefore not
  permitted by the frozen protocol.

## Verification

- 18 public discovery cases ran twice with byte-identical complete synthesis
  exports.
- Outcome totals are 0 pass, 6 unsupported, 0 unsafe, and 12 exhausted.
- Every nonpassing case has a nonempty typed, obligation-anchored root
  frontier; no case was promoted or represented as a pass.
- The checksum manifest, compressed artifact file hashes, artifact self-hashes,
  aggregate self-hash, frontier self-hash, corpus binding, evaluator binding,
  and obligation binding reproduce in the local structural verifier.
- Held-out source, ciphertext plaintext, and external keys were not opened.
