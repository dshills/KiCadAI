# Closed-Loop Open-Set V7 Corpus Freeze Audit

Status: frozen; outcome-blind validation passed

The corpus contains behavior-only public requirements from three isolated authors. No synthesis, simulation, feasibility, classification, ranking, or outcome inspection occurred during authoring, validation, or publication.

- validated cases: 36
- discovery cases published as plaintext: 18
- held-out cases published only as authenticated ciphertext: 18
- manifest SHA-256: `cf85a7eb8293abdd7f85215b3f897ccf4b89be993ac5c101d09013f2ca979f06`
- validation report SHA-256: `429f425c73023880b45b1e3d176ecbde3bf3195fd290646181ebf9385924ea40`
- held-out payload SHA-256: `6942b7b7e2146cb99d5e489a022c77fc998c83d8c601ac182bcd5f37923b0867`
- held-out ciphertext SHA-256: `fcbbc5ec59396701e757b89c94d67e10f2a09c16f870cf7bdf2a644f4b072119`
- encryption: `AES-256-GCM/random-nonce-prefixed/length-delimited-aad`
- validator manifest SHA-256: `7fd3c5cb5f456361b1c8417e52212114f5dcae0bcb227f4bef09b5f1346a2f20`
- publisher manifest SHA-256: `fc0ec211c83ce0311071328ac94962048a4615ac379898413c627df68c361af4`
- contract manifest SHA-256: `40d1f64af6f06763bcb3c04275b56fd4d0c24dafe1940577618d78415408020e`
- packet-set SHA-256: `7b0bffb5869cfc215aa97d333bfecb56ee87b730862bceb11fd619181a268451`
- historical commitments SHA-256: `bf39d127b950d0fb09c96a6ed34fdfd20258ee275ba5a49844be2aa2678af00d`

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

The 32-byte V7 held-out source key was created exclusively outside the repository and is not named or committed here. The freeze-parent commit is recorded in the manifest; the corpus-freeze commit is the first Git commit containing this exact manifest checksum. Any later source, policy, validator, assignment, authorship, or key-binding change requires a new experiment version and fresh baseline.
