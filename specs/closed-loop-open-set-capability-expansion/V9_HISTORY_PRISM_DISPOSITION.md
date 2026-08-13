# V9 Historical-Commitment Prism Disposition

Prism reviewed the staged digest-only V9 historical commitments and reported
that older corpus versions do not appear in every semantic commitment layer,
and that their source identifiers use the naming conventions of their original
versions.

Both properties are required provenance, not missing derived data. The frozen
V9 custodian contract requires exactly 240 raw, 168 neutral-semantic, and 144
normalized-semantic commitments. Neutral-semantic identities exist only for
versions whose frozen publishers emitted that representation; normalized
semantic identities begin later for the same reason. Reconstructing absent
historical identities would invent unhashed source semantics after the fact.

Likewise, each source identifier is retained byte-for-byte from its committed
version. Renaming identifiers would break historical identity and the frozen
predecessor chain.

The finding is therefore resolved by preserving the authenticated artifact.
The root context independently reproduced its SHA-256
`eea2e5aba73be6469052ec3a73da5bb1994f11251708c89d31d5268d6359fb2e`
and the frozen history, corpus-validator, and custodian tests passed.
