# Project Status

Last verified: 2026-07-23 by the complete repository suite, the external-review
ladder, and the clean-checkout installed-KiCad promotion corpus.

The six-circuit independent external review is now a release-blocking,
machine-readable regression ladder. Its fixes cover atomic composed placement,
design-scoped stock-library diagnostics, bounded JSON transport, public
function-level discovery, and lane-neutral creation evidence. See
[External Review Regression Ladder](external-review-regression.md) and its
[completion audit](../specs/external-review-mitigation/AUDIT.md).

KiCadAI's direct-file generation workflow is the main functional path. The
project is beyond basic file serialization: supported designs can move through
structured intent, deterministic planning, component and block selection,
schematic and PCB realization, placement, routing, writer validation, and
optional KiCad-backed checks.

## Production-Capable Foundations

### Reproducible Promotion Evidence

An unmodified checkout can now run `make promotion-bundle` without hand-set
library paths. The command resolves the locked KiCad 10.0.3 toolchain or
bootstraps its checksum-pinned distribution, executes the release promotion
matrix twice in isolated roots, and requires clean ERC, strict DRC,
connectivity, route completion, writer correctness, zero normalized round-trip
differences, and equal normalized outputs.

The result is a `sha256-<manifest-digest>` bundle containing immutable
toolchain, command, request, project, validation, and comparison evidence. Its
standalone verifier checks the full inventory and semantic gate contract
without KiCad or network access. The configured `Installed KiCad Promotion`
workflow reruns the same command and publishes the verified bundle as a
commit-named Actions artifact. Ordinary push and pull-request CI remains
offline.

This makes the supported evidence reproducible; it does not expand the
supported circuit, part, simulation, routing, or fabrication envelope.

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

- `intent compile` translates ordinary behavior-first requests into strict v3
  requirements through a fail-closed provider boundary. Source coverage,
  uncertainty, clarification ownership, installed capabilities, architecture
  selection, model provenance, and closed-loop evidence are hash-bound and
  persisted.
- Structured intent derives requirements, constraints, selected blocks,
  calculated values, assumptions, and fail-closed gaps.
- Schematic IR separates circuit intent, layout intent, and repair policy.
- Provider-backed natural language uses strict schemas rather than passing
  free-form prose into KiCad writers.

See [Intent Planning And AI Workflow](intent-planning.md) and
[AI Generation](ai-generation.md).

### Behavioral Intent Compilation

The `behavioral-intent-v1` profile accepts behavior, interfaces, operating
conditions, tolerances, safety limits, and manufacturing-neutral bounds. It
does not accept provider-selected topology, parts, pins, nets, coordinates,
layers, routes, solver controls, model files, or validation claims.

Compilation terminates as exactly one of `ready`, `needs_clarification`,
`unsupported`, or `invalid`. Only `ready` retains an executable requirement,
and only after deterministic architecture search and hash-bound trusted
closed-loop evidence pass. Follow-up answers are bound to the complete original
source, installed-capability snapshot, prior proposal, and prior compilation.

The frozen acceptance corpus contains 24 SHA-256-pinned prompts in 12
paraphrase groups across amplifier, filter, power, protection, sensor, and MCU
domains. It records 12 ready prompts representing six unique supported
contracts, four minimal-clarification prompts, and eight stable unsupported
outcomes. All six supported contracts pass the installed-KiCad promotion lane,
including routing/connectivity, writer correctness, clean ERC, strict DRC,
zero normalized round-trip differences, and deterministic replay. See the
[specification](../specs/uncertainty-aware-behavioral-intent-compilation/SPEC.md)
and [completion audit](../specs/uncertainty-aware-behavioral-intent-compilation/AUDIT.md).

### Proven Bounded Provider Lanes

Two bounded natural-language profiles are promoted:

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

The same `generic-circuit-v1` contract now accepts strict function-level intent
as an alternative to an explicit graph. An AI can state primary functions,
external interfaces, operating domains, semantic connections, and bounded
constraints without supplying pins, pads, support components, coordinates,
layers, or routes. Deterministic catalog-driven synthesis produces the explicit
graph, companion networks, unused-pin decisions, physical defaults, and
resolution evidence before the existing validation pipeline runs.

A frozen eight-circuit held-out corpus covers analog, power/protection,
transistor, sensor/interface, ATmega328P, and combined ESP32/SHT31 designs. All
eight pass the optional KiCad-backed promotion gate with clean ERC and strict
DRC, complete routing/connectivity, writer correctness, zero round-trip
differences, and byte-identical replay. See the
[function-level synthesis specification](../specs/function-level-circuit-synthesis/SPEC.md)
and [capability report](../specs/function-level-circuit-synthesis/CAPABILITY_REPORT.json).

Adversarial multi-function composition extends that evidence to 10 frozen,
behavior-only v2 circuits containing 35 objectives and 3 abstract participants.
The corpus exercises 18 reusable capabilities and 23 whole-circuit constraints,
including shared voltage windows, aggregate loading and current, startup state,
response time, loop margin, integrated noise, thermal margin, isolation,
tolerance evidence, component count, and board area. Every circuit passes
deterministic search and alternatives, lowering, writer and round-trip checks,
complete connectivity/routing, clean installed-KiCad ERC, and strict DRC. The
[specification](../specs/adversarial-multi-function-composition/SPEC.md) and
[acceptance audit](../specs/adversarial-multi-function-composition/AUDIT.md)
define the measured envelope. This is bounded registered-capability evidence,
not arbitrary-circuit support.

An exact block-composition lane supports `ESP32-WROOM-32E-N4` through the
built-in `esp32_wroom_32e_minimal` block. A separate behavior-driven path now
selects verified ATmega328P-A, ESP32-WROOM-32E, or STM32G031K8T6 targets from
catalog MCU evidence, assigns complete peripheral bundles to physical pins,
and expands catalog-declared power, reset, programming, boot, and optional
clock support. Three neutral target-free requests pass deterministic replay and
the installed-KiCad ERC, strict DRC, route/connectivity, writer, and zero-diff
round-trip gates. This is verified-record MCU synthesis, not unrestricted
ESP32-family or arbitrary-device support.

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
set of passives, connectors, regulators, I2C sensors, protection parts,
low-voltage amplifier components, the ATmega328P-A, ESP32-WROOM-32E, and
STM32G031K8T6. MCU records include normalized physical pins, alternate
functions, supply domains, programming interfaces, clocks, boot constraints,
and current budgets.
Generated schematic symbols can carry
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

Simulation-grounded closed-loop synthesis now adds a frozen ten-circuit
behavior-only corpus over the registered catalog and reviewed model registry.
It evaluates deterministic alternatives, runs required analyses and declared
corners, diagnoses failed assertions, applies bounded generic repairs, and
reruns downstream physical gates. The Class-A and Class-AB cases pass installed
KiCad ERC/strict DRC, routing/connectivity, writer correctness, deterministic
replay, and zero normalized round-trip gates. See
[Simulation-Grounded Closed-Loop Synthesis](closed-loop-synthesis.md).

Constraint-driven power-tree and interface synthesis is promoted for a bounded
catalog-backed slice. Typed requirements prove one selected producer per
generated rail, reject rail cycles, aggregate current/quiescent demand, apply
dropout and efficiency evidence, derive transient/stability capacitance, and
check startup sequence, monotonicity, inrush, and thermal bounds. Whole-bus
pull-up windows, level translation, source-series termination, clock
conditioning, passive ADC settling, and catalog-evidence-gated buffered ADC
drive produce calculation evidence and stable unsupported results.

A neutral four-design corpus covers a regulated MCU/sensor subsystem, protected
power-MOSFET load, buffered ADC acquisition path, and Class-AB amplifier power
interface. Every case passes offline deterministic replay plus installed-KiCad
clean ERC, strict DRC, complete routing/connectivity, writer correctness, and
zero-difference round trip. Ten reordered negative cases prove stable failure
codes. These are reusable architecture capabilities, not fixture-specific
circuit families; see the [completion audit](../specs/constraint-driven-power-tree-interface-synthesis/AUDIT.md).

Generic graphs can resolve ideal fixed-regulator, resistor-divider DC, and RC
low-pass AC models through a deterministic trusted registry. They can also use
graph-derived Modified Nodal Analysis assembled from resolved connectivity and
catalog-backed linear primitives, or bounded nonlinear DC operating-point
analysis for reviewed signal-diode and NPN/PNP BJT primitives. A separate
transient workflow covers reviewed capacitors, diodes, and NPN/PNP BJTs with
fixed backward Euler, exact bounded grids, deterministic DC initialization,
and trusted 10%-90% edge measurements. Providers request
bounded analyses and node assertions; they cannot supply topology labels,
device parameters, equations, matrices, integration methods, initial states,
solver settings, executable code, or model files.

The held-out LMV321 buffered two-pole fixture uses automatic hierarchy and two
analyses without a known block/topology model. It passes catalog resolution,
simulation assertions, routing/connectivity, clean KiCad ERC and strict DRC,
writer correctness, zero root/child/PCB round-trip diffs, and byte-identical
recorded replay. The held-out MMBT3904 emitter-degenerated bias fixture adds
deterministic source/gmin continuation evidence and passes the same simulation,
routing, connectivity, KiCad ERC/DRC, writer, round-trip, and recorded-replay
gates without a provider topology classification. The held-out MMBT3904 switch
adds 301 deterministic waveform points plus voltage, rise-time, and fall-time
assertions and passes those same gates. Singular, unstable,
unsupported, nonconvergent, operating-limit, incompatible, and numerically
unbounded requests fail closed. This is bounded deterministic functional
evidence, not arbitrary SPICE compatibility, parasitic, tolerance, thermal,
SOA, or fabrication signoff.

## Amplifier Coverage

Class A and protected Class AB headphone fixtures remain regression coverage.
The bounded speaker-power lane now adds a protected dual-rail complementary-BJT
amplifier delivering at least 10 W RMS into 8 ohms. It has reviewed driver and
output-device evidence, resistive/reactive load cases, tolerance and distortion
budgets, electrothermal/SOA/current-limit checks, DC speaker protection,
stability networks, high-current/Kelvin/star-return layout constraints, and a
KiCad-backed fabrication-candidate `pass` fixture.

This is not a general power-amplifier generator. Bridge operation, materially
higher output power, mains supplies, arbitrary output-device families,
unreviewed heatsink mechanics, and designs outside the checked load/rail/fault
envelope remain unsupported.

## Status Meanings

- `blocked`: a required gate failed or the request is unsupported.
- `candidate`: useful generated artifacts exist, but required evidence is
  warning-level, incomplete, or skipped.
- `ready`: the requested workflow gates completed successfully.
- promotion `pass`: checked-in or generated evidence satisfies the declared
  promotion level; inspect the report for the exact gates that ran.

## Remaining Direction

The next closure step is to expand genuinely unsupported mixed-signal and
power-control primitive/model families, especially dynamic electrothermal and
control-loop evidence that static bounds cannot prove. Broader clock/fanout,
programming-load, converter, isolation, and high-energy protection coverage,
catalog-independent part qualification, and denser-board physical synthesis
remain important. Unknown behavior must continue to produce a stable
capability gap instead of guessed implementation detail.

See the [Roadmap](../specs/ROADMAP.md) for prioritized work and the
[Development Reference](development.md) for repository-level limitations and
test commands.
