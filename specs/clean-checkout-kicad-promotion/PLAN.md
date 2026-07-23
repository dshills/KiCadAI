# Clean-Checkout KiCad Promotion Implementation Plan

Date: 2026-07-23
Status: In progress
Specification: [`SPEC.md`](SPEC.md)

## 1. Implementation rules

- Keep the existing promotion matrix as the scenario inventory.
- Add no scenario-ID, filename, circuit-family, component, net, coordinate, or
  board-size decisions to production code.
- Use strict JSON decoding with unknown-field rejection for versioned inputs
  and evidence.
- Keep all offline tests network-free and independent of installed KiCad.
- Treat unavailable, mismatched, failed, or skipped KiCad evidence as a
  promotion failure.
- Use argument-vector process execution with bounded timeouts.
- Normalize only machine-local and run-local values; never erase meaningful
  project or validation differences.
- Run focused tests after each edit and the affected installed-KiCad fixture
  after each behavior phase.
- Review staged changes with `prism review staged` before every phase commit,
  resolve material findings, commit, push, and verify GitHub Actions before a
  dependent phase. Prism is a contributor review prerequisite required by this
  goal, not part of the runtime KiCad promotion toolchain or clean-checkout
  reproduction command.

## 2. Phase 0: Freeze contract and baseline

Deliver this specification and plan. Record the existing matrix, manual
toolchain variables, absence of an installed promotion artifact job, and the
current passing external-review evidence as the baseline.

Acceptance:

- scope, schemas, failure policy, normalization boundary, and CI split are
  explicit;
- no production behavior changes; and
- Prism review, commit, push, and CI pass.

Commit: `Specify clean-checkout KiCad promotion`

## 3. Phase 1: Lock and resolve the toolchain

Add the versioned KiCad promotion lock and an internal toolchain package.
Implement strict parsing, host/platform selection, explicit/discovered path
resolution, exact version validation, deterministic library identity hashing,
and immutable bootstrap planning. Add a small command surface for `resolve`
and `bootstrap`; bootstrap must verify the locked digest before extraction or
container use.

Acceptance:

- fake toolchains prove resolution and mismatch behavior offline;
- explicit and discovered resolution share one validator;
- the local KiCad installation resolves without hand-set library paths;
- the locked bootstrap is immutable and checksum/digest verified; and
- all existing tests remain green.

Commit: `Pin and resolve KiCad promotion toolchain`

## 4. Phase 2: Implement manifest-driven two-run promotion

Add the promotion orchestrator package and command. Strictly load the existing
matrix, translate lanes through a fixed generic lane registry, build the
repository CLI, and execute every positive scenario twice in isolated roots.
Capture structured command evidence and require the matrix artifact and gate
contract.

Acceptance:

- unit tests use a fake CLI and fake toolchain;
- reordered or malformed matrices fail deterministically;
- unknown lanes fail closed;
- no scenario-specific branch exists;
- real compact LED promotion passes with installed KiCad; and
- USB-C I2C protected evidence remains green.

Commit: `Run promotion matrix through generic orchestrator`

## 5. Phase 3: Normalize and compare evidence

Implement a narrow normalized file inventory for generated KiCad projects and
creation evidence. KiCad files must use the existing
`internal/kicadfiles/roundtrip.NormalizeBytes` semantic normalizer that already
backs strict round-trip evidence; this phase must not introduce another KiCad
field mask. Canonical JSON may replace only schema-declared machine-local
checkout, toolchain, and isolated output roots. Deterministically generated
UUIDs, generator versions, and all other semantic fields remain compared.

Acceptance:

- semantic mutations are detected;
- host-path and isolated-root differences normalize;
- bundles are platform-specific and record their exact lock platform;
- unsafe paths and all symlinks fail closed;
- every scenario records a pass comparison; and
- installed KiCad promotion still passes.

Commit: `Require deterministic promotion replay`

## 6. Phase 4: Build and verify content-addressed bundles

Add canonical manifest generation, sorted SHA-256 inventory, top-level checksum,
content-addressed final directory, and standalone verification. Copy request
and run evidence only from regular files and reject symlinks. Reject extra/missing files,
tampering, gate skips, or directory-address mismatch.

Acceptance:

- repeated fake runs produce the same manifest digest;
- tamper tests cover manifest, request, project, report, and checksum changes;
- verification needs neither KiCad nor network access; and
- a local installed-KiCad bundle verifies successfully.

Commit: `Package verifiable promotion evidence`

## 7. Phase 5: Wire clean-checkout command and CI publication

Add one Make target and a thin bootstrap wrapper. Add an offline contract test
to normal CI and a configured installed-KiCad workflow/job that runs the target,
verifies the bundle, and uploads the content-addressed directory. Pin every
third-party action by full commit SHA.

Acceptance:

- `make clean-checkout-promotion` is the exact reproduction command;
- ordinary offline CI does not download or require KiCad;
- configured promotion CI cannot skip the installed lane;
- uploaded artifact name includes the source commit; and
- workflow syntax and repository tests pass.

Commit: `Publish clean-checkout promotion bundles`

## 8. Phase 6: Documentation, full corpus, and audit

Update development, validation, external-review, project-status, and AI
readiness documentation. Run the full test/lint suite, external-review matrix,
installed KiCad promotion, writer and round-trip gates, USB-C, power/interface,
function-level, amplifier, and ESP32 evidence. Record exact commands, versions,
bundle digest, Prism results, commit hashes, and final Actions run in `AUDIT.md`.

Acceptance:

- a fresh checkout follows one command without manual library paths;
- all required evidence is present and independently verifies;
- no stale documentation claims manual setup for promotion;
- Prism has no unresolved material finding;
- the final commit is pushed; and
- GitHub Actions is green.

Commit: `Finalize clean-checkout promotion audit`
