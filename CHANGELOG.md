# Changelog

All notable user-visible changes are recorded here. KiCadAI follows semantic
versioning for its documented CLI and versioned input contracts.

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
