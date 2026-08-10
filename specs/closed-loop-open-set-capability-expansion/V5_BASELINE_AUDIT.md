# Closed-Loop Open-Set V5 Baseline Audit

Status: discovery and rank-one package selection frozen; isolated held-out
baseline evaluated and sealed; selected-package implementation may begin

## Immutable lineage

All commit IDs in this audit refer to the
[`dshills/KiCadAI`](https://github.com/dshills/KiCadAI) repository.

- V5 starting commit:
  `d8e98b4dee3212823525c5955e8e025bd0039d03`
- contract freeze commit:
  `a9249879d5e02575fe047925d613458ffec62030`
- corpus freeze commit:
  `82f0b7ce6b704fd3c7ca832f8ad0b194c0e38f8b`
- package-ranking infrastructure commit:
  `3cf8e3c8df4ea6f2b4898dfd79871d2ca1590314`
- discovery runner parent commit:
  `f61476915f841215b1c5f35d1a3c73345cffbdb1`
- selection freeze commit:
  `ffcff3881ca2e03a454fd350124637664bf4d4e4`
- blind-baseline publisher infrastructure commit:
  `1aa43c067167113c4508175da0e46b350bc8a980`
- artifact-free held-out baseline harness commit:
  `0b531385329ffc692c372c1c9c83d0914d247a6f`
- held-out baseline publisher parent commit:
  `71f2a0df2a832b3edd44c16c51c3136c614c11b9`

The V5 corpus manifest SHA-256 is
`d703608d09d7d7bd834bb45698446dd03bb0dbe7b00733b636dd73250cac3f6d`.
It freezes 36 independently authored behavior-only cases: 18 discovery and 18
encrypted held-out cases, balanced across the six reporting domains and three
isolated author slots.

## Discovery baseline

All 18 discovery cases ran twice under the frozen inventory, catalog, model
registry, synthesis environment, evaluator, and policy ceilings. Canonical
synthesis JSON was byte-identical between both runs for every case.
Installed-KiCad promotion was required twice for any pass; no baseline case
reached pass, so no physical promotion was applicable.

`opentopologysynthesis.Synthesize` accepts and consumes no stochastic seed or
random source. Each replay runs sequentially with the same immutable decoded
requirement, deterministically ordered inventory, catalog and model registry,
fixed policy ceilings, canonical search ordering, and identical simulation
environment. The harness serializes every exported synthesis field and
requires exact byte equality. Any ordering, concurrency, floating-point,
environment, or other exported volatility therefore fails the baseline rather
than being normalized away.

The byte-equality claim is scoped to the two sequential replays inside one
bound baseline environment; it is not an unsupported claim that arbitrary CPU
architectures produce identical floating-point bytes. The sealed manifest
records the exact promotion platform and toolchain identity. A different
platform, executable, library tree, table, catalog, model registry, or policy
produces a different commitment and cannot silently reproduce or compare
against this baseline.

The frozen public outcomes are:

| Outcome | Count |
| --- | ---: |
| pass | 0 |
| unsupported | 4 |
| unsafe | 0 |
| exhausted | 14 |

`exhausted` means the deterministic synthesis search reached one or more
frozen state, graph, value-trial, simulation, corner, repair, or related policy
ceilings without producing a pass and without a terminal unsupported or unsafe
classification. It is a bounded fail-closed result, not a timeout retry signal.

The discovery baseline hash is
`2c12965bbe54a44408e963a0e4b732120f4838f69bb5a87ec658d2c00efcbd1b`.
The complete ranking hash is
`8cc41daa115ef4bbf79f6c90423d51a669392244bf87e3c428e6923aec3db406`.
Four capability packages passed the frozen eligibility policy.
Package eligibility does not mean a circuit passed. It means the package's
typed root gaps affected at least two failed discovery cases across at least
two reporting domains, making the missing capability reusable enough to rank.

## Frozen rank-one package

The selected generic package is:

- scope: `simulation`
- capability: `electrothermal_solver`
- exact root member `(stage, scope, capability, code)`:
  `(simulation, simulation, electrothermal_solver, SIMULATION_INVALID)`
- affected discovery cases: 3
- reporting domains: analog and power
- required evidence: trusted deterministic electrothermal analysis,
  convergence, corner, and assertion evidence

`SIMULATION_INVALID` is the canonical terminal diagnostic code emitted by the
frozen synthesis evaluator for this exact typed root gap. The word `INVALID`
describes the failed simulation result; it is not an uninitialized value,
placeholder, wildcard, or editable label.
The first two tuple values are intentionally equal: this gap is emitted during
the `simulation` stage and belongs to the `simulation` capability scope.

The selection hash is
`f9083b9e138718761c42d3feacfa42446253bbd2659748d94a77862402c9967d`.
The executable generic-plan hash is
`84363b328225915c60c7cd9ca106a48046247398c1fe4062387bd30b041236f9`.
No held-out source, outcome, gap, diagnostic, membership, or promotion detail
entered package eligibility, ranking, or selection.

## Held-out baseline seal

The committed harness independently enforces:

- an exact clean Git publisher-parent commit derived inside the custodian run;
- the frozen contract, corpus, discovery baseline, ranking, selection, generic
  plan, implementation boundary, evaluator, and policy commitments;
- the exact discovery inventory, catalog, model registry, and synthesis-policy
  hashes for all held-out runs;
- the committed KiCad promotion lock, platform, KiCad version, resolved
  `kicad-cli` executable SHA-256, symbol/footprint table SHA-256 values, and
  complete symbol/footprint tree identities and counts;
- 18 manifest-ordered held-out identities and strict requirement decoding;
- two byte-identical synthesis runs per case;
- two clean-root installed-KiCad promotions for every synthesis pass;
- distinct source and baseline key paths outside the repository;
- a fresh exclusive 32-byte AES-256-GCM baseline key stored with Unix
  filesystem permissions `0600` (`rw-------`);
- one fresh 96-bit nonce from `crypto/rand`, prefixed to the ciphertext and
  covered by the complete-ciphertext SHA-256 commitment;
- authenticated length-delimited metadata, canonical public artifacts,
  checksums, and atomic no-replace publication; and
- outcome-silent failures and completion logging.

The authorized external baseline-key path is the persistence boundary. After
successful publication the key remains at that path as a regular 32-byte file
owned by the custodian with Unix filesystem permissions `0600` (`rw-------`);
it is never copied into the repository,
artifacts, logs, or review input. The one-time blind-final custodian must
revalidate its path, type, permissions, size, and distinction from the source
and final keys before use. Publication failure before the artifact commit
removes only a key created by that failed attempt. Successful publication does
not delete or replace the key. Loss or corruption of the persisted key retires
V5 fail-closed; regeneration, retry, or substitution is prohibited.

The nonce is never caller-selected or reused: a new exclusive baseline key is
created first and the publisher reads the nonce directly from `crypto/rand` for
its single seal operation. Its byte count is recorded in the self-hashed
manifest; the nonce-prefixed sealed bytes, payload hash, algorithm identifier,
and length-delimited policy, selection, and environment metadata are verified
before decryption. GCM authentication and the independent ciphertext
commitment both fail on nonce or ciphertext mutation.

The public sealed state is:

- the external held-out source key exists as a 32-byte file with Unix
  filesystem permissions `0600` (`rw-------`);
- the source key has not been read by the implementation context;
- the distinct external held-out baseline key exists as a 32-byte file with
  Unix filesystem permissions `0600` (`rw-------`); and
- the repository contains exactly the canonical four-file encrypted baseline
  artifact set.

The authorized isolated custodian evaluated all 18 manifest-ordered held-out
cases under the frozen selection and atomically published only authenticated
ciphertext and non-revealing public commitments. The held-out manifest
self-hash is
`f2fe78f48a3300d3af296b3432ecd5d5943a3630460f678f83e1d0736c89431c`.
No requirement, outcome, gap, diagnostic, package membership, or promotion
detail was disclosed to the implementation context.

The outcome-neutral installed-toolchain preflight resolves the committed
`toolchain/kicad-promotion.lock.json` on `darwin/arm64`, requires KiCad `10.0.3`,
and successfully hashes the actual executable, both library tables, and both
complete library trees. The updater resolves that evidence once before source
decryption and passes only those exact paths into every physical promotion.
It resolves and hashes the environment again after all held-out runs and
refuses publication if any content identity changed during evaluation.
The non-path content identities are included in the authenticated public
baseline binding, so verification does not depend only on a repository commit,
ambient `PATH`, or mutable global KiCad configuration.
The `darwin/arm64` value authenticates the platform on which this one-time
baseline was evaluated; it is not an ambient-platform requirement for public
seal verification. Another platform may verify the canonical artifact bytes,
but it may not replay or replace this consumed baseline. A future independent
evaluation must bind and publish its own locked toolchain identity.

The manifest `hash` is its canonical semantic self-hash, computed with that
field omitted. The `manifest.json` entry in `CHECKSUMS.sha256` is deliberately
different because it hashes the complete serialized file, including the
populated self-hash. The public verifier checks both commitments.

Normal local tests exercise only the public freeze verifier and skip the gated
updater. The complete local Go test suite, `go vet`, and golangci-lint pass at
the harness commit. Prism reported no high- or medium-severity issue in the
exact staged harness bytes; its remaining low observation concerned the
intentional exact-byte replay comparison required by the frozen protocol.

## Next phase

The one-time held-out baseline boundary is complete and may not be rerun. The
implementation context may now implement only the frozen rank-one generic
`electrothermal_solver` capability package. Held-out baseline ciphertext and
keys remain sealed until the separately authorized one-time final comparison.
