# Closed-Loop Open-Set V7 Corpus Publication Protocol

Status: publisher freeze candidate; no valid post-validator author bundle has
been consumed

## Preconditions

Publication is a one-time isolated-custodian action after all three fresh
bundles pass the re-frozen `V7_VALIDATOR.sha256` boundary and public electrical
sanity review. The custodian must start from the exact clean publisher commit,
receive explicit authorization naming a new external key path, and prove that
the destination corpus directory and key path do not exist.

The command accepts the three isolated bundle roots, frozen contract,
validator, and publisher manifests, immutable lineage commits, an empty
in-repository destination, and a distinct external key path. The starting,
contract-freeze, author-packet, and validator commits must exactly match the
compiled V7 boundary. The command revalidates the corpus from source before
publication; a previously written validation report is not trusted as input.

## Atomic publication

The publisher:

1. verifies every checksum manifest and fixed lineage identifier, resolving
   intentional parent-relative entries only while every canonical target
   remains inside the repository root;
2. reloads the exact V7 packet and sanitized V1–V6 historical commitments;
3. mechanically revalidates all 36 cases without synthesis, simulation,
   feasibility, classification, ranking, or outcome inspection;
4. creates one exclusive 32-byte external key with Unix mode `0600`;
5. publishes the 18 discovery sources as canonical plaintext;
6. seals the 18 held-out sources as one AES-256-GCM payload using a fresh
   random nonce and V7-specific magic, version, schema, and length-delimited
   authenticated metadata;
7. verifies the ciphertext by reopening it in memory before publication;
8. writes only canonical manifest, aggregate validation report, authorship,
   discovery, ciphertext, non-revealing audit, and checksum artifacts to a
   same-filesystem staging directory; and
9. syncs and atomically renames the complete tree without replacement.

Any failure removes the staging tree and only a key created by that failed
attempt. Successful publication persists the key externally and prohibits
regeneration, substitution, or a second publication under V7.

## Version separation and re-freeze boundary

The V7 manifest is `kicadai.closed-loop-open-set-corpus.v7`; the encrypted
payload uses `KICADAI-V7-HELDOUT` magic and version 7; and the authenticated
schema is `kicadai.closed-loop-open-set-held-out-source.v7`. V5 and V6 openers
must reject V7 ciphertext, and the V7 opener must reject cross-version manifest
metadata and modified ciphertext.

`V7_VALIDATOR_REFREEZE.md` records the pre-authoring checksum correction. Only
fresh author contexts started after that re-freeze commit are admissible.
`V7_PUBLISHER.sha256` binds this protocol, contract and validator roots, the V7
command, the V7 publication extension and tests, and every inherited
filesystem, checksum, crypto, serialization, and atomic-rename dependency it
reuses.
