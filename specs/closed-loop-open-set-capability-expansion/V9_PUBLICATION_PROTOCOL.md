# V9 Corpus Publication Protocol

Status: implementation prepared; final publisher freeze remains pending the
authorized V9 history commitment and final validator freeze.

## 1. Outcome-neutral input boundary

The publisher accepts only a complete V9 validator report and the six exact
validated author bundles. It requires 48 assignment-bound behavior cases in
canonical order: 24 discovery and 24 held-out, with four cases from each
partition for every author. Publication performs no synthesis, simulation,
feasibility evaluation, classification, ranking, or outcome inspection.

The publisher rechecks the report schema, commit bindings, author set,
authorship hashes, source hashes, case/source identities, partition membership,
and assignment metadata before creating any key or destination.

## 2. Public and sealed artifacts

The atomic publication contains exactly:

- 24 public discovery requirement JSON files;
- a public validation report containing full discovery evidence and only a
  count plus aggregate commitment for held-out validation evidence;
- one record-encrypted held-out source set containing exactly 24 records;
- public discovery obligations and one aggregate held-out obligation
  commitment;
- a sanitized authorship-attestation artifact;
- the corpus manifest, audit, and canonical checksum manifest.

The sanitized authorship artifact preserves author slot, exact authorship hash,
packet and assignment bindings, timestamps, uncertainty count, and boolean
isolation attestations. It excludes author context identity, tool/model identity,
quarantine paths, returned bundle roots, requirement paths, per-requirement
source hashes, and all held-out identities or contents.

## 3. Encryption and binding

The publisher creates a new 32-byte external source key as a regular `0600`
file using exclusive creation. It never writes the key or held-out plaintext to
the repository. Each held-out record uses AES-256-GCM with a unique random
nonce. Length-delimited associated data binds the V9 schema, record index,
validation report, sanitized authorship artifact, policy, packet set, contract,
validator, publisher, historical commitments, and freeze-parent commit.

The sealed-set manifest commits the ciphertext, every record ciphertext,
plaintext aggregate, associated-data aggregate, metadata aggregate, nonce
format, and record count. V8 and V9 record and set magic/version values are
disjoint.

## 4. Atomic fail-closed publication

All artifacts are written exclusively into a same-parent temporary directory,
checksummed canonically, synchronized, and installed with a no-replace atomic
operation. An existing destination is never overwritten. Any failure before
installation removes the temporary stage and newly created key. Publication
does not remove author quarantines.

## 5. Independent authentication

The public verifier opens no key. It rejects symlinks, special files,
unexpected files or directories, incomplete or noncanonical checksums, schema
or count drift, commitment mismatches, held-out metadata disclosure, and any
discovery or obligation evidence that does not reproduce.

The keyed verifier first performs the complete public verification, then opens
all 24 held-out records, verifies canonical order and author allocation,
reproduces the aggregate hidden validation commitment, and rederives both
public and held-out obligation commitments. It returns aggregate counts only
and clears opened held-out source buffers before returning.

Authenticated publication and keyed verification are prerequisites for any
later removal of quarantine plaintext. The preparation tests use only synthetic
requirements, entropy, paths, and keys; they do not access the external V9 key
path or any author quarantine.

## 6. Final freeze boundary

After the authorized digest-only V9 history custodian publishes the historical
commitment and the validator is finally frozen, the publisher CLI and checksum
manifest will bind the committed V9 starting, contract, packet, validator, and
freeze-parent commits. No corpus authoring or real key creation may begin from
this preparation snapshot alone.
