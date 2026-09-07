# KiCadAI v1.0.1

This security maintenance release includes the merged security, release
provenance, input-validation, and experimental evaluator correctness fixes.
Upgrade from v1.0.0 to receive the patched binaries. The documented v1 CLI,
schema, artifact, and supported-circuit boundaries remain unchanged.

## Security and source-build compatibility

- Official binaries use Go 1.26.8, replacing Go 1.23.12 in v1.0.0.
- `golang.org/x/text` is updated to 0.39.0 and `golang.org/x/sys` to 0.44.0
  to address known vulnerability findings.
- Source builds now require Go 1.26.8 or newer. Canonical local and release
  builds use the exact patch in `go.mod`; binaries require no Go installation.
- A pinned vulnerability scanner checks source dependencies against the current
  official Go vulnerability database. Release verification also scans all four
  platform binaries. A clean scan is time-bound, not a guarantee of security.

Release builds authenticate the embedded commit against the checked-out source,
compile from the intended repository root, and reject malformed version,
timestamp, and concurrency inputs. The commit timestamp supplies reproducible
build metadata. Existing tags and published artifacts are not rewritten.

## Correctness and experimental work

V20 adds opt-in, authenticated analysis/model/solver admission with typed
fail-closed diagnostics. Its preserved frozen evaluation does not enlarge the
stable v1 support contract.

V21 maintenance revision 1 corrects unsound structural certificates, stale
evidence hashes, critical-obligation regressions, tampered initial evaluations,
and late resource/cancellation checks. Security and correctness tightening may
refuse inputs that older builds accepted incorrectly.

**Corrected V21 has not completed a new frozen public evaluation.** The original
evaluation remains immutable historical evidence; its six-case advancement
claim is not validation of the repaired evaluator. V21 remains experimental,
and reevaluation is a separate milestone. V19 remains retired; V18 remains the
latest admitted public capability.

The release also incorporates deterministic bounded-test coverage sharding,
authenticated proof reuse, and updated official GitHub Actions pins. These
changes do not expand supported generation behavior.

## Installation and limits

Assets include CGO-free macOS and Linux binaries for AMD64 and ARM64,
`RELEASE_MANIFEST.json`, and `SHA256SUMS`. Verify checksums before installing;
see the [installation guide](docs/installation.md). `kicadai version` reports
application, commit, build-date, toolchain, and platform identity;
`kicadai kicad-version` remains the connected-KiCad probe.

KiCad 10.0.3 remains the installed-KiCad reference. This release is not an
arbitrary-circuit generator or a fabrication approval system. Human electrical,
thermal, mechanical, regulatory, and manufacturing review remains required.

See the [support contract](SUPPORT.md), [security policy](SECURITY.md),
[changelog](CHANGELOG.md), and [September 6 review](docs/project-review-2026-09-06.md).
