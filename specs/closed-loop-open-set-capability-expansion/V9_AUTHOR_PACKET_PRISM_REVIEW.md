# V9 Author-Packet Prism Review

Prism reviewed the exact staged V9 author packet, deterministic generator, and
executable verifier through the configured external Gemini provider.

## Remediated finding

V9 call sites now use V9-named wrappers for the existing byte-frozen strict
checksum, JSON, hashing, and path helpers. This preserves V8 verifier bytes
while making cross-version reuse explicit.

## Contract-cited disposition

Prism suggested that the update-gated packet publisher delete an existing V9
packet root before regeneration. That suggestion is not accepted. The packet
is a frozen evaluation input, not a developer cache. `V9_PLAN.md` phase 1
requires checksum-bound packets, and `V9_BASELINE_PROTOCOL.md` section 9
requires fresh temporary publication, atomic no-replace rename, and independent
verification. Removing an existing authenticated packet would permit silent
replacement after authorship and would contradict the no-overwrite invariant.

The update test therefore fails before writing when the destination exists.
The generator's byte-for-byte reproduction test provides deterministic rebuild
evidence in a temporary root without mutating the frozen publication.

No high findings remained. The medium finding received the reproducible
contract-cited disposition above.

## Dependency and write-path remediation

A later Prism pass considered `V9_CORPUS_RULES.md` missing because it was
already committed in the preceding V9 contract freeze and therefore absent
from the staged packet diff. The generator now verifies that source against the
frozen V9 contract hash before publication. Its no-replace preflight uses
explicit existing/missing/error branches, and all writes go through one helper
that creates the required parent directory before checking the write error.

The packet verifier also declares its own explicitly named frozen corpus-rules
hash. This avoids relying on the identical package constant in the already
committed V9 contract verifier and makes staged-diff review self-contained.

## Partition-index disposition

Prism later suggested that held-out authors should be skipped while incrementing
the assignment domain index. V9 has no held-out-only authors: each of the six
authors receives four discovery and four held-out entries. Both partitions use
their own complete 0–23 index; the held-out partition then rotates domain by
two and circuit role by three to improve cross-partition independence.

The executable verifier proves that every domain and circuit role occurs
exactly four times in each partition, all 48 role/domain/circuit-role triples
are unique, and every author has the frozen 4/4 membership. A skip counter would
violate that model, so the proposed change is not accepted.
