# Closed-Loop Open-Set V6 Corpus Freeze Audit

Status: frozen; outcome-blind validation passed

The corpus contains behavior-only public requirements from three isolated authors. No synthesis, simulation, feasibility, classification, ranking, or outcome inspection occurred during authoring, validation, or publication.

- validated cases: 36
- discovery cases published as plaintext: 18
- held-out cases published only as authenticated ciphertext: 18
- manifest SHA-256: `0445db99a32b5d62e8fc897d532e994fe85ff072aceba294733183d06f6b685a`
- validation report SHA-256: `8d29fb93a335f9f6683370602e2d1a2a84168e7d6d6ffb64c9ac6aa5e43a5558`
- held-out payload SHA-256: `010fa2ed259c5224f7993d0fbdb2ee81dddd6cd0b58f563df9eb99e1189fbcd7`
- held-out ciphertext SHA-256: `bbfe097e477783b2f1b89e22d0ffe157580c1d585aba5bfb787afba4f18dda32`
- encryption: `AES-256-GCM/random-nonce-prefixed/length-delimited-aad`
- validator manifest SHA-256: `de2becb7ba23cb9b6addc5d31876124d59326b201325d2e0b629dd8c6894c887`
- publisher manifest SHA-256: `3847161b137137a658e7883c7b7f2209f77b6303b3d018e7b5367028db988ffd`
- contract manifest SHA-256: `61f76c00477f2f6eb350556f4e2d0ba85b338846a9b61ed92691263c9552f591`
- packet-set SHA-256: `664b6d20ceb1433509e20016e0fbe3ddf98f6c8c1da01f5aeca7f50f2db6f31a`
- historical commitments SHA-256: `eb329517366df07d5364bdc43424a8caf2f86d8bd806086b0af8ea68f02f5807`

## Aggregate role/domain counts

- discovery / analog: 3
- discovery / digital: 3
- discovery / mcu: 3
- discovery / mixed_signal: 3
- discovery / power: 3
- discovery / sensor: 3
- held_out / analog: 3
- held_out / digital: 3
- held_out / mcu: 3
- held_out / mixed_signal: 3
- held_out / power: 3
- held_out / sensor: 3

The 32-byte V6 held-out source key was created exclusively outside the repository and is not named or committed here. The freeze-parent commit is recorded in the manifest; the corpus-freeze commit is the first Git commit containing this exact manifest checksum. Any later source, policy, validator, assignment, authorship, or key-binding change requires a new experiment version and fresh baseline.
