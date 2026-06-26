# Component Intelligence

Component intelligence is the layer that turns generic design intent into
specific KiCad-resolvable component bindings. It is intentionally conservative:
when the catalog cannot prove a component is safe for the requested acceptance
level, selection blocks before schematic or PCB files are written.

## Catalog Location

The default catalog lives in:

```text
data/components/
```

The catalog is split across deterministic JSON files. Each file can define
families and component records. Records contain symbol bindings, package
variants, footprint IDs, function pins, pad functions, ratings, values, and
verification metadata.

Use a custom catalog with:

```sh
kicadai --json --catalog-dir ./my-components component validate
```

## Confidence Levels

- `verified`: symbol, footprint, pinmap, and package evidence is explicitly
  checked.
- `library_derived`: derived from KiCad library metadata but not fully proven.
- `rule_inferred`: safe inference for limited cases, mainly symmetric passive
  parts.
- `placeholder`: useful for draft structure only; not safe for connectivity or
  fabrication readiness.
- `blocked`: record is known unsafe or incomplete.

Acceptance levels gate confidence. Draft output may use placeholders with
warnings. Structural output allows safe rule-inferred records for shape and file
generation. Connectivity, ERC/DRC, and fabrication-candidate output require
verified evidence except for narrowly allowed passive rule-inferred records.

## CLI Commands

List records:

```sh
kicadai --json component list
```

Show one record:

```sh
kicadai --json component show resistor.generic.0805
```

Find candidates:

```sh
kicadai --json component find --family resistor --package 0805 --value-kind resistance --value 10k
```

Select from a request file:

```sh
kicadai --json --request examples/components/select_resistor.json component select
```

Validate the catalog:

```sh
kicadai --json component validate
```

## Design Workflow Integration

`design create` runs a `component_selection` stage after block planning and
before schematic, PCB realization, placement, routing, or file writes. The stage
loads the catalog, selects required block components, applies selected
symbol/footprint metadata back into the generated transaction, and blocks early
when required components are missing or unsafe for the requested acceptance.

Request JSON can include `component_policy`:

```json
{
  "component_policy": {
    "catalog_dir": "data/components",
    "minimum_confidence": "rule_inferred",
    "acceptance": "connectivity",
    "package_preferences": {
      "status.resistor": "0805"
    },
    "overrides": {
      "status.led": {
        "component_id": "led.generic.0805",
        "variant_id": "0805",
        "minimum_confidence": "verified"
      }
    }
  }
}
```

Override keys are checked in this order:

- `<instance_id>.<component_role>`;
- `<block_id>.<component_role>`;
- `<component_role>`.

## Verified Regulator Slice

The checked-in catalog and workflow now include a concrete 5 V to 3.3 V linear
regulator selection path for common breakout-style designs:

- `regulator.linear.ams1117_3v3.sot223` for the regulator role;
- `capacitor.ceramic.0805` for regulator input and output capacitor roles;
- nominal-value matching for capacitor capacitance plus rating-aware checks for
  regulator input voltage, regulator output current, and capacitor voltage;
- workflow evidence in the `component_selection` stage and persisted
  `.kicadai/workflow-result.json` output.

When an intent power rail includes `current_ma`, the planner converts that value
into an `output_current` requirement for the regulator role. Input and output
capacitor voltage ratings are selected from the next common voltage class after
a 25 percent design margin against the nominal rail voltage currently modeled
by the planner. For example, a nominal 5 V input rail requires at least a 6.3 V
capacitor rating. Source tolerance, surge, and derating beyond the nominal rail
are not modeled yet. This is the current minimum automated selection rule, not
a professional MLCC derating recommendation; fabrication-candidate designs
should use explicit component policy overrides for higher voltage classes such
as 10 V or 16 V on 5 V ceramic-capacitor rails unless part-specific DC-bias
evidence proves the lower class is acceptable.

The generated request also persists these requirements in
`.kicadai/generated-request.json` under `component_policy.overrides`, so agents
can audit why a selected regulator or capacitor was accepted. Missing or
insufficient ratings remain blocking for connectivity-oriented acceptance.

This is not yet an analog-stability proof. The current selector checks catalog
ratings and KiCad-resolvable bindings; it does not model LDO output-capacitor
ESR windows, MLCC DC-bias capacitance loss, thermal dissipation, or transient
response. In particular, manually verify linear-regulator power dissipation
with `Pd = (Vin - Vout) * Iout` and confirm the selected package and PCB copper
can handle it. Treat generated regulator designs as structurally and
connectivity-oriented evidence until a block or catalog record carries
part-specific stability and derating evidence.

## Current Limitations

- The seed catalog is intentionally small and biased toward built-in blocks and
  examples.
- Many active components are placeholders and only allowed for draft output.
- Full manufacturer part selection, lifecycle checks, availability, cost, and
  derating are not implemented.
- Component selection rewrites transaction symbol/footprint metadata, but many
  block generators still carry local schematic values and pin placement hints.
- Resolver-backed evidence is available through package APIs, but broader
  external KiCad library validation is not required by default tests.
