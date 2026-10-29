# Schematic review catalog testing and NetForm integration

Items 7–10 have local software acceptance. The [workflow guide](../../docs/project-review-and-integration.md)
covers read-only existing-project review, quarantined component regression
kits, installation diagnostics, the existing offline board-family lane and the
versioned [NetForm client](../../integrations/netform-client/README.md).

The [schematic review](schematic-review/report.html) records supported internal
checks and native KiCad ERC on an immutable snapshot. The original source
bundle remained unchanged. Its [JSON report](schematic-review/report.json)
and manifest preserve the exact reviewed source and verification results.

The [component test report](component-self-test/report.json) records a passing
candidate-bound DC regression with installed KiCad library evidence. The
Example Analog AX101 is a synthetic test fixture, not a real manufacturer
qualification; its status remains quarantined. Candidate, source document,
test kit, resolved plan and simulation observations are retained.

```sh
make build
bin/kicadai --target specs/review-catalog-doctor-netform/schematic-review review verify
bin/kicadai --target specs/review-catalog-doctor-netform/component-self-test component self-test verify
bin/kicadai doctor
make install-experience
```

[Acceptance](acceptance.json) records tests, negative controls, development
failures, review dispositions and platform limits. Two complete single-chunk
Prism reviews used Anthropic `claude-sonnet-5-5`; both found zero high/medium
issues. The three low-severity findings were fixed and locally retested.
[Source hashes](source-files.json) identify the changed implementation and
documentation files tested in this development snapshot.

Four macOS/Linux release-format targets were built, and the macOS ARM64
candidate passed expanded first-use smoke tests. These used the explicit
dirty-build override and are not publishable release evidence. Remote CI and
other host-platform execution remain unverified. NetForm's GUI and its
uncommitted working tree are unchanged; the narrow adapter and standalone
client are available for later GUI integration. No hardware validation is
claimed and no release or PR was published.
