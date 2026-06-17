# Circuit Block Verification Harness Implementation Plan

## 1. Objective

Implement the Circuit Block Verification Harness described in `SPEC.md`.

The end state is a deterministic suite that proves each built-in circuit block
against semantic expectations, generated KiCad project correctness, and optional
KiCad ERC/DRC evidence.

This plan should be implemented in small, reviewable phases with Prism review
and a commit between phases.

## 2. Implementation Rules

- Build on existing `internal/blocks`, `internal/writercorrectness`,
  `internal/libraryresolver`, `internal/kicadfiles/checks`, and
  `internal/reports`.
- Do not introduce a second block model or a second KiCad writer.
- Keep default tests independent of external KiCad repositories and KiCad CLI.
- Prefer semantic assertions over raw `.kicad_sch` or `.kicad_pcb` goldens.
- Every phase must include tests.
- Run `gofmt` on edited Go files.
- Run focused tests for touched packages after each phase.
- Run `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` before the final
  phase commit.
- Run `prism review staged` before each phase commit and address material
  findings.
- Commit between phases.

## 3. Phase 1: Verification Manifest Model

### Goal

Add the manifest schema and validation model without changing block generation.

### Work

- Add `internal/blocks/verification` package.
- Define:
  - `Manifest`;
  - `RequestSpec`;
  - `Expected`;
  - `ExpectedComponent`;
  - `ExpectedPort`;
  - `ExpectedNet`;
  - `ExpectedPin`;
  - `ExpectedPCB`;
  - `ExpectedPlacement`;
  - `ExpectedWriter`;
  - `ExpectedERCDRC`.
- Add evidence level constants:
  - `definition_only`;
  - `schematic_verified`;
  - `transfer_verified`;
  - `pcb_verified`;
  - `erc_drc_verified`;
  - `reference_verified`.
- Add `LoadManifest(path)` and `ValidateManifest(manifest, registry)`.
- Validate:
  - case ID format;
  - block ID existence;
  - request params;
  - duplicate refs;
  - duplicate net names;
  - malformed expected pins;
  - invalid evidence levels.

### Tests

- Valid LED manifest loads and validates.
- Unknown block ID blocks.
- Duplicate expected net blocks.
- Invalid case ID blocks.
- Invalid block params block.
- JSON marshal shape is deterministic.

### Acceptance Criteria

- Manifest model compiles and validates independently of generation.
- Existing block tests still pass.

### Commit Message

```text
Add block verification manifest model
```

## 4. Phase 2: Semantic Assertion Engine

### Goal

Verify generated block schematic semantics without writing projects.

### Work

- Add `RunCase` skeleton with stages:
  - manifest;
  - request validation;
  - instantiate;
  - semantic assertions.
- Instantiate blocks through the public block registry/API.
- Extract actual semantic summary:
  - generated refs;
  - component roles;
  - symbol IDs;
  - footprint IDs;
  - ports;
  - nets;
  - net pin memberships.
- Implement assertions for expected:
  - components;
  - ports;
  - nets;
  - pin memberships;
  - footprint assignments.
- Add stable issue paths:
  - `verification.<case>.components.<role>`;
  - `verification.<case>.nets.<name>`;
  - `verification.<case>.pins.<ref>.<pin>`.

### Tests

- LED default semantic case passes.
- Missing expected component blocks.
- Wrong expected symbol blocks.
- Missing net pin membership blocks.
- Extra generated content warns only in non-strict mode.

### Acceptance Criteria

- A case can prove schematic-level block semantics from Go tests.

### Commit Message

```text
Verify block schematic semantics
```

## 5. Phase 3: Built-In Manifest Corpus

### Goal

Add one default verification manifest for every built-in block.

### Work

- Add fixtures under:

```text
internal/blocks/testdata/verification/<case-id>/manifest.json
```

- Initial cases:
  - `led_indicator_default`;
  - `connector_breakout_4pin`;
  - `voltage_regulator_3v3`;
  - `i2c_sensor_pullups`;
  - `opamp_gain_stage_noninverting`;
  - `usb_c_power_5v_sink`;
  - `mcu_minimal_basic`.
- Use current block request examples as the source of default params where
  practical.
- Keep expectations honest:
  - use `schematic_verified` for schematic-only evidence;
  - use `transfer_verified` or `pcb_verified` only where PCB evidence is
    actually asserted.
- Add suite loader that discovers manifests in stable sort order.

### Tests

- Suite discovery returns all seven cases.
- Every built-in block has at least one manifest.
- Every manifest validates.
- Unsupported or partial evidence is explicit, not inferred.

### Acceptance Criteria

- The project has a checked-in block verification corpus.

### Commit Message

```text
Add built-in block verification manifests
```

## 6. Phase 4: Project Writing And Writer Correctness Integration

### Goal

Run generated block projects through writer correctness.

### Work

- Extend `RunOptions` with:
  - output directory;
  - overwrite;
  - keep artifacts;
  - writer correctness options;
  - library roots/cache.
- Add project writing stage using existing block project/design workflow helpers.
- Run `writercorrectness.Validate` for cases that request writer evidence.
- Map writer issues back to case ID and block ID where possible.
- Add artifact records for retained generated projects.
- Respect expected writer behavior:
  - expected ok;
  - allow unrouted;
  - require resolver;
  - require round-trip.

### Tests

- LED project case passes writer correctness.
- Intentionally broken expected writer case blocks.
- Missing output without project-writing request is skipped, not failed.
- Writer issues include case and block context.

### Acceptance Criteria

- Block verification can prove generated KiCad file correctness through the
  existing writer gate.

### Commit Message

```text
Run writer checks for block verification
```

## 7. Phase 5: PCB Assertions

### Goal

Assert block-level PCB evidence beyond generic writer correctness.

### Work

- Extract actual PCB summary from generated output:
  - placed refs;
  - footprint IDs;
  - coordinates;
  - pad nets;
  - routes/tracks;
  - zones;
  - unrouted nets.
- Implement expected PCB assertions:
  - placement within tolerance;
  - expected footprint;
  - required pad net memberships;
  - required local route present;
  - required zone present;
  - forbidden wrong-net copper absent.
- Keep placement tolerances explicit.
- Treat PCB assertions as skipped for schematic-only evidence levels.

### Tests

- LED PCB assertion passes for expected placements/routes.
- Wrong footprint expectation blocks.
- Wrong pad-net expectation blocks.
- Missing required route blocks.
- Placement outside tolerance warns or blocks based on manifest setting.

### Acceptance Criteria

- Blocks that claim PCB evidence have block-local PCB checks, not only parse
  checks.

### Commit Message

```text
Assert block PCB verification evidence
```

## 8. Phase 6: Optional ERC/DRC Evidence Hooks

### Goal

Allow manifests to request KiCad-backed ERC/DRC evidence without making default
tests depend on KiCad.

### Work

- Integrate with `internal/kicadfiles/checks`.
- Extend `RunOptions` with:
  - KiCad CLI path;
  - require ERC;
  - require DRC;
  - allowlist path;
  - timeout;
  - artifact options.
- Add skipped stage result when KiCad is unavailable and not required.
- Add blocking issue when KiCad is required but unavailable.
- Support expected violations for intentionally bad fixtures.

### Tests

- Optional ERC/DRC missing KiCad returns skipped.
- Required ERC missing KiCad blocks.
- Mocked check result with unallowlisted violation blocks.
- Mocked passing check promotes evidence to `erc_drc_verified`.

### Acceptance Criteria

- KiCad-backed validation can be layered into block evidence when available.

### Commit Message

```text
Add optional ERC DRC block evidence
```

## 9. Phase 7: CLI Command

### Goal

Expose the verification harness to humans and AI agents.

### Work

- Add command:

```sh
kicadai --json block verify --case <manifest.json>
kicadai --json block verify --suite <dir>
kicadai --json block verify --builtins
```

- Add flags:
  - `--output`;
  - `--overwrite`;
  - `--keep-artifacts`;
  - `--artifact-dir`;
  - `--update-goldens`;
  - `--require-writer`;
  - `--require-erc`;
  - `--require-drc`;
  - `--allow-unrouted`;
  - existing library resolver flags.
- Return `reports.Result` JSON.
- Exit non-zero on blocking verification failures.

### Tests

- `block verify --builtins` returns all cases.
- `block verify --case` returns one case.
- Missing case path returns structured issue.
- Blocking case exits non-zero with JSON report.
- Non-JSON invocation behavior is consistent with other block commands.

### Acceptance Criteria

- AI agents can run block verification independently of Go tests.

### Commit Message

```text
Expose block verification CLI
```

## 10. Phase 8: Golden Reports

### Goal

Lock down report shape and failure behavior.

### Work

- Add golden JSON outputs for:
  - built-in suite summary;
  - passing LED case;
  - intentionally blocked case;
  - skipped ERC/DRC evidence.
- Add update flag if not covered by CLI phase.
- Normalize paths and volatile timestamps.
- Prefer semantic report goldens over raw KiCad file goldens.

### Tests

- Golden reports match checked-in expectations.
- Update flag refreshes reports.
- Path normalization is cross-platform.

### Acceptance Criteria

- Report regressions are caught by default tests.

### Commit Message

```text
Add block verification golden reports
```

## 11. Phase 9: Design Workflow Evidence Integration

### Goal

Let `design create` cite block verification evidence for blocks used in a
generated project.

### Work

- Add optional lookup from block ID to strongest passing built-in verification
  case.
- Include block evidence level in design workflow stage output.
- Warn when a requested block has no matching verification case.
- Do not block draft designs solely because verification evidence is missing.
- Block stronger acceptance levels when a block claims fabrication readiness
  without verified evidence.

### Tests

- Design workflow report includes evidence level for LED block.
- Unknown custom block reports missing evidence warning.
- Fabrication-candidate acceptance blocks unverified block readiness claim.

### Acceptance Criteria

- AI-facing workflow output can explain why each block is trusted, partial, or
  unsupported.

### Commit Message

```text
Report block verification in design workflow
```

## 12. Phase 10: Documentation And Roadmap Update

### Goal

Document how to use and extend the verification harness.

### Work

- Add `docs/circuit-block-verification.md`.
- Update README with:
  - command examples;
  - evidence levels;
  - default vs optional KiCad-backed checks;
  - how to add a manifest;
  - how to update goldens.
- Update `specs/ROADMAP_GAP.md`:
  - mark Circuit Block Verification Harness implemented or in progress;
  - identify the next recommended gap.

### Tests

- Documentation examples use real command names and paths.
- Full `go test ./...` passes.

### Acceptance Criteria

- A developer can add a new block verification case without reading the
  implementation.

### Commit Message

```text
Document block verification harness
```

## 13. Final Completion Checklist

- All built-in blocks have at least one manifest.
- Manifest validation catches malformed cases.
- Semantic assertions verify refs, components, ports, nets, pins, symbols, and
  footprints.
- Writer correctness runs for project-writing cases.
- PCB assertions run for PCB-evidence cases.
- Optional ERC/DRC stages skip by default and block when required.
- CLI can run a case and the built-in suite.
- Golden report tests pass.
- README/docs explain usage and limits.
- `GOCACHE=/private/tmp/kicadai-go-cache go test ./...` passes.
- Prism has no unresolved high-severity findings.

