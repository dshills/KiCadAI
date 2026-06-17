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
