# Circuit Block Library Readiness Review

Last verified: 2026-07-21.

The built-in registry currently exposes 25 blocks. Most retain conservative
`structural` block-level verification; the exact
`esp32_wroom_32e_minimal` contract is `erc_drc_verified`. Stronger evidence for
a composed design is recorded in its design-promotion report and must not be
generalized to every parameterization of its blocks.

## Current Evidence

- The block verification harness checks strict request validity, schematic
  semantics, schematic-to-PCB transfer, PCB realization, pads/nets, required
  local routes and zones, timing fixtures, writer correctness, internal board
  validation, and optional required KiCad ERC/DRC.
- `design create` includes block-readiness evidence in `block_planning`, then
  runs component selection, schematic/PCB realization, placement, routing,
  writer, connectivity, and configured KiCad gates.
- Exact protected USB-C LED, protected USB-C/3.3 V/I2C, concrete BMP280,
  ESP32-WROOM-32E-N4, Class-A, protected Class-AB headphone, and protected
  10 W/8 ohm speaker compositions have checked-in KiCad-backed `pass` fixtures.
- The 10 W speaker lane includes reviewed component, load, tolerance,
  distortion, electrothermal, SOA, current-limit, DC-fault, mute, high-current,
  Kelvin/star-return, mechanical, KiCad, writer, round-trip, and fabrication
  evidence. It is not general power-amplifier support.
- Default Go tests remain hermetic. KiCad-backed block/design evidence runs only
  when the relevant KiCad CLI and library paths are configured.

Use the installed registry rather than a copied list when constructing tooling:

```sh
kicadai block list
kicadai block show esp32_wroom_32e_minimal
kicadai --builtins block verify
```

See [Circuit Blocks](circuit-blocks.md) for behavioral boundaries and
[Circuit Block Verification](circuit-block-verification.md) for manifest and
evidence semantics.

## Readiness Boundaries

| Area | Proven now | Remaining boundary |
| --- | --- | --- |
| Registry | 25 built-in block contracts with parameters, ports, rules, PCB constraints, and readiness metadata. | New blocks and variants require catalog, resolver, pinmap, rule, and evidence coverage. |
| Sensors | Generic I2C template plus concrete BME280, BMP280, and SHT31 profiles. | Unknown sensors, SPI modes, and inferred pin roles fail closed. |
| MCU | Legacy ATmega/ESP32 blocks plus catalog-driven selection and deterministic pin assignment for verified ATmega328P-A, ESP32-WROOM-32E, and STM32G031K8T6 records. | Unverified devices, broader module/flash variants, RF optimization, and electrical constraints absent from catalog evidence remain unsupported. |
| Power/protection | Fixed 3.3 V LDO profiles, USB-C sink power, fuse/TVS/bulk options, ESD, and reverse-polarity primitives. | Broader regulator stability/thermal envelopes, USB-C variants, PD, mains, and safety isolation need reviewed evidence. |
| Amplifiers | Bounded Class-A, headphone Class-AB, and protected 10 W/8 ohm speaker evidence. | Bridge operation, materially higher power, arbitrary loads/devices, and unreviewed thermal/mechanical choices remain unsupported. |
| Placement/routing | Deterministic block-local copper, inter-block route trees, endpoint/contact proof, and bounded correction for promoted shapes. | General dense-board rip-up/reroute, RF/high-speed, controlled impedance, and unrestricted geometry are not guaranteed. |
| Writers/validation | Writer correctness, normalized round trips, connectivity, route completion, optional required ERC/DRC, and promotion reports. | A pass proves only the declared fixture and gates; sourcing, manufacturer review, safety, and fabrication release remain separate. |

## Verification Commands

Hermetic checks:

```sh
go test ./internal/blocks ./internal/blocks/verification
```

Installed-KiCad block corpus:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI=/path/to/kicad-cli \
go test ./internal/blocks/verification -run '^TestOptionalKiCadBlockCorpusSmoke$' -count=1 -v
```

Installed-KiCad design fixtures use `KICADAI_KICAD_CLI` and are documented in
the [KiCad-backed example README](../examples/design/kicad-backed/README.md).

## Release Interpretation

- `structural` means the reusable block contract and modeled operations are
  checked; it is not a fabrication claim.
- `erc_drc_verified` means the block's declared KiCad evidence is available for
  its verified envelope.
- A design-promotion `pass` means that exact composition satisfied its declared
  gates.
- Unknown parts, modes, operating conditions, or physical requirements must
  remain warnings, clarifications, or blockers—never inferred support.
