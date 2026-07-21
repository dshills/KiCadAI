# Uncertainty-Aware Behavioral Intent Compilation Specification

## 1. Purpose

Build an AI-facing compiler that translates ordinary circuit-design requests
into the strict `kicadai.open-set-requirement.v3` behavioral contract. The
compiler is a fail-closed trust boundary: model output is a proposal, never an
executable circuit request by itself.

This milestone connects natural-language intent to the existing deterministic
architecture search, closed-loop simulation, lowering, routing, writer, KiCad,
and replay pipeline. It does not add a topology language or authorize the model
to choose parts, nets, coordinates, layers, routes, solver controls, or
validation evidence.

## 2. Public Compilation Contract

Untrusted provider output uses `behavioralintent.Proposal` version 1. The
deterministic result uses
`kicadai.behavioral-intent-compilation.v1` and has exactly one status:

- `ready`: a normalized strict v3 requirement is executable;
- `needs_clarification`: the minimum blocking user choice is required;
- `unsupported`: a stable capability-gap record explains why trusted
  generation or verification is unavailable;
- `invalid`: provider output violated the compiler contract.

Only `ready` may contain an executable requirement. Every ready result enables
simulation, all-corner evaluation, model provenance, closed-loop evidence,
ERC, strict DRC, complete routing, connectivity, writer correctness,
zero-difference round trip, and deterministic replay.

## 3. Source Coverage

Prompt hashing and statement boundaries are compiler-owned and deterministic.
The provider accounts for every statement exactly once as compiled behavior,
clarification, capability gap, or non-material context. Each record includes a
rationale and typed references.

Every objective, operating case, and behavioral requirement must have source
statement evidence. This reverse-coverage gate prevents unrequested objectives,
corners, performance guarantees, and safety claims from being invented.

## 4. Behavioral Scope

Ready requirements describe only:

- external interfaces and voltage/current domains;
- observable behavior and registered metrics;
- explicit units and lower/upper bounds;
- nominal and worst-case supply, input, load, temperature, tolerance, and model
  conditions;
- safety-critical limits;
- manufacturing-neutral board limits and acceptance gates.

The compiler rejects unknown fields and delegates the normalized requirement to
`architecturesearch.Validate`, which remains authoritative for semantic IDs,
bindings, canonical units, registered analyses, metrics, corners, and policy
bounds.

## 5. Uncertainty And Clarification

Every identified uncertainty records a stable semantic ID, affected requirement
path, kind, explanation, and one resolution:

- `explicit`: directly specified by the user;
- `bounded`: represented by declared ranges or operating cases;
- `clarification`: owned by exactly one blocking clarification;
- `capability_gap`: owned by an unsupported capability record.

A clarification has one unique requirement path and may resolve multiple
related uncertainties. Duplicate questions for one path are invalid so the
compiler asks the minimum necessary question. No executable requirement is
released while any clarification remains.

## 6. Capability Gaps

Unsupported behavior produces a deterministic record with semantic capability,
requirement path, reason, and the evidence needed to close the gap. Capability
IDs describe behavior or verification, never prompt text, fixture identity,
expected topology, component family, or corpus path.

Missing or malformed installed-capability context blocks model execution.
Provider configuration, authentication, transport, refusal, timeout, and
incomplete-output failures remain typed provider failures and cannot be
converted into guessed circuit intent.

## 7. Provider Boundary

The AI profile uses a strict, fully-required JSON schema generated from the Go
proposal and v3 requirement types. Provider context contains compiler-owned
source statements, a caller-supplied immutable capability snapshot, the target
contract, and generic policy prohibitions.

Retries may address deterministic diagnostics only. They may not weaken a
requirement, discard source coverage, fabricate capability evidence, or switch
to a topology-bearing output profile.

## 8. Held-Out Acceptance Corpus

Before completion, freeze and SHA-256 pin independently authored prompts and
paraphrases spanning:

1. amplifiers, including Class-A and Class-AB behavior;
2. active and passive filters;
3. regulated and converted power;
4. input, output, transient, and load protection;
5. analog and digital sensors;
6. MCU GPIO, I2C, SPI, UART, power, reset, and programming interfaces.

Fixtures contain prompt text and expected behavioral outcomes only. Ready
fixtures may assert semantic behavior, bounds, corners, and interfaces; they
must not name topology, parts, nets, coordinates, layers, routes, or expected
repair actions. Ambiguous fixtures assert the minimum clarification. Unsupported
fixtures assert stable capability gaps.

## 9. End-To-End Promotion

Every supported held-out prompt must reach deterministic architecture search,
trusted simulation, all declared corners, lowering, route completion,
connectivity, writer correctness, normalized zero-difference round trip, clean
KiCad ERC, and strict DRC. Recorded replay must be byte-identical.

Existing open-set, closed-loop, amplifier, ESP32/MCU, sensor, power/protection,
USB-C, routing, writer, round-trip, and KiCad-backed promotion evidence must
remain green.

## 10. Prohibitions

Production code may not branch on prompt text, fixture ID, source hash, corpus
path, project name, requested topology, expected component/value, or expected
layout. It may not contain fixture-specific schemas, coordinate exceptions,
allowlists, block families, repair overrides, or weakened promotion gates.

## 11. Completion Evidence

Completion requires a requirement-by-requirement audit linking every clause to
current source, tests, frozen corpus artifacts, and fresh command evidence;
Prism review of the complete staged change; a committed and pushed exact commit;
and green GitHub Actions for that commit.
