# Numerical conformance and workflow diagnostics

Items 4–6 are implemented with local acceptance: production Go numerical
conformance, compact failure capture and replay, and a supported-workflow
capability explorer. The [user guide](../../docs/numerical-conformance.md)
contains commands and interpretation; the [generated inventory](../../docs/capability-explorer.md)
contains supported bounds, tools, examples and evidence levels.

The [numerical report](evidence/report.json) records 12 conformant cases against
independent equations and ngspice 47: four DC, six AC and two transient. The
complete portable bundle includes every input, resolved plan, Go report,
generated SPICE netlist and external command result. Its manifest verifies
integrity, not authorship or physical accuracy.

```sh
make build
bin/kicadai --target specs/conformance-repro-explorer/evidence conformance verify
bin/kicadai --target specs/conformance-repro-explorer/evidence --output ./out/replay conformance replay
bin/kicadai capability explore
```

The [negative reproduction](reproduction-negative/reproduction.json) preserves
an intentionally unconfirmed session. Capture and replay must exit nonzero at
the confirmation stage while saving a verifiable diagnostic bundle:

```sh
bin/kicadai --target specs/conformance-repro-explorer/reproduction-negative repro verify
bin/kicadai --target specs/conformance-repro-explorer/reproduction-negative --output ./out/reproduced repro replay
```

[Acceptance](acceptance.json) records test commands, limitations, development
failures and review dispositions. Two full single-chunk Prism reviews used
Anthropic `claude-sonnet-5-5`; both found zero high/medium issues. The remaining
low-severity issues were corrected or explicitly dispositioned and locally
tested. [Source hashes](source-files.json) bind the changed implementation
files to this evidence; build identities correctly record a dirty development
snapshot rather than claiming a clean release.

The SHA256-pinned Ubuntu CI job is configured but has not run remotely. Native
ngspice and KiCad acceptance here was measured on the current macOS host.
These results establish bounded software agreement, not hardware validation.
