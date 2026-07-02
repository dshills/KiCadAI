# Component Hint Enforcement Implementation Plan

Date: 2026-07-02

## Implementation Rules

- Implement phases in order.
- Commit each phase independently after validation and `prism review staged`.
- Keep default tests hermetic. Do not require KiCad CLI.
- Prefer existing placement, routing, retry, component-selection, and rationale
  data structures over new parallel systems.
- Unsupported or ambiguous hints must produce evidence, not silent behavior.
- Do not make hints stronger than explicit block rules, locked placement, or
  user-provided request constraints.

## Phase 1: Hint Inventory And Workflow Evidence Shape

Objective: expose selected component hints in workflow-local evidence without
changing placement or routing behavior yet.

Tasks:

1. Audit checked-in catalog `placement_hints` and `routing_hints`.
2. Document the supported first subset under
   `specs/component-hint-enforcement/AUDIT.md`.
3. Add selected component hint fields to `ComponentSelectionEntry`:
   - `placement_hints`;
   - `routing_hints`.
4. Add compact selected-component summary evidence:
   - placement hint count;
   - routing hint count;
   - supported/unsupported counts if classification is available.
5. Add tests proving regulator and AP2112K selected component summaries include
   catalog hints.

Validation:

- `go test ./internal/components ./internal/designworkflow`
- `prism review staged`

Commit:

- `Expose selected component hints in workflow`

## Phase 2: Hint Normalization Model

Objective: convert raw catalog hints into deterministic workflow hint evidence.

Tasks:

1. Add a small workflow-local model for normalized component hints:
   - hint ID;
   - instance ID;
   - block ID;
   - component role;
   - component ID;
   - selected ref when known;
   - hint kind;
   - target role/net role;
   - value/unit;
   - status;
   - message.
2. Implement normalization from `ComponentSelectionEntry`.
3. Classify supported kinds:
   - placement: `near`, `edge`, `keepout`;
   - routing: `net_class`, `tie`, `no_connect`.
4. Mark all unknown kinds `unsupported`.
5. Add deterministic ordering and deduplication.

Validation:

- `go test ./internal/designworkflow`
- `prism review staged`

Commit:

- `Normalize component hint evidence`

## Phase 3: Placement Hint Consumption

Objective: consume supported placement hints where selected refs and roles are
available.

Tasks:

1. Map selected component roles to generated schematic/PCB refs using existing
   component-selection and transaction facts.
2. Convert `near` hints into proximity checks or placement constraints when
   both source and target refs are known.
3. Surface skipped statuses for missing refs, missing target roles, fixed or
   locked components, and unsupported target forms.
4. Preserve existing block-local placement constraints as authoritative.
5. Add tests for:
   - regulator output capacitor near regulator;
   - missing target ref produces `skipped_missing_ref`;
   - unsupported placement hint does not mutate placement.

Validation:

- `go test ./internal/placement ./internal/designworkflow`
- `prism review staged`

Commit:

- `Apply component placement hints`

## Phase 4: Routing Hint Consumption

Objective: consume supported routing hints where net-role evidence exists.

Tasks:

1. Map selected component routing hints to known block-local or workflow net
   roles.
2. Convert `net_class` hints into route width/net-role requirements where the
   current routing model supports them.
3. Record `tie` and `no_connect` as `satisfied_by_block` when block operations
   already emit the tie or no-connect.
4. Emit `skipped_missing_net_role` when the workflow cannot map a net role.
5. Add tests for:
   - regulator power net-class width evidence;
   - AP2112K EN tie satisfied by block;
   - AP2112K NC no-connect satisfied by block;
   - unsupported routing hint remains evidence-only.

Validation:

- `go test ./internal/routing ./internal/designworkflow ./internal/blocks`
- `prism review staged`

Commit:

- `Apply component routing hints`

## Phase 5: Validation And Diagnostics

Objective: make hint outcomes actionable in workflow issues and summaries.

Tasks:

1. Add deterministic component-hint diagnostics with:
   - component ID;
   - role;
   - ref;
   - hint kind;
   - target role/net role;
   - requested value/unit;
   - observed value when available;
   - status.
2. Block only required hint failures at stricter acceptance levels; keep normal
   unsupported/skipped hints as warnings.
3. Add workflow summary counters:
   - total hints;
   - enforced;
   - satisfied by block;
   - skipped;
   - unsupported;
   - failed.
4. Add regression tests for warning/blocking severity behavior.

Validation:

- `go test ./internal/designworkflow ./cmd/kicadai`
- `prism review staged`

Commit:

- `Report component hint enforcement diagnostics`

## Phase 6: CLI And Rationale Output

Objective: make hint enforcement visible to AI callers.

Tasks:

1. Expose component hint evidence in `design create` workflow JSON.
2. Add rationale evidence records for component placement/routing hints.
3. Add CLI tests proving:
   - selected regulator hint evidence appears in workflow output;
   - AP2112K tie/no-connect are explained;
   - unsupported hints are reported without blocking connectivity.
4. Keep JSON output deterministic.

Validation:

- `go test ./internal/rationale ./cmd/kicadai`
- `go test ./...`
- `prism review staged`

Commit:

- `Expose component hint enforcement to AI callers`

## Phase 7: Documentation And Roadmap Update

Objective: close the project with accurate docs and roadmap status.

Tasks:

1. Update `docs/component-intelligence.md`.
2. Update `docs/layout-routing.md`.
3. Update `docs/kicadai-agent-skill.md`.
4. Update `specs/ROADMAP.md` Priority 1 status and remaining work.
5. Mention that hint enforcement is layout evidence, not fabrication proof.
6. Run the full suite.

Validation:

- `go test ./...`
- `git diff --check`
- `prism review staged`

Commit:

- `Document component hint enforcement`

## Completion Criteria

- Catalog hints are visible in workflow and rationale output.
- Supported placement/routing hints affect downstream checks or constraints
  when enough role/ref/net evidence exists.
- Unsupported or ambiguous hints produce deterministic evidence.
- AP2112K EN/NC and regulator power-net hints are explainable to AI callers.
- Existing default tests remain KiCad-independent.
