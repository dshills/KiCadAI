# KiCadAI v1.0.0-rc.1

KiCadAI v1.0.0-rc.1 is the first release candidate for the bounded,
evidence-backed KiCad generation CLI.

The release turns documented behavior and structured circuit intent into
deterministic native KiCad candidates when the required catalog, simulation,
physical, writer, ERC/DRC, and round-trip evidence is available. Unsupported,
unsafe, ambiguous, or unproven requests produce structured refusal evidence
instead of an unqualified project.

Highlights:

- reviewed analog, protected-current, low-energy switching, mixed-domain, MCU,
  sensor, interface, power-tree, amplifier, and hierarchical physical slices;
- native KiCad schematic, PCB, project, and local-library output;
- deterministic search, repair, placement, routing, and replay evidence;
- strict fail-closed capability and safety boundaries;
- checksummed macOS and Linux binaries for AMD64 and ARM64;
- application build identity separate from connected-KiCad version reporting.

KiCad 10.0.3 is the reference release toolchain. This release candidate is not
an arbitrary-circuit generator or a fabrication approval system. Human
electrical, thermal, mechanical, regulatory, and manufacturing review remains
required.

Pre-v1 compatibility note: `kicadai version` now reports the KiCadAI build.
Use `kicadai kicad-version` for the live connected-KiCad version probe that the
old command provided.

See [SUPPORT.md](SUPPORT.md) for the exact compatibility and capability
boundary, [SECURITY.md](SECURITY.md) for vulnerability and electrical-safety
guidance, and [CHANGELOG.md](CHANGELOG.md) for user-visible changes.
