# V22 bounded electrical continuation

Status: reviewed implementation with independent and local preservation checks;
not a frozen public evaluation. `V22_VALIDATION.md` records the completed gates.
The committed root-cause report remains the selection authority.

## Scope

Continue from an exact V21-complete graph and its retained failed electrical
evaluation. Generate only catalog-declared input-terminal rebindings to existing
compatible graph nodes. No components, values, supply/output bindings, external
interfaces, requirement bounds, numerical models, or physical layout coordinates
are edited. Historical V18/V20/V21 entry points and v1 behavior remain unchanged.

Every intermediate graph must retain all structural obligations. Cycles require
explicit graph-bound feedback evidence identifying classified output and control
terminals, a direct or validated passive return path, and a required observation
in the affected causal cone. This permits numerical evaluation, not electrical
acceptance. Unknown electrical roles and incompatible domains fail closed.

## Deterministic bounds and search

The default independent count limits are depth 4, beam width 8, 4096 considered
nonidentity bindings, 128 candidate simulations, and 4096 corner evaluations.
Existing evaluator policy limits may further restrict simulations and corners.
These defaults are not tuned using successor-corpus results. The public protocol
must explicitly freeze them together with source and environment identities.

Enumeration normalizes graph ordering and sorts proposals by graph hash, including
on a work-limit exit. Every considered binding consumes work, even if invalid.
Admission refusal consumes an evaluation invocation. Full numerical consumption
is counted, including evaluator retries. States are ranked by passing assertion
count, numerical penalty, and graph hash. A neutral nonregressing state may stay
in the bounded beam so a repair can require multiple edits. No elapsed-time
cutoff determines a search outcome; cancellation is a distinct terminal state.

Every previously passing assertion/corner must still pass before a state can
continue. A newly observed later failure is recorded as unknown-to-failed, not
mistaken for loss of a previous pass. Critical failures cannot enter the beam.
The V19 predicate that regards a diagnosis shared by all prior candidates as
"universal" is not an impossibility proof for this electrical continuation:
even a single failed candidate satisfies it. This is not permission to bypass
admission, solver, structural, critical, or final numerical gates.

## Evidence and provenance

The certificate replays the exact sequence from the initial graph, checks each
intermediate invariant, rejects cycles in the repair-state sequence, and binds
all intermediate graph hashes. The final certificate authenticates every
required assertion/case/corner, exact numerical plan/report, actual and bound,
workflow model, admission decision, source/record/model-claim and parameter
digests. It makes no standalone physical-promotion claim.

V22 corrects admission identity locally: assertion ID is included with analysis,
case and corner. V20's historical map can otherwise alias two different
assertions. A regression checks that V22 changes provenance but preserves the
exact numerical plan, report, values and coverage. V20 source is not modified.

Rejected search trials retain compact graph/evaluation identities, decisions and
counts; only the selected evaluation retains full solver reports in the result.
The beam's live reports are bounded by its width. Historical synthesis evidence
is shared read-only when producing a successor, never modified. The V22 wrapper
binds predecessor, synthesis and electrical-repair hashes. Before continuing an
eligible predecessor it authenticates the complete source by a cancellation-aware
streaming hash; no additional multi-GB replay buffer is allocated. The wrapper's
own identity then hashes only the three identities, not another full replay.

## Independent checks and remaining work

The single monitor demonstrates direct negative feedback, with positive-feedback
alternatives rejected by unchanged numerical checks. The dual monitor requires
two edits and three complete assertions; one-edit search must remain exhausted.
Both pass installed-KiCad promotion and two-root project replay. The separate
comparator fixture remains a documented refusal because its passive terminal
role metadata cannot prove the required pull/reference paths.

Completed checks include certificate and path tampering, incompatible domains,
unknown roles, supply/output/value edit refusal, cancellation, deterministic
replay, resource limits and prior-pass/critical guards. Local bounded, coverage,
race, lint, external-review, educational and installed-KiCad preservation checks
passed; staged Prism findings are dispositioned in `V22_REVIEW.md`. The committed
public source/environment freeze, exact 24-case/two-replay evaluation, aggregate
comparison, clean-checkout final gates and final PR are still required. No historical
or held-out evaluation data or keys are accessed or changed by these fixtures.
