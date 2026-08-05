# Evidence Interpretation

## Claim

For the exact checked-in behavioral requirement, catalog, model registry,
primitive inventory, and policy recorded in the receipt, KiCadAI found a
simulation-passing architecture and produced the same KiCad project twice. The
generated candidate passed the required routing, connectivity, writer, KiCad
ERC, strict KiCad DRC, and normalized round-trip gates.

The authoritative compact record is [evidence/receipt.json](evidence/receipt.json).
Its hashes bind the input, search policy, selected topology, physical lowering,
full synthesis report, promotion report, and normalized project output.

## Architecture Decision

The search produced two complete, physically ready candidates. Both passed
the trusted simulations. Deterministic ranking selected the 12-primitive,
8-internal-node candidate because its worst normalized requirement margin was
0.0438475, ahead of the 14-primitive candidate's 0.0399319. Hashes are the
final tie breakers, so filesystem ordering and map iteration cannot change the
choice.

The selected primitive set includes parallel D45H11G TO-220 pass transistors,
two 0.22 ohm power ballast resistors, LM358 and OPA992 control amplifiers, and
catalog-backed bias and feedback resistors. Exact identities and values are in
the receipt and generated transaction.

## Physical Gates

Both promotion runs reported:

- 15 of 15 components placed;
- 12 of 12 required nets routed, with no failed nets;
- zero unconnected route endpoints;
- 10 writer checks passed with no failures, skips, or warnings;
- seven board-validation and eight project-evaluation checks passed;
- KiCad 10.0.3 ERC passed with zero findings;
- strict KiCad 10.0.3 DRC passed with zero violations and zero unconnected
  items; and
- normalized writer round trip passed with zero differences.

The two normalized projects have the same SHA-256 project hash:
`3f195f2576d0abda73ba01752d444ae464e89ab94332d767dba5e0716324fcb5`.

## Refusal Evidence

[refusal-requirement.json](refusal-requirement.json) asks for 48 V to 5 V at
4 A and 85 degrees C while requiring at least 92% efficiency, no more than
95 degrees C junction temperature, and 1.5x SOA margin in a 45 mm by 40 mm,
16-component design. The public replay command fails closed and the demo script
asserts that no `.kicad_pro` is created.

The exact public result is preserved in the compact
[refusal receipt](evidence/refusal-receipt.json).

The public CLI's default bounded policy may summarize that result as search
exhaustion rather than claim a unique physical cause. The deeper frozen corpus
test verifies thermal/SOA or rated-envelope rejection evidence for the same
case; see the
[nonlinear switching audit](../../../specs/nonlinear-switching-architecture-synthesis/AUDIT.md).
This distinction is intentional: exhaustion proves only that KiCadAI did not
find a supported safe design within its reviewed bounds.

## What This Does Not Prove

The run does not prove production readiness, regulatory compliance, thermal
performance with a particular enclosure or heatsink, lifetime, EMC, analog
accuracy on manufactured hardware, or suitability for a user's application.
KiCad ERC and DRC are necessary checks, not substitutes for engineering review
and hardware validation.

The checked-in project is a transparent snapshot for inspection. Rerunning the
demo is the authoritative way to reproduce the evidence against the current
source tree and installed KiCad libraries.
