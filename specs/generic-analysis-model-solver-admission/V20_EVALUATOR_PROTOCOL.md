# V20 Generic Analysis, Model, and Solver Admission Protocol

Status: frozen before public evaluation. This document does not authorize or
record a V20 public run.

V19 is a retired historical experiment. Its corpus bindings, evaluator,
generation-zero report, retirement evidence, and checksums remain immutable.
V20 is a version-isolated successor that reuses only the authenticated public
24-case V10 discovery corpus and the exact V18 comparison boundary.

Before execution, authenticate the immutable corpus, V18 evaluator manifest,
V20 contract, and V20 evaluator manifests. Execute from a clean committed Git
tree into fresh report and working roots. The evaluator has no held-out key or
blind-result access.

V20 uses the frozen V17 serial transport, processes all 24 discovery cases in
manifest order, and evaluates each case exactly twice. V20 first executes
`SynthesizeV18WithLegacy`. A V18 pass, unsafe result, invalid or infeasible
requirement, canceled run, or result outside the selected public typed
analysis/model/solver frontier is returned byte-for-byte.

An eligible nonpassing result receives one fresh bounded production synthesis
run. Before topology search, admission derives every required analysis from
the behavior contract, authenticates the model registry sources, selects the
one immutable enabled solver for each canonical analysis, and proves inventory
model coverage. Before every numerical evaluation, admission authenticates
the exact graph and harness models for that operating case and corner and
binds their model, source, parameter, solver, and compatibility digests into
the attempt evidence.

Admission refuses with exactly one or more of these stable typed categories:
`MISSING_MODEL`, `INCOMPATIBLE_MODEL`, `MISSING_ANALYSIS_DEFINITION`,
`UNSUPPORTED_ANALYSIS`, `SOLVER_UNAVAILABLE`,
`SOLVER_MODEL_INCOMPATIBLE`, or `INVALID_MODEL_PARAMETERS`. It never performs
implicit model substitution. The existing synthesis count limits, physical
promotion gates, and installed-KiCad requirements remain unchanged.

Only public frontier evidence selects V20 eligibility. The selected analysis
set is DC operating point, DC sweep, AC sweep, transient, and stability.
Electrothermal and all other capability families remain excluded. Direct
`dc_voltage` and `dc_current` observations are normalized only to their
existing generic voltage/current measurement quantities.

A pass still requires two clean-root installed-KiCad promotions with clean
ERC, strict DRC, connectivity, route completion, writer correctness, and zero
round-trip differences. Replay, promotion, environment, corpus, manifest,
resource, provenance, or output inconsistency fails closed.

Material improvement requires at least one selected case to pass or a selected
generic availability leaf to advance to a strictly later, more specific typed
blocker without any V18 pass or safety regression. Renaming a failure does not
count. Publication is atomic and no-replace.
