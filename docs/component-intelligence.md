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
kicadai --json --request examples/components/select_concrete_resistor.json component select
```

Validate the catalog:

```sh
kicadai --json component validate
```

Validate a local lifecycle/availability source snapshot alongside the catalog:

```sh
kicadai --json --source-dir data/component-sources component validate
kicadai --json --source-dir data/component-sources component coverage
```

Select with local procurement evidence:

```sh
kicadai --json --source-dir data/component-sources \
  --request examples/components/select_regulator.json \
  component select
```

`--source-dir` loads local JSON snapshots only. It does not query live
distributors, scrape websites, or imply real-time stock, pricing, lead time, or
manufacturer approval.

## Verified Alternatives

The checked-in catalog now includes a small verified alternative slice for
common generated-design parts:

- `resistor.yageo.rc0805fr_0710kl.0805` for 10 kOhm 0805 pull-up and
  current-limiting roles;
- `capacitor.murata.grm21br71h104ka01l.0805` for 100 nF 0805 decoupling and
  filter roles;
- `led.liteon.ltst_c170kgkt.0805` for green 0805 indicator LEDs;
- `connector.samtec.tsw_104_07_l_s.1x04` for 1x04 2.54 mm vertical pin
  headers.

Concrete records carry manufacturer, MPN, lifecycle status, symbol bindings,
footprints, pad-function mappings, and rating/value metadata. Generic records
remain available for draft and structural workflows where a design needs shape
or connectivity scaffolding before choosing an exact part.

Records can include optional `equivalence` metadata:

```json
{
  "equivalence": {
    "group": "resistor.10k.0805",
    "role": "preferred"
  }
}
```

Supported roles are `preferred`, `alternate`, and `fallback`. Catalog
validation rejects invalid roles, duplicate preferred records in a family/group
scope, groups without a preferred record, and group members with incompatible
family, package, value, footprint, or pad-function metadata. Differing ratings
or tolerances are acceptable only when each group member still satisfies the
same required selection gates; weaker records belong in a separate group.
Coverage still reports group-health fields so custom or partially migrated
catalogs can be diagnosed before those catalogs are accepted for normal use.

Selection is deterministic:

- connectivity and stronger acceptance prefer concrete records over generic
  fallbacks when both pass safety gates;
- draft and structural acceptance preserve generic fallback behavior;
- equivalence roles order otherwise equivalent records;
- non-equivalent candidates that remain tied after explicit preference signals
  still block as ambiguous unless `allow_alternatives` is set.

`component coverage` reports alternative breadth through `alternative_coverage`.
Per-family coverage includes:

- `concrete_records`;
- `generic_fallback_records`;
- `equivalence_groups`.

Aggregate group-health fields include:

- `groups_missing_preferred`;
- `groups_with_duplicate_preferred`;
- `concrete_records_missing_mpn`.

## Lifecycle And Availability Source Snapshots

Source files are provider-neutral JSON files under a caller-selected directory.
Every `*.json` file is loaded in sorted order and validated deterministically.

```json
{
  "schema": "kicadai.component.source.v1",
  "source_id": "curated_seed_procurement",
  "generated_at": "2026-06-26",
  "records": [
    {
      "manufacturer": "Diodes Incorporated",
      "mpn": "AP2112K-3.3",
      "lifecycle": {
        "status": "active",
        "source": "curated",
        "source_date": "2026-06-26",
        "confidence": "curated"
      },
      "availability": {
        "status": "not_checked",
        "source": "curated",
        "source_date": "2026-06-26",
        "confidence": "not_checked"
      }
    }
  ]
}
```

Lifecycle statuses are `active`, `mature`, `nrnd`, `eol`, `obsolete`, and
`unknown`. Availability statuses are `in_stock`, `limited`, `backorder`,
`unavailable`, `unknown`, and `not_checked`. Confidence values are `curated`,
`provider_snapshot`, `manual_review`, `not_checked`, and `unknown`.

Selection joins source records by normalized manufacturer and MPN. By default,
explicit EOL/obsolete lifecycle blocks connectivity and stronger selection,
stale lifecycle warns for connectivity and blocks fabrication-candidate
selection, and availability is advisory unless policy explicitly requires it.
Selected component output includes a `procurement` object with source ID,
lifecycle/availability status, source dates, freshness booleans, and policy
outcome. Rejected candidates include procurement issue codes when source
evidence blocks selection.

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
    "source_dir": "data/component-sources",
    "procurement_policy": {
      "require_lifecycle": true,
      "require_availability": false
    },
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

The checked-in catalog and workflow now include concrete 5 V to 3.3 V linear
regulator selection paths for common breakout-style designs:

- `regulator.linear.ams1117_3v3.sot223` for the regulator role;
- `regulator.linear.ap2112k_3v3.sot23_5` for low-current 3.3 V rails at or
  below the modeled 150 mA autonomous-selection slice;
- `capacitor.ceramic.0805` for regulator input and output capacitor roles;
- nominal-value matching for capacitor capacitance plus rating-aware checks for
  regulator input voltage, regulator output current, and capacitor voltage;
- workflow evidence in the `component_selection` stage and persisted
  `.kicadai/workflow-result.json` output.

When an intent power rail includes `current_ma`, the planner converts that value
into an `output_current` requirement for the regulator role. For 3.3 V rails
fed from inputs at or below 6 V, and at or below 150 mA, the intent planner
selects the AP2112K SOT-23-5 profile, ties `EN` to VIN, and emits an explicit
schematic no-connect marker for the NC pin through the block transaction path.
Higher-current rails or other voltage families do not use AP2112K unless a
future verified profile is added.

Input and output capacitor voltage ratings are selected from the next common
voltage class after a 25 percent design margin against the nominal rail voltage
currently modeled by the planner. For example, a nominal 5 V input rail
requires at least a 6.3 V capacitor rating. Source tolerance, surge, and
derating beyond the nominal rail are not modeled yet. This is the current
minimum automated selection rule, not a professional MLCC derating
recommendation; fabrication-candidate designs should use explicit component
policy overrides for higher voltage classes such as 10 V or 16 V on 5 V
ceramic-capacitor rails unless part-specific DC-bias evidence proves the lower
class is acceptable.

The generated request also persists these requirements in
`.kicadai/generated-request.json` under `component_policy.overrides`, so agents
can audit why a selected regulator or capacitor was accepted. Missing or
insufficient ratings remain blocking for connectivity-oriented acceptance.

This is not yet an analog-stability proof. The current selector checks catalog
ratings, modeled dropout/headroom, EN/NC handling, and KiCad-resolvable
bindings; it does not prove LDO output-capacitor ESR windows, MLCC DC-bias
capacitance loss, thermal dissipation, or transient response. In particular,
manually verify linear-regulator power dissipation with
`Pd = (Vin - Vout) * Iout` and confirm the selected package and PCB copper can
handle it. Treat generated regulator designs as structurally and
connectivity-oriented evidence until a block or catalog record carries
part-specific stability and derating evidence.

## Current Limitations

- The seed catalog is intentionally small and biased toward built-in blocks and
  examples.
- Many active components are placeholders and only allowed for draft output.
- Lifecycle and availability evidence is local snapshot evidence only; live
  distributor availability, pricing, alternates, cost ranking, and provider
  imports are not implemented.
- Component selection rewrites transaction symbol/footprint metadata, but many
  block generators still carry local schematic values and pin placement hints.
- Resolver-backed evidence is available through package APIs, but broader
  external KiCad library validation is not required by default tests.
