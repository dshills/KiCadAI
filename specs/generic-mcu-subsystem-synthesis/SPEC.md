# Generic MCU Subsystem Synthesis Specification

Date: 2026-07-21

## Summary

KiCadAI can generate boards containing an ATmega328P or an ESP32-WROOM-32E,
but those paths do not yet constitute general MCU synthesis. Device-specific
blocks still choose pin roles and support circuitry, while the generic catalog
provider treats a programmable controller largely as a single component with
one I2C binding and a pool of GPIO names.

This project replaces that narrow behavior with catalog- and resolver-driven
MCU subsystem synthesis. A behavioral request selects a supported MCU or
module, assigns complete peripheral bundles to real physical pins, derives its
power, reset, clock, boot, programming, connector, and protection networks from
verified catalog evidence, and reaches the same KiCad acceptance gates as other
generated boards. The implementation must work without target-name dispatch,
fixture-specific coordinates, or pre-wired topology families.

The initial proof set is:

- Microchip ATmega328P-A in TQFP-32;
- Espressif ESP32-WROOM-32E module;
- STMicroelectronics STM32G031K8T6 in LQFP-32.

The STM32G031 is the new family because it exercises SWD, grouped alternate
functions, internal or external clock policy, analog inputs, interrupts, PWM,
and multiple serial-peripheral choices without requiring a device-specific
layout algorithm.

## Goals

- Select an MCU or module from verified catalog evidence according to the
  requested voltage, peripheral resources, programming mode, clock policy,
  current load, package, and board constraints.
- Assign GPIO, UART, I2C, SPI, PWM, ADC, interrupt, and programming roles to
  physical pins deterministically.
- Allocate protocol bundles consistently: all signals for an instance must use
  a compatible peripheral instance and alternate-function mapping.
- Validate pin conflicts, direction and electrical mode, voltage domains,
  boot-strap states, clocks, per-pin and aggregate current, and peripheral bus
  loading before layout.
- Derive decoupling, bulk capacitance, reset, boot, programming, oscillator,
  connector, level translation, pull-up, and external protection circuitry from
  catalog policy and request requirements.
- Produce stable capability-gap or clarification output when no supported
  candidate or pin assignment exists.
- Prove the three initial targets through deterministic replay, writer and
  round-trip checks, connectivity, route completion, installed-KiCad ERC, and
  strict DRC.
- Freeze neutral multi-peripheral corpus cases that describe behavior and
  electrical constraints without naming a target MCU or expected topology.

## Non-Goals

- Do not infer undocumented alternate functions or electrical limits.
- Do not support arbitrary unverified MCUs merely because KiCad contains a
  symbol.
- Do not generate firmware, pin-initialization source, or a software SDK
  project in this slice.
- Do not optimize high-speed RF layout, antenna matching, DDR memory, USB HS,
  Ethernet PHYs, or switch-mode power converters.
- Do not treat successful routing as proof that oscillator, analog, EMC, or RF
  performance is fabrication-ready.
- Do not add fixture names, project-name dispatch, target allowlists,
  device-specific coordinates, topology schemas, or one block family per MCU.

## Catalog Evidence Contract

Every selectable MCU record must retain the existing verified symbol,
footprint, package-pad, rating, and companion evidence and add an `mcu` evidence
object. That object is data, not executable target dispatch, and contains:

- architecture and family identifiers used only for reporting and capability
  comparison;
- supply domains, legal voltage ranges, estimated supply demand, and required
  power/ground physical pins;
- every physical I/O pin, its canonical catalog function, package pad, GPIO
  identity, electrical modes, and alternate-function choices;
- alternate functions normalized as kind, peripheral instance, and signal,
  such as `uart/usart1/tx`, `i2c/i2c1/scl`, `spi/spi1/miso`,
  `timer/tim1/ch1`, `adc/adc1/in0`, or `programming/swd/swdio`;
- GPIO source/sink limits, aggregate I/O limits, input-only restrictions,
  open-drain capability, analog capability, interrupt line, pull behavior, and
  voltage tolerance where verified;
- programming interfaces and their required signals;
- available internal and external clock options, frequency ranges, required
  pins, and confidence;
- boot and strap constraints, including required reset-time level and whether
  an external bias is mandatory;
- support-network policy expressed through generic companion requirements and
  placement/routing hints.

Catalog validation must reject an MCU record when:

- an MCU physical function is absent from the selected symbol;
- a symbol pin cannot be mapped to exactly one package pad, except an explicitly
  documented no-connect or duplicate-power-pin case;
- an alternate function names an unknown physical pin or malformed bundle;
- a required supply, reset, boot, clock, or programming signal lacks evidence;
- duplicate physical pins, package pads, alternate-function entries, or
  contradictory electrical limits are present;
- a verified record depends on placeholder evidence.

Input JSON ordering must not affect normalized catalog order, candidate order,
or assignment output.

## MCU Selection

Selection evaluates every verified MCU candidate before ranking it. A candidate
is feasible only if:

- all required supply domains contain the requested operating voltage;
- the required peripheral counts, modes, and speeds can be assigned together;
- required programming and clock policies are supported;
- requested logic levels are directly compatible or a catalog-supported level
  transition can be synthesized;
- estimated MCU demand plus externally sourced pin current fits supply and
  aggregate I/O budgets;
- package and board constraints are satisfied;
- boot constraints remain satisfiable after all requested loads and external
  biases are applied.

Candidates are ranked by explicit user preferences first, then verified
capability fit, smallest supported package area, lower unused-resource count,
and finally component ID and variant ID. Catalog input order is never a
tie-breaker. Rejected candidates retain stable reason codes for diagnostics.

A behavior may indirectly select a target by electrical and capability needs:
5 V operation can select the ATmega, wireless capability can select the ESP32,
and a 3.3 V SWD/multi-peripheral request can select the STM32. Corpus cases must
not name those devices merely to force the expected answer.

## Deterministic Pin Assignment

The resolver builds normalized role demands from participant ports, bound
signals, interface constraints, and programming/clock policy. It then performs
a deterministic constraint search over physical pins.

Assignment rules:

1. Reserve non-negotiable supply, ground, reset, boot, programming, and selected
   external-clock pins.
2. Allocate the most constrained complete peripheral bundles first.
3. Keep all signals of a UART, I2C, SPI, timer, ADC, or programming request on a
   compatible instance unless the protocol explicitly permits independent
   GPIO implementation and that mode is requested.
4. Enforce signal direction, open-drain/push-pull mode, analog capability,
   interrupt capability, speed, voltage tolerance, and shared-function
   conflicts.
5. Rank viable assignments by preservation of programming/debug access,
   avoidance of boot pins, fewest exceptional electrical modes, shortest
   generic role-to-edge placement preference where catalog evidence provides
   one, then normalized physical-pin identity.
6. Backtrack deterministically when a later bundle conflicts. Equivalent
   catalog orderings and repeated runs must produce byte-identical assignment
   evidence.

Each selected role records the component ID, peripheral instance, signal,
canonical physical function, package pad, catalog evidence sources, and the
alternatives rejected by constraint code. Downstream schematic and PCB
generation consumes those physical functions; it does not re-solve or infer
pins.

## Validation

Validation runs before component realization and again against the realized
graph:

- `mcu.pin_conflict`: one physical pin was assigned incompatible roles;
- `mcu.bundle_conflict`: requested interface signals cannot share a valid
  peripheral instance;
- `mcu.voltage_domain`: pin or supply voltage is outside verified limits;
- `mcu.logic_level`: source and sink thresholds are incompatible without a
  synthesized transition;
- `mcu.boot_strap_conflict`: external loading violates a required boot state;
- `mcu.clock_unavailable`: no clock option meets the policy and frequency;
- `mcu.pin_current`: a pin source/sink limit would be exceeded;
- `mcu.aggregate_current`: aggregate MCU I/O or supply current is exceeded;
- `mcu.peripheral_loading`: I2C capacitance/pull-up current, SPI/UART fanout,
  ADC source impedance/settling, or other modeled loading is invalid;
- `mcu.programming_conflict`: required programming access was consumed or
  electrically blocked.

All issue paths point to stable behavioral requirement or synthesized-role
paths. Numeric checks retain required, offered, and margin evidence.

## Catalog-Derived Support Networks

Once an MCU and assignment are selected, generic companion expansion emits:

- one decoupling network per catalog supply-domain policy, plus required bulk
  capacitance;
- reset bias and optional reset connector elements;
- the selected programming/debug connector and any required isolation or bias;
- boot-strap bias networks that are not already safely provided internally;
- external oscillator, load capacitors, and damping only when the chosen clock
  policy requires them;
- I2C pull-ups based on voltage, speed, capacitance, and sink-current evidence;
- connectors and protection requested at external boundaries;
- level translation when supported and required by incompatible domains.

Companion expansion must be conditional and idempotent. It must not create an
external crystal when an acceptable internal oscillator was selected, duplicate
pull-ups already proven elsewhere, or hide unsupported protection/translation
behind a warning.

## Clarifications And Capability Gaps

If more than one materially different feasible subsystem remains because a
missing user choice changes clock source, programming accessibility, voltage
domain, or external interface, compilation returns a stable clarification.

If no candidate is feasible, compilation returns a stable capability gap whose
ID is derived from the normalized failed capability, not the prompt wording or
candidate name. Output includes the requirement path, failure code, compact
required/offered evidence, and deterministic rejected-candidate summaries.

Examples include `mcu.required_peripheral_unavailable`,
`mcu.voltage_domain_unsupported`, `mcu.pin_assignment_impossible`, and
`mcu.programming_interface_unavailable`.

## Acceptance Evidence

The project is complete only when:

- verified ATmega328P-A, ESP32-WROOM-32E, and STM32G031K8T6 catalog records pass
  symbol/package/pinmap and MCU-evidence validation;
- focused solver tests cover GPIO, UART, I2C, SPI, PWM, ADC, interrupt, and
  programming assignment, conflicts, bundle consistency, and shuffled-input
  determinism;
- focused electrical tests cover voltage, logic, boot, clock, per-pin current,
  aggregate current, and peripheral loading;
- support-network tests prove conditional decoupling, reset, programming,
  oscillator, connector, translation, pull-up, and protection expansion;
- a frozen neutral held-out corpus includes unfamiliar multi-peripheral
  behaviors that reach supported, clarification, and capability-gap outcomes;
- at least one generated KiCad project for each target has clean ERC, strict
  DRC, complete connectivity and routing, writer correctness, and zero
  round-trip differences;
- all three target replays and corpus replays are byte-identical;
- the existing ATmega, ESP32, USB-C I2C sensor, LED indicator, and amplifier
  fixture evidence remains green;
- staged changes receive a clean Prism review, the full repository test suite
  passes, changes are committed and pushed, and GitHub Actions succeeds.
