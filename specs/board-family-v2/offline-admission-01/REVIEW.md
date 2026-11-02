# Offline correction review

Status: development correction verified; **not release acceptance**. Reviewed by
the implementing agent, not an independent engineer or external model. No Gemini
review or other provider request was performed in this correction.

## Findings and resolution

- The model previously approved heater operation as a firmware choice despite the
  fixed heater-off constraint. Admission now reads the original request locally.
  Positive heater operation is refused; unrecognized temporal conditions and
  paraphrases clarify. A valid native configuration cannot waive the request gate.
- Independent root/clause tags, copied clause text and configuration nullability
  produced contradictory decisions. Application disposition, complete prompt
  retention and configuration availability now have one local construction point.
  Raw model annotations remain unchanged and visible. An overall non-design
  provider response is never promoted to a design.
- Numeric validity alone allowed substitution: an in-range default could replace
  the requested value. The locally derived nine-field configuration must match
  the proposed configuration exactly. Unsupported profile/resistance/clock
  combinations and numeric conflicts do not select an alternative.
- The first development candidate passed its automated suite, but local review
  found that `400 kHz measurements` could become the bus clock and `1000 mA I2C`
  could become source capability. These are not acceptable interpretations.
  Explicit quantity-role rules and seven new regression cases now withhold those
  requests. Bare unknown measurement terms are not ignored as filler.

The first development run remains at
`.cache/board-family-v2/offline-admission-01-run-01`; its receipt SHA-256 is
`2b14b9ff8e92cadf7d2e28fec616150d5f6be04005b9553c8d77e2c3567b2ff4`.
That run was superseded by local semantic review; its automated pass is not the
qualification of the corrected source. Published verification comes from fresh
`offline-admission-01-run-02`, with distinct source hashes and a new binary.

## Verified evidence

- Seen-response regression: **14/14 expected application outcomes** (all five
  requested supported configurations reproduced, three clarifications, six
  refusals). This is training/development feedback on already observed cases.
- **76** adversarial suffix cases, **50** positive family/profile grammar cases,
  **22** numeric/substitution cases and **7** quantity-role cases passed, alongside
  malformed-response, original-text, unknown-vocabulary insertion and
  non-design/nullability tests. Counts are named table-test subcases, not a
  statistical sample or a sum of every internal assertion.
- Full short Go regression, targeted race checks, a 15-second fuzz exercise and
  full repository lint passed; lint reported zero issues. The Go run reused
  cached results where Go determined inputs were unchanged. The fuzz exercise
  checks invariants and robustness, not completeness of English interpretation.
- All **17** existing Node evaluation/normalization safeguards passed. CI now also
  authenticates the byte-identical original failed publication.
- The exported provider contract is unchanged. **Zero API requests and zero new
  API spend**; no credential, firewall, ledger or original binary changes.
- Two fresh explicit-config smoke builds: BMP280 standard **4.468 s**, SHT31
  standard **4.026 s** process wall time. Each passed **14/14** native gates;
  **81** native/BOM/preview/manufacturing deliverables matched the existing
  reviewed examples under the existing timestamp-only normalization policy.
- Historical evidence authentication passed before and after, retaining all
  original failures, 677 first-family historical publication files, the v2
  reviewed examples, all 330 final-run evidence copies, and exhausted ledgers.

The published receipt binds command arguments, terminal results, durations,
source hashes, logs, coverage data, original-history hashes, the new binary hash
and complete cached native-output hashes. Local SHA-256 is integrity checking,
not a cryptographic provider attestation or independent review.

Recheck published bytes with `node specs/board-family-v2/offline-admission-01/check.mjs`.
Add `--current` in the originating workspace to also check the exact current
source, retained binaries, ledgers and new cached native bundles.

## Remaining release boundary

The grammar is intentionally narrow and conservative. It recognizes reviewed
atomic phrases, not all word orders, paraphrases or arbitrary English semantics.
Unfamiliar requests may need restatement even when a human would consider them
valid. A model can still falsely refuse or provide the wrong configuration;
neither case is silently upgraded. No general reliability improvement has yet
been measured on unseen requests.

The frozen live evaluation remains **4/14 raw-contract and 8/14 application
passes**, including the unsafe heater output as failed evidence only. The new
local replay does not change those scores. A future acceptance protocol must
identify this local admission policy and retain separate raw-model and final
application results. No extra request is authorized by this review.

PR #14 must remain draft pending separately approved, newly qualified live
acceptance. Software/native validation also does not replace fabrication,
assembly, firmware or bench characterization. No measured-board, safety,
regulatory, accuracy or physical performance claim is made.
