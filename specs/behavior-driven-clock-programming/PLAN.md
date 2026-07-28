# Behavior-Driven Clock And Programming-Interface Synthesis Plan

## Phase 1: Contract And Corpus

- Audit existing controller, standalone-clock, fanout, reset, and programming
  evidence.
- Freeze representative external-crystal ISP, UART bootloader, and unsupported
  unpowered-SWD requirements.
- Record inherited canned-oscillator, buffered-fanout, and mixed-voltage
  programming evidence.

## Phase 2: Catalog Evidence

- Require programming maximum-frequency evidence.
- Require external-crystal MCU drive and startup evidence.
- Qualify a concrete active crystal record and its load/drive/stability data.
- Add verified physical geometry for the selected standard KiCad footprint.

## Phase 3: Generic Synthesis

- Select a qualified crystal deterministically.
- Calculate and select the two load capacitors.
- Emit finalized crystal worst-case calculation evidence.
- Emit finalized programming-interface loading, voltage, entry, frequency, and
  isolation calculation evidence.
- Preserve stable fail-closed diagnostics for incomplete or incompatible
  requirements.

## Phase 4: Shared Graph And Writer

- Publish strict passive-crystal and load-capacitor function capabilities.
- Lower the crystal catalog family to the oscillator graph role.
- Recognize the calculated load network as satisfying the crystal companion
  obligation.
- Route the supported cases without fixture-specific physical data.

## Phase 5: Promotion And Closeout

- Run deterministic architecture and component tests.
- Run the complete offline promotion workflow for both supported cases.
- Run the installed-KiCad promotion workflow locally.
- Refresh derived catalog provenance and schematic goldens.
- Run the preserved USB-C, sensor, ESP32, standalone-clock, MCU, and amplifier
  regressions plus the complete local suite.
- Publish the completion audit.
