# Closed-Loop Open-Set Capability Expansion V15 Addendum

V15 starts only from the permanent, unopened V14 generation-zero retirement.
It reuses the authenticated immutable V10 corpus and the frozen observation,
promotion, and gate semantics. V14 partial checkpoints are retirement evidence
only and may not be resumed or used for capability selection.

V15 corrects the remaining production resource defect generically. Value
trials retain compact catalog selections, graphs are materialized on demand,
and failed evaluations retain only the deterministic best graph per topology
needed by the later repair phase. Topology-proposal sizing streams its ordered
value variants and stops at the existing per-proposal limit instead of
materializing unused whole-graph variants. Large repair results are hashed by
streaming the same canonical JSON bytes directly into SHA-256 instead of
allocating a complete encoded copy. No circuit identity, coordinate, role,
domain, outcome, or held-out information may influence retention.

V15 preserves V14's exactly-one-case-worker, exactly-one-live-replay, and
post-replay full garbage-collection rules. All other fail-closed boundaries
remain unchanged. A public baseline is eligible only after all 24 discovery
cases complete exactly two identical replays and all required evidence gates.
The discovery evaluator has no held-out key access surface.
