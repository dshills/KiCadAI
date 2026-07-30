# Protocol-Aware Bus Buffering And Level Translation Plan

## Phase 0: Freeze The Contract

- Record the measured open-world gap and the behavior-only authoring policy.
- Freeze positive and negative I2C, SMBus, SPI, and UART cases.
- Hash prompts, requirements, and the manifest.
- Add strict tests that reject component names, topology hints, coordinates,
  request identities, and other implementation details.

Exit: the corpus is immutable and production support is not used to author it.

## Phase 1: Normalize Protocol Semantics

- Derive one consistent protocol, signaling mode, lane direction, and default
  state from port contracts and objective constraints.
- Normalize whole-bus frequency, fanout, receiver count, and capacitance.
- Reject inconsistent port/constraint declarations with stable typed errors.

Exit: reordered equivalent requests produce byte-identical provider requests.

## Phase 2: Deterministic Architecture Selection

- Add a generic bus-buffering/translation capability.
- Reuse verified bidirectional open-drain, fixed push-pull, and
  direction-controlled push-pull primitives.
- Add a segmented open-drain architecture selected only by segment count,
  loading, and isolation requirements.
- Order selected devices, support components, alternatives, calculations, and
  repair variables deterministically.

Exit: every positive corpus family selects an architecture and each negative
family fails closed.

## Phase 3: Whole-Bus Electrical Proof

- Solve trunk and branch pull-ups from capacitance, rise time, and sink current.
- Verify aggregate-load allocation and receiver/segment counts.
- Qualify voltage, frequency, load, fanout, direction, inactive state, and
  partial-power behavior from catalog evidence.
- Bound repair ranges without fixture-specific values.

Exit: architecture evidence contains complete, reproducible whole-bus
calculations.

## Phase 4: Startup, Hot-Plug, And Contention Safety

- Represent rail-loss, startup, hot-plug, disable, and contention events.
- Extend trusted simulation planning for the new capability.
- Assert unpowered and disabled high impedance, unaffected-branch continuity,
  no back-power, inactive startup state, and bounded contention current.
- Reject any requested behavior absent from both catalog and model evidence.

Exit: every requested safety behavior has an analytic or trusted-simulation
proof.

## Phase 5: Catalog And Pin Mapping

- Validate datasheet-backed translator/buffer records and evidence locators.
- Verify manufacturer identity, voltage/frequency/channel bounds,
  partial-power semantics, KiCad symbol/footprint mappings, and
  function-to-pin maps.
- Add only generally reusable records needed by the frozen behavior families.

Exit: every selected active component and simulation terminal resolves through
one verified function map.

## Phase 6: Lowering And Physical Constraints

- Lower trunk and branch lanes to distinct semantic nets.
- Emit deterministic local placement for translators, bypass capacitors, and
  pull-ups.
- Preserve voltage-domain grouping and branch isolation in routing.
- Add generic route and return-path constraints suitable for two-layer boards.

Exit: lowered graphs are connected, route-ready, and stable under input
reordering and reversed voltage domains.

## Phase 7: Negative And Metamorphic Tests

- Add failures for omitted partial-power proof, branch-role gaps, inconsistent
  loading, excessive speed/load/fanout, unsupported segment count, ambiguous
  direction, and unsafe contention.
- Add permutations of port order, objective order, segment count, voltage
  ordering, and semantically equivalent numeric encodings.
- Verify stable codes, paths, rationales, candidate fingerprints, and payloads.

Exit: no negative request silently degrades to an unsafe architecture.

## Phase 8: Promotion Matrix

- Add requirement-lane promotion scenarios for each positive corpus family.
- Require architecture, component, simulation, safety, routing, connectivity,
  route-completion, writer, round-trip, and deterministic-repeat gates.
- Include explicit negative cases in the matrix.

Exit: the promotion matrix is complete and references only frozen requirements.

## Phase 9: Local KiCad-Backed Verification

- Run focused and full Go suites locally.
- Run race-sensitive and determinism tests locally.
- Run each promotion scenario twice.
- Run installed-KiCad ERC and strict DRC locally.
- Require writer correctness, connectivity, route completion, and zero
  round-trip differences.
- Preserve existing MCU/sensor, amplifier, clock, writer, and open-world pass
  evidence.

Exit: every required local gate is green with reproducible artifacts.

## Phase 10: Audit, Review, And Commit

- Scan production changes for request IDs, fixture names, coordinates,
  allowlists, and topology shortcuts.
- Update the capability index and open-world audit.
- Stage only milestone files.
- Run Prism on the staged diff and resolve all high and medium findings.
- Commit the reviewed implementation.

Exit: the committed tree and evidence satisfy this specification without
fixture-specific production logic.
