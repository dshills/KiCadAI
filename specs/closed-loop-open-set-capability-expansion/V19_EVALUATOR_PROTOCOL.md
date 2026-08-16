# V19 Generic Causal-Topology Repair Evaluator Protocol

Status: frozen before public evaluation. This document does not authorize or
record a V19 public run.

Authenticate the V18 evaluator manifest, immutable 24-case V10 discovery
corpus, V19 contract, and V19 evaluator manifests before execution. Run only
from a clean committed repository into a fresh output root.

V19 reuses the frozen V17 serial two-replay transport and separately binds the
exact V17 legacy, V18, and V19 environments. The V19 synthesis entry point must
execute `SynthesizeV18WithLegacy` first. Any V18 pass, unsafe result, invalid or
infeasible requirement, canceled run, non-causal typed frontier, universal
model/solver/assertion gap, or non-replayable diagnosis returns the V18 result
byte-for-byte.

Only a terminal frontier consisting entirely of `causal_topology_repair` may
enter the V19 extension. The V18-selected failed graph must be reconstructed
from its hash-bound topology, value plan, value trial, repair initial hash, and
evaluation hash, then evaluated once under the V19 environment to seed depth
zero.

The repair search is fixed at depth four, beam width eight, 12 evaluated
candidates per depth with unused quota rolling forward, and 48 evaluated
causal candidates for the entire invocation. A proposal contains at most two
logical changes and a path at most eight. Every generated graph is charged,
including global graph-hash duplicates; a duplicate is never simulated twice.
All inherited policy limits remain unchanged. Candidate operations and states
use the ordering, monotonic rule, plateau rule, invariant boundary, and
fail-closed behavior frozen by `V19_SPEC_ADDENDUM.md`.

The evaluator has no held-out key or blind-result access. A later Phase 6 goal
may run all 24 public discovery cases in manifest order exactly twice. A pass
must complete two clean-root installed-KiCad promotions with clean ERC, strict
DRC, connectivity, route completion, writer correctness, and zero round-trip
differences. Replay, promotion, environment, corpus, manifest, resource, or
output inconsistency fails closed.

This Phase 5 freeze permits only build, contract, deterministic replay,
resource-bound, fail-closed, and fresh-output self-tests. It must not invoke the
runner against the public corpus or publish a generation report.
