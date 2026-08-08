# Frozen Corpus Erratum: Illumination Compliance Envelope

## Status

The frozen requirement
`illumination_proportional_power_control.json` remains byte-for-byte unchanged
at SHA-256
`cb489b1b5897767d9bbf7d7aa2d7b8c85dd21732174ca54f3f6b32f9db377b96`.
Its manifest, manifest checksum, historical baseline, and original declaration
as an intended positive also remain unchanged.

The input is structurally valid but physically contradictory. It is therefore
not a supported design case and must fail closed before topology search.

## Reproducible Proof

The frozen behavior simultaneously declares, in one operating case:

- minimum transconductance: `0.29 A/V`;
- illumination input range: `0.1 V` to `2.0 V`;
- load resistance up to `40 ohm`; and
- positive load-supply/output ceiling: `15 V`.

The minimum required current excursion is:

`0.29 A/V * (2.0 V - 0.1 V) = 0.551 A`.

That excursion alone requires this load-voltage excursion at `40 ohm`:

`0.551 A * 40 ohm = 22.04 V`.

Because `22.04 V > 15 V`, no component, topology, tolerance, repair, or routing
choice can satisfy the frozen simultaneous envelope. The proof uses the
minimum accepted transfer and the maximum available voltage, so omitted device
headroom would only make the contradiction stronger.

## Required Handling

The production preflight applies this relationship generically to any
controlled-current transconductance requirement with a simultaneous input and
load-resistance envelope. It emits
`OPEN_TOPOLOGY_REQUIREMENT_INFEASIBLE`, produces no selected graph or physical
artifact, consumes no topology-search budget, and replays byte-identically.

The implementation may not:

- modify the frozen requirement or checksum;
- reduce the input or load range;
- relax the minimum transconductance;
- assume an undeclared higher supply;
- suppress a corner; or
- add a case identity, allowlist, or fixture-specific branch.

If a corrected requirement is desired later, it must be independently authored
and frozen as a new corpus version rather than replacing this historical input.
