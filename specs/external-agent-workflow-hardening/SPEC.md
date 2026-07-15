# External Agent Workflow Hardening Specification

Date: 2026-07-15
Status: Proposed

## 1. Purpose

Define the work required for an independent AI agent to generate and iterate on
a catalog-resolved KiCadAI design without discovering avoidable contract gaps
late in the workflow.

This project is based on an external-agent attempt to generate a split-supply
Class A headphone amplifier through `generic-circuit-v1`. The attempt exposed
four code defects and several workflow problems:

- verified BJT records referenced the wrong KiCad symbol library nickname;
- the circuit graph could not express the negative-power lane required by the
  schematic IR;
- schematic routing and final writing disagreed about transformed multi-unit
  pin anchors;
- the default provider output budget was too small for a moderately complex
  generic graph;
- live-provider iteration lacked a direct capture-to-replay path;
- diagnostics arrived in validation-stage batches rather than one complete,
  actionable preflight report;
- catalog placeholders were discovered only after provider generation;
- a failed overwrite was reported to have left only metadata in a previously
  complete output directory.

The goal is not to special-case a headphone amplifier. The goal is to remove
generic tool and contract defects that this circuit happened to expose.

## 2. Current Baseline

KiCadAI already provides:

- strict `generic-circuit-v1` provider output;
- trusted component catalog and KiCad library resolution;
- generic multi-unit component lowering;
- deterministic schematic layout, PCB placement, and PCB routing;
- recorded provider fixtures and optional live OpenAI execution;
- bounded provider correction attempts;
- managed-project provenance and overwrite protection;
- KiCad ERC/DRC, connectivity, route completion, writer correctness, and
  round-trip evidence;
- offline default tests and optional environment-gated KiCad/provider tests.

The repository also contains uncommitted patches produced during the external
agent attempt. They are evidence and prototypes, not approved implementation.
In particular, the waypoint fallback must not be committed unchanged because
it may discard requested geometry and silently create a different route.

## 3. Goals

The implementation must:

1. Validate all selectable catalog symbol and footprint IDs against configured
   KiCad libraries before provider generation when requested.
2. Correct BJT catalog identities and keep every evidence string consistent
   with the resolved library identity.
3. Add one safe verified polarized electrolytic path suitable for connectivity
   and ERC/DRC generation without overstating voltage, lifetime, ESR, ripple,
   or fabrication evidence.
4. Let `generic-circuit-v1` explicitly represent split positive, ground, and
   negative schematic lanes.
5. Make schematic layout and final writing use one authoritative transformed
   pin-anchor calculation for every symbol unit.
6. Keep invalid or unexplained waypoint mismatches fail-closed.
7. Make provider output budgets explicit, bounded, configurable, observable,
   and appropriate for generic circuit complexity.
8. Produce actionable provider diagnostics for token exhaustion.
9. Make any successful provider response directly replayable without `jq` or
   manual JSON-envelope conversion.
10. Return all independent graph, catalog, endpoint, layout, and policy
    preflight findings that can be computed safely in one run.
11. Prove that failed overwrite attempts preserve every previously committed
    project artifact byte-for-byte.
12. Document the efficient live capture, recorded replay, and blocker-fix loop
    for human and AI users.
13. Prove the combined improvements with one identity-neutral split-supply,
    multi-unit analog regression graph.

## 4. Non-Goals

This project does not:

- add a headphone-amplifier provider schema or circuit block;
- add arbitrary analog component families;
- claim analog stability, noise, distortion, thermal, SOA, load-drive, or
  safety performance from ERC or DRC;
- permit model-selected symbol pins, footprint pads, or coordinates without
  trusted resolution;
- weaken provider schema validation, catalog confidence gates, ERC/DRC,
  connectivity, writer correctness, or round-trip checks;
- suppress genuine findings;
- silently repair arbitrary waypoint geometry;
- make provider calls part of the default test suite;
- require KiCad for ordinary unit tests;
- promise fabrication readiness for the analog regression fixture.

## 5. Design Principles

### 5.1 Fail early

Failures that can be found from the catalog and configured KiCad libraries must
be found before a paid provider call. Failures that require provider output
must be returned before placement or project writing.

### 5.2 Preserve intent

No repair may change components, units, nets, pin functions, footprints, or
requested schematic route shape without an explicit typed repair policy and
evidence. An unexplained anchor mismatch remains an error.

### 5.3 Make iteration deterministic

A provider response is the reproducibility boundary. Once a response is
received, users must be able to replay it through the identical resolver,
lowering, placement, routing, and validation path without another provider
call.

### 5.4 Preserve completed output

`--overwrite` authorizes replacement only after a complete new project has
been staged and validated for writing. It does not authorize deleting a valid
project before preflight, provider, resolution, or workflow success.

### 5.5 Separate structural proof from engineering proof

Verified symbol and pin mappings permit structural generation. Analog ratings
and performance remain explicit review evidence and may continue to prevent a
fabrication-ready classification.

## 6. Catalog And Library Preflight

### 6.1 Catalog identity correction

The MMBT3904 and MMBT3906 records must resolve to:

- `Transistor_BJT:Q_NPN_BEC`;
- `Transistor_BJT:Q_PNP_BEC`.

These identities were verified directly against the checked-out KiCad symbol
library: both `Q_NPN_BEC` and `Q_PNP_BEC` are defined under
`Transistor_BJT`, and neither is defined under `Device`. Resolver-backed
preflight remains authoritative for installations with different library
versions or nickname mappings; the catalog must fail closed rather than
silently substitute a similarly named symbol.

All related fields must use the same identity, including:

- record-level `symbol_id`;
- variant-level `symbol_id`;
- pinmap IDs;
- verification sources;
- catalog test evidence;
- generated provenance.

Changing only `symbol_id` while leaving `builtin_pinmap:Device:*` evidence is
not acceptable.

### 6.2 Resolver-backed catalog validation

`component validate` must support configured library evidence using the same
root/cache options as `design create`.

For every non-blocked selectable record it must verify:

- symbol ID exists;
- footprint ID exists;
- declared symbol pins exist in the selected symbol/unit;
- declared package pads exist;
- required pin and pad functions are complete;
- pinmap evidence names the same symbol and footprint identities;
- library lookup ambiguity is reported.

Blocked records may refer to intentionally unavailable evidence only when the
blocked reason is explicit and they remain unselectable.

The command must return all catalog resolution findings in deterministic order.

### 6.3 Polarized electrolytic support

Add at least one catalog path with:

- `Device:CP` or another verified KiCad polarized-capacitor symbol;
- verified positive and negative symbol pins;
- a verified footprint with matching positive and negative pads;
- explicit package dimensions or package identity;
- voltage-rating evidence sufficient for the declared connectivity use;
- review-required fields for voltage derating, ripple current, ESR, lifetime,
  temperature, and physical size when not proven.

The generic placeholder may remain for draft-only use. Connectivity and
stronger workflows must select the verified record or fail with a diagnostic
that names the closest unavailable/placeholder candidate and required evidence.

## 7. Split-Supply Schematic Contract

### 7.1 Graph schema

`SchematicLanes` must add:

```json
{
  "power": "top",
  "signals": "middle",
  "power_negative": "lower",
  "ground": "bottom"
}
```

The graph lane enum must support `lower`. The provider JSON Schema, Go model,
strict decoder, normalizer, semantic projection, hashing, and docs must agree.

### 7.2 Validation

When any net has role `power_neg`:

- `schematic.lanes.power_negative` is required for new provider output;
- the value must be `lower` or another explicitly supported non-top lane;
- it must not collide semantically with the positive-power lane;
- lowering must map it to schematic IR `lanes.power_negative`;
- negative-power components must remain below primary signal flow;
- ground remains the bottom reference unless an explicitly supported policy
  says otherwise.

For already captured v1 graphs, a deterministic compatibility normalizer may
derive `lower` only when exactly one distinct net has role `power_neg`. Multiple
negative rails without explicit lane intent are ambiguous and must fail with a
structured diagnostic. A successful compatibility result must also carry a
structured diagnostic so new provider output is not allowed to rely on the
legacy omission.

## 8. Canonical Multi-Unit Pin Anchors

### 8.1 Problem

The schematic router can generate waypoints using a placement snapshot that
does not match the builder's final grid-snapped, rotated, mirrored, unit-aware
pin anchor. Later symbol units can therefore reach project writing with route
endpoints uniformly offset from the final pins.

### 8.2 Required architecture

There must be one canonical operation that computes an absolute pin anchor
from:

- final symbol instance position;
- selected logical unit;
- library pin position;
- symbol rotation;
- symbol mirror state;
- grid normalization policy.

The layout/router and builder must either call the same function or consume the
same immutable resolved-anchor table. Grid snapping must occur exactly once at
a documented boundary.

### 8.3 Waypoint handling

- Correctly anchored orthogonal waypoints are accepted unchanged.
- Duplicate, diagonal, degenerate, or wrongly terminated waypoints fail.
- A uniform translation repair may exist only as an explicit compatibility
  repair with a diagnostic, and only after the canonical-anchor root cause is
  fixed.
- Nonuniform drift must never discard waypoints and silently route directly.
- A direct fallback may be selected only by the schematic layout engine before
  writing, where normal readability and collision validation is rerun.

### 8.4 Regression matrix

Tests must cover:

- unit A, B, and power unit anchors;
- rotations 0, 90, 180, and 270 degrees;
- supported mirror transforms;
- positions on and off the major grid;
- self-loop nets between two pins on one logical unit;
- routes between different units of one package;
- deterministic transaction and schematic output;
- write/read and optional KiCad ERC validation.

## 9. Provider Output Budget And Errors

### 9.1 Configuration

Add an explicit provider output budget with:

- a conservative profile default;
- a larger bounded default for `generic-circuit-v1`;
- CLI override such as `--ai-max-output-tokens`;
- optional environment configuration;
- minimum and maximum bounds;
- inclusion in sanitized request evidence.

The default must be based on the selected profile and schema complexity, not a
single hidden constant for every provider request.

### 9.2 Diagnostics

An incomplete response caused by `max_output_tokens` must report:

- provider and model;
- configured output-token limit;
- usage returned by the provider when available;
- incomplete reason;
- whether retry is permitted;
- a safe suggested next action, such as increasing the bounded limit or using
  a recorded response;
- no API key, authorization header, or raw secret-bearing request data.

Retry remains bounded. KiCadAI must not automatically double the budget in an
unbounded loop.

## 10. Provider Capture And Replay

### 10.1 Replay artifact

After any provider response that passes envelope decoding, KiCadAI must write a
sanitized replay artifact before catalog resolution or downstream workflow
execution. The artifact must be accepted directly by `--provider recorded`
without conversion.

The artifact contains:

- replay schema/version;
- provider/model/response ID metadata;
- normalized intent payload;
- sanitized usage and completion status;
- hash of the provider capability/schema contract;
- no API key, headers, or unrestricted provider request body;
- no raw system prompt, developer prompt, user prompt, tool transcript, or
  correction prompt.

Only the strict-decoded structured intent payload and bounded non-sensitive
metadata may be persisted. Prompt correlation may use a one-way digest, but
the source prompt text must not be recoverable from the artifact. Sanitization
must happen before the first filesystem write, including failure paths.

### 10.2 CLI behavior

Users must be able to:

1. run a live provider once;
2. find the replay artifact at a stable path;
3. rerun the exact downstream lane with `--provider recorded`;
4. compare live and replay critical graph projections;
5. preserve a failed graph for deterministic validation repair.

The CLI result and agent documentation must print the exact replay command.
The artifact must be atomic and deterministic after normalization.

## 11. Aggregated Preflight Diagnostics

Preflight must be organized into deterministic stages:

1. strict envelope/schema decode;
2. graph structural validation;
3. catalog component resolution;
4. unit/function/pin/pad resolution;
5. endpoint and required-pin validation;
6. schematic layout contract validation;
7. PCB constraint validation;
8. policy and acceptance validation.

Each stage returns all independent findings it can safely compute. Later stages
may run with partial data only when they can avoid misleading cascades. A
missing component, for example, should not generate one error for every net
endpoint that references it; dependent findings should be summarized under the
root component failure. This relationship must be represented by stable issue
IDs and explicit parent/root-cause IDs. String matching, path proximity, and
message similarity must not be used to suppress diagnostics.

Diagnostics must provide:

- stable code;
- JSON path;
- concise message;
- evidence or candidate identity where useful;
- retry scope (`provider`, `catalog`, `layout`, `environment`, or `user`);
- suggested correction;
- deterministic ordering and deduplication.

Bounded provider retries should receive the full eligible diagnostic set, not
only the first item.

## 12. Overwrite Safety

### 12.1 Required invariant

Given a complete managed project at `OUTPUT`, any later command using
`--overwrite` that fails before a complete replacement is committed must leave:

- `.kicad_pro` unchanged;
- `.kicad_sch` unchanged;
- `.kicad_pcb` unchanged;
- project-local symbol/footprint tables and libraries unchanged;
- the current manifest/provenance unchanged.

Failure evidence must be written outside the committed project replacement or
inside a separately namespaced attempt area that cannot be mistaken for the
current project manifest. Retention is bounded: keep at most the five most
recent failed attempts per output by default, permit a lower configured bound,
and prune oldest attempts only after the newest failure evidence is durable.

### 12.2 Reproduce before repair

The reported metadata-only directory is currently unconfirmed. Tests must first
exercise failure at every relevant boundary:

- provider failure;
- strict graph failure;
- catalog resolution failure;
- schematic transaction failure;
- placement/routing failure;
- writer failure;
- artifact/promotion-report failure;
- cancellation or timeout.

Each test starts with a known-good generated project and compares core artifact
hashes after the failed overwrite attempt.

If no deletion path is reproduced, implementation changes are not justified.
The regression tests and documentation still remain required.

### 12.3 Commit protocol

When replacement succeeds:

1. generate into a sibling staging directory;
2. validate required project artifacts;
3. write manifest and provenance for the staged project;
4. fsync required files/directories where supported;
5. use an atomic directory exchange when the platform provides one;
6. otherwise write a recovery journal, rename the existing output to a sibling
   backup, rename the staged directory to the target, and restore the backup on
   any failed commit step;
7. fsync the parent directory where supported after each namespace change;
8. clean journals and backups only after the replacement is durable.

The move-aside fallback is recoverable but is not described as an atomic
replacement: the target path may be briefly absent. Startup preflight must
detect its journal and finish or roll back an interrupted commit before reading
or writing the managed project.

Existing atomic writer behavior should be reused and strengthened, not
reimplemented in the CLI.

## 13. Analog Regression Fixture

Add one recorded, identity-neutral split-supply analog graph that combines the
features needed to prove this project:

- positive and negative rails plus ground;
- one multi-unit op-amp package;
- verified small-signal BJT records;
- one verified polarized capacitor where electrically appropriate;
- at least one self-loop or local feedback net;
- explicit schematic groups and lanes;
- no topology-specific provider dispatch or production coordinates.

The fixture may be inspired by the external Class A attempt, but completion is
structural and KiCad-backed only. It must retain `review_required` evidence for
analog performance and must not claim fabrication readiness.

The fixture must prove:

- recorded provider replay;
- complete preflight diagnostics for intentionally broken variants;
- valid catalog/library resolution;
- readable schematic generation without visible machine metadata;
- correct multi-unit anchors and wires;
- PCB connectivity and route completion for required nets;
- clean KiCad ERC;
- strict DRC at the declared candidate level or exact documented real findings;
- writer correctness and normalized round-trip stability.

## 14. Testing Strategy

### 14.1 Unit tests

- catalog identity and evidence consistency;
- library-backed catalog resolution;
- split-supply schema, validation, normalization, and lowering;
- canonical unit pin transforms;
- waypoint fail-closed behavior;
- provider budget selection and bounds;
- token-exhaustion diagnostics;
- replay artifact strict decode and determinism;
- diagnostic aggregation and dependency suppression;
- overwrite hash preservation at each failure boundary.

### 14.2 Integration tests

- live-response fixture capture followed by offline replay;
- recorded graph through resolver, schematic IR, design workflow, and writer;
- multi-unit schematic write/read;
- known-good project followed by each failed overwrite mode;
- existing provider-backed fixtures to detect regressions.

### 14.3 Optional external tests

Environment-gated tests use configured KiCad source/libraries, `kicad-cli`, and
OpenAI credentials. Default tests remain offline and credential-free.

## 15. Acceptance Criteria

This project is complete when:

- every selectable checked-in catalog record passes configured library
  preflight or reports a deliberate blocked/placeholder status;
- BJT catalog identities and evidence are internally consistent;
- a verified polarized electrolytic path is selectable at connectivity level;
- `generic-circuit-v1` can explicitly encode and lower a negative-power lane;
- multi-unit route endpoints exactly match final builder anchors without a
  silent direct-route fallback;
- generic provider budgets are configurable and token exhaustion is actionable;
- a live provider result can be replayed directly with no manual conversion;
- one run returns all independent preflight findings;
- failed overwrites preserve an existing complete project byte-for-byte;
- the analog regression fixture reaches its declared KiCad-backed acceptance;
- existing provider-backed pass fixtures remain clean;
- `go test ./...` and `make lint` pass;
- Prism has no unresolved high findings;
- documentation includes exact capture, replay, catalog-preflight, and recovery
  commands;
- the final worktree is clean.

## 16. Open Questions

1. Should provider output-token budget be exposed only as a CLI flag, or also
   in configuration files and environment variables?
2. Should the replay artifact be written automatically under `.kicadai/`, or
   only when an explicit capture option is supplied?
3. Should legacy split-supply graphs be normalized with a warning or rejected
   so providers must emit the new lane immediately?
4. Which concrete polarized capacitor provides the best first verified record
   without implying current market availability?
5. Should failed-attempt evidence live beside the output directory or under a
   project-local `.kicadai/attempts/` namespace?
6. Is uniform waypoint translation repair needed after anchor unification, or
   can it be removed entirely?
