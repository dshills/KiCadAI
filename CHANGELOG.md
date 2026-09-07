# Changelog

All notable user-visible changes are recorded here. KiCadAI follows semantic
versioning for its documented CLI and versioned input contracts.

## 1.0.1 - 2026-09-07

### Security

- Build release binaries with Go 1.26.8 instead of 1.23.12 and update
  `golang.org/x/text` to 0.39.0 and `golang.org/x/sys` to 0.44.0 to address
  known vulnerability findings. Source builds now require Go 1.26.8 or newer;
  released binaries do not require a Go installation.
- Pin source, local verification, and release CI to the same Go patch by
  default. Add a separate vulnerability gate using pinned `govulncheck` and
  the current official vulnerability database.

### Fixed

- Authenticate release commit metadata against the checked-out source, compile
  the intended repository even when invoked from another directory, and
  reject malformed version, timestamp, and worker-count inputs.
- Correct experimental V21 structural certification, finalized evidence hashes,
  critical-obligation preservation, initial-evaluation authentication, and
  resource/cancellation checks. Previously accepted invalid inputs may now
  fail closed. Maintenance revision 1 preserves historical evidence separately.
- Corrected V21 has not completed a new frozen public evaluation. Its original
  advancement report does not validate the repaired evaluator; V21 remains
  experimental and excluded from the v1 supported surface.
- Run the historical V8 negative-admission regression in the existing dedicated
  case-sensitive-filesystem audit lane, preserving frozen filenames and bytes.

### Changed

- Add experimental, version-isolated analysis/model/solver admission for
  open-topology generation. Required analyses, exact reviewed component and
  harness models, normalized parameters, source records, and immutable solver
  definitions are hash-bound before search or numerical evaluation.
- Add stable fail-closed diagnostics for missing/incompatible models, missing
  or unsupported analyses, unavailable solvers, solver/model incompatibility,
  and invalid model parameters. No implicit model substitution is permitted.
- Add strict admission-artifact decoding, tamper validation, deterministic
  replay tests, and public admitted/refused evidence examples. V19 remains
  immutable and retired. The frozen V20 evaluation preserved V18's admitted
  pass and advanced the selected model-availability leaf to a later blocker.
- Deterministically shard and authenticate the bounded coverage suite, with
  exact test/package inventory checks and content-addressed proof reuse.
- Upgrade official GitHub Actions pins to verified Node.js 24-compatible
  revisions.
- Preserve the stable v1 CLI, schema, artifact, and supported-circuit boundaries.
  Experimental V20/V21 work does not add an admitted v1 generation capability.

## 1.0.0 - 2026-09-02

### Changed

- Promoted `v1.0.0-rc.2` to the first stable release after a nine-day
  stabilization soak with successful pull-request, `main`, and release
  workflows.
- Finalization changes only release identity and documentation; production
  code is unchanged and the supported capability surface remains functionally
  identical to RC2.

### Safety

- The bounded pass-or-refuse contract remains unchanged. v1.0.0 does not claim
  arbitrary-circuit generation, fabrication approval, unrestricted routing,
  or suitability for mains/high-energy designs.

## 1.0.0-rc.2 - 2026-08-24

### Added

- Reproducible routing, placement, writer, comparison, simulation, synthesis,
  and topology-cache benchmarks through `make performance-report`.
- Explicit fast, bounded, and exhaustive local verification tiers.
- A process-wide worker budget controlled by `KICADAI_MAX_WORKERS`.

### Changed

- Closed-loop evidence hashes stream canonical JSON directly into SHA-256
  while retaining byte-identical digests.
- Resistor-path discovery and noise-transfer solves reuse bounded topology and
  scratch state to reduce nonlinear simulation allocation churn.
- Independent promotion scenarios and release targets run with bounded
  concurrency while results, diagnostics, manifests, and checksums retain
  canonical order.
- The RC1 circuit-generation capability and refusal boundaries remain frozen;
  RC2 adds no circuit-generation capability.

## 1.0.0-rc.1 - 2026-08-20

### Breaking pre-v1 CLI transition

- `kicadai version` now reports the KiCadAI application build, following the
  conventional CLI meaning. Scripts that used the pre-v1 command as a live IPC
  probe must migrate to `kicadai kicad-version`. The equivalent global
  `kicadai --version` form is also available without a KiCad connection.

### Added

- A frozen v1 support and compatibility contract for the bounded
  pass-or-refuse CLI.
- Reproducible CGO-free macOS and Linux release binaries for Intel/AMD64 and
  ARM64, with a release manifest and SHA-256 checksums.
- `kicadai version` and `kicadai --version` application build identity,
  including version, commit, build date, Go version, and platform.
- `kicadai kicad-version` for the existing live KiCad API version probe.
- Release smoke and byte-for-byte reproducibility checks.

### Changed

- `make install` now installs the built binary into `~/.local/bin` by default;
  set `INSTALL_DIR` to override it.
- KiCad 10.0.3 is explicitly the supported v1 promotion reference. KiCad 9 is
  experimental for promotion claims.

### Safety

- v1 remains bounded and fail-closed. It does not claim arbitrary-circuit
  generation, unrestricted routing, fabrication approval, or suitability for
  mains/high-energy designs.
- The bounded V19 public evaluation completed deterministically but failed its
  advancement and preservation gates. V19 is permanently retired and excluded
  from the v1 supported surface; V18 remains the latest admitted public
  capability.
