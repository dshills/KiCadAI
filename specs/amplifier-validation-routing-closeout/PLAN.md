# Amplifier Validation and Routing Policy Closeout Plan

## Phase 1: Capture Structural Validation Baseline

Goal: reproduce the protected amplifier's current structural validation blocker
and classify it before changing behavior.

Tasks:

1. Run the protected fixture through the design workflow into an ignored output
   directory.
2. Extract normalized promotion evidence:
   - stage order and status;
   - issue code/path;
   - schematic versus PCB issue source;
   - affected labels, symbols, footprints, pads, and nets when available;
   - route-policy state such as `skip_routing`;
   - routed, partially routed, and unrouted net summaries.
3. Add or tighten a focused test helper that asserts the fixture no longer
   regresses to old placement, endpoint-binding, project-write, or
   writer-correctness blockers.
4. Document the exact current blockers in a baseline note if the evidence is too
   detailed for a compact unit assertion.

Acceptance:

- The current blocker is reproducible without KiCad CLI.
- Baseline evidence distinguishes schematic label/connectivity issues from PCB
  route-completion issues.
- No production behavior changes are made in this phase.

Review and commit:

- Run focused promotion/design workflow tests.
- Run `prism review staged`.
- Commit as `Capture amplifier validation routing baseline`.

## Phase 2: Clean Up Generated Schematic Labels

Goal: remove false schematic structural blockers caused by generated labels,
floating labels, or label/net alias conflicts.

Tasks:

1. Use Atlas to locate schematic label emission, schematic connectivity
   validation, and amplifier schematic realization paths before reading large
   files.
2. Classify each protected amplifier label issue as:
   - unattached generated label;
   - decorative text incorrectly emitted as a label;
   - alias mismatch between canonical and local nets;
   - real unsupported topology;
   - validation false positive.
3. Patch the responsible layer:
   - label coordinates when labels are near but not attached;
   - schematic generation when labels should be wires or no-connect markers;
   - alias mapping when canonical nets are lost;
   - validation reporting when the topology is valid but diagnostics are vague.
4. Add focused tests for amplifier label attachment on the known failing nets.
5. Ensure generated schematics remain human-readable: signal flow left to right,
   supplies vertical, lower-voltage/reference nets lower on the sheet, and
   labels placed at wire endpoints instead of over symbols.

Acceptance:

- The protected fixture no longer reports generated label/connectivity blockers
  unless a real unsupported topology remains.
- Any remaining schematic blocker names the exact label, coordinate, and
  expected net.
- Existing schematic layout readability tests still pass.

Review and commit:

- Run focused schematic writer, schematic validation, amplifier realization,
  and design workflow tests.
- Run `prism review staged`.
- Commit as `Clean up amplifier schematic label connectivity`.

## Phase 3: Enable Explicit Routing Policy

Goal: replace stale route skipping with an explicit fixture/capability routing
decision.

Tasks:

1. Inspect the protected fixture metadata and generated request for
   `skip_routing` or equivalent routing gates.
2. Determine whether routing is now safe to attempt by checking:
   - all required route endpoints bind to concrete pads or explicit external
     endpoints;
   - required nets are known;
   - unsupported amplifier topology blockers are absent;
   - board envelope and placement evidence are clean.
3. Patch route-policy decision code or fixture metadata so the protected
   amplifier:
   - runs routing when the request is complete; or
   - reports a current, precise route-policy blocker when routing must not run.
4. Add tests for:
   - routing enabled for the protected amplifier when endpoints are complete;
   - routing blocked when a required endpoint is removed or ambiguous;
   - unrelated expected-fail fixtures retaining their existing route policy.

Acceptance:

- `skip_routing` is no longer a stale, implicit blocker for the protected
  amplifier.
- Route-policy evidence is visible in the promotion report.
- Routing still fails closed on missing or ambiguous endpoints.

Review and commit:

- Run focused routing policy, promotion, and design workflow tests.
- Run `prism review staged`.
- Commit as `Enable amplifier routing policy evidence`.

## Phase 4: Classify Required Amplifier PCB Nets

Goal: make PCB structural validation understand amplifier required-net
connectivity and partial route evidence.

Tasks:

1. Inventory the protected amplifier PCB nets emitted after Phase 3.
2. Classify each relevant net as:
   - required inter-block;
   - block-local;
   - supply/reference;
   - output/load;
   - external interface;
   - explicitly allowed unrouted;
   - blocking if unrouted.
3. Patch block metadata, design workflow net classification, route-tree
   evidence, or PCB validation so same-net connectivity is evaluated against
   this classification.
4. Preserve local-route evidence from amplifier blocks and ensure it contacts
   generated pads on the same canonical net.
5. Add tests that assert partial routes report both connected and missing pad
   sets.
6. Verify same-net completion is never inferred from placeholder geometry or
   approximate overlap.

Acceptance:

- Required amplifier nets have explicit route/connectivity status.
- Single-pad and external-interface nets do not produce false unrouted blockers.
- Incomplete required nets produce precise blocking diagnostics.

Review and commit:

- Run focused board validation, route-tree, amplifier block, and design workflow
  tests.
- Run `prism review staged`.
- Commit as `Classify amplifier required PCB nets`.

## Phase 5: Hand Off to KiCad Evidence

Goal: make KiCad ERC/DRC the next authority once writer-level schematic and PCB
structural evidence is sufficiently clean.

Tasks:

1. Run the protected fixture after Phases 2-4 and capture the first remaining
   blocker.
2. If writer-level structural validation is clean enough, update promotion
   classification so optional KiCad ERC/DRC evidence is requested when
   configured.
3. If KiCad CLI is unavailable, ensure reports state that KiCad evidence was not
   run and keep readiness aligned with metadata.
4. If KiCad CLI is available, run the fixture once and record deterministic
   ERC/DRC evidence in the promotion report.
5. Ensure KiCad failures are reported as KiCad evidence, not collapsed into
   generic structural validation blockers.

Acceptance:

- The protected amplifier reaches optional KiCad evidence when writer-level
  blockers are resolved.
- Missing KiCad CLI remains non-flaky and clearly reported.
- KiCad ERC/DRC failures, if present, become the documented first blocker.

Review and commit:

- Run focused KiCad-backed promotion tests that do not require KiCad CLI.
- Optionally run KiCad CLI evidence locally if configured.
- Run `prism review staged`.
- Commit as `Hand amplifier validation to KiCad evidence`.

## Phase 6: Metadata, Docs, and Regression

Goal: make project status, fixture metadata, and regression coverage match the
new behavior.

Tasks:

1. Update `class_ab_headphone_protected.metadata.json` known gaps and expected
   stages.
2. Update KiCad-backed example docs.
3. Update `README.md` and `specs/ROADMAP.md` where the current amplifier status
   changes.
4. Add or update tests that lock the new status:
   - no stale placement/endpoint/project-write/writer-correctness blockers;
   - no false generated-label blockers;
   - explicit route-policy evidence;
   - required-net classification evidence;
   - KiCad evidence handoff when available or correctly skipped when absent.
5. Run the full repository test suite with a repository-local Go build cache:

   ```sh
   mkdir -p .cache/go-build
   GOCACHE=$(pwd)/.cache/go-build go test ./...
   ```

Acceptance:

- Metadata and docs agree on the first remaining amplifier blocker.
- Full tests pass.
- Worktree contains only intentional changes.

Review and commit:

- Run `go test ./...` with the local cache.
- Run `prism review staged`.
- Commit as `Update amplifier validation routing status`.
