# Public Open-Topology Requirement Contract for V6 Authoring

This is the complete author-facing structural and vocabulary contract. It
describes behavior, never an implementation.

## Root object

Each file is one JSON object containing exactly:

- `schema`: `kicadai.open-topology-requirement.v1`
- `version`: `1`
- `project`: object
- `requirements`: object
- `acceptance`: object

Unknown fields are invalid. Every ID matches `^[a-z][a-z0-9_]{0,63}$` and is
unique in its namespace. Every number is a finite JSON number.

## Project

- `name`: opaque semantic ID, 1–64 characters
- `title`: opaque text, 1–256 characters
- `description`: neutral behavior-study text, 1–4096 characters

Do not encode role, reporting domain, safety metadata, expected support, or an
implementation clue in project text. Manifest-only `v6_case_*` and
`v6_source_*` identities are prohibited everywhere in a requirement file.

## Requirements

The object contains exactly:

- `domains`: 2–16 domain objects
- `ports`: 2–24 port objects
- `operating_cases`: 2–16 operating-case objects
- `behavioral_requirements`: 4–32 assertion objects
- `constraints`: board-limits object

### Domain object

Fields:

- `id`: semantic ID
- `kind`: `reference` or `supply`
- optional ordered `min_voltage_v`, `nominal_voltage_v`, `max_voltage_v`
- optional positive `max_current_a`, at most 1000
- `source`: `external`

At least one reference domain is required. A supply expected to prove
all-corner behavior uses explicit finite minimum, nominal, and maximum voltage.

### Port object

Fields:

- `id`: semantic ID
- `kind`: `analog_current`, `analog_voltage`, `controlled_current`, `digital`,
  `power`, or `reference`
- `direction`: `bidirectional`, `sink`, or `source`
- `domain`: declared domain ID
- `electrical`: object

Optional electrical fields are:

- ordered `min_voltage_v`, `nominal_voltage_v`, `max_voltage_v`
- positive `max_current_a`, at most 1000
- positive `input_impedance_min_ohm`, at most 1e15
- `default_state`: `low`, `high`, or `floating`

At least one source or controlled-current output and at least one non-power,
non-reference sink excitation are required.

### Operating-case object

Fields:

- `id`: semantic ID
- `conditions`: 1–32 condition objects, unique by axis plus target
- optional `events`: event objects; at most 32 events over the whole file

Condition fields:

- `axis`: `ambient_temperature`, `input_current`, `input_voltage`,
  `load_capacitance`, `load_current`, `load_inductance`, `load_resistance`,
  `model_corner`, `supply_voltage`, or `tolerance_corner`
- `target`: declared port or domain ID
- finite `min` and `max`, with `min <= max`
- `unit`: one allowed unit below

Event fields:

- `id`: semantic ID unique within the operating case
- `kind`: `input_step`, `load_step`, `power_step`, `rail_loss`,
  `short_circuit`, or `startup`
- `target`: declared port ID
- nonnegative finite `trigger_time_s`
- finite `initial` and `applied`
- `unit`: one allowed unit below

### Behavioral assertion object

Fields:

- `id`: semantic ID
- `metric`: one allowed metric below
- `analysis`: one allowed analysis below
- optional `excitation`: observation object
- `observation`: observation object
- at least one finite `min` or `max`, ordered when both exist
- `unit`: allowed unit
- optional positive `frequency_hz`, at most 1e12
- `operating_cases`: 1–16 unique declared operating-case IDs
- optional `critical`: boolean

An observation object contains exactly `kind` and `id`. `kind` is `port`,
`domain`, or—for an assertion observation only—`circuit`. Port/domain IDs must
be declared. A circuit observation uses a neutral semantic ID for whole-
assembly behavior; it does not name an internal component or node. That ID is
an assertion-local label, not a reference to a separately declared object.
Reuse it only when assertions intentionally observe the same whole-assembly
quantity.

Allowed analyses:

- `ac_sweep`, `dc_operating_point`, `dc_sweep`, `distortion`,
  `electrothermal`, `noise`, `stability`, `startup`, `thermal`, `transient`

Allowed metrics for V6 authoring:

- `bandwidth`, `cutoff_frequency`, `dc_current`, `dc_voltage`, `duty_cycle`,
  `fall_time`, `falling_threshold`, `hysteresis`, `input_impedance`,
  `junction_temperature`, `line_regulation`, `load_regulation`,
  `lower_threshold`, `off_state_current`, `on_state_voltage`,
  `oscillation_frequency`, `output_current`, `output_high_voltage`,
  `output_low_voltage`, `output_noise_rms`, `output_power`, `output_ripple`,
  `output_swing`, `output_voltage`, `peak_current`, `peak_voltage`,
  `phase_margin`, `propagation_delay`, `quiescent_current`, `rise_time`,
  `rising_threshold`, `settling_time`, `soa_margin`, `startup_current`,
  `startup_output_voltage`, `startup_overshoot`, `threshold_current`,
  `threshold_voltage`, `total_harmonic_distortion`, `transconductance`,
  `transimpedance`, `upper_threshold`, `voltage_gain`,
  `voltage_gain_at_frequency`, `conversion_efficiency`

Use `total_harmonic_distortion`; the legacy alias `thd` is prohibited even if
an older decoder accepts it.

Allowed units:

- `%`, `A`, `A/V`, `F`, `H`, `Hz`, `V`, `V/A`, `V_pp`, `V_rms`, `W`, `deg`,
  `degC`, `ohm`, `ratio`, `s`

Use each metric's ordinary electrical meaning and a dimensionally matching
unit.

### Board limits

`constraints` contains positive:

- `max_components`: 1–64
- `max_width_mm`: at most 500
- `max_height_mm`: at most 500

These are neutral resource bounds, not placement instructions.

## Acceptance

The object contains exactly these 14 fields, all `true`:

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

## Electrical sanity review

Before delivery, check each file without choosing an implementation:

- units match metrics and event/condition axes;
- all-corner output bounds are compatible with declared external energy unless
  the behavior explicitly requires energy conversion;
- current, load, voltage, and power bounds are finite, physical, and mutually
  compatible;
- reference domains are used consistently;
- event initial/applied values fit the interface envelope unless the event is
  an explicitly bounded fault; and
- no assertion pair imposes mutually exclusive behavior in the same case.
