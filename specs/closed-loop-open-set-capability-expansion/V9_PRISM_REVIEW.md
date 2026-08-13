# V9 Contract Prism Review

Prism reviewed the exact staged V9 contract freeze through the configured
external Gemini provider.

## Remediated finding

The verifier originally parsed checksum lines with `strings.Fields`, which
would split a path containing spaces. It now parses the exact two-space
`sha256sum` delimiter. Clause checks also normalize CRLF to LF before matching.

## Contract-cited disposition

Prism suggested requiring the V9 contract directory to contain no files absent
from `V9_CONTRACT.sha256`. That suggestion is not accepted because this is a
shared versioned directory containing the frozen V1–V8 protocols and, after the
contract phase, separately checksum-bound V9 author, validator, publisher,
evaluator, selection, implementation, and round artifacts.

`TestVersionNineContractChecksumManifest` instead requires the contract
manifest to contain exactly its declared ordered input set and verifies every
listed byte hash. Repository cleanliness, absence of V9 evaluation output
paths, and no-replace publication are separately mandatory runtime preconditions
in `V9_BASELINE_PROTOCOL.md` sections 1 and 9. Conflating those evaluation
preconditions with directory exclusivity would make the frozen phase plan
internally contradictory and would not reliably detect Git-untracked files.

That medium finding received the reproducible contract-cited disposition above.

## Verifier root-of-trust disposition

A later Prism pass suggested hardcoding both `v9_contract_test.go` and
`V9_CONTRACT.sha256` hashes inside the verifier. That is not accepted: the
verifier hash changes when its own hash is inserted, the manifest hash changes
when the verifier hash is updated, and inserting the manifest hash changes the
verifier again. No finite ordinary SHA-256 self-hash construction satisfies
that circular proposal.

The noncircular chain is deliberate: `V9_CONTRACT.sha256` hashes the verifier;
the verifier requires the exact ordered manifest entries and validates their
bytes; and the reviewed verifier plus manifest are atomically anchored by the
Git commit. This implements `V9_SPEC_ADDENDUM.md`'s status rule that the
documents, manifest, and executable verifier become frozen only when committed
together. The same construction is used by the existing V8 contract.

The shared `v8*` helper names noted in a low finding are also retained
intentionally. They are package-local generic decoding, hashing, and path
helpers already exercised by the frozen V8 contract. Renaming them would alter
the V8 verifier byte commitment without changing behavior; duplicating them
would create two implementations of the same integrity logic.

No high findings remained. Both medium findings were either remediated or
received the reproducible contract-cited dispositions above.
