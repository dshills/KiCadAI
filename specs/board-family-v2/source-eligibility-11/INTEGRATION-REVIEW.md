# Candidate 11: offline integration review

Status: **offline integration verified; live acceptance not demonstrated**.
This is a local implementation review, not an independent or external model
review. No provider request, firewall change, publication, merge or primary
checkout update was performed for this increment.

## Scope and findings

The explicit `source-eligible-v8` mode adds source-owned eligibility metadata,
source-anchored feature assertions and precision-quantity guards. Its exported
contract, provider preparation and local replay use the same schema and input.
The default remains `typed-v2`; v7 extraction and native board admission remain
unchanged. Internal compilation is not a correction of captured provider text:
an invalid assertion rejects the whole extraction, with original bytes retained.

Accounting, admission, journal and audit identities are distinct from v7. Tests
reject cross-protocol ledger use in both directions, even with identical model,
goal and limits. Full-model pricing, the 16,000-byte request bound and 1,600-token
output cap remain within the existing $0.05 reservation. All 14 corpus payloads
were measured through in-memory HTTP transports: 7,213–10,780 bytes. This is
neither an API token measurement nor provider schema acceptance.

Unknown outcomes, incomplete responses and model mismatch stop further calls
on that ledger. Model mismatch retains evidence but is not assigned a cost for
the wrong model. Request/cost limits, duplicate IDs, missing or wrong accounting
profiles, immutable journals and no-retry behavior are exercised offline.
Native generation cannot start after a failed extraction or evidence write.

Review found one test-artifact directory collision: v7 and v8 native tests shared
the same retained output name. The v8 test now uses `source-eligible-native` and
v7 keeps its existing `native` name. The full check sequence was rerun after the
correction. No blocking defect was found in the reviewed integration scope.

## Final verification

The final local receipt is `.cache/source-eligibility-11-04/verification.json`:

```text
SHA256 75cdd7059e0419b27f2c1d004031b60be806cc128dc8c8f8683dca49ced75acb
binary 4687f23f3a6441e64a7fd5d5f2dfc57ae56691a59aee30726f9765e2a7b102d1
```

All 11 recorded commands reached an observed terminal exit of zero:

- Focused tests: 147 passing test events, including subtests.
- Short regression suite: boardfamily, board-family command and AI provider.
- Race checks for the new mode and existing full-model mode.
- Five-second fuzz target: 21,116 executions, no failure.
- Read-only replay of all 14 original full-model journals; v8 rejects those
  journals as its own evidence.
- Synthetic extraction through real generation and KiCad 10.0.3 validation:
  five bundles across both board families; all 14 gates and 19 manufacturing
  export files per bundle. Native hashes match the reviewed examples.
- Vet, lint, binary build, formatting and diff checks.

The receipt authenticates 17 source/document files and 351 evidence/runtime
files. The other 7,388 files in the evaluated source manifest remain byte-for-byte
unchanged in this candidate. The frozen publication checkout remains clean at
`ca9ca4df07730c8d7d042eb9a9dfa2a6735f771d`. Earlier `11-01`, `11-02` and `11-03`
receipts remain historical and are not the current integration's final receipt.

## Limits and next boundary

The last actual model evaluation remains **12/14 complete cases and 5/5 native
bundles**. v8 has zero live responses. Hand-authored synthetic fixtures, exact
replay and lexical guards do not prove semantic correctness, completeness or
reliability on unseen requests. A mentioned feature can still receive the wrong
state; the suite deliberately retains a counterexample. No physical boards were
manufactured or tested.

Next, prepare and review a separate evaluation package against this source and
binary, preserving the existing corpus, scoring rules and prior failed evidence.
A new live run requires explicit payload/budget approval and approval of any
new executable-specific network rule. Old batch slots cannot be reused or
resumed. No new live budget, request, PR publication or physical work is implied
by this review.
