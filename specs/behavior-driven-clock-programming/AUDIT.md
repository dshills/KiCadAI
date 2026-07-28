# Behavior-Driven Clock And Programming-Interface Synthesis Audit

Date: 2026-07-28

## Result

The milestone is implemented through the shared graph with no fixture-specific
coordinates, schemas, allowlists, request identities, or topology-specific
block families.

## Evidence

- `external_crystal_isp.json` deterministically selects the verified
  ATmega328P-A, Abracon 16 MHz crystal, calculated 22 pF load capacitors, ISP
  header/reset support, and shared-pin isolation.
- `uart_bootloader.json` deterministically selects the verified
  ESP32-WROOM-32E integrated-clock UART bootloader path.
- `unsupported_unpowered_swd.json` retains the stable
  `MCU_PROGRAMMING_LOAD` rejection.
- Crystal and programming calculations are finalized, hash-bound, and replay
  stable.
- The standard KiCad 5032 crystal footprint geometry is verified against the
  installed KiCad 10 library.

## Promotion

The supported corpus passes:

- architecture replay and component/calculation identity checks;
- strict function-level validation and catalog resolution;
- schematic and PCB lowering;
- placement and route completion;
- connectivity and writer correctness;
- clean KiCad ERC and strict DRC;
- zero normalized round-trip differences; and
- deterministic replay.

The installed-KiCad run used the local KiCad 10 application and its bundled
symbols, footprints, and templates. No GitHub Actions run was required.

## Regression Verification

- The complete local short suite passed.
- The protected USB-C LED, protected USB-C I2C sensor, ESP32 minimal-system,
  Class A preamplifier, Class AB headphone, and protected Class AB speaker
  installed-KiCad examples passed.
- The neutral MCU, standalone packaged/relaxation clock, and held-out digital
  clock installed-KiCad promotion corpora passed.
- An additional unshortened repository-wide run produced no assertion failure
  but reached Go's ten-minute package timeout while routing the unrelated
  adversarial multi-function corpus. It is not counted as pass evidence.

## Remaining Boundary

This evidence is a bounded catalog-backed capability. It does not establish
arbitrary clock-tree, PLL, RF, differential-clock, JTAG, or programmer
generation.
