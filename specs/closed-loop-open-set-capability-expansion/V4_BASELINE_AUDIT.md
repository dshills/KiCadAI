# V4 Discovery Baseline and Rank-One Selection Audit

Status: discovery frozen; blind held-out baseline sealed

## Bound inputs

- corpus-freeze commit:
  `35675e05bff9599be4492edfbe38bccdfa7a594f`
- corpus-manifest SHA-256:
  `4c6bc5c8c94bccc16a066f69ac0a4c69c326e8f77aa8db02f9b9ef327ef732a6`
- evaluator policy: `closed-loop-capability-policy-v2-realizability`
- impact-registry hash:
  `64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377`
- synthesis-policy hash:
  `4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4`
- gap-transition-policy file hash:
  `ba73b2db190f48c70b31bc77b7689240df122f73b41e8b63624e540635139aa8`

The run used the frozen Go 1.23 / KiCad 10.0.3 Darwin arm64 environment and
the committed synthesis ceilings. Held-out source, outcomes, gaps, and
diagnostics did not participate.

## Execution

All 12 discovery requirements ran twice from isolated synthesis roots. The two
normalized synthesis runs were byte-identical for every case. The update mode
completed in 174.33 seconds and wrote evidence atomically only after all 24
synthesis executions and aggregation succeeded.

No discovery case reached synthesis `passed`, so no baseline installed-KiCad
promotion was applicable. The baseline classification is:

| Outcome | Count |
| --- | ---: |
| pass | 0 |
| unsupported | 2 |
| unsafe | 0 |
| exhausted | 10 |

The committed report reproduces byte-for-byte from the 12 per-case evidence
files, frozen impact registry, and policy-v2 evaluator. Held-out count rows are
all zero and are tested not to accept discovery leakage.

Consumption field `maximum_frontier` measures only queued heap/beam states.
Bounded direct relationship enumerators can expand states and generate complete
graphs without allocating a queue, so a zero maximum frontier alongside
nonzero expanded/generated counts is valid and does not indicate missing
instrumentation.

## Rank-one selection

Discovery-only deterministic ranking selected:

- stage: `topology_search`
- scope: `topology`
- capability: `complete_topology`
- code: `OPEN_TOPOLOGY_SEARCH_EXHAUSTED`
- affected discovery cases: 4
- affected reporting domains: 4 (`digital`, `mcu`, `mixed_signal`, `sensor`)
- analysis kinds: 7
- safety score: 14
- reuse score: 5

Required evidence is reviewed reusable topology construction plus complete-
graph evidence. The expansion plan requires a declarative provider and the
full simulation, connectivity, routing, writer, round-trip, installed-KiCad,
workflow, and deterministic-replay promotion gates.

This selection is a generic topology-completion capability, not a circuit
family, fixture, coordinate set, template, or allowlist. Its affected case IDs
are evidence metadata only and may not enter production logic or tests.

## Frozen artifacts

- discovery baseline report SHA-256:
  `8fef640be4a0750d5d52148d49cd354c9ae8620d9cf1fba81d5f9bf4f1ab292a`
- rank-one selection SHA-256:
  `bc11358c5cbde58b53464f50d35cb76273e0586f68135869d7fc354ae14bf12b`
- expansion-plan hash:
  `20624c4ed6204cfd5e00e3fc722da8e4992aa4996bf0e46f0d2d7f5d5643854d`
- selection content hash:
  `a82d7c26191e15b42692e5de9fb365947ae76915b683c07bd512b4bf96404a9b`

Update modes refuse overwrite, report/selection mutation changes their content
hashes, and normal tests reproduce committed bytes read-only. The held-out
baseline was sealed only after this discovery selection commit.

## Blind held-out baseline seal

The 12 held-out requirements were decrypted only inside the baseline test
process after the committed discovery selection reproduced byte-for-byte. Each
requirement ran twice through the frozen synthesis policy, inventory,
environment, replay comparison, promotion rule, policy-v2 observation, and
separate held-out aggregation path. The blind run completed in 213.39 seconds.

The runner emitted no requirement content, per-case progress, outcomes, gaps,
diagnostics, or aggregate classifications. This audit intentionally records no
held-out result distribution. No held-out plaintext artifact was written to
the repository.

The normalized baseline payload was encrypted with AES-256-GCM using a fresh
random nonce prefixed to the ciphertext and a new 256-bit key stored outside
the repository. Authenticated additional data binds the payload schema, corpus
manifest, discovery selection, evaluator policy, impact registry, synthesis
policy, gap-transition policy, and plaintext payload hash. The source and
baseline use separate external keys.

- selection commit:
  `40431aa4f563eda2db6556af5aba7d417593a759`
- held-out case count: 12
- payload hash:
  `392bb710267ba031599ab40718243c343b89a9866fb6db11fca234270117654d`
- ciphertext SHA-256:
  `72d90f6680a6b99af64c5c1bb00528903d0539552075efb581256cce097561b7`
- seal content hash:
  `48814e837f466b579c7bef0fccedf605b778ae6bf865b4a5d04141ddf37840c2`

Normal tests verify the public seal metadata, checksum, ciphertext hash, and
discovery-selection reproduction without a key. A key-gated test authenticates
and validates the encrypted payload contract without logging its content.
Updater mode refuses to overwrite the ciphertext, seal, or checksum.
