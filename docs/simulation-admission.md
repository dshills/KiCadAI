# Simulation Admission

Simulation admission is the post-v1 trust boundary between a behavior-only
open-topology request and topology search or numerical evaluation. It proves
that the requested analyses can run with one deterministic solver and exact
reviewed models; it does not claim that the eventual circuit will pass.

```text
behavioral assertions
  -> canonical analysis plan
  -> enabled immutable solver
  -> authenticated model sources
  -> exact inventory preflight
  -> topology search
  -> exact graph + harness model admission per case/corner
  -> numerical evaluation
```

## Trusted sources

Admission accepts the embedded model-provenance registry, a reviewed project
overlay, or an explicitly configured reviewed overlay. Every source is
normalized and content-addressed. A merged overlay is split back into its exact
base and added records; removing or rewriting a base identity is rejected.
Conflicting records for the same catalog/model identity are rejected even if a
source ordering would otherwise make one appear first.

Provider requests cannot supply model IDs, equations, model files, solver IDs,
solver options, include paths, commands, or source priority. There is no
fallback after an incompatibility or simulation failure.

## Evidence

`kicadai.simulation-admission.v1` records:

- request and execution-environment digests;
- authored and canonical analysis identities;
- immutable solver ID, versioned definition digest, and workflow model;
- exact component and catalog identities;
- complete selected model claims and claim digests;
- normalized model parameters and circuit value digests;
- reviewed provenance records and record digests;
- registry source identity, kind, and digest; and
- compatibility status, reason, rejected claims, and typed diagnostics.

Strict decoding rejects unknown fields, trailing values, oversized artifacts,
noncanonical ordering, invalid digests, and internally inconsistent status.
Offline validation recomputes the complete artifact hash, parameter hash, model
claim hash, provenance-record hash, and immutable solver identity.

## Current boundary

The compiled solver registry covers the deterministic analyses already
implemented by KiCadAI: DC operating point (including authored DC sweeps), AC
sweep, transient, noise, stability, startup, distortion, thermal, and
electrothermal. Availability remains explicit per execution environment.
Admission does not add arbitrary SPICE dialects, arbitrary external solvers,
unreviewed device models, provider-controlled tolerances, or a guarantee that
a compatible topology exists.

The V20 successor evaluates only the public analysis/model/solver frontier
selected before implementation: DC operating point, DC sweep, AC sweep,
transient, and stability. Electrothermal and unrelated capability families are
outside that evaluation expansion. V19 remains immutable and retired.
The completed V20 run produced 1 pass, 5 unsupported, 1 unsafe, and 17
exhausted cases. It preserved V18's pass and safety result and advanced the
selected DC operating-point model leaf to bounded topology repair.
