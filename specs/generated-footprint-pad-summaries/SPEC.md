# Generated Workflow Footprint Pad Summaries Specification

## 1. Purpose

Resolve the generated `design create` routing blocker where placement components
reach routing without footprint pad summaries.

The current routing adapter requires each placement component to include
`placement.PadSummary` records. Pad-backed seed fixtures can exercise full-board
placement-routing retry, but true generated workflows can still stop with:

```text
component has no footprint pad summaries for routing
```

This project closes that gap so generated schematic-to-PCB workflows can route
against real component pads, produce meaningful connectivity evidence, and
participate in full-board retry tests.

## 2. Goals

This project must:

- hydrate `placement.Component.Pads` for generated `design create` components;
- use resolver-backed KiCad footprint pad geometry where library data is
  available;
- assign pad net names through verified symbol-pin to footprint-pad mappings;
- preserve component bounds and footprint graphics hydration already implemented
  in the placement and design API paths;
- provide explicit evidence for pad summary source, hydrated pad count, missing
  pad count, and affected references;
- keep unsafe or under-evidenced footprints blocked with actionable issues
  rather than silently inventing pads;
- convert the documented generated LED retry fixture from
  `rejected_missing_pad_summaries` to a routed or routing-diagnostic candidate;
- make generated full-board retry evidence depend on real generated workflow
  output, not only hand-built pad-backed seed boards.

## 3. Non-Goals

This project does not:

- implement a complete KiCad footprint parser beyond the pad metadata already
  available through the resolver path;
- guarantee fabrication readiness for every generated board;
- build a general autorouter;
- expand the component catalog by itself;
- add natural-language intent planning;
- mutate imported user projects;
- use network access or remote library lookups in default tests.

## 4. Current Baseline

Implemented foundations include:

- resolver-backed footprint hydration for footprint bounds, graphics, pads, and
  models;
- `placement.BoundsFromFootprint(record)` and
  `placement.HydrateComponentFootprint(component, record)`;
- placement components with `Pads []placement.PadSummary`;
- routing adapter conversion from placement pads to routing pads;
- generated workflow stages for block planning, component selection, schematic
  generation, PCB realization, placement, routing, validation, repair, and
  artifacts;
- placement-routing retry fixtures and full-board seed evidence;
- a documented generated workflow boundary fixture that currently blocks on
  missing footprint pad summaries.

Current gap:

- generated PCB realization and placement request construction can carry
  footprint IDs and bounds without carrying pad summaries with net names, so
  `internal/routingadapters.RequestFromPlacement` blocks before routing can
  produce real diagnostics.

## 5. Required Data Contract

Each generated component passed to routing must include:

- `Ref`: stable component reference;
- `FootprintID`: resolved footprint library identifier;
- `Bounds`: positive footprint bounds;
- `Pads`: one record for every routable footprint pad needed by connected nets.

Each `PadSummary` must include:

- `Name`: KiCad footprint pad number/name;
- `Net`: generated net name when the pad participates in a known net;
- `XMM`, `YMM`: footprint-local pad location in the coordinate frame expected by
  the routing adapter;
- `WidthMM`, `HeightMM`: positive pad dimensions.

The implementation must verify and document the coordinate convention before
changing behavior. If the router expects pad positions relative to footprint
origin before component rotation, the hydration path must preserve that. If it
expects absolute board coordinates, the adapter contract must be corrected and
tested.

## 6. Pad Summary Source Priority

Pad summaries must be generated from the highest-confidence available source:

1. Resolver-loaded KiCad footprint records, including real pad geometry.
2. Project or local footprint library tables when wired through the existing
   resolver configuration.
3. Verified built-in seed templates for known catalog parts where the pinmap and
   footprint geometry are explicitly checked into the repository.
4. A blocked workflow issue when no trusted source can prove pad names and
   geometry.

The writer must not create arbitrary dummy pads for connected components. A
fallback is allowed only when it is tied to a named verified component or block
fixture and has tests proving pin-to-pad and net assignment behavior.

## 7. Pinmap And Net Assignment

Hydration must map schematic connectivity to footprint pads through the selected
component evidence:

- symbol reference and pin name/number;
- selected concrete component or catalog record;
- footprint ID;
- verified pinmap entries;
- schematic or PCB net endpoints.

Rules:

- A net endpoint must resolve to the matching footprint pad name before routing.
- A pad with no connected net may remain present with an empty `Net` value.
- A connected symbol pin with no footprint pad mapping is a blocking issue.
- Duplicate pad mappings for distinct connected pins are blocking unless the
  component record explicitly marks the pins as equivalent.
- Polarity-sensitive components must retain anode/cathode, input/output, power,
  and ground pin roles in evidence where the catalog provides them.

## 8. Workflow Evidence

Generated workflow output should expose pad hydration evidence in a compact,
machine-readable form. The evidence may be attached to PCB realization,
placement, routing, or a dedicated internal summary, but it must be available to
tests and CLI JSON output.

Required evidence fields:

- total generated components considered;
- components with hydrated pads;
- components missing pad summaries;
- total pads hydrated;
- source counts by source type, such as `resolver`, `verified_template`, or
  `missing`;
- references with missing or incomplete pad maps;
- blocking issue count caused by pad hydration.

Routing stage issues caused by missing pads must remain explicit, but successful
hydration should allow routing to continue far enough to produce real route,
connectivity, or rules diagnostics.

## 9. Test Requirements

Default tests must not require KiCad, network access, or global user library
state.

Required coverage:

- unit tests for resolver-backed pad extraction into `PadSummary`;
- unit tests for pin-to-pad net assignment;
- unit tests for missing pinmap and missing footprint behavior;
- workflow tests proving generated LED design no longer blocks on missing pad
  summaries;
- full-board retry fixture update proving the generated candidate reaches real
  routing diagnostics or improvement evidence;
- CLI selected-field tests proving pad hydration evidence is visible and stable;
- regression tests that prevent arbitrary dummy pads from satisfying connected
  components.

Optional coverage:

- tests that use locally configured KiCad library roots when available;
- KiCad CLI validation after generated routing when configured.

## 10. Safety Requirements

- The implementation must be deterministic.
- Missing pad evidence must produce blocking issues with reference and footprint
  context.
- Tests must avoid absolute local paths in snapshots.
- Generated projects must remain writable even when hydration blocks routing, so
  users can inspect partial artifacts.
- Existing pad-backed seed fixtures must keep passing.
- Existing resolver-backed design API `PlaceFootprint` behavior must not
  regress.

## 11. Acceptance Gates

This project is complete when:

- seed generated `design create` workflows no longer fail routing solely because
  components have no footprint pad summaries;
- connected generated LED, resistor, connector, regulator, and common block
  components carry named, dimensioned pad summaries into routing;
- net names on pad summaries match schematic/generated net endpoints through
  verified pinmaps;
- unsafe components still block with actionable pad hydration issues;
- the `generated_led_rejected` full-board retry fixture is renamed or reworked
  to reflect real generated routing behavior;
- CLI output includes stable pad hydration evidence;
- `go test ./...` passes.

