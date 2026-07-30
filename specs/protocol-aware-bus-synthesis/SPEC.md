# Protocol-Aware Bus Buffering And Level Translation

## Objective

Add deterministic, behavior-driven synthesis of buffered and voltage-translated
digital buses. The synthesizer must choose an architecture from electrical and
protocol requirements, qualify concrete catalog parts, and emit a physically
realizable KiCad design without relying on request identity, fixture names,
topology hints, or hand-authored coordinates.

This milestone closes the largest unsupported cluster measured by the
open-world capability evaluation: protocol-aware bus buffering and level
translation.

## Behavioral Input Contract

Requests describe only observable boundaries and behavior:

- protocol, signaling mode, lane direction, and inactive/default state;
- voltage domains and their valid powered and unpowered ranges;
- maximum bus frequency, rise/fall-time limits, capacitive loading, receiver
  count, and fanout;
- partial-power-down, back-power, startup, hot-plug, and contention behavior;
- required segmentation or branch-fault isolation; and
- manufacturing-neutral board size and layer limits.

Requests must not name components, packages, pins, nets, topology families,
providers, models, coordinates, layers, routes, or repair actions.

## Supported Architecture Families

### Open-drain I2C and SMBus

The synthesizer must:

- preserve bidirectional wired-low semantics on every data and clock lane;
- solve pull-ups from the declared rise time and worst-case capacitance while
  respecting sink-current limits;
- account for aggregate, trunk, and per-segment capacitance;
- choose a verified translator or buffer with explicit open-drain frequency and
  voltage-domain evidence;
- require partial-power isolation evidence whenever either domain may be
  unpowered; and
- create independent translated branches when branch isolation is required.

### Push-pull SPI and UART

The synthesizer must:

- derive lane direction from the external contracts;
- preserve declared inactive states;
- distinguish fixed-direction from direction-controlled translation;
- qualify output drive, input thresholds, frequency, load, and fanout;
- prove disabled or unpowered high-impedance behavior when required; and
- reject ambiguous or conflicting bidirectional push-pull contracts.

## Safety Semantics

Acceptance requires explicit evidence for every requested safety behavior:

- no back-powering through signal or control pins when either supply is absent;
- deterministic startup state;
- high impedance for disabled, unpowered, or isolated branches;
- hot-plug containment without corrupting an unaffected branch;
- bounded current during declared contention events; and
- stable fail-closed diagnostics when the catalog cannot prove the behavior.

Unknown evidence is not proof. A request that requires partial-power, hot-plug,
or contention safety must be rejected if the selected component record and
simulation model do not establish it.

## Whole-Bus Calculations

For every accepted open-drain bus, evidence must record:

- receiver and segment count;
- aggregate, trunk, and per-segment capacitive load;
- allowed rise time and operating frequency;
- selected pull-up resistance and tolerance;
- worst-case rise time and low-state sink current; and
- allocation consistency between aggregate and segmented loads.

For every accepted push-pull bus, evidence must record frequency, fanout,
capacitive load, direction, inactive state, and qualified output-drive margin.

All calculations and alternative ordering must be deterministic.

## Catalog And Model Evidence

Every active device must resolve to a verified catalog record with:

- manufacturer orderable identity;
- datasheet source and evidence locator;
- KiCad symbol and footprint mapping;
- function-to-pin mapping;
- supported voltage, frequency, direction, signaling, channel-count, load, and
  partial-power bounds; and
- a trusted simulation model whose terminal contract matches the catalog
  function map.

Passive values must be solved generically. Production code must not contain
fixture-specific coordinates, request IDs, part allowlists, schemas, or block
families.

## Physical Realization

The lowered design must:

- preserve distinct trunk and branch nets;
- place each translator with its local supply bypass capacitors;
- place each open-drain pull-up in its own electrical segment;
- keep local support parts within a bounded distance of the active device;
- keep translated voltage domains distinguishable through placement and
  routing;
- route every required endpoint without merging isolated branches; and
- remain deterministic under reordered but semantically equivalent input.

## Frozen Corpus

Before the capability is considered promoted, freeze an identity-neutral corpus
containing:

- a partially powered I2C translation;
- a partially powered SMBus multi-branch bus with aggregate loading and branch
  isolation;
- an SPI translation with mixed lane directions;
- a UART translation with an explicit inactive state;
- reversed voltage-domain ordering;
- startup, rail-loss, hot-plug, and contention operating cases; and
- negative cases for missing partial-power proof, inconsistent load
  allocation, excessive capacitance/frequency/fanout, missing branch roles,
  unsupported segment count, ambiguous push-pull direction, and unsafe
  contention.

Each positive corpus case must be replayable from its strict requirement file.
The manifest must bind prompt and requirement hashes and forbid implementation
details.

## Acceptance

Completion requires all positive corpus cases to pass:

- strict schema and behavior-only checks;
- deterministic architecture search and component evidence;
- safety assertions and trusted-model simulation;
- lowering, placement, routing, connectivity, and route completion;
- clean installed-KiCad ERC and strict DRC;
- writer correctness and zero-difference round trip; and
- normalized two-run deterministic replay.

All negative cases must fail closed with stable, typed diagnostics. Existing
clock, amplifier, MCU, sensor, writer, and open-world promotion suites must
remain green. Prism must report no unresolved high- or medium-severity findings
before committing.

## Non-Goals

This milestone does not claim USB, MIPI, LVDS, Ethernet, CAN, RS-485,
transmission-line compliance, protocol-controller synthesis, or arbitrary
high-speed signal-integrity closure. It does not infer missing safety
requirements on behalf of a request.
