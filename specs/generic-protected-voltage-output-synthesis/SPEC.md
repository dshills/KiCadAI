# Generic Protected Voltage-Output Synthesis

## Goal

Make behavior-only linear voltage-output requirements synthesize through
reusable relationship, architecture, value, model, repair, lowering, and
physical-promotion capabilities. The synthesis engine must derive a safe
linear implementation from external behavior rather than selecting a named
regulator block or embedding a fixture topology.

The first envelope covers:

1. a low-noise adjustable positive output;
2. a protected high-power positive output; and
3. a bidirectional midrail output that must source and sink load current.

Switching conversion is explicitly outside this milestone.

## Frozen Inputs

Implementation is driven by the checksum-locked corpus under
`internal/opentopologysynthesis/testdata/protected_voltage_output_corpus/`.
All three requirements are independently authored from external interfaces,
operating conditions, events, measurable limits, and acceptance gates. They
contain no component, primitive, topology, internal-net, model, coordinate,
layer, route, solver, repair, or fixture-specific implementation instruction.

The manifest and untouched-engine baseline must be committed before production
implementation begins. Production work must not modify a frozen requirement,
manifest, checksum, baseline report, or freeze constant.

## Required Behavior

### Voltage transfer and reference scaling

- Derive fixed, command-dependent, and midpoint voltage relationships from
  declared ports, domains, operating cases, and output assertions.
- Derive reference and feedback ratios with equation provenance and reviewed
  tolerance evidence.
- For adjustable outputs, satisfy every declared command/output pair rather
  than treating the command as an unused bias input.
- For a midpoint output, regulate around the declared fraction of the supply
  while both sourcing and sinking the full load envelope.

### Line, load, and dropout closure

- Enforce output-voltage accuracy across every declared supply, load,
  tolerance, model, and temperature corner.
- Measure line and load regulation from trusted sweeps.
- Treat minimum input-output differential as a dropout/headroom contract for
  the complete control and pass path.
- Reject candidates that regulate nominally but saturate at any required
  corner.

### Stability, noise, and startup

- Derive compensation and output-support values from the selected control
  relationship and declared capacitive-load envelope.
- Require trusted phase-margin and settling evidence at every critical corner.
- Measure integrated output noise over the declared analysis band; missing or
  untrusted noise evidence is blocking when noise is critical.
- Prove startup trajectory, settling, and overshoot without relying on the
  final operating point alone.

### Current limiting and short-circuit response

- Derive a current-limit relationship from the declared normal, overload, and
  short-circuit envelopes.
- Keep startup permission distinct from overload and short-circuit protection.
- Demonstrate bounded output/pass-path current after every declared fault
  transition and deterministic recovery when recovery is required.
- A candidate without trusted fault-event coverage must fail closed and must
  not reach physical promotion.

### Thermal and safe operating area

- Select active power devices only from reviewed model, rating, thermal-path,
  and safe-operating-area evidence.
- Account for worst-case input voltage, output voltage, load/fault current,
  ambient temperature, tolerance, thermal resistance, and event duration.
- Permit parallel or otherwise distributed pass paths only when derived from
  the required dissipation/current envelope and supported by deterministic
  current-sharing evidence.
- Require every critical junction-temperature and SOA assertion to be
  measured. Missing, inapplicable, nonconvergent, or untrusted evidence is a
  blocking result.

### Deterministic repair

- Diagnose transfer ratio, reference headroom, dropout, current limit,
  compensation, bidirectional drive, model coverage, thermal, SOA, lowering,
  placement, and routing failures separately.
- Re-enter the earliest affected stage and apply the smallest generic repair.
- Preserve unrelated graph branches, values, placement, and copper.
- Bind proposals, causal analysis, outcomes, rollback, and selected repairs to
  deterministic replay evidence.

### Readable schematic and physical realization

- Lower conventional left-to-right power flow: input protection/support,
  reference and error control, pass path, protected output, then load
  interface.
- Place feedback and reference paths below the active power path, supply
  support above it, and current-limit/fault sensing adjacent to the affected
  pass path.
- Show bidirectional midpoint sourcing and sinking relationships explicitly;
  labels must not hide the regulating loop.
- Require complete connectivity and routing, writer correctness, clean ERC,
  strict DRC, and zero normalized schematic and PCB round-trip differences.

## Genericity Constraints

Production code must not contain corpus IDs, project names, requirement-file
names, fixture-specific coordinates, catalog-ID exceptions, allowlists,
special schemas, or named voltage-regulator block families. Decisions may
depend only on normalized behavior, graph relationships, reviewed
catalog/model/rating evidence, physical constraints, and measured diagnoses.

## Preservation Requirements

- Protected current-output synthesis and its three physical promotions remain
  green.
- The frozen multi-branch benchmark remains exactly 8/8, and both neutral
  multi-branch physical promotions remain green.
- Existing architecture, dynamic electrothermal/control-loop, amplifier,
  protected LED, and protected I2C evidence remains green.
- Existing unsafe and insufficiently evidenced cases continue to fail closed.

## Acceptance

The goal is complete only when every valid frozen voltage-output case:

1. passes deterministic architecture and value search with explainable
   relationship and equation provenance;
2. passes all applicable voltage accuracy, line/load regulation, dropout,
   stability, noise, startup, current-limit, short-circuit, thermal, and SOA
   assertions;
3. produces a conventionally readable schematic with explicit regulation and
   protection flow;
4. passes two isolated local installed-KiCad runs with clean ERC, strict DRC,
   complete connectivity/routing, writer correctness, and zero round-trip
   differences;
5. produces byte-identical normalized evidence and project hashes on replay;
   and
6. leaves every preservation requirement green.

Unsafe, unsupported, switching, or insufficiently evidenced variants must
remain deterministic, actionable, physical-artifact-free failures.
