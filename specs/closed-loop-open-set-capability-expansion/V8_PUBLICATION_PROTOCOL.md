# V8 Corpus Publication Protocol

The publisher consumes exactly the committed V8 contract, six-author packet,
validator, sanitized V1–V7 commitments, six isolated bundles, and the
validator report. It performs no synthesis, simulation, feasibility,
classification, ranking, or outcome inspection.

## Atomic publication

The destination must not exist and must resolve inside the repository. The
source-key path must resolve outside it and must not exist. The custodian
creates a random 32-byte key with mode 0600, stages every artifact in the
destination parent, synchronizes files and directories, and publishes with a
same-filesystem no-replace rename. Any failure removes the stage and newly
created key. Existing destinations and keys are never replaced.

Exactly 18 discovery requirements are published byte-for-byte. Exactly 18
held-out requirements are absent as plaintext and encoded as independently
authenticated AES-256-GCM records. Every record has a unique random 96-bit
nonce. Its AAD length-prefixes the frozen schema, record index,
validation/contract/packet/publisher commitments, historical commitment, and
freeze-parent commit. Per-case metadata is authenticated inside each encrypted
record and is not published separately. The record set commits every
ciphertext, the ordered plaintext-hash aggregate, the ordered AAD-hash
aggregate, and only an aggregate of the per-case metadata hashes. The publisher
immediately opens and byte-compares all records before publication.

The public manifest and sanitized validation report contain exact discovery
metadata, aggregate held-out counts and commitments, and no held-out case ID,
path, source ID, semantic digest, or author-to-case mapping. Raw authorship JSON
is verified against its frozen hash but is not published. The source key is the
only route to the held-out metadata and requirement bytes.

## Non-circular obligation identity

`manifest.json` commits the validated discovery entries and aggregate held-out
record-set metadata. Its exact SHA-256 is then the first field of every
obligation anchor. This avoids a self-referential manifest while preserving the
frozen formula without disclosing a held-out mapping.

For every assertion/operating-case pair, the publisher length-prefixes these
ordered UTF-8 fields with unsigned 32-bit big-endian lengths and hashes them:

1. corpus manifest SHA-256;
2. corpus role;
3. case ID;
4. operating-case ID;
5. assertion ID;
6. observation kind;
7. observation ID; and
8. observed output ID, using `@circuit` only for whole-circuit observations.

Duplicate or unresolved anchors fail publication. Discovery anchors and their
public semantic inputs are written to `discovery_obligations.json`. Held-out
anchors are sorted and represented publicly only by count and an ordered
length-prefixed aggregate SHA-256 in
`held_out_obligation_commitment.json`. Both artifacts and `manifest.json` are
bound by `CHECKSUMS.sha256`; neither anchor artifact is an input to the manifest
hash.

After checksum verification and authenticated publication, the custodian must
remove all quarantine plaintext. Cleanup happens outside the publisher only
after the committed repository artifacts and external key are independently
verified.
