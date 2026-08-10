# Closed-Loop Open-Set V5 Corpus Freeze Audit

Status: frozen; outcome-blind validation passed

The corpus contains behavior-only public requirements from three isolated authors. No synthesis, simulation, feasibility, classification, ranking, or outcome inspection occurred during authoring, validation, or publication.

- validated cases: 36
- discovery cases published as plaintext: 18
- held-out cases published only as authenticated ciphertext: 18
- manifest SHA-256: `d703608d09d7d7bd834bb45698446dd03bb0dbe7b00733b636dd73250cac3f6d`
- validation report SHA-256: `1b65c6c01fcdd3b48ba561ba3f3442c535866ef18d693f703d5b48edac143c89`
- held-out payload SHA-256: `9af50274f8d0154e60431712012d3dd0915a87702319f897c0028fd9956f7e3a`
- held-out ciphertext SHA-256: `c5d0da5b106e6a25d9a5803b818cdec39a5f73ff63a1f330526dae112a2bef7d`
- encryption: `AES-256-GCM/random-nonce-prefixed/length-delimited-aad`
- validator manifest SHA-256: `c11ff9a03f060a5ca9d1ad9e22aa4b99ac5f341e1ed44084353f38428183972e`
- publisher manifest SHA-256: `4b0b30f8cdb1ab55e755483c434bc233a27a94a1f96be28aae8e6bf530723c6b`
- contract manifest SHA-256: `c6d842fc14bcc84d236dcde513c8e242c71c563adbfc6c06ba98e3baab811c7e`
- packet-set SHA-256: `004dc3ab1325e34d12190cf0358adb607597f2e7bc8fff44eb412309b63c42b9`
- historical commitments SHA-256: `0de93fd451ab322d6b0dbaaf1a74cc088e208dce28ea22e6f4513435bc95e700`

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

The 32-byte held-out key was created exclusively outside the repository and is not named or committed here. The freeze-parent commit is recorded in the manifest; the corpus-freeze commit is the first Git commit containing this exact manifest checksum. Any later source, policy, validator, assignment, authorship, or key-binding change requires a new experiment version and fresh baseline.
