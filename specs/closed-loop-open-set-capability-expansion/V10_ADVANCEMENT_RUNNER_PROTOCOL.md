# V10 Production Public Advancement Runner

Status: production advancement runner implemented with synthetic pass-uplift,
regression, source-confinement, and fail-closed tests. No real V10 outcome,
selection, implementation, successor report, or held-out material was loaded
while preparing this freeze.

`kicadai-capability-advance-v10` is the production before/after gate for one
selected V10 generic capability. It consumes only an authenticated public
generation-zero baseline, the frozen public ranking and effect-plan bytes, a
post-implementation public evaluator report, and a reviewed implementation-
seal request. It has no source-key, baseline-key, held-out, decryption, corpus
authoring, synthesis, or implementation-mutation surface.

The implementation seal requires a clean repository at the stated
implementation commit with the pre-implementation commit as an ancestor. The
complete Git path delta must equal the sorted transition list. Every changed
path must have been statically mapped by the selected effect plan as a
production or verification file; its base-commit bytes must match the plan's
pre-change hash and its current regular-file bytes must match a distinct
post-change hash. Unmapped, deleted, renamed, dirty, stale, missing, oversized,
or symlinked files fail closed. The seal also commits focused tests, complete
local regressions, installed-KiCad checks, Prism completion, and absence of
fixture-specific content.

The successor report must independently satisfy the same V10 24-case,
two-replay, promotion, fourteen-gate, deterministic, and fail-closed evidence
contract as generation zero. Corpus, environment, and evaluator commitments
must be unchanged. The frozen causal-round engine rederives the complete
selected exposure and then requires:

- no baseline pass regression and no unsafe-to-pass transition;
- exact byte preservation of every non-exposed case and nonselected sibling;
- selected paths removed only by obligation satisfaction or bounded,
  append-only, nonweaker causal successors;
- advancement of at least two active cases across at least two reporting
  domains and two circuit roles; and
- strict public pass uplift from the original active cohort for admission, or
  bounded diverse progress within the two-round budget for continuation.

The runner builds both the implementation seal and public round before any
write. It then atomically publishes exactly the canonical seal, round,
reproducible audit, and checksums to a fresh no-replace directory. Any binding,
environment, lineage, exposure, regression, budget, or publication failure
produces no admitted artifact.
