# Function-Level Circuit Synthesis Specification

Date: 2026-07-17

## 1. Summary

KiCadAI already accepts a strict `generic-circuit-v1` graph and resolves
catalog-backed component functions to verified KiCad symbol pins and footprint
pads. That graph is intentionally explicit: the provider currently supplies
the complete component list, support components, nets, schematic grouping, PCB
regions, board layer count, and placement relationships.

This project adds a second, function-level input form to the same
`generic-circuit-v1` provider profile. A provider describes primary functions,
external interfaces, operating domains, semantic connectivity, and bounded
constraints. A deterministic KiCadAI synthesizer lowers that input to the
existing explicit circuit graph before catalog resolution, schematic lowering,
placement, routing, and writing.

The lowered explicit graph is a first-class evidence artifact. Existing graph
validation and KiCad-backed gates remain authoritative; synthesis cannot waive
or weaken them.

## 2. Goals

- Accept function-level intent without provider-supplied symbol pins, footprint
  pads, support components, coordinates, board layers, routes, equations, or
  executable model content.
- Deterministically select catalog-backed primary and support components.
- Resolve semantic component functions to verified units, pins, and pads using
  the existing resolver.
- Insert required decoupling, bias, pull-up, protection, power-source, power-
  flag, and unused-pin handling from reviewed catalog evidence.
- Derive schematic organization, board size/layer policy, placement proximity,
  net classes, endpoint access, and routing policy from topology and catalog
  evidence.
- Fail closed with stable, actionable capability diagnostics.
- Prove generalization against a corpus frozen before implementation and kept
  independent of fixture names.
- Publish per-circuit and aggregate capability evidence.

## 3. Non-Goals

- No claim of unrestricted RF, high-speed, high-voltage, thermal, safety-
  critical, or dense-board design.
- No fixture-specific coordinates, allowlists, schemas, support recipes,
  routing cases, or block identities.
- No provider-generated SPICE, equations, scripts, file paths, or commands.
- No fallback to placeholder or blocked catalog records for connectivity,
  ERC/DRC, or fabrication-candidate acceptance.
- No mutation of imported user projects.

## 4. Provider Contract

`generic-circuit-v1` accepts exactly one of its existing explicit graph form or
a function-level form. Both use `kicadai.circuit-graph.v1` and version `1` so
existing profile selection, replay capture, and policy remain stable.

The function-level form contains:

- project identity and requested acceptance;
- primary function requirements, each with a semantic role, catalog query or
  catalog component identity, value/parameter requirements, ratings, and
  required semantic functions;
- external interfaces expressed as named signals, not connector pin numbers;
- power domains with voltage/current limits and source status;
- semantic connections between primary functions, interface signals, and power
  domains;
- bounded physical constraints such as maximum board dimensions and permitted
  layer count, where omission selects a documented deterministic default;
- policy permission for safe inference and correction.

The function-level form does not expose:

- reference designators;
- symbols, footprints, units, symbol pins, or footprint pads;
- support-component instances;
- schematic groups, lanes, or placements;
- PCB coordinates, regions, keepouts, zones, layers, or routes;
- no-connect markers or KiCad power flags.

## 5. Deterministic Lowering

Lowering is a pure operation over normalized function intent, one immutable
component catalog snapshot, and a versioned synthesis-policy document. It must
produce the same explicit graph and synthesis report for the same inputs.

The ordering key is semantic identity, never map iteration, fixture order, or
catalog file order. Generated identifiers derive from normalized parent IDs,
support-role IDs, and stable ordinal suffixes. Reference assignment remains the
existing writer's responsibility.

Lowering proceeds in this order:

1. Validate and normalize function-level intent.
2. Select each primary catalog record and package using existing acceptance,
   rating, function, lifecycle, and confidence rules.
3. Materialize external interfaces using the smallest verified connector that
   satisfies their signal count and acceptance.
4. Bind semantic connections and power-domain connections to catalog functions.
5. Expand required catalog companion recipes into support components and nets.
6. Merge electrically identical generated nets by stable domain/function
   identity.
7. Mark verified unused functions according to catalog unused-pin policy;
   otherwise stop with a missing-policy diagnostic.
8. Add external-source power flags only to source domains that have no internal
   `power_out` driver.
9. Derive schematic groups and relative placements from the connection graph.
10. Derive board defaults, component proximity, edge access, net classes,
    widths, clearances, and route-retry policy from operating limits and catalog
    hints.
11. Validate the explicit graph with the existing strict validator.

## 6. Catalog Support Recipes

Required support networks are catalog data, not code-selected blocks. A
companion requirement may contain one or more component recipes. Each recipe
declares:

- stable recipe ID;
- catalog family/query and semantic component role;
- required value, ratings, and functions;
- connections from companion semantic functions to parent semantic functions,
  power domains, or named interface signals;
- optional reviewed unused-pin disposition;
- placement and routing hints.

The synthesizer interprets this generic relationship vocabulary. It must not
switch on catalog component ID, fixture name, project name, or prompt text.
Missing or ambiguous recipe evidence blocks synthesis.

## 7. Diagnostics And Capability Report

Synthesis diagnostics use stable categories:

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

Required blocking codes include:

- `SYNTHESIS_INTENT_INVALID`
- `SYNTHESIS_COMPONENT_UNRESOLVED`
- `SYNTHESIS_INTERFACE_UNSUPPORTED`
- `SYNTHESIS_POWER_DOMAIN_INVALID`
- `SYNTHESIS_CONNECTION_UNRESOLVED`
- `SYNTHESIS_SUPPORT_RECIPE_MISSING`
- `SYNTHESIS_SUPPORT_RECIPE_AMBIGUOUS`
- `SYNTHESIS_UNUSED_PIN_POLICY_MISSING`
- `SYNTHESIS_LAYOUT_CONSTRAINT_UNSUPPORTED`

Each issue identifies the function-intent path, capability category, root cause,
and a bounded suggested action. Diagnostics must never suggest exact fixture
coordinates or weakening validation.

The generated capability report contains:

- corpus manifest hash and synthesis-policy version;
- input-intent, lowered-graph, catalog, library, resolution, request, generated-
  file, and round-trip hashes;
- selected primary and support components;
- generated support requirements and unused-pin decisions;
- derived electrical and physical constraints;
- every required gate and its result;
- failure category and root diagnostic when a circuit does not pass;
- aggregate pass counts by circuit domain and failure category.

## 8. Frozen Held-Out Corpus

The corpus manifest and intent files live in
`internal/circuitgraph/testdata/function_corpus`. Its membership is frozen by a
checked-in manifest hash before synthesis implementations may inspect fixture
identity. The initial eight circuits are:

| ID | Domains | Generalization pressure |
| --- | --- | --- |
| `buffered_thermistor_frontend` | analog | op-amp supply, gain network, decoupling, input/output interfaces |
| `dual_stage_active_lowpass` | analog | repeated stages, deterministic branch ordering, simulation |
| `usb_c_ap2112_3v3_supply` | power/protection | CC bias, ESD, regulator companions, source flags, power routing |
| `npn_low_side_status_driver` | transistor | base bias, current limiting, switched load, nonlinear simulation |
| `sht31_i2c_breakout` | sensor/interface | decoupling, two pull-ups, address/reset/unused policy |
| `bme280_i2c_breakout` | sensor/interface | mode straps, address selection, decoupling, pull-ups |
| `atmega328p_isp_controller` | MCU | multi-supply pins, reset bias, programming interface, unused pins |
| `esp32_sht31_controller` | MCU/sensor | two companion sets, shared I2C, boot/enable policy, dense routing |

Corpus circuits may expose missing capabilities during implementation. A
fixture is not removed, weakened, or renamed to make the corpus pass. The
implementation must add generic evidence/capability or preserve a precise
blocked result until that gap is closed.

## 9. Acceptance

For every corpus circuit, success requires:

- strict function-intent decode and validation;
- deterministic lowering with no fixture-specific implementation;
- verified catalog and library resolution to symbols, units, pins, footprints,
  and pads;
- complete required support networks and unused-pin handling;
- trusted simulation where the circuit has an applicable registered model;
- valid schematic semantics and readability;
- deterministic placement and complete required-net routing;
- clean KiCad ERC and strict DRC;
- connectivity and route-completion evidence;
- writer-correctness evidence;
- zero schematic and PCB round-trip differences;
- byte-identical replay of the complete generated file set.

The existing explicit generic-circuit corpus, including the protected USB-C I2C
sensor and protected USB-C LED fixtures, must remain green.

Normal unit tests must not require network access, KiCad CLI, or external
library roots. KiCad-backed corpus tests remain optional locally and required in
the configured CI acceptance environment.

## 10. Completion Rule

This project is complete only when all eight frozen circuits pass every
applicable gate, the aggregate capability report has no unclassified failure,
the explicit graph regression corpus remains green, Prism has no unresolved
high/medium findings, and GitHub Actions passes on the pushed commit.
