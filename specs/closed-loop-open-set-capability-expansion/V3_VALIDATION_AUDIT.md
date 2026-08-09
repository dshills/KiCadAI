# Closed-Loop Open-Set Capability Expansion V3 Validation Audit

Status: blind validation failed; V3 closed fail-closed; held-out set consumed

Date: 2026-08-09

## Frozen experiment

V3 used the committed, independently authored 24-case behavior-only corpus,
the discovery-only rank-one selection, the authenticated encrypted held-out
baseline, and the Prism-reviewed implementation seal. The final run did not
change the frozen corpus, policies, catalog, model registry, work ceilings,
baseline evidence, ranking, or acceptance gates.

- corpus commit:
  [`b222db5aa36c00e0f3bf60a5d1768d02062d2fd7`](https://github.com/dshills/KiCadAI/commit/b222db5aa36c00e0f3bf60a5d1768d02062d2fd7)
- discovery baseline and selection commit:
  [`99d1c33a9ad6a2967a110935ff69999f81193835`](https://github.com/dshills/KiCadAI/commit/99d1c33a9ad6a2967a110935ff69999f81193835)
- held-out baseline seal commit:
  [`8f59d3bccb1aba0c47b5b0b21e516d2a0fad0ce3`](https://github.com/dshills/KiCadAI/commit/8f59d3bccb1aba0c47b5b0b21e516d2a0fad0ce3)
- reviewed implementation commit:
  [`361aebff9c16f741886335c39aa6fd4cc86d0fb7`](https://github.com/dshills/KiCadAI/commit/361aebff9c16f741886335c39aa6fd4cc86d0fb7)
- selected capability: `dc_operating_point_solver`
- [`V3_SELECTION.json`](./V3_SELECTION.json) hash:
  `82798387cb85e6af912a1f1ff135c1a4e21d4f76251f2f9e56985732584d904c`
- [`V3_REVIEWED_IMPLEMENTATION.json`](./V3_REVIEWED_IMPLEMENTATION.json)
  hash:
  `29162ccc1272f77a87c251e5c405f451f5a4d3bfddc1d44746fea11740cb5882`

The implementation was sealed before the final run. Prism reported no
actionable correctness finding on that exact boundary; its remaining comments
were low-severity performance observations on bounded graph/domain
collections.

## One-time blind result

Discovery ran to completion before any held-out key was opened. It proved the
required strict discovery and selected-cluster uplift, permitting the one-time
held-out execution. The held-out source and baseline were then decrypted only
inside the automated final verifier. No held-out requirement content,
identity, per-case result, gap, diagnostic, or circuit family is recorded in
this audit.

| Contract evidence | Baseline | Final | Required result | Verdict |
| --- | ---: | ---: | --- | --- |
| Discovery passes | 0 | 1 | strict increase | pass |
| Held-out passes | 0 | 3 | strict increase | pass |
| Rank-one-affected corpus passes | 0 | 4 | strict increase | pass |
| Baseline pass regression | none | none | none | pass |
| Unsafe evidence converted to pass | none | none | none | pass |
| Remaining nonselected gap identities | frozen | changed | exact identity match | fail |

The final verifier failed closed with `RemainingGapsStable: false`. That flag
means at least one case that remained outside `pass` did not preserve the exact
set of nonselected gap identities required by the frozen V3 comparison
contract. The aggregate result does not identify the role, case, circuit,
stage transition, or replacement gap, and those held-out details must not be
inspected to continue development.

The pass-count improvements do not override the failed preservation gate.
Section 7 of [`V3_SPEC_ADDENDUM.md`](./V3_SPEC_ADDENDUM.md) requires all listed
conditions, including stable remaining gaps, so V3 completion is not claimed.

## Artifact and safety behavior

The verifier evaluated the full corpus and reached its strict comparison only
after the synthesis and promotion paths completed. Because the complete
contract failed, it wrote no `V3_FINAL_REPORT.json`,
`V3_FINAL_COMPARISON.json`, `V3_PROMOTION_MATRIX.json`, final held-out
ciphertext, or final per-case evidence directory. Their absence is expected
and confirms that no partial success artifact escaped the validation gate.

Before the blind run, the sealed implementation passed these local gates:

- the broad `internal/opentopologysynthesis` package in 386.011 seconds,
  excluding only the documented historical
  `TestWindowedHeatingPowerPassesElectricalAndSafetyCorners` test;
- `internal/capabilityfeedback`, excluding only the intentionally absent V1
  final-artifact sentinel;
- the V3 specification package and `internal/simmodel`;
- the installed-KiCad `usb_c_led_indicator_protected` fixture; and
- the installed-KiCad `usb_c_i2c_sensor_3v3_protected` fixture.

Both installed-KiCad fixtures passed their required ERC, strict DRC,
connectivity, route-completion, writer-correctness, deterministic replay, and
zero round-trip-difference gates. GitHub Actions were not manually started or
inspected.

The excluded historical heating-power test is an explicit local coverage
limitation, not a pass, waiver, or evidence that its safety corners were
validated by V3. The known failure is documented in the
[`behavioral-contract-feasibility-realizability` audit](../behavioral-contract-feasibility-realizability/AUDIT.md):
the legacy path synthesized one candidate with zero required active current
and exhausted generic repair after 551.73 seconds. That path was outside the
selected V3 capability and was not modified here. The protected KiCad fixture
results and the other local gates remain valid evidence, but they do not
replace the omitted test. Risk is contained by the fail-closed V3 verdict and
the absence of final artifacts. Before a V4 implementation seal is eligible,
the legacy active-current/repair failure must be diagnosed under a separate
public test goal, corrected generically, and the unfiltered test must pass
locally.

## Trust and release decision

The V3 held-out set has now served its one permitted validation use. It is
retired and may not be rerun, tuned against, inspected per case, copied into a
new corpus, or reused as blind evidence. No corpus mutation, budget change,
gate relaxation, comparison change, or implementation adjustment may convert
this V3 result into a pass.

The reviewed generic implementation remains committed because it passed the
public discovery uplift and the full local safety/regression boundary, but the
V3 milestone remains incomplete. This commit is implementation evidence, not
a claim that the V3 open-set experiment passed.

## Required continuation

Any continuation must be a new V4 experiment with a freshly and independently
authored discovery/held-out corpus. Exact nonselected gap identity remains the
default V4 preservation rule. A different monotonic rule is permitted only if
a standalone public protocol first defines a deterministic partial order over
gap stage, scope, capability, code, and required evidence; proves with
hand-checkable boundary and adversarial tests that the order cannot hide a
regression; and is reviewed and committed before any V4 corpus authoring.
Absent that prerequisite, V4 must use exact identity. No choice or ordering may
be derived from V3 held-out details.

V4 must retain the same blind boundary, independent authorship, unchanged
safety and physical-promotion gates, implementation seal before reveal, and
one-time fail-closed held-out execution.
