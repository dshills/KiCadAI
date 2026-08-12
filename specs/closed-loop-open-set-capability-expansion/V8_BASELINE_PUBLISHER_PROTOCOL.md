# V8 Blind Baseline Publisher Protocol

This outcome-neutral publisher is frozen before any V8 corpus synthesis. It
accepts exactly 18 opaque evidence records produced by the isolated held-out
baseline custodian. It does not parse requirements, outcomes, gaps, anchors,
paths, diagnostics, frontiers, membership, timing, or promotion details.

## Commitment boundary

The authenticated binding commits the V8 contract, corpus, validator, corpus
publisher, baseline publisher, validation report, author packet, historical
commitments, encrypted source, public obligation artifacts, discovery baseline,
frontier, ranking, selection, generic plan, evaluator, registries, policies,
inventory, catalog, model registry, seeds, resource ceilings, and exact
installed-KiCad promotion environment. Every binding field is validated and
encoded exactly once in declaration order. The executable registry test proves
that the JSON contract and authenticated order remain identical.

## Record encryption

Each opaque record is encrypted independently with AES-256-GCM and a fresh
random 96-bit nonce. A record's AAD length-prefixes the V8 schema, version,
algorithm, every binding field, zero-based record index, and exact record count.
Nonce reuse, a partial cohort, an empty record, more than 16 MiB total
plaintext, authentication failure, trailing bytes, or aggregate mismatch fails
publication.

The public manifest contains only the exact case count, whole-ciphertext hash,
ordered plaintext-hash aggregate, ordered AAD-hash aggregate, randomized
per-record ciphertext hashes, nonce size, complete public binding, and a
self-hash. It contains no per-case plaintext hash or semantic metadata. The
record set is opened and byte-compared before publication.

## Key and publication boundary

The baseline key is created outside the repository with mode 0600 and must be
distinct from every reserved source or final-result key. Key material is never
written to repository artifacts. Failure removes a newly created unpublished
key.

The destination must be a new path inside the repository. The publisher writes
the record ciphertext, canonical manifest, non-revealing audit, and canonical
checksums to a sibling staging directory, synchronizes them, and performs a
same-filesystem no-replace rename. The verifier accepts exactly those four
bounded regular files, independently checks every commitment, and rejects
semantically equivalent noncanonical rewrites.

This freeze does not authorize opening a V8 source key or running a baseline.
Those actions remain gated on an authenticated published corpus, committed
generation-zero public selection, isolated custodian context, and the external
key authorization in the frozen V8 baseline protocol.
