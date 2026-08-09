# Closed-Loop Open-Set V3 Baseline Audit

Status: discovery baseline and rank-one selection complete; held-out baseline authenticated and sealed

## Bound inputs

- Outcome-changing production start:
  `859c8df068db8254b715042b691c441a0d135fab`
- Corpus-freeze commit:
  `b222db5aa36c00e0f3bf60a5d1768d02062d2fd7`
- Corpus manifest SHA-256:
  `f721b8abc859a47030d17f92ea86dc301a3d3817b1cc52bd32261b54bb79c49e`
- Capability-feedback policy:
  `closed-loop-capability-policy-v2-realizability`
- Impact-registry SHA-256:
  `64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377`
- Synthesis-policy SHA-256:
  `4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4`
- Frozen environment: Go 1.23 minimum, KiCad 10.0.3, Darwin arm64

## Discovery execution

All 12 discovery requirements ran in manifest order. Each requirement was
strictly decoded and synthesized twice from the same frozen inputs. The two
normalized synthesis results were byte-identical for every case.

Baseline outcomes:

- pass: 0;
- unsupported: 10;
- unsafe: 0; and
- exhausted: 2.

No baseline case reached passing synthesis, so installed-KiCad physical
promotion was not applicable. The harness requires any future passing
synthesis to pass the promotion lane's two clean-root replays before it can be
observed as a pass.

The discovery evidence directory, report, checksum, complete deterministic
cluster ranking, rank-one selection, and expansion plan were written only
after all discovery runs completed.

## Sealed rank-one selection

The discovery-only evaluator selected:

- capability: `dc_operating_point_solver`;
- outcome/stage/scope: `unsupported` / `simulation` / `simulation`;
- terminal code: `SIMULATION_INVALID`;
- affected discovery cases: 7;
- affected reporting domains: 5;
- affected analysis kinds: 6; and
- safety score: 14.

Required evidence is trusted deterministic DC operating-point analysis,
convergence, corner, and assertion evidence. The sealed expansion plan permits
only that generic capability and inseparable prerequisites; it does not permit
case identities, fixture families, expected outcomes, or held-out-derived
behavior in production code.

- Discovery baseline report content hash:
  `2f4ce67e718f69371000f6a551c93dcd88d73ed611dab7f1b0208570d38104ef`
- Rank-one selection content hash:
  `82798387cb85e6af912a1f1ff135c1a4e21d4f76251f2f9e56985732584d904c`
- Expansion plan content hash:
  `726b58657399cde0abf0e2e56f6fb7bcf0b2fb81fc86465de0b67a34be6883c3`

## Blind-boundary statement

No held-out requirement was decrypted, classified, synthesized, observed, or
used in clustering, ranking, or planning before the selection was committed.
No held-out result altered the selection.

After selection commit `99d1c33a9ad6a2967a110935ff69999f81193835`, the
sealed harness authenticated and decrypted the held-out source internally,
executed all 12 cases in manifest order through the same two-run synthesis,
policy-v2 observation, and held-out-only aggregation path, and suppressed
per-case requirements, diagnostics, outcomes, and logs. Any passing synthesis
was required to pass the physical promotion lane's two clean-root replays.

The complete normalized held-out baseline payload was authenticated-encrypted
under a fresh 32-byte key distinct from the source-corpus key and stored outside
the repository. The repository records only:

- case count: 12;
- algorithm: `AES-256-GCM/HMAC-SHA-256-payload-bound-nonce`;
- payload SHA-256:
  `8d5ac890b06b5fb4529848ce61c6ad8463fc36289a169699ab7a52d9291074aa`;
- ciphertext SHA-256:
  `d30b689cb044bcffff7b6daa38703c4d005f1f5030350c766bfc67fa133735aa`;
  and
- seal content hash:
  `851b30821128f6c70702ffabc4f5cfbf21e95f8f90e6c7affbd13d5b817cd867`.

A keyed mechanical check authenticated and decoded the encrypted payload,
reproduced its content hash, verified 12 policy-v2 held-out evidence records,
and confirmed the held-out aggregate exposes no rankable clusters. It printed
no case content or outcome. Normal tests reproduce all public commitments
without either external key.

Phase 4 may now implement only the already sealed
`dc_operating_point_solver` capability. The held-out source and baseline
contents remain unavailable to the implementation context until the reviewed
production diff is sealed and discovery uplift passes.
