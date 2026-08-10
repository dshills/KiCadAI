# V4 Monotonic Gap-Transition Protocol

Status: freeze candidate; normative for V4 only

## Purpose

This protocol distinguishes a genuine regression from legitimate diagnostic
progress. Removing a selected blocker can expose later or independent gaps.
Exact equality rejects that useful new information; unconstrained replacement
can hide regressions. V4 therefore uses a deterministic set-inclusion partial
order frozen before corpus authoring.

## Canonical identities

The selected cluster identity is the length-delimited tuple:

`(stage, scope, capability, code)`

A gap identity is the length-delimited tuple:

`(stage, scope, capability, code, sorted-unique(required_evidence))`

Each UTF-8 value is prefixed by its unsigned decimal byte length and `:`.
Byte-length delimiting makes the encoding injective even when values contain a
separator or multibyte code point. `required_evidence` is sorted bytewise and
deduplicated before encoding. Input order and duplicates therefore do not
change identity. Empty strings and an empty evidence set remain explicit
values. Requirement IDs, operating-case IDs, analysis kinds, evidence hashes,
and downstream symptoms are observations or provenance, not semantic gap
identity.

## Per-case comparison

Case IDs must be unique and the before/after case sets must match exactly.
Outcomes are compared separately:

1. a baseline pass must remain pass;
2. a baseline unsafe case must not become pass; and
3. a final pass is accepted as a gap removal only through every promotion and
   safety gate in the V4 specification.

For a case that is nonpassing both before and after:

1. normalize both gap collections into sets of canonical identities;
2. remove from the baseline set only gaps whose four selected-cluster fields
   exactly equal the frozen selected identity;
3. do not remove any final gap; and
4. require `baseline_nonselected ⊆ final`.

Consequences:

- equality passes;
- a strict final superset passes;
- disappearance of any nonselected baseline identity fails;
- changing stage, scope, capability, code, or required evidence is a new
  identity and fails unless the original identity also remains; and
- sharing only the selected capability name is insufficient for removal.

The relation is reflexive, antisymmetric on normalized sets, and transitive by
set inclusion. It cannot make a pass decision by itself and cannot override
promotion, unsafe, regression, determinism, or KiCad gates.

## Adversarial contract examples

The committed contract test must prove:

- equal normalized sets pass;
- a strict final superset passes;
- input order and duplicate evidence normalize identically;
- exact selected-cluster removal passes;
- another gap with the same capability but different stage, scope, or code is
  not removed;
- removal of an unrelated identity fails;
- renaming or reclassifying any identity field fails;
- adding or removing required evidence without retaining the original identity
  fails; and
- missing/duplicate case IDs fail before comparison.

No V4 outcome may be inspected before this protocol, its machine-readable
policy, its adversarial tests, and their hashes are committed.
