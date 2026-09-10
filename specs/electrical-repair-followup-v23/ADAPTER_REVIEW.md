# V23 admitted-execution adapter review

Base: `83259ecb17aeb7883ef291e0bb336186901ba9af`.
Prism reviewed the staged adapter through its authorized configured Gemini
provider. No corpus or installed-KiCad evaluation was performed in this checkpoint.

## Review disposition

First run: `cb87d80f1f885f5ba0cb318290ddb6c0`; raw JSON SHA-256
`5d5108b30c24e6c4d64dc4524f55d2a7ec7fc34adf529bd8e2579ef57f3ca25e`.

- `4e614b4e7fd8d2f6` and `d7c5745d584d5354` concern a hypothetical report with
  passing assertion values, nonpassing status, and no numerical diagnostics.
  The sealed MNA evaluator returns diagnostics for numerical/measurement failures
  and sets report status to pass when there are none. Worst-case failures also
  return diagnostics. The adapter handles those diagnostics before the flagged
  fallback. With a finite observation outside only an upper bound, its explicit
  upper-bound comparison selects `above_maximum`; normalization requires at least
  one bound. Added maximum-only/minimum-only pass and refusal regressions to
  verify this behavior. No historical diagnostic logic was changed.

During local review, identified and corrected a separate missing verification
check for internal worst-case corner coverage. `VerifyPassingCornersV23` now
reconstructs the unchanged nominal/exhaustive/directed schedule, checks exact
assignments and assertion identities/bounds, and rejects rehashed corner
omissions. This adds a verification gate; it does not change numerical execution,
corner selection, electrical bounds, or historical behavior. The second Prism
review includes this correction.

Second run: `e9d477c4cf4c6c9a13da1f2a03400f12`; raw JSON SHA-256
`ec2780443d259d8dac539b108db2b88802a4bcb9323c717b6273765f3cfd1c58`.

- `5bcd64abc57c6e81`: not a defect. The original requirement is normalized before
  constructing the ID-to-metric map. Metric translation does not change IDs;
  the next normalization preserves canonical IDs and only sorts by them. Map
  lookup is not positional. Added a reordered, whitespace-ID, `dc_voltage`
  regression: explicitly normalized and raw inputs produce identical complete
  V23 evidence, restore the original metric, and verify successfully.
- `c8201065d7442263`: not a safe shared-data optimization. Each attempt has its
  own assertion/case/corner admission identity and potentially different model
  evidence. Sorting and compacting each bounded list preserves exact provenance.
  No profile establishes this as a bottleneck, and the frozen scope excludes
  unmotivated performance changes.

No unresolved valid findings remain.

## Local validation

- Complete short suites for `internal/opentopologysynthesis` (438.193 s) and
  `internal/simmodel`, plus V23, historical source/dependency, and V20 contract
  audits: pass. This broad run preceded the final additive corner guard/tests;
  the affected checks below were rerun after those changes.
- Final focused race tests for every `TestElectricalV23*` and
  `TestPassingCornersV23*`: pass (9.270 s topology; 1.294 s simulation).
- Complete simulation suite after the corner guard: pass (0.459 s).
- V21/post-topology, frozen V22 evaluation, and V22 publication audits: pass.
- Scoped lint of both production packages after the corner guard: zero issues.
- V23 diagnostic, publication, correction, solver-layer, and adapter checksums:
  pass. Historical production, V22 artifacts, and module dependencies unchanged.

Repair search, physical promotion, the frozen 24-case/two-replay successor run,
full milestone release/preservation gates, and the follow-up PR remain pending.
Independent numerical/certificate regression passes are not additional corpus
simulation-and-installed-KiCad passes and do not complete the active goal.
