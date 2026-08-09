# Closed-Loop Open-Set V3 Corpus Audit

Status: independently authored, mechanically validated, frozen, and held-out source sealed

## Isolation and authorship

The fresh V3 author received only the sealed `v3-authoring-packet` and wrote a
new 24-case quarantine bundle: 12 discovery and 12 held-out cases. The
authorship record identifies the isolated context, records its authoring
window, attests that the packet was its only input, and discloses the public
schema ambiguities it encountered.

The author did not receive repository history, prior corpora, implementation
code, examples, synthesis behavior, diagnostics, selections, or outcomes. It
did not run KiCadAI synthesis, classification, simulation, or outcome tools.

## Mechanical adjudication

The content-blind quarantine validator checked:

- strict public-schema decoding and all 14 acceptance gates;
- fixed identities, role/domain/safety assignments, paths, and quotas;
- authorship completion and byte-identical preservation of the author manifest;
- behavior-only wording and absence of manifest identities in requirements;
- raw and project-neutral semantic uniqueness;
- semantic non-overlap with the frozen V1 and V2 corpora; and
- the frozen supply, observation, analysis, variation, event, multi-output,
  converging-excitation, and critical-domain diversity minima independently for
  discovery and held-out roles.

Mechanical validation returned only invariant names and aggregate counts. The
author corrected discovery structural diversity, held-out observation and
variation coverage, and held-out structural diversity before freeze. No
synthesis status, classification, diagnostic, or expected outcome was
available during those corrections.

The final forced quarantine validation passed.

## Freeze and blind boundary

The accepted discovery bytes were copied unchanged into
`testdata/closed_loop_open_set_v3_corpus/discovery`. Original SHA-256 hashes for
all 24 requirements are committed in the corpus manifest.

The 12 held-out source requirements were serialized into one deterministic
payload and authenticated-encrypted with
`AES-256-GCM/HMAC-SHA-256-payload-bound-nonce`. The 32-byte key is stored
outside the repository. The repository contains only ciphertext, its payload
and ciphertext commitments, case count, algorithm metadata, and the frozen
manifest metadata. The temporary plaintext quarantine was deleted after a
keyed mechanical authentication/decryption check passed.

- Corpus manifest SHA-256:
  `f721b8abc859a47030d17f92ea86dc301a3d3817b1cc52bd32261b54bb79c49e`
- Held-out plaintext payload SHA-256:
  `cf2fc67e2a4b1168cc99e548ff3fc46085688ba325616b2dcc882a31ed8f8f81`
- Held-out ciphertext SHA-256:
  `97049f5b3de49d6fd6c3d404ea2db0783d7b7f0396dbe77f8fc6d679ee00968b`
- Held-out case count: 12

No held-out plaintext directory exists in the frozen corpus. The normal test
path verifies the corpus and ciphertext commitments without the external key;
the optional keyed path verifies authenticated decryption and every original
held-out requirement hash without printing requirement content.

## Local evidence

- forced V3 quarantine validation: pass;
- V3 corpus freeze and checksum reproduction: pass;
- keyed held-out source authentication, decryption, and hash binding: pass;
- authenticated-encryption determinism, opacity, and tamper rejection: pass;
- capability-feedback suite excluding the intentionally absent final-artifact
  sentinel: pass; and
- complete closed-loop-open-set specification contract suite: pass.

No discovery or held-out synthesis has run. Phase 2 may now execute the frozen
discovery baseline and selection; held-out source and outcomes remain sealed.
