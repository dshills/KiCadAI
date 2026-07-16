# Generic Circuit Agent Repair Harness

Date: 2026-07-15
Status: Implemented

## Goal

Provide a deterministic, provider-free command that lets an external AI decide
whether a failed `generic-circuit-v1` graph has exactly one safe, executable
repair candidate. The command models an agent consuming capability output,
preflight diagnostics, and constrained patch operations; it never mutates a
graph or creates a KiCad project.

## Command

```sh
kicadai circuit repair-plan --request graph.json
```

The result contains the normalized input hash, full preflight evidence, one
selected repair option and generated `kicadai.circuit-patch.v1` document only
when safe, convergence state, stop reason, diagnostics, and external evidence
still required for KiCad-backed or fabrication claims.

States are `ready`, `repair_available`, `needs_review`, and `blocked`.

## Selection Policy

- `ready`: preflight is `ready_for_write`; no patch is produced.
- `repair_available`: exactly one `agent_selectable` option exists and its
  patch is fully populated from trusted graph/catalog evidence. Any option
  requiring an agent-supplied value remains review-only.
- `needs_review`: zero executable repairs exist, or more than one safe repair
  exists. The result preserves the upstream stage/retry scope and any
  review-required metadata; electrical, thermal, safety, fabrication, and
  external KiCad categories are never inferred from prose.
- `blocked`: malformed input, invalid capability/patch contract, repeated
  input hash, attempt ceiling, or an unsafe candidate.

V1 does not apply a patch, invoke `circuit create`, call a provider, or use an
LLM. The caller explicitly executes the emitted patch, saves the corrected
graph, and re-runs the command/preflight.

## Safety

Generated patch documents are validated with `ValidatePatch`; the planner only
uses existing graph identities and exact existing patch operations. It never
chooses catalog substitutions from a list of alternatives, invents a pin
selector, changes immutable graph identity, or resolves competing options.
The attempt ceiling is three normalized semantic graph hashes supplied via
repeatable `--previous-hash`; a repeated hash is blocked before planning.
Hashes are computed after strict decode and `Normalize`, so whitespace, key
ordering, and other non-semantic JSON differences cannot bypass loop detection.

## Evidence

Preflight, patch, and creation remain authoritative. `repair_available` is not
an electrical, ERC/DRC, round-trip, KiCad-backed, or fabrication claim.

## Test Strategy

Cover RC selector repair through patch/re-preflight/create, LM358 unit repair,
bounded region repair, ready recorded generic graphs, ambiguous multiple-option
cases, review-only routing/external cases, repeated hashes, no-write behavior,
deterministic output, and existing generic/provider fixtures. The current
generic protected USB-C LED graph has unresolved CC routing and therefore
correctly returns `needs_review`, rather than receiving an unsafe patch.
Optional KiCad checks remain environment-gated.
