# V10 Production Capability Selection Runner

Status: production runner implemented and synthetic tests passing. No real V10
frontier, outcome, effect plan, selection, or held-out material was loaded
while preparing this freeze.

`kicadai-capability-rank-v10` is the production bridge from an authenticated
24-case public generation-zero baseline to the frozen V10 complete effect-
exposure ranking engine. It has no source-key, baseline-key, held-out,
decryption, synthesis, simulation, or implementation mutation surface.

The runner requires a clean committed repository, an independently verified
public baseline publication, a canonical mechanically evidenced effect-plan
set, and a fresh output path. The plan set is bound to the exact public
baseline manifest and report plus the already-frozen effect-exposure engine.
Unknown fields, trailing JSON, noncanonical JSON, malformed hashes, stale
source evidence, path escapes, symlinks, duplicate plans, incomplete
mechanical evidence, unbounded lookup, unmapped consumers, and replace
attempts fail closed.

Each executable generic effect plan identifies direct typed gap atoms and
exact members, bounded closure atoms and members, and the complete planned
member set. Its static evidence binds the exact pre-implementation production
and verification file bytes, reverse call graph, focused non-corpus runtime
consumers, and any registry, configuration, catalog/model, or data references.
The plan hash is derived from the complete canonical plan bytes; a boolean
`mechanically_proven` claim alone is insufficient.

Only unsupported and exhausted public cases enter the active cohort. Passes
and unsafe cases receive no unlock credit. The frozen engine enumerates the
complete bounded active-case closure and ranks generic capabilities by fully
covered cases, reporting-domain diversity, circuit-role diversity, and safety
weight, then by exposed noncovered cases, nonselected sibling burden, atom
count, and exact-member count. It publishes the complete eligible list, full
semantic co-rank-one set, and deterministic canonical selection.

The ranking commits the authenticated baseline, exact plan-set bytes, frozen
effect-exposure engine, generation-zero state, full candidate evidence, and
selected capability. Output installation uses a same-directory temporary
file and atomic no-replace link.

Example invocation after public baseline publication and review of a frozen
effect-plan set:

```text
go run ./cmd/kicadai-capability-rank-v10 \
  --repository-root . \
  --baseline-root internal/capabilityfeedback/testdata/closed_loop_open_set_v10_baseline \
  --plans specs/closed-loop-open-set-capability-expansion/V10_GENERATION_ZERO_EFFECT_PLANS.json \
  --output /private/tmp/kicadai-v10-generation-zero-ranking.json
```
