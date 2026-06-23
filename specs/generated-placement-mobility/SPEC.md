# Generated Placement Mobility Specification

## 1. Purpose

Make generated block-local PCB placements movable under the bounded
placement-routing retry loop while preserving local-route intent, generated net
intent, and hard mechanical constraints.

The current generated retry corpus proves pad hydration, retry evidence, safe
stops, and boundary behavior. It also documents the remaining blocker:
components realized from circuit blocks with local routes are treated as fixed,
so full generated boards can expose retry diagnostics without letting retry
move the components that most need adjustment. This project defines a safe
mobility model for generated placements so AI-generated boards can improve
through retry instead of only reporting why retry stopped.

## 2. Problem Statement

Generated block PCB fragments often contain:

- verified relative component positions;
- block-local routes;
- connector or mechanical constraints;
- generated footprint refs and net assignments;
- routing intent that should survive placement changes.

Today the safest interpretation is to freeze many of those placements. That
avoids corrupting verified fragments, but it also prevents the retry loop from
resolving congestion, proximity, edge pressure, and route-length issues on real
generated boards.

AI callers need the system to distinguish:

- hard-fixed objects that must never move;
- group-locked block fragments that may translate or rotate together;
- locally movable support parts whose local route intent can be regenerated;
- soft preferred positions that can be overridden by retry evidence;
- generated routes that must be invalidated and rebuilt after placement moves.

Without that distinction, a generated design can be parseable and diagnosable
but still not meaningfully repairable.

## 3. Goals

This project must:

- introduce explicit generated placement mobility semantics;
- keep hard constraints immutable during retry;
- allow eligible generated block components to move under bounded retry;
- preserve block-local intent by moving eligible local-route groups together or
  by invalidating and regenerating local routes when safe;
- record mobility decisions in machine-readable placement and retry evidence;
- prevent stale routes, pad summaries, or net assignments after movement;
- update generated full-board retry fixtures to prove actual movement
  improvement;
- keep default tests deterministic and independent of KiCad CLI;
- preserve current generated output when retry is disabled.

## 4. Non-Goals

This project does not:

- implement a general global PCB placer;
- implement push-and-shove routing;
- move imported or user-authored content;
- infer mobility from KiCad GUI locks unless represented in the KiCadAI model;
- solve every local-route topology after arbitrary rotation;
- make retry enabled by default;
- require KiCad DRC in normal tests;
- change schematic generation semantics.

## 5. Current Baseline

Implemented foundations include:

- generated block PCB realization fragments;
- deterministic placement requests and results;
- component bounds, pads, groups, keepouts, board regions, and constraints;
- placement quality reports and repairable diagnostics;
- generated footprint pad hydration through resolver-backed and seed records;
- routing handoff from placement;
- bounded placement-routing retry with attempt history and best-attempt
  selection;
- generated full-board retry fixtures that prove pad hydration, no-eligible
  boundary behavior, and generated multi-block net intent;
- generated transaction provenance for safe repair replay.

Known gaps:

- generated block placements with local routes are too often fixed;
- retry does not have a typed mobility policy for generated refs or groups;
- local-route operations do not consistently declare whether they are
  transformable, rebuildable, or hard-fixed;
- retry evidence does not state why a generated component was movable or
  immovable;
- generated fixtures do not yet prove a full `design create` board improves by
  moving generated block components.

## 6. Mobility Model

Every generated placement participant must have an explicit mobility class.

Required classes:

- `fixed`: never moved by retry. Used for user-fixed refs, hard mechanical
  anchors, board-edge connectors with fixed placement, mounting holes, and any
  content without safe ownership evidence.
- `group_transform`: may translate, and later rotate if supported, only as
  part of a declared generated group. Relative positions inside the group stay
  unchanged.
- `local_rebuild`: may move independently if affected generated local routes
  are dropped and rebuilt from current pads.
- `soft_preferred`: starts at a preferred generated location but may move when
  retry evidence justifies it.
- `unowned`: not eligible for mutation. This is the default for imported,
  preserved, stale, or unsupported content.

Each class must expose:

- reason;
- owner scope, such as block ID, workflow stage, or transaction operation ID;
- allowed transforms;
- route handling policy;
- constraints that can block movement;
- evidence path suitable for JSON output and tests.

## 7. Ownership And Safety

Retry may move only generated content with fresh ownership evidence.

Minimum ownership evidence:

- generated project provenance is valid when mutation persists to disk;
- placement participant came from the current workflow or generated
  transaction;
- footprint ref and UUID are stable;
- pad summaries are present for moved refs before rerouting;
- net assignments can be checked after movement;
- local-route operations are known and attributable.

Movement is blocked when:

- the target is imported, stale, malformed, or preservation-only;
- a footprint lacks usable geometry or pads;
- a route touches an unowned ref;
- a generated route cannot be classified as transformable or rebuildable;
- a hard keepout, fixed region, board edge, side, or rotation constraint would
  be violated;
- movement would remove route or net evidence without replacement.

## 8. Local-Route Intent Preservation

Generated local routes must be handled explicitly.

Supported policies:

- `transform_with_group`: routes between refs in the same
  `group_transform` group translate with the group. No route topology change is
  allowed.
- `invalidate_and_rebuild`: local routes are removed before retry movement and
  regenerated from hydrated pads after placement.
- `preserve_fixed`: route stays fixed and blocks movement of any connected ref.
- `unsupported`: movement is blocked with a structured issue.

After any movement:

- stale route geometry must not be written;
- route operations must point at the updated refs and pads;
- connectivity validation must run against the moved board;
- retry evidence must state whether local routes were transformed, rebuilt,
  preserved, or blocked.

## 9. Retry Integration

The bounded retry loop must consume mobility semantics before generating a
placement adjustment.

Requirements:

- retry hint resolution must filter candidates by mobility class;
- hard-fixed and unowned refs must remain unchanged;
- group movement must move all group members together;
- independent movement must invalidate local routes before re-routing;
- retry must produce a no-safe-adjustment stop when all candidate refs are
  immovable;
- best-attempt selection must include movement evidence;
- repeated-placement detection must account for group movement;
- failed attempts must preserve the best previous attempt and explain why the
  movement was rejected.

## 10. Evidence Contract

Generated retry output must include compact mobility evidence.

The placement stage may expose aggregate `MobilitySummary` counts first, such
as eligible, fixed, unowned, group-transform, local-rebuild, soft-preferred,
and route-handling counts. Retry-stage movement evidence must add ref and group
lists once movement is implemented.

Required fields, either in existing stage summaries or a new nested section:

```json
{
  "mobility": {
    "eligible_refs": ["R1", "D1"],
    "fixed_refs": ["J1"],
    "moved_refs": ["R1", "D1"],
    "moved_groups": ["led_indicator/status"],
    "blocked_refs": [
      {
        "ref": "J1",
        "class": "fixed",
        "reason": "edge connector fixed by request"
      }
    ],
    "local_routes": {
      "transformed": 1,
      "rebuilt": 2,
      "preserved": 0,
      "blocked": 0
    }
  }
}
```

Tests should assert selected fields instead of full volatile JSON snapshots.

## 11. Fixture Requirements

Default fixtures must live near the existing generated full-board retry corpus
and must run without KiCad CLI.

Required fixture classes:

1. **Group Transform Improvement**
   - A generated block group moves together.
   - Relative intra-group positions are preserved.
   - Routing or validation evidence improves.

2. **Local Rebuild Improvement**
   - At least one support part moves independently.
   - A generated local route is invalidated and rebuilt.
   - Net connectivity remains valid.

3. **Hard Constraint Preservation**
   - Fixed connector, keepout, board outline, and edge constraints stay
     unchanged while other generated refs move.

4. **No Safe Movement Boundary**
   - Retry diagnostics exist, but every candidate is fixed, unowned, or missing
     evidence.
   - The workflow reports a clear stop reason and does not mutate placement.

5. **Multi-Block Generated Board**
   - Two or more blocks share nets.
   - Retry moves eligible generated content without breaking inter-block net
     intent.

## 12. CLI Contract

`design create --json` must expose stable selected mobility fields when
`routing_retry` is enabled.

CLI tests should assert:

- retry enabled;
- attempt count;
- moved ref count;
- moved group count;
- blocked ref count;
- local route handling counts;
- final routing or validation status;
- stable stop reason when no movement is possible.

No full JSON snapshot is required.

## 13. Validation Requirements

Every moved generated board must pass default structural checks:

- transaction validation;
- writer correctness where generated files are written;
- board validation;
- net-to-pad connectivity checks;
- route completion or explicit unrouted-net evidence;
- no stale local route geometry after movement.

Optional KiCad DRC can be added behind existing opt-in flags later, but it is
not required for this project.

## 14. Backward Compatibility

- Existing requests must behave the same when `routing_retry` is disabled.
- Existing fixed placement behavior must remain available through explicit
  `fixed` or `preserve_fixed` policy.
- Existing generated retry boundary fixtures must continue to pass, although
  expectations may move from "no eligible hints" to a more precise mobility
  blocker when appropriate.
- The transaction and writer output must remain deterministic.

## 15. Acceptance Criteria

This project is complete when:

- generated placements carry explicit mobility classes;
- retry moves eligible generated refs or groups in at least one full generated
  board fixture;
- hard-fixed refs, keepouts, board edges, and local-route constraints are
  preserved;
- stale local routes are never written after movement;
- machine-readable mobility evidence is exposed to tests and CLI callers;
- default `go test ./...` passes without KiCad CLI;
- README and `specs/ROADMAP.md` identify this milestone as implemented and move
  the next priority to fabrication output validation.
