# Pre-Implementation Diagnosis

## Measured boundary

All three frozen requirements are valid and deterministic. The untouched
engine produces graph candidates and ready value plans, then fails in trusted
simulation before any candidate reaches physical lowering:

| Case | Candidates | Ready value plans | Evaluations | First blocking evidence |
| --- | ---: | ---: | ---: | --- |
| Low-side current sink | 16 | 16 | 32 | command slope below minimum or invalid |
| Existing protected current output | 1 | 1 | 32 | command slope/current above or below bounds |
| High-side current source | 16 | 16 | 32 | command slope below minimum or invalid |

Every case has one bounded repair search and zero passing simulations or
physical attempts. The complete immutable counts and hashes are in
`BASELINE_REPORT.json`.

## Stage diagnosis

- **Requirement/schema:** passes. Both controlled-current directions, default
  states, operating corners, startup events, fault input steps, and critical
  safety assertions decode strictly.
- **Architecture search:** incomplete. The dedicated transconductance
  relationship seed constructs only a high-side PNP realization. It does not
  derive low-side sink orientation and does not compose independent startup and
  fault controls into the regulated path.
- **Value search:** reaches `ready`, but its transconductance-specific scales
  recognize only the high-side PNP graph shape. Candidate values therefore do
  not reliably realize the requested source/sink transfer.
- **Trusted simulation:** blocking. The first measured command-transfer
  assertion is outside bounds or invalid for every retained candidate. Because
  evaluation fails there, compliance, startup, fault, thermal, and SOA closure
  are not yet demonstrated for a passing graph.
- **Repair:** present but insufficient. Bounded repair records deterministic
  causal evidence yet cannot construct the missing orientation/control
  relationship or close the command ratio.
- **Lowering and physical realization:** not reached. There is no evidence yet
  of a KiCad, placement, routing, writer, ERC, DRC, or round-trip defect for the
  new corpus; those stages must be evaluated after electrical closure rather
  than guessed at from the baseline.

## First implementation target

The smallest generic correction is to represent regulated-current orientation
as a derived graph relationship, generate source and sink realizations from
that relationship, and make value scaling operate on the relationship rather
than a hard-coded PNP terminal pattern. Startup permission and fault-dominant
shutdown must then compose as independent controls on the same power path.
