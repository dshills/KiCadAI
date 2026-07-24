# Generic Standalone Clock Generation

## Objective

Close the remaining `clock_generation` architecture gap with deterministic,
catalog-backed synthesis for two electrically distinct standalone clock-source
families:

1. a high-accuracy packaged source whose frequency reference and output driver
   are qualified as one component; and
2. a relaxation source whose frequency is established by a solved timing
   network.

The supported family must be selected from observable requirements rather than
from topology hints in the request.

## Frozen Benchmark

The benchmark contains two behavior-only requirements:

- `precision_logic_clock` exercises tight frequency, jitter, edge, startup,
  fanout, supply, load, and temperature bounds; and
- `relaxed_logic_clock` exercises a lower-frequency, wider-tolerance source
  with explicit duty-cycle and startup bounds.

Requests may state only external interfaces, observable behavior, operating
corners, and manufacturing-neutral board bounds. They must not name topology,
parts, support components, nets, pins, coordinates, layers, routes, providers,
models, or repair actions.

The corpus manifest and untouched baseline are frozen before production
support is registered.

## Architecture Contract

Architecture selection must:

- choose a source family deterministically from required frequency accuracy,
  jitter, startup, supply, load, fanout, edge, and duty-cycle bounds;
- record the chosen family, catalog identity, calculation evidence, rejected
  alternatives, and stable rationale;
- qualify every source and support component from normalized catalog evidence;
- reject requests that exceed proven accuracy, jitter, startup, loading,
  fanout, supply, temperature, edge, or duty-cycle bounds; and
- produce stable fail-closed codes that distinguish those failure classes.

## Electrical Proof

Every accepted source must prove:

- startup within the requested bound;
- steady-state output frequency and duty cycle across all declared supply,
  temperature, and component-tolerance corners;
- output high/low levels and source/sink current compatibility;
- rise and fall time at the worst declared capacitive load;
- RMS jitter within the requested limit;
- fanout and total capacitive loading;
- supply current and local bypass adequacy; and
- model provenance for every simulated or analytically bounded quantity.

The relaxation family must solve and record its timing network rather than
selecting fixture-specific values. The packaged family must use a concrete
frequency-qualified catalog record rather than a generic footprint proxy.

## Physical Design

Generated boards must:

- place the clock source, local bypass, and timing components as a compact
  timing group;
- give the bypass a short supply/ground return;
- keep relaxation timing nodes short and isolated;
- route the clock output on the preferred layer with a bounded source-to-load
  path and an adjacent return path;
- apply source damping when required by the qualified source/load contract;
- preserve resonator-specific symmetry and keepout rules if that architecture
  is selected; and
- emit machine-checkable placement, timing, route-length, and return-path
  evidence.

## Acceptance

Completion requires:

- both frozen clock cases pass deterministic architecture selection, component
  evidence, corner proof, lowering, placement, routing, writer, ERC, strict
  DRC, zero-difference round trip, and replay;
- the original held-out `digital_clock_source` becomes a full pass and the
  existing benchmark improves from 11/12 to 12/12;
- negative tests prove stable fail-closed behavior for unsupported accuracy,
  startup, loading/fanout, jitter, edge, duty-cycle, supply, and temperature
  requirements;
- no fixture-specific production coordinates, allowlists, schemas, block
  families, or request identities;
- no regression in the existing promotion matrix, amplifier lanes, or
  MCU/sensor cases;
- a clean-checkout installed-KiCad bundle is published for both clock
  families; and
- Prism reports no unresolved high or medium findings before the final commit.

## Non-Goals

This milestone does not claim general RF synthesis, PLL design, differential
clock trees, spread-spectrum compliance, phase-noise integration outside the
declared RMS jitter band, or fabrication release.
