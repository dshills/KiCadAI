# Independent feedback-binding experiment

This follow-up was discovered after the immutable root-cause publication at
`8e5f0bc5`. It does not alter its traces, measurements, or historical V21 evidence.

The new hand-authored fixture uses a 12 V supply, a fixed 0.75 V input, a
12.5 kohm load, and two requirements: output noise at most 10 microvolts RMS
over the declared 1 kHz noise range, and DC output between 0.74 and 0.76 V.
Both signal inputs of a catalog-backed amplifier initially share the input node.
It uses only the ordinary catalog/model loader; no held-out requirement is read.

The initial candidate fails. Changing exactly its inverting control input to
the existing output node makes the unchanged V20-admitted electrical evaluator
pass both requirements. However, the frozen V21 structural evaluator rejects
that candidate with `graph.causal_cycle`: it lacks typed feedback evidence.
That initially prevented the control-rebinding prototype from returning the
electrically passing graph. The V22-only feedback certificate now explains the
direct connection without changing the frozen V21 check. The single-monitor
simulation and certificate test passes; this is not yet a corpus advancement.

Source-confirmed details:

- `AnalyzeTopologyV21` always creates `CausalInvariantContextV19{}` and invokes
  `validateCausalCycles`. It has no feedback-evidence input.
- `validateCausalCycles` requires one validated typed feedback path per cycle.
- The historical typed-feedback validator restricts the obligation to a
  feedback-sensitive metric-string classification. Plain `output_noise_rms`
  and `output_voltage` do not meet that classification.
- The older `RedirectPrimitiveTerminal` also forbids attaching any two
  terminals of an instance to the same node. The role-based V19 operation does
  not have that blanket restriction, but still needs its typed invariant context.

The implementation adds successor-only, operation-bound feedback evidence
derived from terminal roles, actual graph cycles, the affected observation cone,
and exact admitted model/solver evidence. It supports direct feedback without
inventing a passive component solely to satisfy a graph rule. Other structural
and domain invariants still apply; bindings and final numerical evidence are
authenticated. No sealed V21 validator was edited and no error is ignored by
message matching.

## Compound continuation and admission identity

A second independent fixture has two output monitors. Both initially lack
feedback. A depth-bounded numerical beam now finds a two-edit repair, retaining
three passing assertion/corner reports and an exact replayable operation chain.
The continuation is integrated after a complete V21 certificate and lowers the
passing graph through the existing physical-generation path. Replay, cancellation,
count limits, prior-pass preservation, critical-failure refusal, and rehashed
path tampering have focused regressions.

This fixture exposed a separate provenance defect in the frozen V20 evaluator:
its admission map is keyed by analysis, operating case, and corner, but not the
assertion ID. Different assertions sharing those three fields overwrite one
another's admission identity. The numerical results can pass while their exact
assertion-specific admission evidence fails verification. The V22-only evaluator
adds assertion identity to the key. It uses the unchanged numerical engine and
does not change historical V20 code, model selection, requirements, or bounds.

## Explicit remaining comparator boundary

An independent open-collector indication fixture is currently refused before
repair. Its catalog-backed resistor omits electrical pin roles, so the frozen
structural checks cannot authenticate a passive pull path or the separate
comparison input's physical reference. The ordinary and V18 extension catalogs
do not provide the missing declarations for that part. This is retained as a
negative regression, **not** counted as a repaired comparator or cross-class pass.
Inferring missing roles from terminal names or adding a special-case part rule
would not meet this milestone's generic boundary.

The independent single and dual monitors have now passed the installed-KiCad
promotion test in two clean roots apiece, including numerical admission,
physical lowering, ERC, strict DRC, routing, connectivity, writer correctness,
zero round-trip differences, and identical project replay. Their project hashes
were respectively `70e8442b8cc96472fe6d5a3abde382e2dafb3ec258c01c4c383adb131b3652b6`
and `207ae84c8d11a306e0c8b81355e0e81201bf843f919181500aafcc06f72a881a`.
This is development-check evidence from an uncommitted implementation, not a
frozen successor-corpus result or a new supported v1 capability.

The reviewed implementation and its local coverage, race, negative/provenance,
preservation and Prism evidence are recorded in `V22_VALIDATION.md` and
`V22_REVIEW.md`. The exact public population, source/environment seals and
execution limits must still be committed before the 24-case/two-replay
evaluation. No new full corpus pass has been claimed.
