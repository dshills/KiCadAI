# Generic Standalone Clock Generation Plan

## Phase 0: Freeze Evidence

1. Add the behavior-only two-case corpus and immutable manifest checks.
2. Record the untouched `ARCHITECTURE_CAPABILITY_UNSUPPORTED` baseline.
3. Commit the corpus, specification, plan, and baseline before registering
   production capability.

## Phase 1: Catalog And Models

1. Add concrete packaged-source and relaxation-source component evidence.
2. Add normalized source accuracy, jitter, startup, output-drive, load,
   fanout, supply, temperature, and duty-cycle bounds.
3. Add trusted simulation primitives or bounded analytical models with
   provenance and deterministic uncertainty intervals.
4. Add catalog and model validation tests, including incomplete-evidence
   rejection.

## Phase 2: Architecture And Solvers

1. Register generic `clock_generation` support.
2. Parse the full behavioral contract without request-identity branching.
3. Rank qualified families deterministically.
4. Solve relaxation timing values across tolerance and temperature corners.
5. Emit calculation, selection, alternative, and rationale evidence.
6. Add fail-closed tests for every unsupported requirement class.

## Phase 3: Closed-Loop Proof

1. Map frequency, duty cycle, startup, logic levels, edges, jitter, loading,
   fanout, and supply current into closed-loop evidence.
2. Exercise all declared operating corners.
3. Require complete model provenance and stable replay hashes.

## Phase 4: KiCad Realization

1. Lower both architectures to concrete symbols, footprints, nets, and
   support components.
2. Add generic timing-group, bypass-return, sensitive-node, source-damping,
   and clock-route constraints.
3. Verify placement and routing evidence before accepting strict KiCad gates.

## Phase 5: Promotion And Delivery

1. Promote both frozen clock cases and the original `digital_clock_source`.
2. Run the full held-out, simulation-grounded, amplifier, MCU/sensor, and
   installed-KiCad matrices.
3. Publish final reports, checksums, clean-checkout bundles, and an audit.
4. Review the staged diff with Prism and resolve every high and medium finding.
5. Commit, push, and verify GitHub Actions for the exact revision.
