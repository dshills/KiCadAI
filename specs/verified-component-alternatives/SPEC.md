# Verified Component Alternatives Specification

Date: 2026-06-27

## Summary

KiCadAI can select seed catalog parts and now attaches local lifecycle and
availability evidence when source snapshots are provided. The next Priority 1
gap is catalog breadth: generated designs still rely heavily on generic
passive records and a small number of concrete active parts. This project adds
a bounded, verified alternative model for common passives, LEDs, and connectors
so AI-generated designs can choose concrete, catalog-backed parts without
becoming ambiguous or unsafe.

This is not a distributor integration project and not a broad catalog import.
It defines how curated alternatives are represented, validated, selected, and
reported while preserving deterministic behavior.

## Roadmap Context

This addresses Priority 1 remaining work:

- expand from seed records to larger verified families and real alternatives;
- replace structural/generic templates with concrete component-backed parts
  where fabrication readiness is desired;
- keep selected components tied to symbol, footprint, pinmap, rating,
  lifecycle, and optional source-snapshot evidence.

## Problem Statement

The current catalog contains useful generic passives and selected concrete
active parts, but several generated workflows still have only one generic
candidate for common roles such as:

- resistors for pull-ups, dividers, LED limiters, reset networks;
- ceramic capacitors for decoupling, regulator input/output, filters;
- LEDs and simple diodes;
- pin headers and connectors.

Generic records are useful for draft and connectivity work, but they are weak
for BOMs, fabrication-candidate workflows, and AI explanations because they do
not provide exact manufacturer/MPN identity. Adding concrete records naïvely can
also break deterministic selection by creating equal-score ambiguous candidates.

## Goals

- Add a small curated alternative model for verified component families.
- Add concrete manufacturer/MPN records for a first bounded set of common
  passives/connectors/indicators.
- Preserve existing generic records for draft and generic fallback use.
- Make fabrication-candidate selection prefer concrete records when ratings,
  value, package, lifecycle, and confidence satisfy policy.
- Prevent equal-score ambiguity by adding deterministic equivalence and
  preference rules.
- Surface chosen alternative evidence in component selection, design workflow,
  rationale, BOM, and coverage reports through existing structures.
- Keep tests hermetic; no live provider calls or scraping.

## Non-Goals

- Do not import full KiCad, Digi-Key, Mouser, Octopart, or manufacturer
  catalogs.
- Do not rank parts by price, stock, lead time, popularity, or package
  availability.
- Do not infer electrical equivalence beyond explicitly modeled ratings,
  values, tolerances, package, footprint, and verified pin/pad mappings.
- Do not replace local lifecycle/availability source snapshots with live
  procurement.
- Do not make fabrication readiness depend on a single preferred vendor.

## Initial Scope

The first implementation slice should add alternatives only where the current
catalog and writer already have strong support:

- 0603 and 0805 resistors with concrete common values used by current blocks
  and tests, such as `10k`, `4.7k`, `1k`, and LED-limiter values already
  calculated by the planner.
- 0603 and 0805 ceramic capacitors for common decoupling and regulator values,
  such as `100n`, `1u`, `4.7u`, `10u`, and `22u`, with voltage ratings modeled.
- 0603 and 0805 indicator LEDs for currently supported LED block use.
- 2.54 mm pin headers used by connector breakout and programming-header flows.

Records should use real manufacturer/MPN examples only when the project has
checked-in, reviewable evidence for the symbol, footprint, value/rating, and
package mapping. Where exact parts are not yet verified, add the equivalence
model and tests first, not placeholder manufacturer records.

## Data Model

### Component Record Additions

Add optional fields to component records only if current structures cannot
express the needed evidence:

```json
{
  "id": "resistor.yageo.rc0805fr_0710kl.0805",
  "family": "resistor",
  "generic": false,
  "manufacturer": "Yageo",
  "mpn": "RC0805FR-0710KL",
  "values": [{"kind": "resistance", "typ": "10k", "unit": "ohm"}],
  "ratings": [{"kind": "power", "max": "0.125", "unit": "W"}],
  "tolerances": [{"kind": "resistance", "typ": "1", "unit": "%"}],
  "equivalence": {
    "group": "resistor.10k.0805",
    "role": "preferred"
  }
}
```

If adding a field is necessary, keep it small and explicit:

- `equivalence.group`: stable string identifying parts treated as equivalent
  for selection.
- `equivalence.role`: `preferred`, `alternate`, or `fallback`.
  `fallback` here means a lower-priority member inside a declared concrete
  equivalence group; it is distinct from generic fallback records used for
  draft and structural workflows.
- `equivalence.notes`: optional human-readable review notes.

The model must avoid broad claims. Equivalence means "acceptable for the
current modeled value/package/rating requirements," not full electrical,
environmental, or procurement equivalence.

### Package Variants

Concrete alternatives should still use normal package variants:

- package ID;
- KiCad footprint ID;
- package type;
- pad functions;
- pinmap ID where applicable;
- package-level MPN override only when the ordering code differs by package.

### Ratings And Tolerances

Selection must continue using existing rating/value/tolerance checks:

- value kind and value must match the query or required rating;
- power/voltage/current limits must satisfy requested requirements;
- tolerance must not be used unless the selector validates it;
- unknown ratings should not satisfy fabrication-candidate requirements.

## Selection Behavior

### Default Selection

Draft and structural requests may still select generic records. Connectivity
and stronger requests should prefer verified concrete records when all else is
equal and the request does not explicitly require generic behavior.

Selection order should be deterministic:

1. required electrical/package/confidence gates;
2. lifecycle/procurement gates when source evidence is supplied;
3. existing score/confidence ranking;
4. concrete-vs-generic preference for connectivity and stronger acceptance,
   with generic fallback preserved for draft and structural acceptance;
5. equivalence role order, but only among records in the same declared group:
   `preferred`, `alternate`, `fallback`;
6. component ID and variant ID.

Selection requests may use existing safety flags:

- `require_concrete: true` rejects generic records even if they otherwise
  satisfy the query.
- `allow_alternatives: true` permits a deterministic first candidate when
  non-equivalent records remain tied after explicit preference signals.

### Ambiguity

Adding alternatives must not make common requests fail with
`COMPONENT_AMBIGUOUS`. If two concrete parts are intentionally equivalent, the
equivalence model or deterministic preference rules must decide the selected
candidate. Ambiguity should remain blocking only when the records are not
declared equivalent and the selector cannot choose safely.

### Generic Fallback

Generic records remain available for:

- draft workflows;
- structural workflows;
- families where concrete alternatives are not yet verified;
- explicit query or policy that permits generic records.

Fabrication-candidate workflows should warn or block when generic fallback is
used for assembly-critical rows.

## Coverage And Reporting

Component coverage should report alternative readiness:

- per-family `concrete_records`;
- per-family `generic_fallback_records`;
- per-family and aggregate `equivalence_groups`;
- aggregate `groups_missing_preferred`;
- aggregate `groups_with_duplicate_preferred`;
- aggregate `concrete_records_missing_mpn`;
- concrete records missing lifecycle/source evidence when a source directory is
  supplied.

Existing selection output should already surface manufacturer, MPN, lifecycle,
procurement, rejected candidates, and warnings. Add new fields only if coverage
or diagnostics cannot explain alternative selection.

## Validation Rules

Catalog validation should reject or warn on:

- duplicate component IDs;
- concrete alternative without manufacturer/MPN;
- equivalence role outside `preferred`, `alternate`, `fallback`;
- empty equivalence group on a concrete record that declares equivalence role;
- equivalence groups without exactly one preferred record;
- multiple preferred records in the same equivalence group;
- equivalence groups spanning incompatible family, package, value kind, or
  footprint class;
- alternatives that lower confidence below the requested acceptance level;
- concrete passives without value/rating sufficient to satisfy current
  selection queries.

For this implementation, incompatible equivalence metadata means mismatched
normalized family, package type, canonical value kind/value, primary footprint
reference, or pad-function map. Differing ratings or tolerances can be modeled
only when the weaker record still satisfies the same required selection gates;
otherwise it must live in a separate equivalence group.

Validation should remain deterministic with stable issue ordering.

## Source Snapshot Interaction

Source snapshots continue to join by normalized manufacturer and MPN. Concrete
alternatives can be selected without source snapshots for connectivity-level
work, but fabrication-candidate selection should require fresh lifecycle
evidence when the existing procurement policy says so.

No live availability lookup is added in this project.

## CLI And Workflow Behavior

No new top-level command is required. Existing commands should gain richer
output:

```sh
kicadai --json component validate
kicadai --json component coverage
kicadai --json --source-dir ./data/component-sources component coverage
kicadai --json --request ./examples/components/select_resistor.json component select
kicadai --json --request ./request.json --output ./out/project design create
```

If useful, add example selection request files under `examples/components/` for
common value/package alternatives.

## Test Requirements

Add tests for:

- catalog validation of equivalence metadata;
- deterministic concrete alternative selection for common resistor values;
- concrete capacitor selection with voltage rating requirements;
- generic fallback remaining available for draft/structural;
- fabrication-candidate blocking or warning when only generic fallback exists;
- ambiguity blocking for non-equivalent equal-score records;
- no ambiguity for declared equivalent alternatives;
- coverage output including equivalence group counts;
- source snapshot joins for at least one concrete alternative;
- workflow component-selection summary preserving selected manufacturer/MPN and
  procurement evidence.

## Risks

- Too many concrete records can create noisy ambiguous selection.
  Mitigation: add equivalence groups and deterministic preferred roles before
  expanding breadth.
- Curated manufacturer records can go stale.
  Mitigation: keep lifecycle/availability evidence in separate source snapshots
  and require explicit dates.
- Part equivalence can be overstated.
  Mitigation: equivalence is scoped to modeled requirements only and does not
  imply full datasheet interchangeability.
- BOMs may appear more production-ready than they are.
  Mitigation: docs and output must keep snapshot/procurement caveats visible.

## Success Criteria

- Common generated seed workflows can select concrete alternatives for at least
  one resistor, one capacitor, one LED, and one connector/header role.
- Common resistor/capacitor selection tests remain deterministic and no longer
  rely solely on generic records where concrete alternatives exist.
- Coverage reports show alternative/equivalence health.
- `go test ./...` passes without network, KiCad, or distributor credentials.
