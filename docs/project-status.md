# Project Status

Last verified: 2026-07-28 by the complete local short suite; the three-case MCU
power-integrity, three-case neutral MCU, two-case clock/programming, and
four-case power/interface installed-KiCad corpora; protected USB-C LED and I2C,
ESP32 minimal-system, Class-A, and Class-AB fixture regressions; and independent
clean-checkout promotion bundles.

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

Behavior-driven controller support now includes calculated external-crystal
networks and first-class programming-interface electrical evidence. A frozen
corpus proves calculated external-crystal ISP and integrated-clock UART
bootloader designs through the full local installed-KiCad lane, while SWD with
an unpowered target fails closed. See the
[specification](../specs/behavior-driven-clock-programming/SPEC.md) and
[completion audit](../specs/behavior-driven-clock-programming/AUDIT.md).

Controller power support is also calculation-backed for the verified
ATmega328P-A, ESP32-WROOM-32E, and STM32G031K8T6 records. Per-rail evidence
drives source-drop, brownout, local-noise, and bulk-ripple budgets; capacitor
selection requires concrete tolerance, effective capacitance, ESR, ripple,
voltage-derating, temperature, pin-map, package, and fabrication evidence. The
generator emits one local capacitor per MCU supply domain and one bulk
capacitor per rail group with finalized calculations and placement bounds.
Three target-free cases pass the full local installed-KiCad lane, while
unreviewed transients, exceeded brownout budgets, and unqualified temperatures
produce stable unsupported results. See the
[specification](../specs/behavior-driven-mcu-power-integrity/SPEC.md) and
[completion audit](../specs/behavior-driven-mcu-power-integrity/AUDIT.md).

Every normalized creation request now passes a deterministic capability gate
before filesystem mutation. Requests are classified as `supported`,
`experimental`, or `unsupported` from linked architecture, catalog, model,
physical, and verification evidence. Supported requests proceed normally;
experimental requests require explicit `--experimental` authorization and can
never receive fabrication-ready or promotion-pass status; unsupported requests
return stable actionable gaps without writing a project. The assessment is
re-evaluated monotonically through downstream workflow stages and embedded in
workflow, promotion, and manifest evidence. See
[Capability-Aware Generation Gate](capability-gating.md).

Unsupported is now actionable without becoming permissive. The
evidence-driven capability-expansion path deterministically maps typed gaps
from electrically different domains into reusable architecture, component,
model, physical, routing, or verification needs. It accepts only bounded,
hash-verified, explicitly claimed source records; keeps every generated
provider or registry artifact experimental; generates representative and
adversarial promotion cases; and seals results into a content-addressed
review-ready bundle. Promotion requires an explicit approval bound to that
bundle and explicit mutation authorization. Fresh behavior-only requirements
can load the resulting supported registry with `--capability-registry`; the
original failed request remains unsupported and no request-specific exception
is installed.

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
without KiCad or network access. Local clean-checkout execution is the
authoritative development and promotion gate. Existing automatic workflows may
rerun repository checks independently, but they are not started, inspected, or
awaited as part of the local closeout loop.

This makes the supported evidence reproducible; it does not expand the
supported circuit, part, simulation, routing, or fabrication envelope.

The open-world capability matrix uses the same content-addressed bundle
contract. Run `make open-world-capability-promotion-bundle` from an unmodified
checkout to reproduce its five newly promoted held-out cases.

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

### Held-Out Capability Expansion

A frozen twelve-case benchmark now measures behavior-only circuit requirements
across analog, power, digital, MCU, sensor, and mixed-signal domains. Inputs
contain no topology, components, nets, pins, coordinates, layers, or routes.
The initial installed-KiCad baseline passed 5/12 cases and ranked unsupported
families before production changes.

### Open-World Capability Evaluation

A separate frozen discovery/held-out pair measures unfamiliar behavior-only
requirements without topology, part, pin, net, coordinate, route, provider, or
expected-outcome hints. The evaluator publishes deterministic case outcomes,
normalized failure clusters, safety/reuse ranking, affected-case lookup, and
baseline-to-final verification.

The untouched held-out baseline contained 1 ready, 1 clarification, 8
unsupported, 1 ambiguous, and 1 budget-exhausted case. Gap-driven generic
expansion promotes five cases, producing 6 ready, 1 clarification, and 5
unsupported cases. The promoted set spans clock fanout/loading, MCU debug
electrical loading and shared-pin arbitration, unidirectional push-pull
translation with partial-power-down behavior, asymmetric functional isolation,
and protected isolated conversion with startup, shutdown, thermal, inrush, and
fault evidence.

Each promoted case passes deterministic architecture/component selection,
applicable simulation and safety assertions, lowering, routing/connectivity,
writer correctness, clean installed-KiCad ERC, strict DRC, zero normalized
round-trip differences, and byte-identical replay. Unsupported cases retain
stable fail-closed identities; the largest remaining cluster is broader bus
buffering/level translation. See the
[completion audit](../specs/open-world-capability-evaluation/AUDIT.md).

### Evidence-Backed Component Onboarding

KiCadAI can now propose support for an unfamiliar component from behavior
requirements plus immutable manufacturer documents. The production path is
identity-neutral. It validates content hashes, publishers, revisions,
licenses, exact excerpts, value/unit anchoring, claim conflicts, ratings,
temperature, derating, KiCad symbol pins, footprint pads, and registered
simulation-model provenance before ranking candidates.

Candidates remain quarantined and cannot enter ordinary selection until an
independent review approves the exact hash and two normalized runs pass
simulation, connectivity, route completion, writer correctness, zero-difference
round trip, ERC, and strict DRC. Approved component/model overlays are consumed
by component selection, behavioral intent, architecture search, closed-loop
simulation, and design creation without mutating the embedded catalogs.

A frozen identity-neutral corpus covers unfamiliar op-amp, transistor,
regulator, converter, sensor, logic, and interface parts. All seven pass the
installed-KiCad lane twice in each of two clean source roots; 56 canonical
project and simulation artifacts compare identically. The milestone expands
parts within already registered family/model semantics. Unknown physics,
unlicensed models, absent package geometry, incomplete evidence, and
fabrication approval remain fail-closed. See the
[component-onboarding audit](../specs/evidence-backed-component-onboarding/AUDIT.md).

The original expansion report passes 11/12 with the same evaluator and gate
profile. Constant-current regulation passes in power-output, MCU-peripheral,
and sensor-excitation contexts. Precision rectification passes alone and
composed with ADC acquisition. A subsequent generic standalone-clock
milestone adds fixed packaged and resistor-programmed sources and closes the
unchanged benchmark at 12/12. Every row includes
trusted simulation, deterministic architecture/component evidence, complete
physical routing/connectivity, writer correctness, clean ERC, strict DRC,
zero-difference round trip, and byte-identical replay.

The
[baseline](../specs/held-out-capability-expansion/BASELINE_REPORT.json) and
[11/12 expansion report](../specs/held-out-capability-expansion/FINAL_REPORT.json)
are checksum-pinned, and the
[clock closeout](../specs/standalone-clock-generation/AUDIT.md) records the
12/12 result. The five first-wave cases are also bound into a
clean-checkout
[promotion matrix](../specs/held-out-capability-expansion/PROMOTION_MATRIX.json).
This remains bounded benchmark evidence, not proof that every requested
circuit can be generated.

### Hierarchical Multi-Domain Synthesis

A frozen six-system V4 corpus extends the prior flat composition envelope to
multi-block, multi-domain systems. Requirements provide behavior, interfaces,
operating and fault corners, safety limits, and board limits without providing
subsystem membership, topology, parts, nets, pins, coordinates, or routes.

The generated evidence includes a canonical system/subsystem/block hierarchy,
typed boundary contracts, shared rail/reference/clock/reset/protection
accounting, deterministic complete-candidate backtracking, block and
end-to-end verification coverage, physical partitions, and requirement-to-
transaction/KiCad traceability. The corpus covers protected Class-AB,
precision acquisition/alarm, regulated MCU/sensor/communications, isolated
mixed-voltage gateway, current-limited switched load, and split-supply
precision monitor systems. All six pass deterministic two-run offline and
installed-KiCad ERC, strict DRC, connectivity, routing, writer, and zero-diff
round-trip gates.

The [specification](../specs/hierarchical-multi-domain-synthesis/SPEC.md) and
[promotion matrix](../specs/hierarchical-multi-domain-synthesis/PROMOTION_MATRIX.json)
define the measured boundary. Unsupported capabilities, missing
safety-relevant evidence, and exhausted bounded search still fail closed.

### Dynamic Electrothermal And Control-Loop Synthesis

A frozen six-circuit V5 corpus extends behavioral synthesis into dynamic
feedback, power-control, thermal, fault, and protection evidence without
providing topology, parts, equations, models, nets, pins, coordinates, layers,
or routes. It covers a reactive-load line driver, precision analog servo
supply, efficient step-down stage, protected inductive actuator driver,
Class-AB dynamic output stage, and sequenced dual-rail controller.

The implementation resolves reviewed, hash-bound electrical and thermal
models; derives feedback loops and return ratio from connectivity; evaluates
declared supply, load, temperature, tolerance, parasitic, and operating-mode
corners; couples transient electrical loss into finite thermal networks; and
checks event response, protection, recovery, and transient SOA. Dynamic
evidence participates in deterministic candidate rejection/ranking and bounded
repair while immutable safety requirements remain enforced. Two cases prove a
statically acceptable favorite can be rejected in favor of a dynamically safe
alternative.

All six circuits pass local simulation, internal validation, routing,
connectivity, writer correctness, clean installed-KiCad ERC, strict DRC, and
zero-difference round trips. Two independent clean roots, with two executions
per scenario, produced the same 418-file content-addressed bundle and zero
normalized differences. Run `make dynamic-electrothermal-promotion-bundle`
from an unmodified checkout to reproduce that evidence. See the
[specification](../specs/dynamic-electrothermal-control-loop-synthesis/SPEC.md),
[audit](../specs/dynamic-electrothermal-control-loop-synthesis/AUDIT.md), and
[capability report](../specs/dynamic-electrothermal-control-loop-synthesis/CAPABILITY_REPORT.json).

This proves dynamic reasoning inside the reviewed catalog/model envelope. It
does not qualify arbitrary control ICs, converter families, thermal
assemblies, protection devices, RF/high-speed behavior, mains safety, or
unreviewed component substitutions.

### Open-Topology Primitive Synthesis

KiCadAI now has a production API and CLI lane that constructs primitive
terminal-level circuit graphs from strict behavior-only requirements without
selecting a named provider expansion or pre-authored block family. The lane
derives its primitive inventory from accepted catalog, pin/pad, rating,
package, value-domain, and model-provenance evidence; canonicalizes graph
identity; performs deterministic bounded topology/value search; evaluates
declared cases through the trusted simulator; and continues failed candidates
through generic graph-changing repair.

The frozen eight-case corpus passes six complete simulation and installed-KiCad
promotions. The active filter and sensor conditioner each require a topology
change after failed simulation. All six physical results pass schematic
electrical checks, placement, routing/connectivity, writer correctness,
installed-KiCad ERC, strict DRC, zero-difference round trip, and identical raw
project hashes across two clean roots. The discrete regulator and
voltage-window monitor remain stable `OPEN_TOPOLOGY_REPAIR_EXHAUSTED`
outcomes.

Use `kicadai open-topology create` for this lane. Full search and physical
evidence are retained under `.kicadai/`; stdout contains bounded hashes,
consumption, selected topology, status, replay, and artifact references. See
the [completion audit](../specs/simulation-guided-open-topology-synthesis/AUDIT.md)
and [promotion matrix](../specs/simulation-guided-open-topology-synthesis/PROMOTION_MATRIX.json).

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
The catalog now also contains exact, KiCad-resolvable G5V-1 DC5, ULN2803A,
ADS1115IDGSR, MCP4725A0T-E/CH, BD139-16, and CD74HC4053E records for a
transistor-tester path. Unknown ESP32, ADC, DAC, and OLED plug-in boards are
represented only by draft placeholders; they cannot pass connectivity or
fabrication gates until their actual headers and mechanics are captured.
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

Open-topology synthesis now proves discovery for six of eight frozen
requirements. The next closure step is to generalize the search grammar and
diagnosis-to-repair mapping until the remaining regulator and dual-threshold
cases pass without fixture logic, then evaluate new identity-neutral corpora
outside the development distribution. Broader clock/fanout,
programming-load, converter, isolation, and high-energy protection models,
catalog-independent part qualification, and denser-board physical synthesis
remain important. Those gaps can produce deterministic expansion proposals,
but they still require real engineering sources, representative
simulation/workflow/KiCad evidence, and review before entering the supported
registry. Unknown behavior must continue to produce a stable capability gap
instead of guessed implementation detail.

See the [Roadmap](../specs/ROADMAP.md) for prioritized work and the
[Development Reference](development.md) for repository-level limitations and
test commands.
