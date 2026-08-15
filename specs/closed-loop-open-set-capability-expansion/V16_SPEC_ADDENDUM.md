# Closed-Loop Open-Set Capability Expansion V16 Addendum

V16 starts only from the permanent, unopened V15 generation-zero retirement.
It reuses the authenticated immutable V10 corpus and the frozen observation,
promotion, and gate semantics. V15 partial checkpoints are retirement evidence
only and may not be resumed or used for capability selection.

V16 corrects the remaining production resource defect generically. V15 already
streams large repair-result hashes, but top-level synthesis finalization still
encoded the complete synthesis run through `json.Marshal` before hashing it.
V16 streams the same canonical JSON bytes directly into SHA-256 for top-level
synthesis finalization as well. The returned run, hash, selection, ranking,
reports, attempts, budgets, and replay bytes remain identical. Encoding errors
fail closed. No circuit identity, coordinate, role, domain, outcome, or
held-out information may influence hashing.

V16 preserves every V15 search, value-trial, repair, ranking,
exactly-one-case-worker, exactly-one-live-replay, and post-replay memory-release
rule. All other fail-closed boundaries remain unchanged. A public baseline is
eligible only after all 24 discovery cases complete exactly two identical
replays and all required evidence gates. The discovery evaluator has no
held-out key access surface.
