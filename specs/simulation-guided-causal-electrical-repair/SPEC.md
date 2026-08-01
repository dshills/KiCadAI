# Simulation-Guided Causal Electrical Repair Specification

## Objective

Turn trusted simulation failures into bounded causal experiments before any
electrical repair is selected. The repair engine must perturb component values,
device choices, feedback polarity, bias/reference connectivity, and
compensation edges independently; measure their effect on every failed and
previously passing operating corner; rank reproducible sensitivities; and apply
the smallest safe change before re-entering value sizing and simulation.

## Frozen baseline

The implementation baseline is commit `bc58cf93`. The protected programmable
current-output requirement has canonical hash
`d071ce2d8be3c27692c2b96339f68dbc4d13d57e4b489815e50379862f0e3185`.
Under the architecture-generalization policy it fails closed with result hash
`a73d7087606e4a19f83b63adeea27e7e7c32df936c50755222668e458080aa8d`.

Its best retained pre-repair evaluation has one measured failure:
`rated_current = 0.112418066511 A`, outside `0.095…0.105 A`. Other retained
value choices include zero transconductance or active-state nonconvergence.
The old repair loop consumes 64 value trials and 16 topology repairs but emits
no per-perturbation sensitivity or coordinated-value evidence.

The aggregate dynamic-electrothermal package currently exceeds its documented
30-minute local timeout in the sequenced dual-rail controller case. Focused
simulation-grounded preservation remains passing; completion requires the full
dynamic corpus to finish within its documented timeout.

## Causal evidence contract

Each electrical repair search emits a content-addressed
`kicadai.simulation-guided-causal-repair.v1` analysis containing:

1. the exact requirement, inventory, graph, evaluation, and policy hashes;
2. explicit trial and coordinated-change budgets and consumption;
3. normalized perturbations with instance, terminal, node, device, and value
   before/after state;
4. assertion- and corner-level baseline/trial violation, margin change,
   sensitivity, and criticality;
5. new failures, lost evidence, and safety/corner regressions;
6. deterministic ranking, authorization or rejection, selected repair, stage
   re-entry, confirmation evaluation, and whole-analysis hash.

Missing model evidence is not repairable. A trial that improves one assertion
while worsening a critical assertion, losing an evaluated corner, or reducing
another passing corner's required margin is rejected.

## Generic operators

- catalog-valid single- and coordinated multi-value adjustment;
- compatible rated-device substitution;
- active-input or control-terminal polarity correction;
- bias/reference terminal redirection among graph-derived admissible nodes;
- passive feedback or compensation addition/redirection;
- re-entry through value-domain sizing followed by complete trusted
  simulation.

Production selection may depend only on requirement semantics, graph terminal
contracts, catalog/model/rating evidence, measured simulation effects, and
deterministic budgets. Requirement names, fixture IDs, exact fixture values,
component references, topology names, coordinates, allowlists, and hidden
case-specific ranking keys are prohibited.

## Acceptance

- The protected programmable current output either passes trusted simulation,
  thermal, SOA, physical lowering, routing/connectivity, writer, installed
  KiCad ERC/strict DRC, and zero-difference round trip, or its unsupported
  result is strictly narrower and includes complete causal evidence.
- At least two independently frozen held-out electrical failures recover
  without production case identity.
- Tests prove coordinated values, feedback polarity, bias/reference,
  compensation, and rated substitution are generated and evidence-backed.
- Cross-assertion and cross-corner regressions are rejected.
- Two runs are byte-identical and every count budget is enforced.
- The full dynamic-electrothermal corpus completes locally inside its
  documented timeout.
- Existing architecture-generalization, Class A, Class AB, notch, physical
  repair, writer, and installed-KiCad promotion evidence remains passing.
- Prism reviews the staged diff; actionable findings are remediated before the
  final commit.
