# V4 Independent Corpus Freeze Audit

Status: frozen; synthesis and outcome inspection not yet performed

## Chain of custody

- outcome-changing starting commit:
  `3d2d9bb0e8ff3e68ae6a160c136030b5a3b6d7db`
- V4 public-contract freeze commit:
  `f88efb866dca97ab4966a67ac6e8d50ec6e245f4`
- isolated-authoring-packet commit:
  `f42f8f99a66dcb3962a7915646274973e506c155`
- packet-manifest SHA-256:
  `e57a422bc83cd53446bab3389f434baacaafdce9c73c0b6f46ee709770682770`
- author-manifest SHA-256:
  `3c6916db4bfd932cda1d495f1307530b70c94b75030d86acedacfafec31f1580`
- authorship-record SHA-256:
  `48c17b2089080b0b2613d47bb80ca615118066fa944e3d5556d6f7408171307b`
- mechanical-validator SHA-256:
  `fd1994dbddc37c3c37a259b4790c784a2b89f792a08a727a11a43951d1bb5437`

The author worked in a fresh isolated context from 2026-08-10T01:02:02Z to
2026-08-10T01:15:10Z. The committed authorship record attests that the frozen
five-file authoring packet was the only input and that no repository,
implementation, prior corpus, circuit example, conversation, synthesis,
diagnostic, selection, or outcome was available. The exact deployed GPT-5
build identifier was not exposed; no electrical or schema uncertainty remained.

## Mechanical validation

The quarantine validator passed before freeze. It performed no synthesis,
feasibility analysis, classification, ranking, or outcome observation. It
proved:

- byte-identical author manifest and complete authorship attestation;
- exactly 24 requirements and no unexpected bundle files or directories;
- 12 discovery and 12 held-out cases;
- exactly two cases per role/reporting-domain pair;
- strict public-schema decoding, canonical enums/units, finite ordered bounds,
  declaration/reference integrity, and the V4 canonical distortion metric;
- at least two operating cases, four assertions, and two analysis kinds in
  every requirement;
- all 14 acceptance gates present and true;
- raw and project-neutral semantic uniqueness within V4;
- exact raw non-overlap with every committed V3 source hash;
- semantic non-overlap with V1, V2, and visible V3 discovery requirements;
- no manifest-identity leakage or prohibited implementation vocabulary; and
- all frozen per-role supply, observation, analysis, variation, event,
  multi-output, convergent-excitation, and safety-domain diversity minima.

Retired V3 held-out plaintext was not opened. Its committed raw hashes reject
byte-identical reuse; the isolated V4 author had no prior-corpus access from
which to derive a semantic transformation. This preserves the V3 blind-retired
boundary instead of weakening it for a retrospective comparison.

## Frozen corpus

- corpus-manifest SHA-256:
  `4c6bc5c8c94bccc16a066f69ac0a4c69c326e8f77aa8db02f9b9ef327ef732a6`
- held-out ciphertext SHA-256:
  `7347bf563fe8a420a62cd8bde64b09fdcf29ae273fa0f56c6b28a69aa3588740`
- held-out payload commitment:
  `480fc533f83b89048f747cd7d1bae43e072b22f9339d3a34c02fb15577e5e933`
- held-out algorithm:
  `AES-256-GCM/random-nonce-prefixed`

Discovery bytes are committed read-only. Held-out source bytes exist only in
authenticated ciphertext; their 32-byte key is outside the repository. The
plaintext candidate quarantine was destroyed after the encrypted freeze.
Manifest entries retain raw and neutral-semantic commitments for all 24 cases
without exposing held-out electrical content.

The authenticated decrypt-and-commitment test passed using the external key,
as did random-nonce uniqueness, tamper rejection, changed-payload binding,
corpus-manifest reproduction, and the V4 contract package. No case has been
synthesized or assigned pass, unsupported, unsafe, or exhausted status.

## Pre-commit public review adjudication

Prism identified a discovery-only nominal-voltage inconsistency before the
corpus-freeze commit. The isolated author, still without synthesis or outcome
access, re-reviewed only `discovery/request_012.json` and confirmed that a 6 V
nominal delivered level was inconsistent with the authored 1 V nominal command
and 4.8–5.0 transfer gain. The author changed that nominal to the consistent
4.9 V midpoint and corrected one indentation error. Strict decoding and all
mechanical/freeze checks were rerun, and the discovery raw/semantic commitments
plus manifest checksum were refreshed. Held-out ciphertext and commitments
were not opened or changed by this adjudication.

A subsequent Prism pass identified two additional discovery-only compliance
limits. The isolated author confirmed both without synthesis/outcome access:
`request_002` reduced its maximum load from 1000 to 500 ohm, leaving 0.4 V
headroom at the authored worst output-current ceiling and minimum supply; and
`request_012` reduced its maximum command from 1.8 to 1.6 V, leaving 1.0 V
headroom below the minimum delivery supply. Their raw/semantic commitments and
the manifest checksum were refreshed. Separately, the held-out seal was
mechanically migrated to a standard random nonce prefixed to AES-GCM
ciphertext; plaintext was not displayed, the payload commitment stayed
unchanged, and authenticated decryption was reverified.
