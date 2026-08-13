# Closed-Loop Open-Set V8 Corpus Freeze Audit

Status: frozen; outcome-blind validation passed

Six isolated authors supplied behavior-only requirements. Publication performed no synthesis, simulation, feasibility, classification, ranking, or outcome inspection.

- validated cases: 36
- discovery plaintext cases: 18
- held-out record ciphertext cases: 18
- manifest SHA-256: `548d8f38cdbc6186a737d9c1cfdea73906a25f6b1948b9a367e00897f7c66f1c`
- public discovery obligations: 119
- held-out obligation count: 139
- held-out obligation aggregate: `d3f66f5a0966349f0060efc331e08f04552d7ca30762405d756778ae95a4be28`
- held-out ciphertext SHA-256: `4a6ac063a243cfc7e4005abc5f07e7ef934fc54abb5f2d52d3aa1f4df11c718e`
- encryption: `AES-256-GCM/random-unique-nonce-per-record/length-delimited-aad`
- validation report SHA-256: `9f2a77e54c879e8a9bf023497a31498f8cb818a5c71dbfc4d6f8b9a482779076`
- validator manifest SHA-256: `0c58c815386898643c397c726bee596e516f6231614f6ae08f29ccfd6324b707`
- publisher manifest SHA-256: `cd448701a7bf4fe413a5d2cd8d4bea3fedfd58865d0c8df60da4ad60f523974d`
- packet-set SHA-256: `5a243103b6dee088470a521617a88f33685cf2bfb170c68cffa0e1f93bfacc76`
- historical commitments SHA-256: `f56d30c27b30e90f4c8568e06870718bac7e9db7d29ed24dac6c768ad163cebf`

The 32-byte V8 held-out source key was created exclusively outside the repository. Held-out records use independently authenticated unique random nonces. Discovery anchors are public; held-out anchors are represented only by their aggregate commitment. All artifacts are bound by the publication checksum manifest.
