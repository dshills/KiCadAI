# Amplifier Schematic Alias Cleanup Specification

## Background

The KiCad-backed `class_ab_headphone_protected` example now reaches schematic
electrical validation with verified LMV321 op-amp, diode, output-transistor,
and `headphone_output_protection` block evidence. It still stops before PCB
realization because connected schematic wires receive conflicting generated net
labels:

- `headphones_SIG` conflicts with `output_amp_out`
- `output_lower_emitter` conflicts with `output_upper_emitter`

This is no longer a missing component-selection problem. It is a composition
and schematic emission problem: block-local port names, generated labels, and
requested design intent are being projected onto the same connected conductor
without a single canonical net identity.

## Problem Statement

Generated amplifier block compositions must be electrically meaningful and
human-readable while remaining machine-checkable. A conductor in the schematic
can have one intended net identity. If multiple block ports or aliases describe
that same conductor, the design workflow must resolve them before emitting
schematic labels or before running schematic electrical validation.

The current behavior treats some block-local names as independent labels even
after placement and wiring prove they are the same conductor. The result is a
KiCad-visible schematic that may look connected but cannot be promoted because
the workflow correctly detects conflicting labels.

## Goals

- Define canonical net ownership for composed amplifier blocks.
- Remove generated label conflicts from the protected Class AB headphone
  schematic.
- Preserve useful human-readable design intent, including `HP_OUT`,
  `AMP_OUT_DC_BIASED`, load return, and output-stage emitter semantics.
- Advance `class_ab_headphone_protected` past schematic electrical validation
  into downstream PCB realization stages.
- Ensure schematic-to-PCB transfer uses the same canonical net names seen by
  schematic validation.
- Record the next real blocker in fixture metadata if the fixture still remains
  `expected_fail`.

## Non-Goals

- Promote the protected Class AB headphone fixture to fabrication-ready status.
- Implement active fault protection, thermal/SOA signoff, simulation signoff, or
  speaker-power-amplifier safety.
- Replace all schematic labels with global labels.
- Rewrite unrelated block contracts or generic netlist handling.
- Require `kicad-cli` on developer machines where it is not installed.

## Definitions

- Canonical net: the single net identity used by validation, schematic labels,
  PCB transfer, routing, and evidence.
- Local alias: a block-internal or readability name that may describe a
  canonical net but must not be emitted as a conflicting connected label.
- Exported port: a block boundary signal intended for composition with other
  blocks.
- Alias bridge: internal workflow metadata connecting one or more local aliases
  to a canonical net without creating separate schematic labels on the same
  conductor. This is not a KiCad net-tie component. KiCad net ties are only used
  when the design intentionally connects distinct electrical nets.
- Protected amplifier fixture: the KiCad-backed
  `class_ab_headphone_protected` design example.

## Requirements

### 1. Alias Inventory

The implementation must identify every net name emitted by the protected
amplifier flow for:

- input connector and input reference;
- op-amp gain output;
- bias diode network;
- upper and lower output transistor emitters;
- AC-coupled headphone output;
- headphone return and load reference;
- supply rails and decoupling nets;
- protection block ports.

For each name, the implementation must classify it as one of:

- canonical net;
- local alias;
- block-private internal net;
- requested external signal name;
- invalid duplicate.

A valid alias is a secondary name that the canonical policy is permitted to
collapse. An invalid duplicate is a name conflict that violates the canonical
policy, such as two distinct user-requested names on one conductor or a
reference-domain collision. Invalid duplicates are not silently pruned. They
must produce a schematic electrical validation issue with the conflicting names,
connected conductor, and owning blocks.

### 2. Canonical Net Policy

The protected headphone amplifier must use stable canonical net names across
schematic validation and PCB transfer. At minimum:

- the op-amp/output-stage drive net must have one canonical identity;
- the connected output emitter midpoint must have one canonical identity;
- the AC-coupled headphone output must preserve the requested external intent;
- load return and load reference must remain distinct unless a deliberate
  net-tie policy connects them;
- power rails must not be renamed by signal-flow aliases.
- signal nets connected to power rails must be reported as shorts during
  netlist elaboration, before the later naming-only alias resolution step, not
  hidden by selecting the power rail name.
- nets with different reference-domain metadata, such as analog ground, digital
  ground, power ground, load reference, or shield/chassis, must not be merged
  without an explicit net-tie policy.

Power rail detection must use component pin electrical types, such as KiCad
`power_in`/`power_out`, when symbol-library data has been resolved. If library
pin metadata is unavailable at the current workflow stage, the resolver must
fall back to explicit block-contract supply metadata and record the fallback in
evidence. If neither library pin metadata nor block-contract supply metadata is
available, the net enters restricted resolution mode: it may keep its existing
name, but it must not be merged, renamed, or used to suppress a conflict until
metadata is available. A net may exit restricted mode through explicit
user-provided metadata, block-contract metadata propagation from all connected
component pins, or a project fixture allowlist entry stored in the design
example metadata with reviewer notes and the exact net names it permits. It
must not depend only on net-name string matching.

Electrical checks for shorts, incompatible reference domains, and incompatible
pin classes must run on the raw elaborated connectivity graph before alias
suppression or canonical visible-name selection. Alias resolution is only a
naming step after those checks.

If multiple candidate names are available for the same conductor, the resolver
must choose the most external or user-meaningful name when safe, which means
priority levels 1 and 2 in the visible label precedence list below. Otherwise
it must choose the block contract's canonical port name and record aliases as
metadata.

A visible name is safe only when selecting it does not mask unintended
connectivity or hide an electrical short, does not rename a power rail or
protected reference domain, does not conflict with voltage/current/domain
metadata, and does not merge load return, load reference, analog ground, power
ground, or chassis/shield nets without an explicit net-tie policy.

Visible schematic label precedence is:

1. user-requested external net names;
2. exported instance-boundary port names that represent how a block is connected
   in this design;
3. canonical block-contract names from the reusable block definition;
4. generated stable names;
5. local aliases, which must be suppressed as electrical labels when they
   conflict with a higher-priority candidate.

If more than one distinct user-requested external name appears on the same
conductor, the resolver must fail validation because user intent is ambiguous.
If the same name appears more than once, it is a duplicate assertion of the same
intent and may be collapsed.

If candidates have equal precedence, the resolver must use this deterministic
tie-breaker order:

1. block role priority;
2. topological preference;
3. stable hierarchical block path hash;
4. natural alphanumeric ascending order for non-user-generated local aliases
   only.

Block role priority is:

1. parent/request boundary;
2. source or driver;
3. transformation stage, such as gain, buffer, or bias;
4. protection or conditioning;
5. load or sink;
6. passive support.
7. polarity role inside a differential or bridge-tied-load pair, where explicit
   positive/non-inverting and negative/inverting roles are used only after the
   shared parent role has been selected.

Topological preference favors contract-declared forward output-path names over
feedback, sensing, and internal branch names, and externally visible connector
names over local convenience names. For
single-ended Class AB midpoint aliases, positive/upper-side names may outrank
negative/lower-side names only when both labels describe the same physical
midpoint. Differential and bridge-tied-load outputs must not use that
upper/lower rule; they must resolve by explicit polarity role, then stable
hierarchical path. An output-path name is a name attached to a net on the
shortest contract-declared path between the composition's primary input port
and primary output port; when multiple equal-length paths exist, paths marked
`forward` in the block contract outrank feedback or sense paths. If current
block contracts lack an explicit forward/sense/feedback marker, this work must
add one to the affected Class AB headphone block contracts or use existing
equivalent direction metadata before relying on the preference. If neither is
available, the resolver must use the stable hierarchical path hash and record
that topological preference was unavailable, rather than blocking the entire
fixture on manual metadata completion.

### 3. Schematic Emission

The schematic writer or workflow assembly layer must not emit more than one
conflicting label on a connected conductor. It may:

- emit only the canonical electrical label;
- emit high-interest suppressed aliases as non-electrical annotation text when
  an explicit debug/readability option enables it;
- preserve aliases in generated metadata;
- record the resolution in alias bridge metadata without changing KiCad
  electrical connectivity.

Generated schematics must remain readable: signal flow left to right, higher
potential rails above lower potential rails, load/output to the right, and
readability labels near the relevant conductor.

### 4. Validation Behavior

Schematic electrical validation must fail on real label conflicts, but it must
not fail on resolved aliases. The current hard-coded expected conflict checks
for the protected amplifier fixture must be replaced with assertions that prove:

- the conflict pairs are absent;
- resolved aliases are recorded in evidence;
- schematic electrical validation passes or reaches a later, documented
  blocker;
- downstream stages are no longer skipped solely because of alias conflicts.

### 5. PCB Transfer Readiness

The schematic-to-PCB transfer path must consume canonical net names. Footprint
pad net assignment, placement transactions, route planning, and board evidence
must not reintroduce `headphones_SIG`/`output_amp_out` or
`output_lower_emitter`/`output_upper_emitter` as split or conflicting nets.

### 6. Fixture Metadata

`class_ab_headphone_protected.metadata.json` must be updated after behavior
changes:

- remove alias conflicts from known gaps once fixed;
- add the next verified blocker if the fixture remains `expected_fail`;
- include whether PCB realization, placement, routing, writer correctness,
  structural validation, and KiCad checks are reached;
- keep `require_erc` true;
- keep `require_drc` false until the fixture produces a board layout with valid
  footprint placements, a board outline, and basic routed or intentionally
  unrouted connectivity evidence.

### 7. Optional KiCad Evidence

When `KICADAI_KICAD_CLI` or a project-supported KiCad CLI path is configured,
the fixture should attempt KiCad-backed ERC evidence after schematic validation
passes. When KiCad is unavailable, tests must report the skip deterministically
without hiding local validation regressions.

## Acceptance Criteria

- The protected Class AB headphone fixture no longer reports the two current
  connected-net label conflicts.
- Unit tests prove canonical alias resolution for the affected amplifier nets.
- Design example tests prove schematic electrical validation advances beyond
  the current blocker.
- PCB realization is attempted for the protected fixture, or the next skipped
  stage is documented with a new non-alias blocker.
- Metadata and roadmap text describe the new state accurately.
- Full Go tests pass with the repository-local Go build cache.
- Prism review is run on each implementation phase before committing.

## Test Strategy

- Add focused unit tests for amplifier alias classification and canonical net
  selection.
- Update design workflow tests for `class_ab_headphone_protected`.
- Add regression assertions that the previous conflict pairs are absent from
  schematic electrical issues.
- Add evidence assertions for alias resolution metadata.
- Run generated design examples through the existing workflow test suite.
- Run optional KiCad-backed checks only when the environment is configured.

## Risks

- Resolving aliases too aggressively could merge nets that should remain
  distinct, especially load return, signal reference, and power ground policy.
- Choosing a less readable canonical name could make generated schematics harder
  for humans to evaluate.
- Fixing the schematic blocker may reveal downstream PCB, footprint, or route
  blockers. Those should be captured as the next expected failures, not hidden.

## Open Questions

- Should alias evidence live in the workflow promotion report, block-planning
  summary, schematic stage summary, or all three?
- Should non-electrical annotation text be emitted for suppressed aliases, or is
  JSON evidence enough for this phase?
