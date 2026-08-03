# Generic Protected Current-Output Synthesis

## Goal

Make behavior-only programmable current-source and current-sink requirements
synthesize through reusable architecture, value, model, repair, lowering, and
physical-promotion capabilities. A passing design must regulate across its
declared compliance envelope, start in its declared safe state, disconnect
under the declared fault transition, and retain proven thermal and safe
operating-area margins.

## Frozen Inputs

The implementation is driven by the checksum-locked corpus under
`internal/opentopologysynthesis/testdata/protected_current_output_corpus/`.
It contains:

1. the unchanged independently frozen protected programmable-current failure;
2. a new low-side current-sink requirement with independent enable and fault
   controls; and
3. a new high-side current-source requirement with independent permit and
   fault controls.

The corpus manifest is bound to commit `86c7a6e2` and SHA-256
`229cd7821a7ad5cf29ae0bad21f57f40f9e833a5d40cb2e6ce376caa33d8462f`.
The untouched engine result is recorded in `BASELINE_REPORT.json` and is also
checksum locked. Production implementation must not modify any requirement,
manifest, baseline result, checksum, or freeze constant.

## Required Behavior

### Current transfer and compliance

- Derive source-versus-sink orientation from the controlled-current port and
  external domain relationships.
- Generate multiple materially distinct primitive-only architectures when the
  inventory and requirement allow them.
- Derive the feedback/sense relationship from the declared transconductance.
- Enforce output-current accuracy across every declared command, supply, load,
  tolerance, and temperature corner.
- Treat the declared supply/load envelope as a compliance-voltage contract.
  A topology that lacks pass-device and controller headroom at any required
  corner must fail closed with a structured diagnosis.

### Startup, overload, and fault response

- Derive explicit startup and fault controls from behavioral ports, default
  states, operating conditions, and events.
- Keep startup permission distinct from fault shutdown. Fault shutdown is
  dominant when both controls are asserted.
- Prove the required startup-current and off-state-current limits with directed
  transient measurements.
- Evaluate overload and short-circuit corners for dissipation, thermal limits,
  and transient SOA before accepting the fault response.

### Thermal and SOA safety

- Select pass devices only from reviewed model and rating evidence.
- Account for worst-case pass-device voltage, current, ambient temperature,
  tolerance, thermal resistance, and transient duration.
- Require every critical thermal and SOA assertion to be measured. Missing,
  inapplicable, nonconvergent, or untrusted evidence is a blocking result.
- Do not infer a passing safety margin from nominal power alone.

### Deterministic repair

- Diagnose architecture polarity, sense ratio, compliance/headroom, control
  composition, compensation, model coverage, thermal, and SOA failures
  separately.
- Re-enter the earliest affected stage and apply the smallest generic repair.
- Preserve unrelated graph branches, values, placement, and copper.
- Keep every proposal, causal analysis, outcome, rollback, and selected repair
  hash-bound and byte-identical under replay.

### Readable schematic and physical realization

- Lower the regulated power path in conventional left-to-right signal and
  power flow: command/control, error controller, pass device, current sense,
  protected output, then load interface.
- Place supply support above the active path, reference/return below it, and
  startup/fault protection adjacent to the controlled pass path.
- Prefer explicit local wires for the control loop and current path; use labels
  for genuine off-page or shared-domain connections rather than hiding the
  circuit relationship.
- Require complete connectivity and routes, writer correctness, clean ERC,
  strict DRC, and zero normalized round-trip differences.

## Genericity Constraints

Production code must not contain corpus IDs, project names, requirement-file
names, fixture-specific coordinates, part IDs, allowlists, special schemas,
or named current-driver block families. Decisions may depend only on normalized
behavior, graph relationships, catalog/model evidence, physical constraints,
and measured diagnostics.

## Preservation Requirements

- The frozen generic multi-branch benchmark remains exactly 8/8.
- Both independently frozen neutral multi-branch cases retain two-run physical
  promotion.
- Existing Class A, Class AB, protected LED, and protected I2C evidence remains
  green.
- Existing unsafe/adversarial cases continue to fail closed.

## Acceptance

The goal is complete only when every valid protected-current case:

1. passes deterministic architecture and value search with explainable
   equation provenance;
2. passes all trusted current-accuracy, compliance, startup, fault, thermal,
   and SOA assertions;
3. produces a conventionally readable schematic;
4. passes two isolated local installed-KiCad runs with clean ERC, strict DRC,
   complete connectivity and routing, writer correctness, and zero round-trip
   differences;
5. produces byte-identical normalized evidence and project hashes on replay;
   and
6. leaves every preservation requirement green.

Unsafe or insufficiently evidenced variants must remain deterministic,
actionable, physical-artifact-free failures.
