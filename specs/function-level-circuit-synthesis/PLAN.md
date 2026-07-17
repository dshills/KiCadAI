# Function-Level Circuit Synthesis Implementation Plan

Date: 2026-07-17

## Phase 1: Freeze Contract And Corpus

1. Check in this specification and plan.
2. Add the eight function-intent fixtures and immutable manifest.
3. Add a corpus-integrity test that verifies membership, domains, SHA-256
   hashes, and prohibition of pins, pads, coordinates, layers, routes, support
   component IDs, and block identities.
4. Record the current explicit-corpus regression baseline.

Acceptance: corpus integrity and all existing tests pass.

## Phase 2: Function Intent Model

1. Add strict model, normalization, JSON schema, and validation types.
2. Make `generic-circuit-v1` accept either explicit graph or function intent.
3. Add stable synthesis diagnostic codes and categories.
4. Preserve recorded explicit graph compatibility.

Acceptance: both forms strict-decode; malformed or mixed forms fail closed.

## Phase 3: Primary And Interface Resolution

1. Resolve primary function requirements through the immutable catalog.
2. Resolve named external interfaces to the smallest accepted verified
   connector.
3. Bind semantic connections and power domains.
4. Emit the first lowered explicit graph and selection evidence.

Acceptance: selection and topology are deterministic across input/catalog order.

## Phase 4: Catalog-Driven Support Expansion

1. Add generic companion component/network recipe metadata.
2. Populate reviewed recipes needed by the frozen corpus.
3. Expand support recipes, merge nets, and resolve generated components.
4. Add generic unused-pin policy and external-source power-flag lowering.

Acceptance: no synthesized support path switches on fixture or catalog component
identity; missing evidence produces the specified blocker.

## Phase 5: Constraint And Layout Derivation

1. Derive schematic groups and relative placement from graph topology.
2. Derive board size and layer policy from bounded complexity rules.
3. Lower catalog placement/routing hints and operating limits to PCB constraints,
   net classes, endpoint access, and retry policy.
4. Prove stable output under shuffled input order.

Acceptance: no provider coordinates/layers/routes are accepted in function form.

## Phase 6: Corpus Promotion

Promote each frozen circuit without changing corpus intent:

1. analog fixtures;
2. power/protection fixture;
3. transistor fixture;
4. sensor/interface fixtures;
5. ATmega fixture;
6. ESP32 mixed fixture.

After every generic correction, rerun the affected KiCad-backed fixture and the
protected USB-C I2C sensor/LED regression fixtures.

Acceptance: all applicable per-circuit gates pass.

## Phase 7: Capability Reporting

1. Emit per-circuit synthesis evidence and hashes.
2. Aggregate results by domain and diagnostic category.
3. Publish the checked-in corpus capability report.
4. Document unsupported scope without weakening pass criteria.

Acceptance: every failure is classified and every pass links authoritative gate
evidence.

## Phase 8: Closeout

1. Run `go test ./...`.
2. Run all configured optional KiCad-backed corpus and protected regressions.
3. Verify zero round-trip differences and byte-identical replay.
4. Update project status, AI-generation documentation, and roadmap.
5. Stage and run `prism review staged`; resolve all high/medium findings.
6. Commit, push, and verify GitHub Actions.

Acceptance: the specification completion rule is fully satisfied.
