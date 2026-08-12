# V8 Selection and Round Evaluator Protocol

The V8 evaluator is version-isolated from the retired V7 implementation and is
frozen before any V8 synthesis result is available. It consumes normalized
public discovery cases, frozen effect plans, and public round evidence only. It
has no dependency on corpus source paths, fixture identities, held-out data, or
production synthesis implementations.

## Exact identities and frontiers

Every active gap contains one 64-hex immutable obligation anchor and a nonempty
append-only path. Each path leaf has a canonical stage/category, scope,
capability, diagnostic code, and sorted unique nonempty evidence requirements.
Path hashes use ordered unsigned-32-bit-big-endian length-prefixing over the
anchor and each current/historical member plus its evidence. Unknown stages,
invalid categories, duplicate paths, empty frontiers for eligible cases,
frontiers on passes, unknown outcomes, or noncanonical lists fail closed.

## Complete selection closure

The selector enumerates every nonempty subset of the eligible discovery cohort
and deduplicates exact union frontiers. With 18 cases this is at most 262,143
unions, beneath the frozen 262,144 ceiling. Retained unions use deterministic
vocabulary-indexed bitsets so the complete closure has bounded representation.
Overflow fails rather than truncating.

Each exact direct union requires one predeclared effect plan. Plans with an
unbounded dynamic lookup, unmapped consumer, missing mechanical proof,
non-executable step, incomplete member set, malformed identity, or absent
evidence receive no eligibility credit. Closure atoms and members are
deduplicated with direct members and consume identical round/total budgets.
Every atom must own an exact member and every member must own an atom.

A candidate must completely cover at least two eligible cases, two frozen
reporting domains, and two frozen circuit roles; every direct atom must occur in
at least two eligible cases. Unsafe cases remain in validation scope but never
contribute selection credit. Prior atom reselection is rejected.

Candidates are exact-dominance-pruned, then ranked by covered cases, reporting
domains, circuit roles, and safety weight descending, followed by total atoms
and members including closure ascending. The complete semantic co-rank-one set
is published; canonical bundle-key bytes select only among exact semantic ties.

## Adaptive round transitions

Every selected candidate, effect-plan identity, exact case set, metadata,
covered set, diversity count, safety weight, atom support, and key is
independently recomputed before comparison. Deterministic replay, complete
physical promotion, seal-bound environment, and effect-closure evidence must
all be true.

An unchanged gap remains byte-identical. A disappearing gap must be selected or
in the frozen closure and must either become a satisfied obligation or produce
one through four successors. Every successor preserves the exact prior path as
a prefix, appends one different current member, preserves the obligation
anchor, moves to the same or a higher causal stage, retains or strengthens
evidence, and has a unique member and path. Rewriting, truncation, unexplained
new gaps, unknown satisfied obligations, selected-member persistence,
nonclosure change, pass regression, unsafe-to-pass transition, or insufficient
two-case/two-domain/two-role advancement fails closed.

Strict total pass uplift with a newly passing member of the immutable active
cohort is immediate public admission. Otherwise the evaluator may continue
only while round, atom, and exact-member budgets remain. The frozen policy
object itself is compared structurally with the compiled contract, preventing
callers from relaxing any gate or ceiling.
