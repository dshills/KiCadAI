# KiCadAI v1 Support Contract

KiCadAI v1 is a bounded, evidence-backed command-line system. It accepts
documented behavioral, intent, circuit-graph, and schematic-IR inputs; produces
native KiCad projects when every required gate passes; and returns structured
refusal evidence when a request is outside the supported envelope.

It is not an arbitrary-circuit oracle, a general autorouter, or an automatic
fabrication approval system. Generated projects require qualified human review
before construction or manufacture.

## Supported release platforms

Official v1 binaries are built with CGO disabled for:

- macOS 64-bit Intel and Apple silicon;
- Linux 64-bit x86 and ARM.

The release workflow uses Go 1.23.12 and publishes a manifest and SHA-256
checksums. Source builds require Go 1.23 or newer. Windows and other platforms
may build from source but are not release-supported in v1.

## KiCad compatibility

KiCad 10.0.3 is the v1 reference and installed-KiCad promotion version. It is
the supported version for claims involving ERC, strict DRC, native round-trip,
stock-library resolution, or reproducible promotion bundles.

KiCad 9 remains useful for live IPC and direct-file compatibility testing, but
it is experimental for v1 promotion claims because the complete release corpus
is not reproduced against it. A different KiCad version must not inherit the
10.0.3 evidence claim without a fresh version-bound validation run.

## Stable v1 surfaces

Within the v1 release line:

- documented CLI command and flag meanings remain compatible;
- documented versioned input schemas retain their meaning;
- machine-readable reports may add optional fields, but existing fields do not
  silently change meaning;
- unsupported schema versions, components, models, analyses, and physical
  constraints continue to fail closed;
- native KiCad output is supported only for the structures and acceptance
  lanes named by `kicadai capability generation` and the project status.

Experimental commands, explicit `--experimental` output, historical evaluator
internals, test fixtures, and packages under `internal/` are not public API.
The Go module name `kicadai` is intentionally an internal build identity: this
repository publishes a CLI, not an importable Go library. Install released
binaries or build from a clone rather than using `go install ...@version`.

## Supported electrical envelope

The exact runtime boundary is reported by:

```sh
kicadai capability generation
```

The admitted envelope includes reviewed slices of passive and active analog,
protected current output, low-energy nonlinear and switching behavior,
mixed-domain control, MCU support, power trees, sensors, interfaces, and
functional hierarchical four-layer generation. Every result remains bounded by
the available catalog, model provenance, analyses, safety limits, placement,
routing, writer, and KiCad evidence.

Mains and other high-energy safety systems, RF power, unrestricted high-speed
or dense boards, arbitrary parts and models, automatic compliance claims, and
automatic fabrication approval are unsupported.

The V19 causal-topology experiment is also explicitly unsupported in v1. Its
bounded public evaluation failed the frozen advancement and preservation gates
and was permanently retired. V18 is the latest admitted public capability.

Post-v1 analysis/model/solver admission work is experimental until its frozen
successor evaluation and preservation gates pass. Its additive evidence fields
and stricter early refusals do not expand the v1 supported electrical envelope.
Only bundled model provenance and reviewed project/configured overlays are
eligible; unreviewed SPICE files, provider-selected models or solvers, implicit
substitution, and unavailable solver backends remain unsupported.

## Compatibility changes

Breaking changes to a stable v1 CLI or schema require a new major version or a
new explicitly versioned contract. Security, correctness, and fail-closed
tightening may reject an input that earlier builds accepted incorrectly; such a
change must be recorded in the changelog.
