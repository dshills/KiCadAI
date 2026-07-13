# Physical Pad Routing Correctness

## Status

Required by the `generic_usb_c_bmp280_breakout` promotion milestone.

## Problem

KiCad footprints may contain several physical pads that share one logical pad
number. The GCT USB-C connector used by the target fixture has four shield tabs
named `SH`, and its library record explicitly declares
`duplicate_pad_numbers_are_jumpers no`. Every tab therefore needs real board
copper connectivity.

KiCadAI currently handles this in three incompatible ways:

1. a non-empty logical pad list prevents placement from hydrating the complete
   library footprint;
2. inter-block and direct explicit-circuit routing retain only the logical graph
   endpoint `J1.SH`, even after physical hydration;
3. `designapi.Builder.Design` silently inserts straight tracks from the first
   duplicate pad to every later duplicate pad.

The writer-created tracks do not pass through placement, occupancy, routing, or
DRC. On the USB-C connector they cross CC and VBUS pads, creating real shorts.

## Goals

- Treat every hydrated, same-net physical pad on a participating component as a
  required route endpoint.
- Keep routing aliases deterministic and scoped to placement/routing evidence.
- Route those endpoints through the existing obstacle-aware route-tree path.
- Stop the writer from synthesizing unvalidated straight pad-alias tracks.
- Preserve final library pad names and geometry in the written KiCad footprint.
- Fail closed when a required physical pad cannot be resolved or routed.
- Preserve all existing pass fixtures.

## Non-Goals

- Changing electrical graph intent or provider schemas.
- Adding fixture names, references, coordinate tables, or USB-C-specific routing.
- Treating duplicate pad numbers as internally connected when the footprint does
  not provide jumper metadata.
- Replacing KiCad DRC or internal board connectivity checks.
- General autorouter redesign.

## Required Semantics

### Candidate Expansion

Placement must use installed or verified footprint data as the authoritative
physical geometry when it is available. Logical request pads remain net intent,
not a replacement for the physical footprint.

For each inter-block net candidate and direct explicit-circuit routing request:

1. establish the components already participating through explicit graph/net
   endpoints;
2. inspect their hydrated placement pads;
3. add every named pad whose assigned net matches the candidate net;
4. retain the component's existing block and instance provenance;
5. assign deterministic routing-only aliases (`SH`, `SH#2`, `SH#3`, `SH#4`)
   without changing final KiCad pad names;
6. deduplicate by normalized `reference.pad`;
7. run existing local-route-island pruning where applicable.

Pads on components that do not already participate in the net must not create a
new inter-block candidate. Empty-net pads and mismatched nets must not be added.

### Writer Behavior

`designapi.Builder.Design` must be a deterministic projection of explicit
transactions. It must not add copper that was absent from route operations.
Duplicate same-net pads remain legal footprint data, but physical connectivity is
the routing workflow's responsibility.

### Validation

The following remain authoritative:

- route-tree endpoint contact proof;
- route completion;
- internal PCB connectivity;
- writer correctness;
- KiCad strict DRC;
- normalized round-trip comparison.

Missing physical-pad connectivity must block promotion rather than being hidden by
writer-generated copper.

## Tests

- Candidate construction expands one logical shield endpoint to all uniquely
  hydrated same-net shield pad aliases.
- Placement replaces incomplete logical pad lists with complete resolver-backed
  physical geometry.
- Direct explicit-circuit routing clones pads and nets, applies routing-only
  aliases, and requires every matching physical pad endpoint.
- Expansion does not include unrelated components, empty-net pads, or pads on a
  different net.
- Local-route island pruning still occurs after physical-pad expansion.
- `Builder.Design` does not add tracks for duplicate pad names.
- Explicit route operations remain unchanged and are the only source of tracks.
- The captured generic BMP280 replay removes the unsafe cross-connector ties.
- Existing recorded KiCad-backed fixtures remain pass.

## Risks

- Expanding physical endpoints can expose previously hidden unrouted pads.
- More endpoints can increase route-tree search work.
- A routing alias must resolve to the same physical position as the final library
  pad even when the final pad name is duplicated.
- Existing tests may have encoded the unsafe writer behavior as expected output.

These are correctness exposures, not reasons to restore implicit straight ties.
