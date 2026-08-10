# Closed-Loop Open-Set V5 Quarantine and Corpus-Freeze Protocol

Status: operational validator freeze; corpus authoring may begin only from a
commit that contains this protocol and the matching `V5_VALIDATOR.sha256`

## Boundary

This protocol receives three independently authored bundles without invoking
KiCadAI synthesis, feasibility, classification, ranking, simulation, physical
promotion, or outcome logic. It may validate public electrical structure and
commitments; it must not predict whether a case will pass.

Each author receives only the six files named by their committed
`AUTHOR_N_PACKET.sha256`. Each returned quarantine contains exactly:

- `AUTHORSHIP.json`; and
- the twelve JSON requirements at the assignment's fixed relative paths.

The three author contexts, input bundles, and return quarantines remain
separate. The custodian supplies frozen assignments to the validator; an author
does not return or rewrite an assignment.

## Strict authorship evidence

`AUTHORSHIP.json` strictly follows
`kicadai.closed-loop-open-set-authorship.v5`. Unknown or missing fields fail.
The record binds the author slot, exact authoring tool/model version, ordered
RFC3339 interval, per-author packet checksum, contract binding, assignment
checksum, quarantine identity, exact full-byte SHA-256 for all twelve returned
requirements, disclosed uncertainties, and ten affirmative isolation and
no-outcome attestations.

Placeholders, malformed or uppercase hashes, duplicate or unsafe paths, false
attestations, reversed timestamps, extra files, missing files, and commitment
mismatches fail closed.

## Outcome-blind validation

`internal/corpusfreeze` performs only:

1. strict assignment, authorship, and requirement JSON decoding;
2. exact author/role/domain/safety/path and source-hash checks;
3. canonical relative-path and complete file-set enforcement;
4. all 14 acceptance gates and public requirement minima;
5. behavior-only language and manifest-identity leakage checks;
6. raw, project-neutral semantic, and ID-normalized semantic uniqueness;
7. comparison with frozen historical raw and semantic commitments without
   opening retired sources;
8. per-author and per-role diversity, safety, event, analysis, multi-output,
   and converging-excitation quotas; and
9. discovery/held-out structural-signature separation within each author and
   reporting domain.

Validation errors name only the public case identity and violated contract.
The validator returns no requirement text, electrical value, outcome,
diagnostic, feasibility statement, gap, or implementation suggestion.

## Custodian interface

The outcome-blind custodian runs locally with one exact bundle root per frozen
author slot:

```text
go run ./cmd/kicadai-corpus-validate \
  -packet-root specs/closed-loop-open-set-capability-expansion/v5-authoring-packet \
  -history specs/closed-loop-open-set-capability-expansion/V5_HISTORICAL_COMMITMENTS.json \
  -bundle author_1=/external/quarantine/author_1 \
  -bundle author_2=/external/quarantine/author_2 \
  -bundle author_3=/external/quarantine/author_3 \
  -output /external/quarantine/V5_VALIDATION_REPORT.json
```

The loader verifies both packet checksum layers and requires the exact
policy-bound packet-set hash before it accepts assignments.
Each bundle must contain exactly `AUTHORSHIP.json`, the assigned requirement
files, and only their necessary directories; symlinks, special files, extra
paths, and missing paths fail closed. On Unix, every untrusted path component
is opened descriptor-relative with kernel-enforced no-follow semantics.
Historical comparison consumes only the
canonical raw and neutral-semantic commitments in the exact policy-bound
`V5_HISTORICAL_COMMITMENTS.json`. That file was mechanically derived from the
four retired corpus manifests; retired requirement sources were not opened.

Successful stdout contains only the total validated case count and isolated
author count. The atomically written report contains contract commitments,
case identities and hashes, and aggregate quota counts. It contains no source
bytes, expected outcomes, synthesis results, feasibility results, or gap data.

## Public electrical sanity

After mechanical validation, a custodian may review units, finite physical
environmental bounds, reference integrity, external-energy consistency, event
envelopes, and simultaneous-corner coherence. This review may correct an
author's clearly invalid public electrical statement only before corpus freeze,
with the correction and new source hash recorded in that author's provenance.
It must not run synthesis, inspect implementation coverage, or recommend a
realization. Any uncertainty fails closed.

## Freeze

The custodian orders validated entries by case ID, encrypts held-out source
bytes with a fresh external key, removes held-out plaintext from the
implementation workspace, and writes a corpus manifest containing only:

- contract, packet-set, per-author packet, assignment, authorship, validator,
  and historical-commitment hashes;
- author slot, role, reporting domain, safety impact, source identity, stable
  path, raw hash, neutral semantic hash, and normalized semantic hash;
- aggregate quota/diversity results;
- discovery plaintext entries;
- authenticated held-out ciphertext metadata and commitments; and
- starting, contract, packet, validator, and corpus-freeze commits.

The manifest, ciphertext, discovery sources, aggregate audit, and checksums are
committed atomically. Held-out plaintext, behavior summaries, per-case
diagnostics, and keys never enter repository history. Any post-freeze change to
an assignment, authorship record, requirement, policy, validator, manifest, or
key binding creates a new experiment version and requires a fresh baseline.
