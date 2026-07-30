# Phase 8 Audit

Completed: 2026-07-30

Phase 8 corrects fuse clearing, electrothermal SOA evidence, and deterministic
tolerance reporting. The protection-state implementation is closed by commit
`84dbe783` (`Correct protection simulation state`).

## Disposition

| Work stream | Disposition | Evidence |
| --- | --- | --- |
| Fuse I²t physics | closed | Closed-state and clearing fuse primitives share one `integral(I² dt)` state transition. Above-rated samples accumulate full current-squared energy, exactly rated samples hold state, and below-rated samples end the deliberately bounded contiguous-pulse model. Exact-limit clearing uses one `>=` comparison. |
| Accepted-state transaction | closed | Coarse and predictor paths evaluate cloned state. Predictor evaluation starts from prior accepted surge history. One explicit commit operation advances accepted I²t state and opens cleared fuses only after the solved point is validated and recorded. Rejected trajectories cannot mutate accepted history or topology. |
| Electrothermal SOA | closed | The first unsafe accepted timestep contributes its full duration. SOA assertions require an explicit evaluated-evidence marker and fail closed when physical envelope evaluation is absent. |
| Tolerance determinism | closed | Assertion identity includes lower and upper bounds. Duplicate corner rows are removed deterministically before evaluation and reporting. |
| Analytic validation | closed | Constant-current pulse tests bound trip-time error by the timestep, prove non-growing error under refinement, cover at/below-rating behavior, prove split/unsplit equivalence, retain prior history, and verify the accepted-state/topology commit. |
| Promotion evidence contract | closed | Frozen promotion tests now expect `experimental` and fabrication-ineligible classification when required evidence is inferred, while continuing through downstream synthesis and workflow checks. Missing or inferred evidence cannot strengthen a capability claim. |

## Focused validation

The following local suites passed:

```sh
GOCACHE=/tmp/kicadai-go-cache go test ./internal/simmodel -count=1 -timeout=10m
GOCACHE=/tmp/kicadai-go-cache go test ./internal/amplifiers ./internal/blocks ./internal/closedloopsynthesis -count=1 -timeout=10m
GOCACHE=/tmp/kicadai-go-cache go test ./internal/compositionlowering -run 'TestDerived(ThermalUsesRatedPeriodicDrive|ElectrothermalUsesBoundedDynamicGrid)$' -count=1 -timeout=5m
GOCACHE=/tmp/kicadai-go-cache go test ./... -short -count=1 -timeout=20m
GOCACHE=/tmp/kicadai-go-cache make lint
```

## Installed-KiCad acceptance

KiCad CLI passed the optional design-example tier for:

- `class_a_bjt_line_preamplifier`;
- `class_ab_headphone_driver`;
- `class_ab_headphone_protected`; and
- `class_ab_speaker_10w_protected`.

The tier requires applicable fixtures to provide clean ERC, strict DRC,
connectivity, route completion, writer correctness, and zero normalized
round-trip differences.

## Frozen electrothermal corpus boundary

The six-case offline electrothermal corpus no longer stops at the stale
pre-Phase-7 expectation that rule-inferred component evidence is promoted. A
full local run reached the configured 15-minute bound while processing its
third case without a capability or simulation assertion failure. That run
timed out and is not counted as a complete corpus pass; its remaining cases
were not verified separately during Phase 8.

The deterministically sharded `class_ab_dynamic_output_stage` case completed
architecture selection, closed-loop lowering, and electrothermal simulation.
Its later board workflow failed closed after 27 of 30 nets routed; the dense
layout left `composition_net_013`, `composition_net_014`, and
`composition_net_028` incomplete. This is a physical-routing capability limit,
not an open Phase 8 protection-physics finding. Owner: `designworkflow`
routing. Safety disposition: fabrication remains blocked, so the incomplete
board cannot acquire promoted or fabrication-ready evidence.

Phase 8 closure therefore rests on the complete focused protection-model
suites and installed-KiCad amplifier fixtures above. The frozen corpus supplies
additional bounded characterization, not passing acceptance evidence.

## Prism review

Prism reviewed the staged implementation repeatedly as findings were
addressed. The review caused these material changes:

- melting thresholds were standardized at `>=`;
- the contiguous-pulse reset boundary and its solver-chatter limitation were
  documented;
- predictor history is held separately until the accepted-point transaction;
- fuse history and cleared-fuse topology commit through one explicit operation;
- an unused predictor workspace parameter was removed.

The final review's claimed undefined `template` is disproved by the declaration
immediately above its use and by the passing Go compile/test suites. Its
possible zero fuse resistance is excluded by both fuse primitive registry
schemas, which require `cold_resistance_ohm >= 1e-6`. No material Prism finding
remains open.
