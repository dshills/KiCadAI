# External Review Mitigation Specification

Date: 2026-07-22
Status: Proposed
Source review: [`../FEEDBACK.md`](../FEEDBACK.md)

## 1. Purpose

Close the generic workflow defects confirmed by the 2026-07-21 independent
KiCadAI test session so a human or AI agent can move from a valid request to a
reproducible KiCad-backed result on a stock KiCad installation.

The review demonstrated that the core graph, intent, schematic, routing, and
writer architecture can produce useful designs. It also found two P1 blockers:

- composed intent designs can be declared ready and then fail because
  block-local placement is rejected before its rigid group is legalized;
- unrelated diagnostics from the entire installed KiCad library can block a
  design that does not reference the affected symbols.

The remaining findings concern dense placement, bounded machine-readable
output, command discovery, function-level examples, and consistent evidence
artifacts. This specification treats them as one agent-workflow reliability
milestone without weakening electrical, library, writer, ERC, DRC, or
round-trip gates.

## 2. Relationship To Existing Work

This milestone follows, and does not replace, the earlier
`external-agent-workflow-hardening` work. That project established catalog
preflight, graph contracts, canonical schematic anchors, deterministic replay,
aggregated diagnostics, and overwrite safety.

This milestone addresses defects observed after those improvements:

- placement ordering for composed rigid groups;
- design-scoped use of resolver diagnostics;
- stock-install quickstart behavior;
- CLI output and help contracts;
- circuit-lane evidence parity;
- promotion coverage for the independent review ladder.

Existing verified USB-C I2C, LED, power-tree, interface-conditioning, writer,
and analog fixtures remain regression requirements.

## 3. Confirmed Baseline

The findings were checked against commit `cfa5cf23`.

| Review finding | Baseline result | Required disposition |
| --- | --- | --- |
| F1: regulator composition placement | Reproduced | P1 fix |
| F2: whole-library lint blocks unrelated design | Reproduced | P1 fix |
| F3: small-board block placement | Reproduced | P2 fix with F1 |
| F4: JSON output hygiene | Parsing failure not reproduced; excessive volume reproduced | Preserve valid JSON and bound output |
| F5: function-level discoverability | Reproduced | CLI and documentation fix |
| F6: evidence-artifact asymmetry | Reproduced | Shared artifact contract |

The checked-in `examples/intent/regulator_ap2112k_sensor.json` currently reaches
an intent plan reported as ready but fails during placement. The current
placement pipeline attempts individual component placement before applying
`translate_as_unit` group legalization. Failed members are then unavailable to
the group translator.

Circuit preflight currently loads resolver diagnostics for configured library
roots before it determines the referenced design closure. Blocking diagnostics
from unrelated stock KiCad symbols therefore prevent simple designs from
running.

JSON stdout was a single parseable document in the current reproduction when
stdout and stderr were captured separately. This milestone must retain that
property. It must not claim that the original observation was invalid; merged
streams and oversized diagnostic payloads remain hostile to agent use.

## 4. Goals

The implementation must:

1. Legalize authored rigid groups as atomic objects before rejecting their
   block-local member positions.
2. Preserve relative component and owned-keepout geometry during group
   translation.
3. Place feasible composed designs deterministically on smaller boards without
   production logic tied to a fixture, block family, or coordinate set.
4. Prevent a plan or preflight result from reporting unconditional readiness
   when an already-computable deterministic placement blocker exists.
5. Make blocking library validation depend on the resolved transitive design
   closure rather than every entry installed under configured roots.
6. Keep referenced malformed, missing, ambiguous, or incompatible library
   objects fail-closed.
7. Keep comprehensive whole-library auditing available through an explicit
   validation workflow.
8. Guarantee one machine-readable JSON document on stdout in JSON mode and
   keep logs on stderr.
9. Bound diagnostic payloads without hiding counts, root causes, or complete
   persisted evidence.
10. Make `circuit` help, function-level capabilities, and one complete public
    example discoverable without reading source or testdata.
11. Give provider, intent, and circuit creation lanes the same core evidence
    artifact contract.
12. Promote the independent review ladder into deterministic regression and
    optional installed-KiCad acceptance coverage.

## 5. Non-Goals

This milestone does not:

- add topology-specific placement rules for regulators, LEDs, sensors, audio
  circuits, or any other named block;
- add request-specific coordinates, allowlists, schemas, or production fixture
  identities;
- demote genuine errors in symbols or footprints referenced by the design;
- make every installed third-party library conform to KiCadAI's full-library
  policy;
- replace KiCad ERC or DRC;
- claim analog performance, thermal safety, fabrication readiness, or sourcing
  readiness from structural success;
- redesign the general placer or autorouter beyond the confirmed ordering,
  feasibility, and diagnostics gaps;
- require KiCad for ordinary unit tests;
- make external sibling repositories part of the default test suite;
- change a stable public JSON schema merely to carry an internal placement
  distinction that existing group semantics can express.

## 6. Design Principles

### 6.1 Existing intent controls coordinate meaning

Membership in a group with `translate_as_unit: true` gives authored member
positions a group-local meaning. Within that group, a fixed member is fixed
relative to the group transform. A fixed component outside such a group remains
board-absolute.

The implementation should derive this distinction from existing normalized
group and mobility data. A new public request field is permitted only if an
unresolvable ambiguity is demonstrated by tests and documented before schema
changes begin.

### 6.2 Correctness precedes scoring

A legal atomic group placement must be found before optional proximity,
density, or region preferences are optimized. Scoring may choose among legal
candidates; it may not invalidate relative geometry or convert an available
legal placement into a false fixed-position failure.

### 6.3 Validate what the design uses

Library inventory is broader than design validity. Indexing may inspect all
configured files, but creation is blocked only by diagnostics attached to the
resolved design closure or by failures that make that closure unknowable.

### 6.4 Fail closed with bounded evidence

Referenced defects remain blocking and complete evidence remains available.
The CLI may summarize repeated diagnostics, but it must report deterministic
counts, stable codes, representative entries, and the location of the complete
artifact when one is written.

### 6.5 Lane-neutral automation

An agent should not need lane-specific assumptions to locate workflow,
validation, promotion, transaction, or manifest evidence. Lane-specific data
may be added, while inapplicable shared gates must be explicit rather than
silently absent.

## 7. Atomic Rigid-Group Placement Contract

### 7.1 Normalization

Before placement:

- remove components omitted by conditional block expansion from group,
  proximity, net, and keepout inputs;
- validate that every remaining group member and anchor exists;
- when a conditional expansion omits the declared group anchor but leaves other
  members, select the first surviving member in normalized component order as
  the transform anchor, use its authored position as the new local origin, and
  recompute every surviving member offset as `member.authored_position -
  new_anchor.authored_position` so no member-to-member geometry changes, then
  emit deterministic normalization evidence; remove an empty group entirely;
- do not silently retarget an independent required-proximity rule whose semantic
  anchor was omitted: remove the rule only when its owning conditional relation
  was also omitted, otherwise report the missing semantic anchor;
- reject overlapping or contradictory hard ownership when no deterministic
  interpretation exists;
- calculate each translatable group's local geometry, including component
  bounds and keepouts owned by the group;
- preserve authored rotations, sides, relative offsets, and copper-relative
  relationships supported by the existing contract.

### 7.2 Placement order

The placer must partition normalized objects into:

1. board-absolute hard objects;
2. atomic translatable rigid groups;
3. ordinary movable components.

Board-absolute objects are committed first. Rigid groups are then placed in a
stable order based on existing priority, dependency, size, and identifier
rules. Ordinary movable components follow.

The current behavior of attempting all member placements and repairing the
group afterward is not acceptable for `translate_as_unit` groups.

### 7.3 Candidate generation and legality

One group candidate is one shared translation applied to every member and owned
keepout. A candidate is legal only when:

- all member footprint bounds fit inside the usable board area;
- all hard board keepouts and already committed hard objects are respected;
- owned keepouts are transformed with the group and remain legal;
- required layer and side constraints remain valid;
- group bounds and applicable hard regions are satisfied;
- no member-to-member geometry changes;
- all checks use the final transformed geometry.

This milestone's rigid-group legalization is translation-only. It preserves
every member's authored board side and therefore does not mirror a group onto
the opposite side. If a later placement contract permits a shared side change,
that operation must mirror component coordinates, rotations, footprint bounds,
owned keepouts, and authored copper as one explicit atomic transform; changing
only the layer is forbidden.

Search must be deterministic and cover the finite legal grid before returning
`no legal translation`. Candidate limits must not permit a false failure when a
finite exhaustive search is required for an all-or-nothing group.

### 7.4 Failure evidence

When no legal transform exists, diagnostics must identify:

- group ID and anchor;
- transformed group dimensions;
- usable board dimensions;
- hard constraint categories that rejected candidates;
- candidate count or bounded search summary;
- whether failure is board fit, keepout conflict, hard-region conflict, layer
  conflict, or occupancy conflict.

The diagnostic must not cascade into a misleading missing-proximity-anchor
message unless that is an independent root cause.

### 7.5 Readiness consistency

Any deterministic placement feasibility check that can run from a normalized
request without writing output must be available to preflight. If a command
reports overall status `ready`, it must not already know that the same request
and environment has no legal placement.

This does not promise that `intent plan` performs every external KiCad check.
It requires readiness fields and documentation to distinguish semantic planning
readiness from validated creation readiness and forbids known blockers from
being hidden behind an unconditional ready score.

## 8. Design-Scoped Library Validation Contract

### 8.1 Indexed diagnostics

The library resolver must retain diagnostics with enough identity to associate
them with:

- library nickname;
- symbol or footprint identifier;
- source file;
- inherited/base symbol where applicable;
- package or variant dependency where applicable;
- global inventory state when no narrower identity is possible.

An invalid unrelated entry must not make every index consumer fail before
resolution begins.

Safe continuation is diagnostic-granular:

- a file-level decode, syntax, truncation, or structural-integrity failure
  invalidates every entry sourced from that file;
- an object-level semantic diagnostic invalidates that object and any object
  that inherits from or otherwise depends on it;
- unaffected objects from a successfully decoded file remain indexable;
- no partial symbol or footprint produced by a failed parser may enter a design
  closure.

### 8.2 Design closure

Circuit and design preflight must derive a deterministic closure containing:

- directly selected symbols and footprints;
- inherited/base symbol definitions needed to resolve them;
- selected units, pins, pads, and package variants;
- generated local-library dependencies;
- catalog evidence required for those selections.

Only diagnostics belonging to that closure are promoted to design blockers.
An index failure that prevents determining whether a referenced object exists
is itself a closure blocker.

### 8.3 Full-library audit

`component validate` or a clearly documented equivalent remains the explicit
whole-library audit path. It may fail because of any configured library entry.
Creation commands must not silently become whole-library audits.

### 8.4 Required behavior

- A simple referenced RC design succeeds with stock KiCad 10 symbol and
  footprint roots even when unrelated installed entries violate KiCadAI lint.
- Selecting one of those malformed entries fails with the precise associated
  issue.
- Missing or ambiguous referenced libraries fail closed.
- Diagnostic grouping and ordering are deterministic.
- Resolver caches include enough version/root information to avoid reusing an
  incompatible scoped result.

## 9. Machine-Readable CLI Contract

When JSON output is requested:

- stdout contains exactly one JSON document;
- progress, logs, and the final human-readable error line use stderr;
- success and failure use the same stable report envelope conventions;
- repeated issue families are grouped deterministically;
- each group contains total count and a bounded representative sample;
- omitted entries are explicitly counted;
- complete diagnostics are persisted under `.kicadai/` when an output project
  or attempt artifact directory is available;
- secrets, absolute credential paths, and raw provider prompts are excluded.

All command handlers and adapters must write through explicitly injected report
and log writers. Package code must not write directly to `os.Stdout`. External
processes must use captured pipes whose output is decoded or redirected to the
log writer; they must never inherit the JSON stdout writer. Panics remain a
process failure rather than a valid JSON response and must use Go's normal
stderr behavior.

Tests must capture stdout and stderr independently. Merging both streams and
then parsing the result is not a supported JSON transport contract, but the
documentation must make the separation explicit.

The regression suite must also exercise repeated and concurrent diagnostic/log
production while a JSON report is emitted. It must prove that no goroutine,
callback, external-check adapter, or cancellation path writes log text to the
JSON stdout writer under load.

No fixed byte ceiling is imposed on every successful design report because
valid transaction evidence may vary. A representative stock-library failure
must, however, remain within a regression-tested bound and must not emit the
entire unrelated library inventory.

## 10. Discoverability Contract

The following commands must exit zero and provide useful usage text:

- `kicadai circuit --help`;
- `kicadai circuit preflight --help`;
- `kicadai circuit create --help`.

Function-level circuit capabilities must be discoverable from a public command
or generated documentation backed by the same registry used for validation.
The list must include stable operation names, required and optional parameters,
supported endpoint roles, and evidence/readiness limits where applicable.

At least one checked-in example must demonstrate the complete function-level
flow:

1. author or copy the request;
2. run preflight;
3. run create;
4. inspect the result and evidence;
5. run internal and optional KiCad-backed checks.

The example must not depend on private testdata or a provider call.

## 11. Core Evidence Artifact Contract

Successful provider, intent, and circuit creation lanes must emit the following
core artifacts when applicable:

- `.kicadai/design-request.json` or the normalized lane-neutral request;
- `.kicadai/transaction.json`;
- `.kicadai/workflow-result.json`;
- `.kicadai/validation-summary.json`;
- `.kicadai/design-promotion.json`;
- `.kicadai/manifest.json`.

Requirements:

- one shared writer or shared normalized model controls common artifacts;
- every shared artifact JSON object and every manifest uses a mandatory,
  documented `schema_version`; existing version fields are reused where
  present, and incompatible schema changes require a version change;
- lane-specific artifacts remain permitted;
- inapplicable checks have explicit status and rationale;
- manifest entries include deterministic hashes and artifact kinds;
- writes use the existing transactional output safety guarantees;
- failed attempts cannot replace valid current promotion evidence;
- equivalent requests produce byte-stable artifacts after path normalization.

## 12. Independent Review Regression Ladder

The six review requests or identity-neutral equivalents must become a durable
acceptance matrix. Default tests must not depend on
`/Users/dshills/Development/projects/test-kicadai` or any other sibling
checkout.

The matrix must cover:

- explicit graph authoring;
- multi-unit and split-supply generation;
- an intent-composed regulator design;
- the compact LED composition;
- stock-library quickstart behavior;
- a function-level circuit request;
- lane-neutral evidence artifacts;
- JSON-mode success and failure output.

Test fixtures may naturally contain circuit geometry. Production code may not
recognize their paths, identities, coordinates, block names, or hashes.

## 13. Verification Gates

### 13.1 Default gates

- focused unit and integration tests;
- `go test ./... -count=1`;
- repository lint target;
- deterministic repeated-run comparisons;
- JSON stdout parse tests;
- artifact schema and hash tests;
- negative tests proving referenced library defects still block.

### 13.2 Optional installed-KiCad gates

For every phase that changes generated schematic, PCB, placement, routing, or
artifact promotion behavior, run the affected optional KiCad-backed fixtures.
Applicable promoted designs require:

- clean ERC;
- strict DRC with zero real findings;
- internal connectivity success;
- required-route completion;
- writer correctness;
- zero normalized round-trip differences.

The compact LED, AP2112K regulator composition, function-level review design,
and existing USB-C I2C protected fixture are mandatory promotion coverage once
their phase is complete.

## 14. Compatibility And Migration

- Existing valid requests retain their meaning.
- Existing ungrouped fixed positions remain board-absolute.
- Existing `translate_as_unit` requests gain the documented atomic behavior.
- JSON response fields must not be removed without a versioned migration.
- New artifact files are additive; manifest consumers must tolerate them.
- Full-library audit behavior remains available explicitly.
- Any golden changes must be explained by semantic changes and regenerated
  deterministically.

## 15. Completion Criteria

This milestone is complete when:

1. The checked-in AP2112K regulator intent passes creation and all applicable
   KiCad-backed gates on its declared board.
2. The compact 40 mm by 25 mm LED composition passes without a board-size
   workaround.
3. A truly infeasible atomic group fails with precise root-cause diagnostics.
4. The RC quickstart passes with stock KiCad 10 libraries despite unrelated
   installed-library lint findings.
5. A referenced malformed library object remains blocking.
6. JSON stdout is one parseable document on success and failure, and the
   stock-library case has bounded output.
7. Circuit help and function-level capability discovery work without source
   inspection.
8. A documented public function-level example passes its declared gates.
9. Circuit creation emits the shared core evidence artifact set.
10. Existing USB-C I2C, LED, power-tree, interface, writer, and analog pass
    evidence remains clean.
11. All default tests and lint pass.
12. Applicable optional KiCad-backed tests pass with clean ERC, strict DRC,
    connectivity, route completion, writer correctness, and zero normalized
    round-trip differences.
13. Prism reports no unresolved high-severity findings.
14. CI passes on the pushed commits and the worktree is clean.

## 16. Explicitly Forbidden Mitigations

The following do not satisfy this specification:

- increasing only the affected fixture board dimensions;
- changing only the affected fixture coordinates;
- adding symbol, library, block, net, or project allowlists;
- ignoring all library diagnostics during creation;
- demoting referenced symbol or footprint failures to warnings;
- skipping placement, routing, ERC, DRC, connectivity, writer, or round-trip
  checks to obtain a pass;
- hiding diagnostics without deterministic counts and retained evidence;
- creating a circuit-only artifact schema incompatible with other lanes;
- claiming the JSON issue fixed solely because a shell happened to keep stdout
  and stderr separate;
- accepting plan-only evidence as proof that creation succeeds.
