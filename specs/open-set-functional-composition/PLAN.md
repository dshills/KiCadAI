# Open-Set Functional Circuit Composition Implementation Plan

## Objective

Add deterministic bounded architecture search over typed electrical contracts,
then prove it against a pre-implementation frozen corpus without weakening any
existing KiCadAI gate.

## Implementation Rules

- Freeze the specification and held-out requirement bytes before search code.
- Commit every phase after focused tests and Prism review.
- Keep production code independent of corpus identity and location.
- Use one strict behavioral request schema; do not add provider-specific input
  schemas.
- Reuse the existing function-level resolver, lowering, writer, round-trip,
  routing, ERC, and DRC pipeline after architecture selection.
- Fail closed whenever a safety or compatibility obligation is unproven.

## Phase 1: Specification And Frozen Corpus

### Work

- Define the behavioral input, typed-port, fragment, search, scoring, evidence,
  rationale, and fail-closed contracts.
- Add five behavior-only held-out requirements and a SHA-256 pinned manifest.
- Add a freeze test with strict decoding, identity-neutrality checks, domain
  coverage, and unmanifested-file detection.

### Acceptance

- Corpus files name no parts, fragments, topologies, pins, footprints,
  coordinates, nets, or routes.
- Corpus membership and every byte are pinned before production search code is
  added.

### Commit

`Freeze open-set composition requirements`

## Phase 2: Behavioral Requirement Model

### Work

- Add production types and a strict decoder for
  `kicadai.open-set-requirement.v1`.
- Normalize quantities, units, ranges, identifiers, constraints, and ordering.
- Validate bindings, domain references, contradictions, and policy budgets.
- Compute a canonical requirement hash.

### Tests

- Strict decode and validation tables.
- Order-neutral canonicalization and byte-identical replay.
- Contradictory and ambiguous requests fail with stable diagnostics.

### Commit

`Add behavioral architecture requirements`

## Phase 3: Typed Electrical Contracts

### Work

- Define typed ports, electrical ranges, logic/protocol traits, directions, and
  evidence.
- Add deterministic compatibility/unification with structured rejection codes.
- Adapt catalog records into typed component contracts without weakening
  catalog confidence or rating rules.

### Tests

- Voltage/current/logic/protocol compatibility matrices.
- Unknown safety evidence fails closed.
- Direction, domain, and range errors are deterministic.

### Commit

`Add typed electrical compatibility contracts`

## Phase 4: Fragment Registry And Bounded Search

### Work

- Add a stable reusable fragment-provider contract and registry hash.
- Model unresolved obligations and immutable candidate states.
- Implement deterministic enumeration, expansion, early rejection, budgets,
  architecture fingerprints, and ranked distinct candidates.
- Publish bounded rejection summaries and budget diagnostics.

### Tests

- Registration, request, and catalog order do not change bytes.
- Every budget is enforced and reported.
- Unsupported, exhausted, and ambiguous searches fail closed.
- Production code contains no corpus identity.

### Commit

`Add bounded deterministic architecture search`

## Phase 5: Value And Tolerance Solvers

### Work

- Add formula-library identity and deterministic preferred-value selection.
- Implement reusable divider, RC/pole, hysteresis, drive, and protection/rating
  calculations required by registered fragments.
- Evaluate nominal and bounded worst-case corners.
- Emit complete calculation and rejection evidence.

### Tests

- Unit and preferred-series selection tables.
- Tolerance-corner results and margins.
- Missing tolerance/rating data fails closed.
- Formula, catalog, and input ordering do not change evidence bytes.

### Commit

`Add deterministic value and tolerance evidence`

## Phase 6: Initial Reusable Providers

### Work

- Register capability-selected providers sufficient to explore threshold
  detection, protected load switching, adjustable regulation, active filtering,
  and open-drain level translation.
- Keep provider contracts reusable and corpus-identity blind.
- Generate at least one distinct alternative where catalog evidence supports
  one and explain selection or alternative rejection.

### Tests

- Provider unit tests use synthetic requirements separate from corpus files.
- Generic mutation tests vary voltages, currents, frequencies, and tolerances.
- Out-of-envelope mutations fail closed rather than matching a nearby fixture.

### Commit

`Add reusable open-set circuit providers`

## Phase 7: Function-Level Lowering And Rationale

### Work

- Lower selected architecture states into normal function-level intents.
- Resolve components through the catalog and recheck all selected contracts.
- Preserve calculation, alternative, and rejection evidence in a stable search
  rationale artifact.
- Refuse lowering if a contract or binding is lost.

### Tests

- Lowered graphs contain no unresolved obligation.
- Search result and function intent replay byte-for-byte.
- Invalid expanded graphs and evidence loss fail closed.

### Commit

`Lower selected architectures to circuit graphs`

## Phase 8: Held-Out Offline Promotion

### Work

- Run all five frozen requirements through search, lowering, existing
  synthesis, writer correctness, round-trip, connectivity, and routing.
- Add authoritative capability evidence and deterministic artifact hashes.
- Keep failures classified; do not edit frozen requirements.

### Acceptance

- All five pass every offline gate.
- Search evidence proves candidate generation, rejection, scoring, value and
  tolerance calculations, and alternatives.

### Commit

`Promote open-set composition corpus offline`

## Phase 9: KiCad Promotion And Regression Closeout

### Work

- Add the optional KiCad-backed corpus lane.
- Require clean ERC, strict DRC, route completion/connectivity, writer
  correctness, zero round-trip diffs, and replay.
- Run the complete Go suite and every applicable existing optional fixture.
- Update roadmap and AI-readiness evidence without overclaiming beyond the
  frozen envelope.
- Review staged changes with Prism, commit, push, and verify GitHub Actions.

### Acceptance

- All five held-out circuits pass the configured KiCad lane.
- Existing amplifier, ESP32, MCU, analog, sensor, USB-C, writer, routing,
  round-trip, and fabrication evidence remains green.
- The pushed commit is green in GitHub Actions.

### Commit

`Complete open-set functional composition milestone`
