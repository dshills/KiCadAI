# KiCadAI v1.0.0

KiCadAI v1.0.0 is the first stable release of the bounded, evidence-backed
KiCad generation CLI. It promotes the frozen RC2 capability surface after a
nine-day stabilization soak and adds no circuit-generation capability beyond
RC2.

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

The final release includes RC2's development and release throughput
improvements without changing generated project semantics:

- explicit fast, bounded, and exhaustive local test tiers;
- representative processing benchmarks and documented reference measurements;
- streaming closed-loop evidence hashing with byte-compatibility proof;
- bounded shared worker limits for independent evaluation work;
- deterministic parallel promotion scenarios and release cross-builds;
- reduced nonlinear-solver topology and scratch-allocation churn.

KiCad 10.0.3 is the reference release toolchain. This release is not an
arbitrary-circuit generator or a fabrication approval system. Human
electrical, thermal, mechanical, regulatory, and manufacturing review remains
required.

The bounded V19 capability evaluation completed all 24 public cases twice but
did not meet its frozen advancement or preservation gates. It is published as
a fail-closed retirement and is not part of this release's supported surface;
V18 remains the latest admitted public capability.

Compatibility note: `kicadai version` reports the KiCadAI build.
Use `kicadai kicad-version` for the live connected-KiCad version probe that the
old command provided.

See [SUPPORT.md](SUPPORT.md) for the exact compatibility and capability
boundary, [SECURITY.md](SECURITY.md) for vulnerability and electrical-safety
guidance, and [CHANGELOG.md](CHANGELOG.md) for user-visible changes.
