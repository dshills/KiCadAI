# Protocol-Aware Bus Synthesis Completion Audit

## Outcome

The milestone adds deterministic, catalog-backed synthesis for behavior-only
I2C, SMBus, SPI, and UART buffering and voltage translation.

The promoted architecture families are:

- ordinary bidirectional open-drain translation with solved pull-ups;
- segmented open-drain translation with independent trunk and branch nets;
- fixed-direction push-pull translation; and
- mixed-direction push-pull composition with separate forward and reverse
  channel groups and one shared-domain power budget.

Production selection depends only on normalized electrical roles and
constraints. It does not inspect project names, prompts, corpus identities,
fixture paths, component names, coordinates, layers, routes, or expected
results.

## Frozen Evidence

The frozen corpus contains four safety-critical behavior-only requirements:

| Case | Protocol | Pressure |
|---|---|---|
| `i2c_partial_power` | I2C | reversed voltage ordering, solved pull-ups, either-side partial power |
| `segmented_smbus` | SMBus | six isolated branches, 360 pF aggregate load, trunk/branch allocation |
| `spi_mixed_direction` | SPI | two forward and one reverse push-pull lanes with shared supplies |
| `uart_inactive_partial_power` | UART | high inactive state and either-side partial-power isolation |

The manifest binds the pre-support base commit, prompt hashes, requirement
hashes, behavior-only authoring policy, protocol families, and safety-critical
classification. Strict tests verify those bindings and reject implementation
details.

## Generic Safety And Electrical Proof

Every accepted request must explicitly state:

- partial-power and back-power prevention;
- hot-plug isolation; and
- either wired-low contention for open-drain buses or exclusive drivers for
  push-pull buses.

Push-pull translation additionally requires an enable boundary whose startup
state is inactive. Mixed-direction requests require exact forward/reverse role
contracts and positive integral channel counts.

Segmented open-drain synthesis validates:

- two through eight segments;
- one trunk and exactly one role per declared segment;
- receiver count;
- aggregate, trunk, and per-segment capacitance allocation;
- rise-time and sink-current-compatible pull-ups; and
- independent branch isolation.

The selected TXS0102 record already carries reviewed voltage, open-drain and
push-pull frequency, channel, partial-power, function-to-pin, package, and
trusted-model evidence. Existing TXS0104E push-pull support is reused for
fixed-direction groups. No new fixture-specific catalog record was added.

## Physical Realization

The segmented topology emits:

- a distinct two-lane trunk;
- one distinct two-lane net pair per branch;
- one translator per branch;
- trunk and per-branch pull-ups;
- local VCCA and VCCB bypass capacitors; and
- deterministic `Near` placement constraints for each branch support group.

Reversed voltage domains correctly attach OE and VCCA bypassing to the
low-voltage VCCA supply rather than assuming the trunk is always side A.
Metamorphic tests reverse input role order and voltage ordering and require
byte-identical normalized payloads.

Mixed-direction composition merges only the shared power, reference, and
enable nets. Forward and reverse signal groups remain separate, and shared
power demand is summed once at the combined obligation boundary.

## Negative Coverage

Eleven promotion-matrix negative identities are backed by focused fail-closed
tests covering:

- missing partial-power evidence;
- missing hot-plug isolation;
- unsafe contention policy;
- active enable at startup;
- missing branch isolation;
- inconsistent aggregate-load allocation;
- receiver/segment undercount;
- missing segment roles;
- unsupported segment count;
- impossible rise time; and
- ambiguous mixed-direction roles.

Provider failures use the existing typed interface-synthesis error family.

## Local Verification

Verification on 2026-07-30 used Go 1.23-compatible repository dependencies and
the installed KiCad application at
`/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli` with its bundled
symbols and footprints.

The following gates passed:

- focused architecture and corpus tests;
- full architecture-search package tests;
- the four-case offline promotion workflow, two runs per case;
- the four-case installed-KiCad promotion workflow, two runs per case;
- repository-wide `go test ./... -short -count=1`;
- `make lint`, including `go vet` and zero golangci-lint findings; and
- `make race-short`.

The installed-KiCad command completed in 78.734 seconds and passed all four
subtests. Each subtest required clean ERC, strict DRC, complete routing and
connectivity, route completion, writer correctness, zero round-trip
differences, and deterministic replay.

## Preserved Evidence

Repository-wide short tests preserve the existing writer, round-trip,
architecture, MCU/sensor, amplifier, clock, open-world, and promotion-runner
regressions. The new implementation reuses registered catalog records,
simulation primitives, lowering operations, placement semantics, routing, and
writer paths rather than adding a parallel fixture lane.

## Boundary

This is a bounded catalog-backed promotion, not arbitrary high-speed digital
signoff. USB, Ethernet, MIPI, LVDS, CAN, RS-485, controlled-impedance
transmission lines, RF effects, protocol-controller synthesis, and fabrication
release remain outside this milestone.
