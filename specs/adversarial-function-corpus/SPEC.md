# Adversarial Function-Level Corpus Specification

Date: 2026-07-17

## 1. Purpose

This milestone measures whether `generic-circuit-v1` generalizes beyond the
first function-level corpus. The second corpus is frozen before production
synthesis, catalog, simulation, placement, or routing behavior is changed for
its cases. Its inputs describe semantic functions, interfaces, operating
domains, operating cases, and physical bounds only.

The corpus is intentionally adversarial. It repeats capability pressures across
independently authored topologies so implementation priorities come from
failure frequency rather than fixture identity.

## 2. Trust Boundary

Corpus inputs may provide:

- semantic component roles and catalog queries;
- required semantic functions and bounded values or ratings;
- external interfaces and named signals;
- signed power domains, maximum current, and source ownership;
- semantic connections;
- bounded transient operating-case parameters;
- maximum board dimensions and preferred spacing.

Corpus inputs must not provide:

- symbols, footprints, units, pins, pads, references, or package coordinates;
- support-component instances for decoupling, bias, pull-up, protection,
  power flags, or unused pins;
- schematic groups, board regions, layers, keepouts, zones, or routes;
- fixture-specific schemas, block identities, allowlists, or repair actions;
- equations, matrices, solver controls, executable models, or scripts.

Semantic multi-channel names such as `CHANNEL_1_IN_PLUS` are logical channel
requirements. They are not KiCad unit or pin identifiers. The catalog and
resolver own their mapping to verified physical units and pads.

## 3. Frozen Corpus

The 18 cases are stored in
`internal/circuitgraph/testdata/adversarial_function_corpus`.

| ID | Primary pressure |
| --- | --- |
| `negative_rail_npn_indicator` | negative-only supply and signed stimulus |
| `bipolar_lmv321_inverting_amplifier` | bipolar op-amp supply and analog source bias |
| `dual_supply_diode_clamp` | mixed op-amp/diode nonlinear evidence |
| `uart_5v_3v3_level_bridge` | two-domain UART translation |
| `i2c_5v_3v3_level_bridge` | bidirectional translation and per-domain pull-ups |
| `usb_c_adjustable_regulator` | adjustable regulator support calculation and protection |
| `regulated_sensor_uart_node` | generated rail plus sensor and UART domains |
| `bmp280_spi_breakout` | SPI mode and six-signal connector |
| `esp32_spi_uart_gateway` | simultaneous SPI, UART, and module companions |
| `atmega_uart_gpio_controller` | ISP companions plus UART/GPIO wide interfaces |
| `esp32_dual_i2c_gpio_hub` | shared high-fanout bus and wider GPIO connector |
| `lm358_dual_channel_conditioner` | one physical dual op-amp, two semantic channels |
| `lm358_window_comparator` | multi-unit analog graph and shared threshold network |
| `npn_rc_edge_shaper` | catalog-derived BJT transient analysis |
| `diode_rc_pulse_clamp` | catalog-derived diode/capacitor transient analysis |
| `pnp_high_side_pulse_driver` | active-low PNP transient analysis |
| `dense_mixed_signal_controller` | dense mixed sensor/SPI/GPIO physical realization |
| `dual_sensor_level_shift_gateway` | repeated translation, shared buses, and dense routing |

Membership and file bytes are immutable through the checked-in manifest and
manifest digest. Cases are not removed, renamed, weakened, or given
fixture-specific implementation to improve the pass rate.

## 4. Baseline And Failure Taxonomy

The untouched implementation is run against every frozen input. The baseline
report records the earliest authoritative outcome in these categories:

- `schema`
- `synthesis`
- `catalog`
- `electrical_validation`
- `simulation`
- `schematic`
- `placement`
- `routing`
- `writer`
- `round_trip`

Every blocked case must include a stable issue code, path, root cause, and
bounded suggested action. A missing model is not a simulation pass. A written
file is not a routing, writer, or round-trip pass.

## 5. Generic Closure Policy

Failures are fixed in descending category/code frequency. A correction is
acceptable only when it is expressed through reusable semantic vocabulary,
reviewed catalog evidence, trusted bounded simulation primitives, or generic
physical algorithms. Production code must not switch on corpus ID, project
name, prompt text, or frozen file path.

After each correction:

1. rerun the affected adversarial cases;
2. rerun the original eight-circuit function corpus;
3. rerun the protected USB-C LED fixture;
4. rerun the protected USB-C I2C sensor fixture.

## 6. Promotion Gates

Every case reported as `pass` requires:

- strict decode and deterministic normalization;
- deterministic synthesis under input and catalog reordering;
- verified catalog, symbol, unit, pin, footprint, and pad resolution;
- complete catalog-derived support and unused-pin policy;
- trusted simulation whenever a complete registered model is applicable;
- schematic semantic/readability pass;
- placement pass;
- complete endpoint-access and required-net routing;
- clean KiCad ERC and strict DRC;
- connectivity and route-completion pass;
- writer correctness;
- zero schematic and PCB round-trip differences;
- byte-identical `.kicad_pro`, `.kicad_sch`, and `.kicad_pcb` replay.

Unsupported cases remain `blocked`; they never become skipped or optimistic
passes. The final report publishes pass rate, failures by category and stable
code, evidence hashes, and the delta from the frozen baseline.

## 7. Completion Rule

This milestone is complete when the frozen corpus, untouched baseline, generic
closure history, final capability report, and all promotion/regression evidence
are checked in and reproducible; Prism has no unresolved high or medium
findings; and GitHub Actions passes the committed result.
