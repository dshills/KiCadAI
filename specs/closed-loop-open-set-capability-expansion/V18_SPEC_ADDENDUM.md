# V18 Versioned Capability Extension

Status: implementation boundary frozen for the first post-V17 capability
change.

## Purpose

V18 carries the selected V17 public-generation capability forward without
mutating any source byte committed to the V6 through V17 historical evaluator,
baseline, implementation, or replay seals. It is an extension of the existing
24-case public corpus, not a new corpus-authoring or blind-evaluation round.

The selected generic capability is deterministic composition and evaluation of
low-voltage, high-input-impedance, multi-output analog threshold circuits. The
public evidence motivating it is the repeated typed open-topology repair/search
frontier in the committed V17 baseline. No fixture identity, coordinate, or
expected outcome is part of the implementation contract.

## Frozen preservation boundary

1. Every path named by a V6, V7, V8, V11, V16, or V17 checksum or reviewed-
   implementation seal remains byte-identical.
2. Legacy component catalogs, model-provenance registries, primitive
   inventories, evaluator constructors, synthesis entry points, and replay
   hashes remain reproducible.
3. V18 additions live in new versioned files and are reachable only through a
   V18 constructor or explicit V18 API.
4. V18 must prove the historical tests before any generation-one comparison is
   accepted. Updating a historical expected hash or adding an unrestricted
   drift allowlist is prohibited.

## Generic capability contract

V18 may add only these reusable behaviors:

- a separately loaded, reviewed low-voltage rail-to-rail op-amp catalog and
  provenance extension;
- requirement-aware rejection of active primitives whose reviewed supply or
  output-headroom envelope cannot satisfy the observed analog output;
- deterministic threshold-window semantics when lower and upper thresholds
  are bounded but output polarity is otherwise unconstrained;
- deterministic early retention of a complete merged multi-output graph;
- contention-free comparator conjunction using a pull-up and deterministic
  diode isolation when multiple active outputs drive one threshold node;
- topology scoring that rejects only proven low-value input-to-rail shunts,
  while retaining legitimate series signal paths; and
- a bounded nonzero DC probe for an input-impedance assertion whose authored
  zero-volt corner would otherwise produce an indeterminate 0/0 measurement.

The implementation must not introduce fixture-specific templates, semantic
IDs, coordinates, allowlists, expected outcomes, or special-case corpus paths.

## Evaluation boundary

Generation one reuses the authenticated V17 discovery corpus and evaluates all
24 public cases exactly twice under one committed V18 environment. A passing
case still requires the complete existing physical-lowering and installed-
KiCad promotion gates. The V17 baseline remains immutable and is compared by
case identity and typed obligation, never rewritten.

Held-out source keys, baseline keys, encrypted records, and blind outcomes are
outside this addendum. They require a separately frozen blind-final protocol
and the existing explicit authorization.

## Admission

V18 is admitted only if all of the following hold:

- every historical local regression and replay seal passes unchanged;
- V18 unit and deterministic-replay tests pass;
- the selected public cases advance without any public regression;
- at least one public case becomes a complete physically promotable pass;
- installed KiCad reports clean ERC, strict DRC, connectivity, route
  completion, writer correctness, and zero round-trip differences for every
  promoted pass; and
- Prism reports no unresolved high- or medium-severity finding on the exact
  staged implementation.
