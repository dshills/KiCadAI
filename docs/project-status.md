# Project Status

Last full bounded Go verification: 2026-09-02, including focused V19
preservation, compositional-beam, deterministic-replay, contract, and
retirement-evidence suites. The release coverage gate reports 80.5% after
excluding generated protobuf bindings, above the required 75% threshold.
The latest installed-KiCad lanes were reproduced on 2026-09-02 with KiCad
10.0.3. The six-scenario clean-checkout promotion bundle passed twice with
identical normalized inventories, and the complete 13-case design-example tier
passed clean ERC, strict DRC, connectivity, route completion, writer
correctness, and zero round-trip differences. The five educational schematics
also remain deterministic and readable. Closed-loop
candidate evaluation now follows a deterministic structural, DC, AC,
transient, thermal/SOA, and exhaustive-promotion schedule with explicit work
budgets, bounded content-addressed reuse, and auditable conservative
dominance. Generic protected-current synthesis continues to promote the frozen
programmable output plus independently authored low-side
sink and high-side source variants twice with deterministic electrical,
physical, and project evidence; clean ERC; strict DRC; complete routing and
connectivity; writer correctness; and zero round-trip differences. The frozen
8/8 benchmark, both neutral physical promotions, four Class-A/Class-AB
fixtures, and protected LED/I2C fixtures remain green. The earlier three-case MCU
power-integrity, three-case neutral MCU, two-case clock/programming, and
four-case power/interface installed-KiCad corpora; protected USB-C LED and I2C,
ESP32 minimal-system, Class-A, and Class-AB fixture regressions; and independent
clean-checkout promotion bundles remain current preservation evidence.

The `v1.0.0` release surface is frozen and is identical to the RC2 capability
boundary. Application version reporting is
separate from connected-KiCad reporting; macOS and Linux AMD64/ARM64 builds are
byte-reproducible and checksummed; clean-install smoke tests cover help,
version, capability reporting, a supported example, and fail-closed refusal.
The supported and excluded boundaries are recorded in
[`SUPPORT.md`](../SUPPORT.md). Historical experimental promotion matrices do
not enter the v1 surface implicitly: without explicit experimental opt-in they
continue to refuse at the capability gate.

The final release incorporates RC2 without adding generation capability. RC2
introduced explicit fast, bounded, and
exhaustive test tiers, representative performance benchmarks, streaming
closed-loop evidence hashing, bounded worker budgets, deterministic parallel
promotion/release work, and lower nonlinear-solver allocation churn. The
reference fast open-topology test loop is approximately 60% shorter while the
full bounded proofs and installed-KiCad promotion evidence remain required for
release acceptance.

The v1.0.1 release-hardening changes are now integrated. They addressed the remaining feedback
latency in the full bounded coverage gate. The v1.0.0 GitHub quality job took
46 minutes 37 seconds, with open-topology synthesis accounting for most of the
serial time. Deterministic cost-based shards now preserve an exact machine-
checked test/package inventory, authenticate set-mode profiles and resource
reports, and reuse only exact content-addressed proofs. This work changes no
generation capability, schema, routing, writer, artifact, or support boundary.
The active post-v1 milestone is generic analysis/model/solver admission. The
production open-topology path now has a version-isolated preflight that derives
required analyses, authenticates bundled and reviewed overlay model sources,
selects an immutable enabled solver, and refuses before topology search when
coverage is absent. Every opted-in numerical attempt is separately admitted
against its exact graph and harness models before evaluation and records model,
parameter, provenance-record, source, compatibility, and solver digests. Seven
stable refusal categories replace implicit fallback. The frozen V20 public
evaluation completed all 24 cases twice: 1 pass, 5 unsupported, 1 unsafe, and
17 exhausted. It preserved V18's admitted pass and safety result, and two
cases that V18 reported as unsupported advanced to later bounded topology
blockers; relative to V18, unsupported therefore fell from 7 to 5 while
exhausted rose from 15 to 17. The selected
model-availability leaf therefore passed the frozen advancement rule. This
remains experimental and does not expand the v1 supported surface.

The V18 versioned extension now produces a complete public pass for a
low-voltage, high-input-impedance, multi-output analog threshold requirement.
The selected graph uses reviewed low-voltage catalog/provenance evidence,
closed-loop endpoint access, coupled threshold-value selection, and a
contention-free diode-wired comparator conjunction. It passes deterministic
replay, physical lowering, and two clean installed-KiCad promotions. The V6–V17
evaluator inputs and replay seals remain byte-identical. See the
[V18 specification](../specs/closed-loop-open-set-capability-expansion/V18_SPEC_ADDENDUM.md).

V19 Phases 1–5 were implemented behind a separate versioned constructor. V19
derives directed causal-graph invariants, applies reusable role-complete stage,
observation-cone, terminal-redirection, and typed-feedback operations, and
composes them with existing repairs in a deterministic depth-four, width-eight
beam capped at 48 evaluated causal candidates. The exact V18 and V17
environments remain separately bound, and every V19-ineligible result delegates
byte-for-byte through V18. The bounded Phase 6 evaluation authenticated the
frozen evaluator and contract, ran all 24 public cases exactly twice, and
completed deterministic replay. Its aggregate outcome was 0 pass, 12
unsupported, 1 unsafe, and 11 exhausted; it also failed to preserve V18's one
admitted pass. V19 is permanently retired, is not part of the v1 supported
surface, and authorizes no capability claim. V18 remains the latest admitted
public capability. See the
[V19 retirement audit](../specs/closed-loop-open-set-capability-expansion/V19_GENERATION_ZERO_RETIREMENT_AUDIT.md).

The behavioral-contract feasibility and realizability prerequisite is now
implemented behind a versioned capability-feedback policy. It conservatively
separates direct-domain topology failures from energy-domain creation,
multi-output composition, and converging multi-control obligations without
changing frozen policy-v1 evidence or legacy synthesis behavior. Public
boundary, deterministic replay, versioned clustering, and broad local
preservation tests pass. See the [completion audit](../specs/behavioral-contract-feasibility-realizability/AUDIT.md).

The independently frozen multi-stage out-of-distribution corpus now closes the
next bounded generalization step. Eight unfamiliar behavior-only combinations
of sensing, decision, feedback, nonlinear transfer, switching power, and
protection pass two-run installed-KiCad promotion. One immutable contradictory
input fails before search with a generic electrical bound, and four unsafe or
unsupported adversarial cases fail closed with stable evidence. The protected
USB-C LED and I2C sensor fixtures remain green. See the [specification](../specs/multi-stage-out-of-distribution-synthesis/SPEC.md)
and [completion audit](../specs/multi-stage-out-of-distribution-synthesis/AUDIT.md).

The active capability-expansion program is
[closed-loop open-set capability expansion](../specs/closed-loop-open-set-capability-expansion/SPEC.md).
V1–V17 remain authenticated historical evidence, including fail-closed
retirements where a generation could not complete. V18 is the current admitted
public-only extension. V19 reused the immutable 24-case V10 discovery corpus
and frozen V17 serial replay transport without opening held-out keys, but its
completed public evaluation failed both advancement and preservation gates.
The V19 constructor is retained only as versioned historical regression
evidence and is excluded from the v1 supported contract.

The successor work is specified separately under
[`generic-analysis-model-solver-admission`](../specs/generic-analysis-model-solver-admission/SPEC.md).
It reuses the authenticated public corpus but cannot mutate or relabel V19.
Its evaluator remains serial and two-replay, and its public result must preserve
the V18 admitted pass and historical safety outcomes before any capability can
be claimed.

The scheduler removes the immediate evaluation-scaling bottleneck without
weakening the trust boundary. Failed candidates stop only at canonical stage
boundaries; selected candidates still prove every required plan and corner.
Cache hits do not consume solver-execution budgets, partial or exhausted
attempts cannot promote, and persisted evidence is invariant across worker
counts. See the
[scheduler specification](../specs/deterministic-synthesis-evaluation-scheduler/SPEC.md)
and [completion audit](../specs/deterministic-synthesis-evaluation-scheduler/AUDIT.md).

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

### Nonlinear And Switching Architecture Synthesis

An independently frozen eight-case behavior-only corpus now covers low-level
bipolar magnitude transfer, bounded bipolar limiting, autonomous square-wave
generation, controlled pulse power transfer, and efficient low-power step-down
conversion, plus two unsafe stress envelopes and one deliberately unsupported
ultra-fast conversion request. Inputs provide only behavior, cases, safety
bounds, and acceptance gates; they do not name parts, primitive families,
equations, models, solver controls, nets, coordinates, or routes.

Generic graph relationships derive polarity folding, limiting, periodic
feedback, control-to-power transfer, and regulated energy-transfer candidates.
The trusted evaluator adds event-aligned nonlinear transient execution,
periodic frequency/duty/ripple/efficiency measurements, bounded convergence
evidence, electrothermal loss coupling, transient SOA, and reviewed
propagation-delay compatibility. Catalog rating exhaustion rejects unsafe
candidates before simulation where possible; unsupported dynamic envelopes
return stable capability-gap evidence and no physical design.

All five positive cases pass deterministic replay, exhaustive declared
corners, readable schematic lowering, placement, complete routing and
connectivity, writer correctness, zero normalized round-trip differences, and
local installed-KiCad ERC and strict DRC. The complete Go suite, the three-case
protected-current corpus, and both protected USB-C KiCad fixtures remain green.
See the
[specification](../specs/nonlinear-switching-architecture-synthesis/SPEC.md)
and
[completion audit](../specs/nonlinear-switching-architecture-synthesis/AUDIT.md).

This is a measured low-energy nonlinear/switching envelope, not unrestricted
power-electronics generation. RF, mains, high-energy storage, arbitrary SPICE
models, unreviewed semiconductors, and fabrication approval remain unsupported.

### Open-Topology Primitive Synthesis

KiCadAI now has a production API and CLI lane that constructs primitive
terminal-level circuit graphs from strict behavior-only requirements without
selecting a named provider expansion or pre-authored block family. The lane
derives its primitive inventory from accepted catalog, pin/pad, rating,
package, value-domain, and model-provenance evidence; canonicalizes graph
identity; performs deterministic bounded topology/value search; evaluates
declared cases through the trusted simulator; and continues failed candidates
through generic graph-changing repair.

The expanded frozen eight-case corpus now passes exactly 8/8 through trusted
simulation and physical lowering. The active filter and sensor conditioner
require topology changes after failed simulation; later generic multi-branch
work also closes the discrete-regulator and voltage-window cases. All physical
results pass schematic electrical checks, placement, routing/connectivity,
writer correctness, installed-KiCad ERC, strict DRC, zero-difference round
trip, and deterministic replay. Two independently frozen neutral cases also
pass two clean installed-KiCad promotions.

Use `kicadai open-topology create` for this lane. Full search and physical
evidence are retained under `.kicadai/`; stdout contains bounded hashes,
consumption, selected topology, status, replay, and artifact references. See
the [completion audit](../specs/simulation-guided-open-topology-synthesis/AUDIT.md)
and [promotion matrix](../specs/simulation-guided-open-topology-synthesis/PROMOTION_MATRIX.json).

### Simulation-Grounded Architecture Synthesis

The primitive-only lane now evaluates and ranks multiple materially different
architectures rather than returning the first pass. Its frozen behavior-only
corpus covers a continuous-conduction Class A audio stage, a complementary
Class AB power stage, and a 60 Hz notch filter. Generic active-stage,
complementary-follower, bias/feedback, protection, and balanced-bridge
relationships produce distinct topology hashes without accepting topology
names, part identities, values, nets, coordinates, or repair hints as input.

Analytic seeds retain equations, units, inputs, and source requirements.
Catalog-backed values are evaluated through required operating-point, AC,
transient, distortion, noise, stability, thermal/electrothermal, and SOA
contracts. Selection compares all physically ready passing topologies by
requirement margin, repair count, complexity, and stable hashes and records a
human-readable reason plus the alternatives. A catalog-valid low-sense
resistor alternative for the Class A stage is rejected by bias, thermal, or
SOA evidence while the safer derived value passes.

All three selected circuits pass readable schematic lowering, routing and
connectivity, writer correctness, installed-KiCad ERC, strict DRC, zero
normalized round-trip differences, and identical project replay across two
clean roots under KiCad 10.0.3. See the
[completion audit](../specs/simulation-grounded-architecture-synthesis/AUDIT.md)
and [promotion matrix](../specs/simulation-grounded-architecture-synthesis/PROMOTION_MATRIX.json).

This is strong bounded architecture generation, not unrestricted arbitrary
electronics. Unreviewed parts/models, RF/high-speed, mains/high-energy safety,
mechanical thermal qualification, and dense arbitrary boards remain outside
the supported envelope and must fail closed.

### Out-of-Distribution Architecture Generalization

The independently frozen follow-on corpus covers regulated low-voltage output,
dual-threshold indication, current-to-voltage conversion, low-level full-wave
transfer, frequency-selective transfer, and protected programmable current
output without prescribing topology, parts, nets, or geometry. Five of six
designs evaluate multiple topology hashes and pass trusted electrical evidence
plus two identical installed-KiCad physical promotions. Four separate unsafe
thermal, SOA, bias, and dynamic envelopes fail closed reproducibly.

That corpus historically left protected current output unsupported. A later
checksum-frozen milestone now closes that measured gap through generic
source/sink orientation, transconductance/value derivation, independent
startup and fault control, compliance, thermal/SOA evidence, readable
lowering, and two-run installed-KiCad promotion. See its
[completion audit](../specs/generic-protected-current-output-synthesis/AUDIT.md).

### Diagnosis-Driven Repair

Electrical topology repair and generated-board physical correction now emit
the same `kicadai.diagnosis-driven-repair.v1` evidence contract. It records the
normalized failure, affected scope, deterministic proposal, expected effect,
stage re-entry, authorization, bounded consumption, outcome, and state hashes.
A six-component, twelve-pad, two-multi-endpoint-net benchmark begins with a
real router failure, applies a diagnosis-derived relative-spacing correction,
and completes both route trees identically on replay. Its installed-KiCad lane
passes writer correctness, connectivity, route completion, zero-difference
round trip, clean ERC, and strict DRC.

The historical electrical case demonstrates the equally important safe
boundary: it narrows its then-unsupported diagnosis and rejects non-improving
or unsafe proposals. The later generic protected-current milestone supersedes
that capability result without altering the frozen repair evidence. See the
[repair results](../specs/diagnosis-driven-repair/RESULTS.md) and
[preservation report](../specs/diagnosis-driven-repair/PRESERVATION_REPORT.md).

### Cross-Stage Autonomous Repair

The versioned `kicadai.cross-stage-autonomous-repair.v1` coordinator now owns
workflow ordering, trial budgets, exact checkpoints, rollback, earliest-stage
re-entry, preservation, regression comparison, independent confirmation, and
canonical replay evidence across simulation, schematic/ERC, placement,
routing, connectivity, DRC, writer, and round trip. Domain logic remains in
the trusted causal, transaction, and generated-output adapters.

An independently frozen nine-stage corpus recovers without identity, path,
component-reference, coordinate, or case-specific production logic. Negative
tests protect unrelated content, required gates, electrical corners, thermal
headroom, SOA, physical margins, cancellation, confirmation evidence, and
nondeterministic replay. The protected USB-C LED and I2C preservation fixtures
both pass the installed-KiCad lane twice. See the
[specification](../specs/cross-stage-autonomous-repair/SPEC.md) and
[completion audit](../specs/cross-stage-autonomous-repair/AUDIT.md).

### Schematic Readability

Generated schematics use deterministic role, stage, and lane classification;
left-to-right flow; power-above and ground-below conventions; orthogonal
routing; spacing checks; and labels for long or shared nets. Readability and
schematic electrical evidence are emitted as workflow stages.

Topology-aware lowering additionally normalizes fragmented nets, preserves
visible local wiring and feedback, builds compact multi-endpoint route trees,
orients passives by electrical role, and selects the smallest fitting standard
sheet. Five checked-in [educational examples](../examples/educational/README.md)
show conventional voltage-source, current-source, differential-amplifier,
low-pass-filter, and voltage-divider layouts.

Large schematic IR inputs can use deterministic hierarchy partitioning. Exact
human-editor-quality layout for arbitrary imported schematics remains outside
the current guarantee.

The human-quality physical corpus adds functional rather than overflow-only
hierarchy. Four independently frozen behavior-only designs derive named child
sheets, preserve multi-unit ownership, distinguish hierarchical signal
interfaces from genuinely global power/reference nets, and retain conventional
stage flow in the written child schematics. All root and child files pass the
strict writer and installed-KiCad checks in two clean roots. See the
[completion audit](../specs/human-quality-hierarchical-multilayer/AUDIT.md).

### PCB Placement And Routing

The workflow supports block-aware placement, block-local copper, inter-block
route trees, pad/contact graph evidence, route completion, net classes, and
bounded placement-routing retry. Promoted fixtures prove required-net
connectivity rather than parseability alone.

The frozen human-quality physical corpus additionally proves exact
`F.Cu`/`In1.Cu`/`In2.Cu`/`B.Cu` stackups, filled ground and bounded power
planes, semantic placement regions, catalog-backed thermal-edge placement,
layer transitions with paired return vias, and deterministic constrained
routing for mixed-signal, amplifier, protected-control, and power cases.

This remains a bounded four-case envelope, not a general-purpose autorouter for
arbitrary dense boards. See
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

Open-topology synthesis proves the expanded frozen benchmark at exactly 8/8,
the first architecture milestone proves ranked generation for Class A, Class
AB, and notch requirements, and the later protected-current milestone closes
the remaining architecture-generalization current-output gap with independent
source and sink variants. The cross-stage repair corpus recovers all nine
workflow stages through shared evidence.
Explicit V6 control-polarity, safe-state, directed-transition, timing, and
state-sequencing semantics are now implemented for the V3 behavioral-
composition envelope. Identity-neutral active-high/active-low cases are
frozen, prerequisite targets resolve fail-closed, and opposite-direction
transient glitches no longer count as responses. The current-sense-protection
and mixed-control/power cases now fail precisely because they demand a
deenergized startup while their sole fault control explicitly starts in its
connected state; they require a separate startup enable or sequencing
dependency. Broader clock/fanout,
programming-load, converter, isolation, and high-energy protection models,
catalog-independent part qualification, and physical generalization beyond the
new bounded four-layer corpus remain important. Those gaps can produce deterministic expansion proposals,
but they still require real engineering sources, representative
simulation/workflow/KiCad evidence, and review before entering the supported
registry. Unknown behavior must continue to produce a stable capability gap
instead of guessed implementation detail.

The first nonlinear/switching milestone is now complete for five positive and
three adversarial frozen cases. The next synthesis milestone should measure
generalization on a new, independently authored held-out corpus that combines
multiple nonlinear stages, switching control, sensing, and protection without
reusing fixture identities. Expansion should remain evidence-driven: add only
the reusable operators, reviewed device/model provenance, diagnosis paths, and
physical constraints exposed by those failures. Denser-board routing,
catalog-independent qualification, RF/high-speed behavior, mains, and
high-energy conversion remain later boundaries.

See the [Roadmap](../specs/ROADMAP.md) for prioritized work and the
[Development Reference](development.md) for repository-level limitations and
test commands.
