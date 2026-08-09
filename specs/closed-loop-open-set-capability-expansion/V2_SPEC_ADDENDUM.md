# Closed-Loop Open-Set Capability Expansion V2 Addendum

Status: historical frozen contract; V2 closed by [V2_VALIDATION_AUDIT.md](V2_VALIDATION_AUDIT.md)

## 1. Inherited contract

V2 inherits `SPEC.md`, `CORPUS_RULES.md`, and every V1 trust gate except where
this addendum necessarily versions the experiment after the failed blind V1
validation recorded in `V1_VALIDATION_AUDIT.md`. No V1 acceptance criterion is
relaxed.

## 2. Immutable starting state

The outcome-changing production starting point is commit:

`8bdc31e668152b7324066bd75182d86d7320d3f8`

That checkpoint contains the reviewed V1 generic capability attempt, the V1
failure audit, and the V2 continuation protocol. The V2 baseline and all final
comparisons bind this full commit hash. Any later production change before the
V2 corpus, manifest, policy, and baseline protocol freeze invalidates V2.

## 3. Fresh corpus and isolation

V2 contains 24 newly authored behavior-only requirements: 12 discovery and 12
held-out, with exactly two cases in each role for analog, power,
digital/control, MCU/interface, sensor, and mixed-signal reporting domains.

The independent author receives no conversation history and may read only the
public requirement model/validation vocabulary and the normative corpus rules.
The author may not inspect V1 requirement bytes, corpus evidence, outcomes,
selection, implementation changes, production search behavior, or existing
examples. The implementation agent may validate syntax, neutrality, hashes,
and aggregate quotas, but may not inspect or summarize V2 held-out requirement
content before the implementation diff is sealed.

Candidate files remain quarantined until automated checks prove strict decode,
neutrality, diversity, source uniqueness, cross-role uniqueness, normalized
non-overlap with V1, and all required acceptance gates. Freeze copies them
byte-for-byte into the versioned V2 corpus and records their hashes; it does not
rewrite their electrical content.

## 4. Frozen policy

V2 retains the V1 evaluator and ranking policy, impact registry, component
catalog, model registry, toolchain contract, and per-case synthesis ceilings:

- 4,000 expanded states;
- 8,000 generated graphs;
- 24 primitive instances;
- 24 internal nodes;
- 128 candidate simulations;
- 2,048 corner evaluations;
- 128 value trials;
- 16 topology repairs;
- 8 retained candidates; and
- 32 diagnostic samples.

A policy or budget change capable of altering an outcome requires a new
experiment version unless that change is itself selected as rank one.

## 5. Baseline, selection, and blind boundary

`V2_BASELINE_PROTOCOL.md` is normative. Every case runs twice from the immutable
starting state. Discovery completes first and alone determines clustering,
ranking, and rank-one selection. Held-out baseline evidence is produced by the
automated harness but remains sealed from the implementation agent until the
generic production diff is fixed.

Only the rank-one reusable capability or inseparable prerequisites expressly
listed in its required evidence may alter production behavior. Corpus IDs,
paths, hashes, role membership, expected outcomes, coordinates, allowlists,
and fixture-specific circuit families are prohibited from production code.

## 6. Success

V2 completes only when identical frozen bytes and unchanged policies prove all
of the following:

- strict discovery pass-count improvement;
- strict held-out pass-count improvement;
- rank-one-affected discovery uplift;
- no baseline-pass regression;
- preserved unsafe evidence;
- deterministic remaining clusters and report bytes;
- complete trusted analysis evidence; and
- two clean-root local installed-KiCad promotions for every new pass, including
  ERC, strict DRC, connectivity, route completion, writer correctness, zero
  round-trip differences, and replay.

Failure consumes the V2 held-out set. It may not be tuned and rerun as blind
evidence.
