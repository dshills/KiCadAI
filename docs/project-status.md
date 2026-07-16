# Project Status

KiCadAI's direct-file generation workflow is the main functional path. The
project is beyond basic file serialization: supported designs can move through
structured intent, deterministic planning, component and block selection,
schematic and PCB realization, placement, routing, writer validation, and
optional KiCad-backed checks.

## Production-Capable Foundations

### KiCad File Generation

- Writes `.kicad_pro`, `.kicad_sch`, `.kicad_pcb`, project-local symbol and
  footprint libraries, and library tables.
- Preserves and round-trips supported KiCad structures with normalized semantic
  diff evidence.
- Provides read-only inspection and evaluation for imported projects.
- Requires explicit authorization before applying transactions to imported
  projects.

See [KiCad Direct File Writers](kicad-file-writers.md) and
[Validation And Analysis](validation-and-analysis.md).

### Structured AI Inputs

- Structured intent derives requirements, constraints, selected blocks,
  calculated values, assumptions, and fail-closed gaps.
- Schematic IR separates circuit intent, layout intent, and repair policy.
- Provider-backed natural language uses strict schemas rather than passing
  free-form prose into KiCad writers.

See [Intent Planning And AI Workflow](intent-planning.md) and
[AI Generation](ai-generation.md).

### Proven Provider Lanes

Two natural-language profiles are promoted:

1. Protected USB-C BMP280 I2C breakout with 3.3 V regulation, pull-ups,
   decoupling, and an external connector.
2. Protected USB-C LED indicator with fuse, TVS, bulk capacitance, and a
   current-limited active-high LED.

Both have checked-in recorded responses, opt-in live OpenAI equivalence tests,
and KiCad-backed promotion fixtures. Their strict lanes reach AI status
`ready` and promotion `pass` with clean KiCad ERC/DRC, complete required-net
routing, writer correctness, and zero unexpected normalized round-trip diffs.

An explicit generic circuit-graph lane now resolves provider topology against
the checked-in component catalog and lowers it through the same deterministic
schematic and PCB workflow. Generic RC filter, protected USB-C LED, protected
USB-C BMP280, LMV321 AC-coupled gain-stage, and dual-LMV321 signal-conditioner
graphs have recorded
KiCad-backed pass evidence without topology-specific schemas. The generic
RC filter, BMP280, and both LMV321 lanes also have live OpenAI pass evidence
through schematic generation, placement, complete required-net routing, writer
correctness, strict ERC/DRC, and round-trip checks. The dual-stage fixture proves
component multiplicity, topology-derived stage ordering, shared VREF/power
trees, independent feedback loops, and deterministic inter-stage routing. Both
LMV321 fixtures keep analog performance claims explicitly review-required. The
protected USB-C LED currently carries recorded, rather than live,
generic-provider pass evidence.

Generic multi-unit lowering is now proven by recorded and live-provider LM358
evidence. One catalog-resolved physical LM358 package produces distinct KiCad
units A, B, and P while retaining one reference, footprint, and BOM identity.
Shared supply pins and unit-to-pad mappings are validated fail-closed. Its recorded lane and
semantically equivalent live-provider graph both have clean KiCad-backed pass
evidence. Live provider execution remains optional and credential-gated.

The generic contract is deliberately strict. It expands topology expression,
but does not bypass catalog, pinmap, placement, routing, ERC/DRC, writer, or
round-trip gates.

### Schematic Readability

Generated schematics use deterministic role, stage, and lane classification;
left-to-right flow; power-above and ground-below conventions; orthogonal
routing; spacing checks; and labels for long or shared nets. Readability and
schematic electrical evidence are emitted as workflow stages.

Large schematic IR inputs can use deterministic hierarchy partitioning. Exact
human-editor-quality layout for arbitrary imported schematics remains outside
the current guarantee.

### PCB Placement And Routing

The workflow supports block-aware placement, block-local copper, inter-block
route trees, pad/contact graph evidence, route completion, net classes, and
bounded placement-routing retry. Promoted fixtures prove required-net
connectivity rather than parseability alone.

This is not yet a general-purpose autorouter for arbitrary dense boards. See
[Placement And Routing](layout-routing.md).

### Components And Circuit Blocks

The checked-in catalogs and resolvers cover the promoted designs plus a growing
set of passives, connectors, regulators, I2C sensors, protection parts, and
low-voltage amplifier components. Generated schematic symbols can carry
component identity, manufacturer, MPN, confidence, lifecycle, rating, and
pinmap evidence.

Catalog snapshots are curated local evidence, not live distributor inventory or
pricing. See [Libraries And Components](libraries-and-components.md),
[Circuit Blocks](circuit-blocks.md), and [AI Readiness Matrix](ai-readiness.md).

### Validation And Fabrication Evidence

Available gates include internal schematic electrical checks, PCB connectivity,
route completion, writer correctness, KiCad ERC/DRC, semantic round-trip,
physical-rule profiles, and fabrication package evidence. Promotion reports
distinguish blocked, candidate, and pass readiness.

A KiCad-backed `pass` proves the requested validation level. It does not replace
part sourcing, manufacturer-specific review, analog performance validation,
thermal analysis, safety review, or fabrication release approval.

## Live KiCad API

Live IPC supports connection probing, version and document discovery, and
capability reporting. KiCad's exposed write API remains too limited to be the
primary generation mechanism, so KiCadAI writes native project files and uses
`kicad-cli` for external validation.

## Generic Functional Evidence

Generic graphs can resolve ideal fixed-regulator, resistor-divider DC, and RC
low-pass AC models through a deterministic trusted registry. They can also use
graph-derived Modified Nodal Analysis assembled from resolved connectivity and
catalog-backed linear primitives, or bounded nonlinear DC operating-point
analysis for reviewed signal-diode and NPN/PNP BJT primitives. Providers request
bounded analyses and node assertions; they cannot supply topology labels,
device parameters, equations, matrices, solver settings, executable code, or
model files.

The held-out LMV321 buffered two-pole fixture uses automatic hierarchy and two
analyses without a known block/topology model. It passes catalog resolution,
simulation assertions, routing/connectivity, clean KiCad ERC and strict DRC,
writer correctness, zero root/child/PCB round-trip diffs, and byte-identical
recorded replay. The held-out MMBT3904 emitter-degenerated bias fixture adds
deterministic source/gmin continuation evidence and passes the same simulation,
routing, connectivity, KiCad ERC/DRC, writer, round-trip, and recorded-replay
gates without a provider topology classification. Singular, unstable,
unsupported, nonconvergent, operating-limit, incompatible, and numerically
unbounded requests fail closed. This is deterministic functional evidence, not
parasitic, tolerance, thermal, transient, SOA, or fabrication signoff.

## Amplifier Coverage

Class A, Class AB, and op-amp headphone examples provide schematic readability,
semantic, component, routing, and simulation evidence for narrow low-voltage
headphone slices. This work is useful regression coverage, not proof of a
general power-amplifier generator.

Power-amplifier generation still needs broader active-device models, SOA and
thermal constraints, compensation/stability analysis, protection topologies,
high-current layout rules, and simulation-backed promotion before it can be
treated as autonomous or fabrication-ready.

## Status Meanings

- `blocked`: a required gate failed or the request is unsupported.
- `candidate`: useful generated artifacts exist, but required evidence is
  warning-level, incomplete, or skipped.
- `ready`: the requested workflow gates completed successfully.
- promotion `pass`: checked-in or generated evidence satisfies the declared
  promotion level; inspect the report for the exact gates that ran.

## Remaining Direction

The next materially different capability should be deterministic transient
analysis for a tightly bounded reviewed subset, starting with capacitor state
and diode/transistor switching behavior, fixed time-step/work limits, and a
held-out switching waveform fixture. Continue to broaden catalog and
pin/function evidence only when that target exposes a concrete gap.

See the [Roadmap](../specs/ROADMAP.md) for prioritized work and the
[Development Reference](development.md) for repository-level limitations and
test commands.
