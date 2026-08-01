# Untouched Baseline Gap Audit

## Boundary

The corpus and this baseline were frozen against commit
`346fcdd4ffb5c99aa3e3945c76110a0652722428` before any production synthesis
change. The manifest SHA-256 is
`fd23189af2ef0471d4e69f0b4693b3bb05a10081a7d7ed15964ae6f98df08f86`.
All ten cases bypassed an external provider and reproduced byte-identical
synthesis results on the second run.

## Result

The untouched engine passes none of the six ordinary designs. It fails closed
for all six ordinary and all four adversarial requirements, and it produces no
physically promotable result. This is the expected baseline, not acceptance.

## Reusable Gaps

| Gap | Evidence | Generic remediation boundary |
| --- | --- | --- |
| Complete regulated-power graph generation | The regulator generated 50,000 graphs but no complete graph; 7,272 were dominated. | Express reusable feedback, reference, pass-device, compensation, and protection obligations and relationships. Do not add a regulator case selector or named regulator block. |
| Trusted measurement coverage | Both transimpedance candidates report `metric_unsupported`; other cases report `simulation_invalid`. | Define analysis/metric contracts from declared excitation, observation, and units, with explicit unsupported fail-closed behavior. |
| Rating and value-domain closure | Transimpedance, precision transfer, and midband transfer reject rating envelopes; midband also empties value domains. | Derive component stress and value intervals from domain, port, load, frequency, gain, and safety bounds before enumeration. |
| Search diversity and dominance | Window, regulator, midband, and unsafe-power searches discard hundreds or thousands of dominated graphs. | Make dominance compare behaviorally relevant obligations and retain structurally distinct feasible partial graphs deterministically. |
| Convergence and dynamic repair | Window and current-driver candidates are nonconvergent; midband and instability cases diagnose instability. | Add diagnosis-driven, bounded generic repairs for bias/reference access, feedback polarity, compensation, and reactive loading. |
| Safety rejection attribution | SOA, dissipation, and invalid-bias cases fail closed but often surface `simulation_invalid` or ordinary assertion failures instead of the intended safety class. | Evaluate bias feasibility, dissipation, junction temperature, and SOA envelopes before promotion and emit stable safety-specific diagnoses. |
| Physical closure | No case reaches physical-ready lowering. | Defer placement, routing, writer, ERC, DRC, and round-trip changes until an electrically passing graph exists; then derive them only from graph and geometry facts. |

## Implementation Order

1. Close graph obligations and relationships sufficiently to produce complete,
   structurally diverse candidates.
2. Close analytic value/rating envelopes and trusted measurement contracts.
3. Add bounded diagnosis-to-repair operators and safety-specific rejection.
4. Promote electrically passing graphs through generic physical lowering.

Each change must be justified by a failure category above, apply to all
requirements with the same graph or measurement facts, and preserve all prior
promotion evidence.
