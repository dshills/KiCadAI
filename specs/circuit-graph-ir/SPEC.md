# Generic Catalog-Resolved Circuit Graph IR Specification

Date: 2026-07-12
Status: Proposed

## 1. Purpose

KiCadAI currently proves natural-language generation through bounded,
topology-specific provider profiles. Those profiles are safe and reproducible,
but each new circuit requires another provider schema and frequently another
block implementation. This specification defines a generic circuit graph IR
that lets an AI describe explicit component instances and connectivity while
KiCadAI remains responsible for all trusted resolution, validation, layout,
routing, file generation, and KiCad-backed evidence.

The graph is an AI-facing contract, not a KiCad file format and not a request to
trust model-selected pins or footprints. Provider output describes design
intent. KiCadAI resolves it against checked-in catalogs and configured KiCad
libraries, fails closed on ambiguity, and emits a separate resolved graph with
provenance before any writer operation.

## 2. Goals

1. Represent arbitrary catalog-supported component graphs without requiring a
   dedicated circuit block.
2. Keep provider output strict, bounded, deterministic after decoding, and
   incapable of directly selecting unverified writer geometry.
3. Resolve every component, symbol, logical function, symbol pin, footprint,
   and footprint pad through trusted local evidence.
4. Lower one resolved graph into both the existing schematic IR and an
   explicit-component design workflow path.
5. Preserve readable schematic and deterministic PCB intent without accepting
   arbitrary model-generated coordinates as authoritative.
6. Preserve provenance from provider response through generated KiCad files
   and promotion evidence.
7. Retain the promoted BMP280 and protected USB-C LED profiles while the
   generic lane matures.

## 3. Non-Goals

- Inferring a safe circuit from unconstrained prose without validation.
- Accepting KiCad S-expressions, arbitrary file fragments, scripts, or paths
  from a provider.
- Treating model-selected symbol pin numbers or footprint pads as proof.
- Replacing verified circuit blocks. Blocks remain useful reviewed macros.
- Solving analog stability, thermal behavior, regulatory safety, signal
  integrity, or part sourcing merely because a graph is structurally valid.
- Promising fabrication readiness for every catalog-supported topology.
- Building a general-purpose PCB autorouter in this milestone.
- Silently substituting components, pins, voltage domains, footprints, or
  electrical values.

## 4. Architectural Position

The new boundary sits between provider output and existing deterministic
generation systems:

```text
natural-language prompt
        |
        v
strict generic provider schema
        |
        v
unresolved circuit graph
        |
        +--> strict parse / normalize / structural validation
        |
        +--> catalog + symbol + footprint + pinmap resolution
        |
        v
resolved circuit graph + provenance
        |
        +--> schematic IR --> schematic transactions/writer
        |
        +--> explicit-component design workflow --> PCB/write/checks
        |
        v
KiCad ERC/DRC + connectivity + writer + round-trip evidence
```

The existing topology profiles continue to use their current intent-planner and
block paths. A new explicit provider profile, `generic-circuit-v1`, selects the
graph schema. Selection is explicit at the CLI/provider boundary and never
depends on project names, fixture paths, output paths, or provider output.

## 5. Versioning And File Shape

The initial schema identifier is `kicadai.circuit-graph.v1`; the numeric version
is `1`. Unknown fields are rejected except inside explicitly declared extension
maps. Extension maps are not interpreted by writers and cannot alter component,
connectivity, layout, policy, or filesystem behavior.

Top-level shape:

```json
{
  "schema": "kicadai.circuit-graph.v1",
  "version": 1,
  "project": {},
  "components": [],
  "nets": [],
  "no_connects": [],
  "buses": [],
  "schematic": {},
  "pcb": {},
  "policy": {},
  "extensions": {}
}
```

Limits:

- maximum encoded document size: 4 MiB;
- maximum component instances: 512;
- maximum nets: 1,024;
- maximum endpoints per net: 512;
- maximum endpoints across all nets: 32,768;
- maximum explicit no-connect entries: 16,384;
- maximum buses: 128;
- maximum string length for names and values: 256 bytes unless narrower;
- maximum project description length: 2,048 bytes;
- no NaN, infinity, negative geometry, or non-finite numeric values;
- stable IDs use `[A-Za-z][A-Za-z0-9_-]{0,62}` and cannot contain dots.

## 6. Project Intent

`project` fields:

| Field | Required | Meaning |
|---|---:|---|
| `name` | yes | Safe project identity matching `[A-Za-z0-9][A-Za-z0-9_-]{0,127}` exactly. Provider graph names are never shell-expanded or path-normalized; any other character, separator, absolute path, dot segment, or traversal is rejected before filesystem use. |
| `title` | no | Human-readable schematic title. |
| `description` | no | Design summary; never used for dispatch. |
| `acceptance` | yes | `structural`, `connectivity`, `erc-drc`, or `fabrication-candidate`. |
| `board.width_mm` | yes | Positive finite board width no greater than 1,000 mm. |
| `board.height_mm` | yes | Positive finite board height no greater than 1,000 mm. |
| `board.layers` | yes | Integer copper layer count belonging exactly to `{2, 4}`; every other value is a structural error. |
| `board.edge_clearance_mm` | no | Non-negative board-edge policy. |

Project text is evidence and labeling only. It cannot change resolver behavior.

## 7. Component Instances

Each component has:

- `id`: stable graph-local identity;
- `reference`: optional requested reference such as `R1`;
- `role`: normalized functional role used by layout and checks;
- exactly one of `component_id` or `query`;
- optional `variant_id` when an explicit component has multiple packages;
- optional `value` and typed `parameters`;
- optional symbol/footprint constraints;
- required ratings and required logical functions;
- optional manufacturer/MPN constraints;
- `population`: `populate` or `do_not_populate`;
- one physical instance per entry; repeated devices use distinct IDs rather
  than a quantity field.

Provided references must use the existing safe reference syntax and be unique
across all instances and units. Reference assignment may fill an omitted value
when policy allows it, but duplicate or conflicting supplied references block.

### 7.1 Explicit component identity

`component_id` must resolve to one checked-in catalog record. `variant_id`, when
present, must belong to that record. When it is omitted, the record must have
exactly one accepted package variant or trusted catalog metadata must identify
one unique preferred variant; otherwise resolution fails as ambiguous.
Manufacturer or MPN constraints must match resolved evidence; they do not create
a component record.

### 7.2 Constrained query

`query` maps to the existing component selection model:

```json
{
  "family": "resistor",
  "package": "0805",
  "value_kind": "resistance",
  "value": "10k",
  "minimum_confidence": "library_derived"
}
```

Queries must resolve to one accepted candidate. Equal top candidates,
placeholder-only results, missing required ratings, or disallowed lifecycle and
availability evidence block resolution. The provider cannot set query limits or
selection scores to force a candidate.

`minimum_confidence` uses the existing ordered values `verified`,
`library_derived`, `rule_inferred`, `placeholder`, and `blocked`. Selection for
generation cannot accept `placeholder` or `blocked`. Functionally equivalent
records may be resolved only through trusted catalog equivalence metadata and a
unique preferred candidate. The provider cannot request `select_first`, alter a
score, or use availability as an unreviewed electrical-equivalence tie-break.

### 7.3 Symbol and footprint constraints

Provider fields may constrain expected library IDs but cannot override trusted
bindings:

```json
{
  "symbol": {"library_id": "Device:R"},
  "footprint": {"library_id": "Resistor_SMD:R_0805_2012Metric"}
}
```

If a constraint differs from resolved catalog/library evidence, resolution
fails. Empty catalog bindings cannot be filled from provider claims.

### 7.4 Values and ratings

Electrical values are bounded strings parsed by existing engineering-value
rules. Typed parameters are restricted to JSON strings, booleans, finite
numbers, or bounded arrays of those primitives. Required ratings use the
existing `kind`, `value`, and `unit` model and must be proven by the selected
component record.

Value and rating comparisons normalize recognized engineering prefixes and
units to exact base-SI rational values before comparison, so equivalent forms
such as `1000mV` and `1V` compare equal without floating-point tolerance.

## 8. Connectivity

Each net declares:

- unique `name`;
- `role`: signal, power, power_pos, power_neg, ground, return, feedback, bias,
  or shield;
- two or more endpoints except explicit no-connect nets;
- optional `required` flag, default true;
- optional `voltage_domain`;
- optional `net_class` and current/width/clearance constraints;
- optional differential-pair membership.

Endpoints are structured, not concatenated strings:

```json
{"component": "u1", "unit": "A", "selector_kind": "function", "selector": "non_inverting_input"}
```

`selector_kind` is required and is one of `function`, `alias`, or `symbol_pin`.
`selector` is the value interpreted only in that namespace.
`unit` is optional for single-unit symbols and required when a selector is not
unique across a multi-unit symbol. It is a canonical library unit identifier,
not a provider-defined slot. The resolver verifies unit membership and shared
physical-package identity.
Namespaces never fall through: `function` matches only canonical logical
functions, `alias` matches only aliases, and `symbol_pin` matches only a
canonical symbol pin. A selector matching more than one entry in its declared
namespace is ambiguous and blocks resolution. Resolution maps it to one
canonical logical function and one or more verified physical bindings. Most
functions produce one symbol-pin/footprint-pad binding; intentionally duplicated
power or ground functions may produce several. An exact `symbol_pin` selector
produces one binding. Catalog symbol function metadata, package pad functions,
pinmap evidence, and resolver data must prove every binding. Provider pin
numbers are selectors to verify, not trusted mappings.

An endpoint may appear on exactly one ordinary net. A no-connect endpoint may
not appear on an ordinary net and must be listed once in the top-level
`no_connects` array using the same structured selector. This collection avoids
creating hundreds of virtual nets for large devices. Each entry must resolve to
an actual symbol pin and is lowered to an individual KiCad no-connect marker.
Required component functions and required symbol pins must be connected or
explicitly no-connected according to trusted unused-pin policy.

### 8.1 Power domains

Power nets may declare nominal voltage and tolerance. A component endpoint may
declare or inherit an accepted voltage domain from catalog evidence. Known
incompatible domains block resolution. Missing domain evidence is a warning at
structural acceptance and a blocker at `erc-drc` when the component requires a
proven supply.

### 8.2 Buses and differential pairs

Buses group existing scalar nets and do not replace endpoint connectivity.
Scalar member nets retain their electrical roles; there is no separate `bus`
net role in this graph IR.
Differential pairs reference exactly two scalar nets, carry polarity, and may
declare width, gap, and impedance intent. V1 validates and preserves these
constraints; it does not claim impedance synthesis without stack-up evidence.

## 9. Schematic Layout Intent

The graph reuses schematic IR semantics rather than introducing coordinates as
the primary contract:

- flow direction;
- centered page origin;
- functional groups and ordered stages;
- power, signal, and ground lanes;
- relative placements (`near`, `above`, `right_of`);
- local support relationships such as decoupling and pull-ups;
- minimum component/group spacing;
- label preference for long/shared nets;
- page and hierarchy preferences.

Provider coordinates, when accepted in a future version, will be hints only. V1
does not accept absolute schematic coordinates. Normalization derives missing
groups and placements deterministically from roles and graph topology. It must
preserve left-to-right flow, positive power above, ground below, and local
support parts near their owners where topology proves the relationship.

## 10. PCB Intent

PCB intent contains constraints, not copper geometry:

- regions with normalized rectangular bounds;
- relative placement and proximity constraints;
- edge-facing component requirements;
- keepouts;
- critical-net routing priority;
- per-net width and clearance requirements;
- allowed/preferred copper layers;
- analog-sensitive, thermal, high-current, and return-path hints;
- zone intent referencing an existing net.

All PCB region and keepout bounds use millimetres in a board-local coordinate
system whose origin is the upper-left board corner, with X increasing right and
Y increasing down. Absolute component positions are optional deterministic
fixture inputs, not provider fields in v1. The placement engine derives positions from regions,
relative constraints, footprint sizes, board edges, and connectivity. The
routing engine receives resolved pads and net constraints. It must fail when a
required route cannot prove endpoint contact.

## 11. Policy And Repair

Policy separates safe normalization from changes requiring clarification.

V1 may allow:

- safe project/reference assignment;
- canonical unit and value formatting;
- deterministic component ordering;
- deterministic net/endpoint ordering;
- layout group inference;
- spacing adjustment;
- label insertion;
- board placement adjustment within declared constraints;
- bounded route retry using existing categories.

V1 always forbids automatic:

- component identity substitution after explicit selection;
- pin-function guessing;
- symbol or footprint substitution without catalog evidence;
- voltage-domain changes;
- adding or deleting required connectivity;
- rating fabrication;
- acceptance-level reduction;
- suppressing ERC/DRC findings;
- arbitrary filesystem access or code execution.

Pin guessing is not represented as a configurable policy field in v1. Any input
requesting it is an unknown field and fails strict decoding.

Each normalization records a stable action code, source path, before/after
value, and policy authorization. Forbidden repair requests fail closed.

## 12. Resolved Graph

Resolution produces a separate immutable result. For every component it records:

- selected component and variant IDs;
- source catalog ID, catalog record source references, and catalog content hash;
- manufacturer and MPN when available;
- confidence and acceptance evidence;
- resolved symbol library ID and unit;
- resolved footprint library ID;
- logical-function to symbol-pin mapping;
- logical-function to footprint-pad mapping;
- pinmap source and resolver evidence;
- symbol and footprint library source roots/identities and content hashes when
  available;
- required rating outcomes;
- provider constraints and whether each matched.

For every endpoint it records the original selector, canonical logical
function, verified symbol-pin/footprint-pad bindings, and evidence sources.
Physical pin ownership is checked after resolution so alternate selectors
cannot assign one pin to multiple nets or both a net and a no-connect. Writers
consume only this resolved representation, never the unresolved provider
document.

V1 permits a physical pad to appear in multiple symbol units only when every
unit uses the same physical symbol-pin identifier. It fails closed when
distinct symbol pins collapse onto one footprint pad. N-to-1 pin/pad mappings
require explicit catalog semantics and are unsupported in v1.

Resolution is deterministic for a fixed graph, catalog, source snapshot,
library index, and KiCadAI version. These identities and hashes are persisted.

## 13. Lowering To Schematic IR

The adapter maps:

- graph metadata to schematic IR metadata;
- resolved components to schematic IR components;
- resolved symbol pins and roles to schematic IR pins;
- graph nets and explicit no-connect entries to schematic IR nets/markers;
- buses to scalar nets plus bus declarations;
- schematic groups, lanes, placements, and rules to existing layout semantics;
- repair policy to the existing schematic IR policy.

The adapter must not invent symbols, pins, body geometry, or values. Body and pin
geometry come from resolver evidence. If geometry required for safe layout is
unavailable, the graph may remain valid at structural acceptance but cannot be
promoted through readable/ERC-clean generation.

The resulting schematic IR must pass existing validation, normalization,
transaction determinism, write/read electrical checks, and readability gates.

## 14. Lowering To The Design Workflow

The design workflow gains an explicit-component path alongside `blocks`.
Production code must not wrap each component in a synthetic block.

The request carries:

- resolved component identities and bindings;
- explicit nets and canonical endpoints;
- schematic IR/layout;
- board and library requirements;
- placement and route constraints;
- validation and retry policy;
- graph provenance.

Block and explicit-component modes may coexist only after macro expansion has
lowered block instances into the same resolved graph. V1 requests use one mode
at a time to avoid duplicate ownership. Existing block requests remain
unchanged.

The explicit path must participate in the existing workflow stages:

1. graph resolution evidence;
2. component selection evidence;
3. schematic realization and electrical checks;
4. PCB footprint/pad realization;
5. placement;
6. routing and route completion;
7. project write;
8. writer correctness;
9. internal validation;
10. KiCad ERC/DRC and round-trip.

## 15. Provider Integration

The generic provider profile is selected explicitly:

```sh
kicadai --prompt-file request.txt \
  --provider openai \
  --ai-profile generic-circuit-v1 \
  --output ./out/project \
  design create
```

Recorded mode uses the same profile and a sanitized response. The generic
schema exposes only unresolved graph fields. Provider attempts remain bounded
to two. Parser, resolver, schematic, PCB, routing, ERC/DRC, and writer failures
are not sent back as open-ended model correction loops.

The existing automatic BMP280/LED semantic dispatch remains available when
`--ai-profile` is omitted. Unknown explicit profiles fail before provider calls
or output creation.

## 16. Threat Model

The provider and graph input are untrusted. Defenses include:

- strict unknown-field rejection;
- bounded sizes and collection counts;
- no provider-controlled paths, commands, environment variables, URLs, or
  library roots;
- safe local IDs and project-name normalization;
- explicit component/query union validation;
- ambiguity rejection;
- catalog-backed identity, rating, symbol, footprint, pin, and pad proof;
- endpoint ownership and cardinality checks;
- voltage-domain checks;
- no-connect completeness;
- finite geometry and board-bound checks;
- policy-controlled normalization audit;
- no hidden dispatch based on names or paths;
- sanitized evidence without prompts, credentials, authorization headers, or
  hidden provider reasoning;
- transactional output after preflight succeeds.

The graph cannot lower acceptance requirements or allowlist genuine ERC/DRC
findings. A malicious or malformed graph must produce structured diagnostics
without provider execution retries beyond the configured bound and without
partial project writes.

## 17. Diagnostics

Diagnostics use stable codes and JSON paths. Initial categories:

- `GRAPH_SCHEMA_INVALID`;
- `GRAPH_LIMIT_EXCEEDED`;
- `GRAPH_COMPONENT_DUPLICATE`;
- `GRAPH_COMPONENT_SELECTION_INVALID`;
- `GRAPH_COMPONENT_UNRESOLVED`;
- `GRAPH_COMPONENT_AMBIGUOUS`;
- `GRAPH_SYMBOL_MISMATCH`;
- `GRAPH_FOOTPRINT_MISMATCH`;
- `GRAPH_PIN_UNRESOLVED`;
- `GRAPH_PAD_UNRESOLVED`;
- `GRAPH_PINMAP_CONFLICT`;
- `GRAPH_NET_INVALID`;
- `GRAPH_ENDPOINT_DUPLICATE`;
- `GRAPH_REQUIRED_PIN_UNCONNECTED`;
- `GRAPH_VOLTAGE_DOMAIN_CONFLICT`;
- `GRAPH_LAYOUT_UNSUPPORTED`;
- `GRAPH_PCB_CONSTRAINT_INVALID`;
- `GRAPH_REPAIR_FORBIDDEN`.

Diagnostics identify whether clarification, catalog work, library resolution,
layout correction, or circuit correction is required. They do not suggest
weakening acceptance.

## 18. Representative Graphs

Checked-in examples will include:

1. protected USB-C LED, reproducing the promoted block design;
2. protected USB-C BMP280, reproducing the promoted sensor design;
3. RC low-pass filter using explicit resistor/capacitor/connectors and no
   dedicated block;
4. multi-stage catalog-only design, initially an input protection + RC filter +
   transistor switch or similarly bounded topology proven by available catalog
   records.

The first two prove compatibility. The latter two prove graph generality.
Production behavior may not inspect their names, paths, generated references,
or coordinates.

An abbreviated RC graph:

```json
{
  "schema": "kicadai.circuit-graph.v1",
  "version": 1,
  "project": {
    "name": "rc_filter",
    "acceptance": "erc-drc",
    "board": {"width_mm": 40, "height_mm": 25, "layers": 2}
  },
  "components": [
    {
      "id": "r1",
      "reference": "R1",
      "role": "resistor",
      "query": {"family": "resistor", "package": "0805", "value_kind": "resistance", "value": "10k"},
      "value": "10k",
      "population": "populate"
    },
    {
      "id": "c1",
      "reference": "C1",
      "role": "capacitor",
      "query": {"family": "capacitor", "package": "0805", "value_kind": "capacitance", "value": "100nF"},
      "value": "100nF",
      "population": "populate"
    }
  ],
  "nets": [
    {"name": "FILTER", "role": "signal", "required": true, "endpoints": [
      {"component": "r1", "selector_kind": "symbol_pin", "selector": "2"},
      {"component": "c1", "selector_kind": "symbol_pin", "selector": "1"}
    ]},
    {"name": "GND", "role": "ground", "required": true, "endpoints": [
      {"component": "c1", "selector_kind": "symbol_pin", "selector": "2"}
    ]}
  ],
  "no_connects": [],
  "buses": [],
  "schematic": {"flow": "left_to_right", "origin": "centered", "groups": [], "placements": []},
  "pcb": {"regions": [], "placements": [], "keepouts": [], "zones": []},
  "policy": {"allow_reference_assignment": true, "allow_layout_inference": true}
}
```

Ports or connector instances complete one-endpoint external nets in the full
fixture. The abbreviated example intentionally omits that detail for clarity;
the strict fixture must satisfy net-cardinality rules.

## 19. Artifacts And Provenance

The generic lane writes under `.kicadai/`:

- `circuit-graph-input.json`: sanitized normalized unresolved graph;
- `circuit-graph-resolution.json`: selected components and pin/pad evidence;
- `circuit-graph-actions.json`: authorized normalization/repair actions;
- `schematic-ir.json`;
- `generated-request.json`;
- existing workflow, promotion, writer, ERC/DRC, and round-trip artifacts.

Provider prompt text and credentials are not persisted. Artifact hashes enter
the manifest and rationale chain.

## 20. Testing Strategy

### Unit tests

- strict decoding, unknown fields, trailing JSON, and limits;
- normalization determinism and caller immutability;
- duplicate IDs, invalid values, endpoint ownership, net cardinality;
- explicit component and constrained-query resolution;
- ambiguous and missing components;
- symbol/footprint constraint mismatches;
- logical function, symbol pin, footprint pad, and pinmap conflicts;
- voltage-domain and required-pin checks;
- policy-authorized and forbidden repairs;
- schematic and PCB constraint validation.

### Adapter tests

- graph-to-schematic IR golden projections;
- graph-to-design-workflow projections;
- deterministic output across ordering, project-name changes, and repeated runs;
- no fixture-name/path dependency;
- block macro and explicit graph semantic equivalence where applicable.

### Integration tests

- write/read schematic electrical and readability checks;
- PCB pad-net and route-completion checks;
- writer correctness and normalized round-trip;
- optional KiCad ERC/DRC promotion fixtures;
- recorded provider end-to-end tests;
- opt-in live provider semantic equivalence.

Default tests remain offline, deterministic, credential-free, and do not
require KiCad. Live OpenAI and KiCad-backed lanes are environment-gated.

## 21. Acceptance Criteria

The milestone is complete only when:

1. The strict graph parser and validator reject malformed and unsafe input.
2. All components and endpoints resolve through trusted catalog/library data.
3. Graphs lower deterministically to schematic IR and explicit PCB workflow
   input without synthetic circuit-specific blocks.
4. Required schematic and PCB connectivity is graph-complete.
5. RC and multi-stage fixtures prove non-block topology support.
6. Generic reproductions of LED and BMP280 are semantically equivalent to
   their promoted critical design projections.
7. Promoted generic fixtures have clean KiCad ERC/DRC, writer correctness, and
   zero unexpected normalized round-trip diffs.
8. Existing provider-backed BMP280 and LED fixtures remain pass.
9. Recorded and live generic provider outputs have equivalent critical
   projections.
10. Full tests, lint, Prism review, documentation, and worktree cleanliness are
    complete.

## 22. Explicit V1 Limitations

- Only catalog and library records available to the local installation can be
  resolved.
- Query ambiguity is not automatically repaired.
- V1 supports two- and four-layer board declarations but promoted fixtures may
  remain two-layer.
- Differential-pair intent is preserved and validated but not claimed as
  impedance-controlled without stack-up evidence.
- Arbitrary analog correctness and simulation are outside structural graph
  validation.
- Layout remains constrained by current placement/routing engines.
- Existing blocks are not yet automatically expanded into graph macros unless a
  reviewed expander is implemented during the milestone.
- Fabrication release remains a separate acceptance decision.

## 23. Open Questions

1. Whether graph endpoints should permit only logical function names after v1,
   removing canonical symbol pin selectors from provider input.
2. Whether block macro expansion belongs in `internal/blocks` or a neutral graph
   expansion package.
3. Which catalog-only multi-stage topology has sufficient current evidence for
   the first KiCad-backed pass without adding component families.
4. Whether four-layer graph fixtures should remain structural until current
   routing and DRC promotion evidence is broader.
5. Whether the generic provider profile should remain explicit permanently or
   later become the default after a larger promotion corpus.
