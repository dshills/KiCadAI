# Closed-Loop Open-Set V6 Corpus Publication Protocol

Status: publisher freeze candidate; no author bundle has been consumed

## Preconditions

Publication is a one-time isolated-custodian action after all three returned
bundles pass the frozen `V6_VALIDATOR.sha256` boundary and public electrical
sanity review. The custodian must start from the exact clean validator commit,
receive an explicit authorization naming a new external key path, and prove
that the destination corpus directory and key path do not exist.

The command accepts the three isolated bundle roots, frozen contract,
validator, and publisher manifests, immutable lineage commits, an empty
in-repository destination, and a distinct external key path. It revalidates
the corpus from source before publication; a previously written validation
report is not trusted as input.

## Atomic publication

The publisher:

1. verifies every checksum manifest and lineage identifier;
2. reloads the exact packet and V1–V5 historical commitment boundary;
3. mechanically revalidates all 36 cases without synthesis or classification;
4. creates one exclusive 32-byte external key with Unix mode `0600`;
5. publishes the 18 discovery sources as canonical plaintext;
6. seals the 18 held-out sources as one AES-256-GCM payload using a fresh
   random nonce and V6-specific magic, version, schema, and length-delimited
   authenticated metadata;
7. verifies the ciphertext by reopening it in memory before publication;
8. writes only canonical manifest, aggregate validation report, authorship,
   discovery, ciphertext, non-revealing audit, and checksum artifacts to a
   same-filesystem staging directory; and
9. syncs and atomically renames the complete tree without replacement.

Any failure removes the staging tree and only a key created by that failed
attempt. Successful publication persists the key externally and prohibits
regeneration, substitution, or a second publication under V6.

## Version separation

The V6 manifest is `kicadai.closed-loop-open-set-corpus.v6`; the encrypted
payload uses `KICADAI-V6-HELDOUT` magic and version 6; and the authenticated
schema is `kicadai.closed-loop-open-set-held-out-source.v6`. The V5 opener must
reject V6 ciphertext, the V6 opener must reject V5 ciphertext, and adding V6
must not change V5 publication output or its frozen source bytes.

`V6_PUBLISHER.sha256` binds this protocol, the complete V6 validator manifest,
the V6 command, the V6 publication extension and tests, and every frozen V5
filesystem, checksum, crypto, serialization, and atomic-rename dependency it
reuses.
