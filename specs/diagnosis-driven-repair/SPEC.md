# Diagnosis-Driven Repair Specification

## Goal

Provide the first bounded, deterministic repair loop that turns normalized
electrical or physical failure evidence into an auditable proposal, re-enters
the correct synthesis stage, and records the result without weakening the
original requirement.

## Frozen corpus

The corpus is frozen at base commit
`2365b35c0be54ec1a3fa9bd89a39dda2be1e9a08` and is defined by
`internal/repairloop/testdata/corpus/manifest.json`.

- The electrical case is the behavior-only protected programmable current
  output. Its untouched result was deterministic exhaustion at simulation,
  hash `596f627523ace41257a6f7ec3fcaed54914c66ad5ee6563168a228d8d23663b5`,
  with bounded nonconvergence and current-transfer errors.
- The physical case expands an existing catalog-resolved, identity-neutral
  seed to six primitives, twelve pads, and two six-endpoint required nets. A
  frozen relative-placement perturbation must produce a real initial routing
  block before correction.

## Generic evidence contract

Every loop emits `kicadai.diagnosis-driven-repair.v1` with:

1. normalized stage, code, category, direction, evidence hash, and affected
   scope;
2. deterministic proposal ID, repair kind, stage re-entry, expected effect,
   scope, authorization, and rejection reason;
3. before, after, and result hashes plus applied, improved, passed, failed, or
   rejected outcome;
4. explicit budget, consumption, and whole-trace hash.

Diagnostics and scope must derive only from graph, electrical, simulation,
placement, route-operation, or geometry evidence. Production code must not
contain corpus identities, component reference assumptions, fixture
coordinates, allowlists, named block families, or acceptance shortcuts.

## Bounded repairs

Electrical categories cover bias/reference access, feedback polarity or
compensation, value-domain response, rating/dissipation/thermal/SOA evidence,
convergence, and unsupported model evidence. Admissible graph operators remain
passive-edge addition, passive-edge redirection, and terminal-compatible rated
substitution. Every accepted graph repair re-enters equation sizing and then
trusted simulation. Missing model evidence and unsafe or ambiguous changes
fail closed.

Physical categories cover component overlap, endpoint access, route-tree branch
order, same-net merge, congestion, foreign-net crossing, and missing layer
transition. Placement actions re-enter placement; operation-scoped actions
re-enter routing and preserve unrelated route operations.

## Acceptance

- Two identical runs produce byte-identical repair traces and selected results.
- Budgets stop repeated or non-improving states.
- The current-driver case either fully passes all trusted electrical and
  physical gates or retains a narrower, stable fail-closed repair trace.
- The dense physical case begins blocked and ends routed with zero failed nets.
- Existing architecture, simulation-grounded, Class A/Class AB/notch, writer,
  KiCad ERC/strict DRC, connectivity, route-completion, and round-trip evidence
  remain green in their local lanes.
