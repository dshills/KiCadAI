# Closed-Loop Open-Set V10 Corpus Freeze Audit

Status: frozen; outcome-blind validation passed

Six isolated authors supplied behavior-only requirements. Publication performed no synthesis, simulation, feasibility, classification, ranking, or outcome inspection.

- validated cases: 48
- discovery plaintext cases: 24
- held-out record ciphertext cases: 24
- manifest SHA-256: `0ec3834c832246e659b417dcef4aaae6d1634cbcd19c734518990280b124dc94`
- public discovery obligations: 170
- held-out obligation count: 171
- held-out obligation aggregate: `dfa4db21b9b1edf11eba0c376be00f04f6c1058b4380311a9437094f21ddc01a`
- held-out ciphertext SHA-256: `9502aa524989db9be68e6e941d0f03c5e2ac5765123a38d84768b22561a264f5`
- encryption: `AES-256-GCM/random-unique-nonce-per-record/length-delimited-aad`
- validation report SHA-256: `fe6917c0fc9fd3e39db023e3cdd6eede3c3f5ab7f1e654472bb9268a0a4b947d`
- sanitized authorship attestations SHA-256: `38f5221196df5fe0829b24d7a6fa7749e79dbbfbcc9078320dc2024caf9edfba`
- validator manifest SHA-256: `c8b51e0dde6814efed026efe1302e177d49bec7d80094df5d3c22ef53fb67de5`
- publisher manifest SHA-256: `34d64b627eaf0ddf43e3a0cfea0b54a280e3769e24c766b5febd3fae91c51c25`
- packet-set SHA-256: `77804df628be0979727ea2821c272f1eb2b39483db45ee6cd91284c54086d423`
- historical commitments SHA-256: `eea2e5aba73be6469052ec3a73da5bb1994f11251708c89d31d5268d6359fb2e`

The 32-byte V10 held-out source key was created exclusively outside the repository. Held-out records use independently authenticated unique random nonces. Discovery anchors are public; held-out anchors are represented only by their aggregate commitment. All artifacts are bound by the publication checksum manifest.
