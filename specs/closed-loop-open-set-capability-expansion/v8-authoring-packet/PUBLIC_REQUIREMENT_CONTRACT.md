# Public Open-Topology Requirement Contract for V8 Authoring

This is the complete author-facing structural and vocabulary contract. It
describes externally observable behavior, never implementation.

## Root and identifiers

Each file is one JSON object with exactly `schema`, `version`, `project`,
`requirements`, and `acceptance`. `schema` is
`kicadai.open-topology-requirement.v1`; `version` is `1`. Unknown fields are
invalid. Every ID matches `^[a-z][a-z0-9_]{0,63}$` and is unique in its
namespace. Every number is a finite JSON number. Manifest-only `v8_case_*` and
`v8_source_*` strings are prohibited throughout the requirement.

`project` contains exactly:

- `name`: semantic ID;
- `title`: 1–256 characters; and
- `description`: 1–4096 characters of neutral behavior-study text.

Project text must not encode corpus role, assignment domain/role, safety
metadata, expected support, or implementation clues.

## Requirements object

`requirements` contains exactly:

- `domains`: 2–16 domain objects;
- `ports`: 2–24 port objects;
- `operating_cases`: 2–16 operating-case objects;
- `behavioral_requirements`: 4–32 assertion objects; and
- `constraints`: one board-limits object.

### Domains

Each domain has `id`, `kind` (`reference` or `supply`), and `source`
(`external`). Optional fields are ordered `min_voltage_v`, `nominal_voltage_v`,
`max_voltage_v`, plus positive `max_current_a` at most 1000. At least one
reference domain is required. A supply used for all-corner behavior declares
finite minimum, nominal, and maximum voltage.

### Ports

Each port has `id`, `kind`, `direction`, `domain`, and `electrical`.

Allowed kinds are `analog_current`, `analog_voltage`, `controlled_current`,
`digital`, `power`, and `reference`. Allowed directions are `bidirectional`,
`sink`, and `source`. `domain` references a declared domain.

Optional electrical fields are ordered `min_voltage_v`, `nominal_voltage_v`,
`max_voltage_v`; positive `max_current_a` at most 1000; positive
`input_impedance_min_ohm` at most 1e15; and `default_state` (`low`, `high`, or
`floating`). At least one non-power/non-reference source output and at least one
non-power/non-reference sink excitation are required unless the assigned role
is autonomous source/bias behavior, which may use a power input plus its source
output.

Multi-output behavior requires at least two distinct non-power/non-reference
source ports, each independently observed by at least one assertion. Merely
duplicating a signal or adding an unused flag is not multi-output behavior.

### Operating cases

Each operating case has `id`, 1–32 unique `conditions`, and optional `events`.
At most 32 events may occur in a file.

A condition has `axis`, `target`, finite ordered `min` and `max`, and `unit`.
Allowed axes are `ambient_temperature`, `input_current`, `input_voltage`,
`load_capacitance`, `load_current`, `load_inductance`, `load_resistance`,
`model_corner`, `supply_voltage`, and `tolerance_corner`. The target references
a declared port or domain. Conditions are unique by axis plus target.

An event has `id`, `kind`, `target`, nonnegative `trigger_time_s`, finite
`initial` and `applied`, and `unit`. Allowed kinds are `input_step`, `load_step`,
`power_step`, `rail_loss`, `short_circuit`, and `startup`. The target references
a declared port.

### Behavioral requirements

Each assertion has `id`, `metric`, `analysis`, optional `excitation`,
`observation`, at least one finite `min` or `max` (ordered if both), `unit`, 1–16
unique declared `operating_cases`, optional positive `frequency_hz` at most
1e12, and optional `critical` boolean.

An observation has exactly `kind` and `id`. `kind` is `port`, `domain`, or—for
an assertion observation only—`circuit`. Port and domain IDs must resolve. A
circuit observation is a neutral assertion-local whole-assembly label, never an
internal component, topology, node, or block name. The publisher later maps a
circuit observation to the `@circuit` output sentinel; authors never write that
sentinel or any obligation-anchor hash.

Allowed analyses are `ac_sweep`, `dc_operating_point`, `dc_sweep`,
`distortion`, `electrothermal`, `noise`, `stability`, `startup`, `thermal`, and
`transient`.

Allowed metrics are `bandwidth`, `cutoff_frequency`, `dc_current`, `dc_voltage`,
`duty_cycle`, `fall_time`, `falling_threshold`, `hysteresis`, `input_impedance`,
`junction_temperature`, `line_regulation`, `load_regulation`, `lower_threshold`,
`off_state_current`, `on_state_voltage`, `oscillation_frequency`,
`output_current`, `output_high_voltage`, `output_low_voltage`,
`output_noise_rms`, `output_power`, `output_ripple`, `output_swing`,
`output_voltage`, `peak_current`, `peak_voltage`, `phase_margin`,
`propagation_delay`, `quiescent_current`, `rise_time`, `rising_threshold`,
`settling_time`, `soa_margin`, `startup_current`, `startup_output_voltage`,
`startup_overshoot`, `threshold_current`, `threshold_voltage`,
`total_harmonic_distortion`, `transconductance`, `transimpedance`,
`upper_threshold`, `voltage_gain`, `voltage_gain_at_frequency`, and
`conversion_efficiency`. The alias `thd` is prohibited.

Allowed units are `%`, `A`, `A/V`, `F`, `H`, `Hz`, `V`, `V/A`, `V_pp`,
`V_rms`, `W`, `deg`, `degC`, `ohm`, `ratio`, and `s`. Metric, axis, event, and
unit meanings must be dimensionally coherent.

Every assertion ID, operating-case ID, observation kind/ID, and observed output
must be stable semantic behavior identifiers. Renaming them after author return
is not permitted merely to influence publisher-derived obligation anchors.

### Board limits

`constraints` contains exactly positive `max_components` (1–64),
`max_width_mm` (at most 500), and `max_height_mm` (at most 500). These are
neutral resource bounds, not placement instructions.

## Acceptance

`acceptance` contains exactly these 14 fields, all `true`:

- `require_primitive_only`
- `require_topology_search`
- `require_simulation`
- `require_all_corners`
- `require_model_provenance`
- `require_closed_loop_evidence`
- `require_complete_routing`
- `require_connectivity`
- `require_writer_correctness`
- `require_round_trip_zero_diff`
- `require_erc`
- `require_strict_drc`
- `require_deterministic_replay`
- `require_fail_closed`

## Electrical sanity

Before return, verify dimensional units, all-corner compatibility with external
energy, physical current/load/voltage/power bounds, coherent references,
bounded event envelopes or explicit faults, and absence of mutually exclusive
assertions in the same operating case. An ideal mathematical open/short or
unbounded source is not an environmental contract; use a finite physically
meaningful bound.
