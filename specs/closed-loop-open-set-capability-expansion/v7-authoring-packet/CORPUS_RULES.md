# Closed-Loop Open-Set V7 Author Corpus Rules

## Fixed allocation

The complete corpus contains exactly 36 new behavior-only requirements: 18
discovery and 18 held-out. Exactly three independent authors each write twelve:
one discovery and one held-out requirement in each of `analog`, `power`,
`digital`, `mcu`, `sensor`, and `mixed_signal`. Assigned identity, role, domain,
safety impact, source ID, and path are immutable. Manifest identities occur only
in assignment and authorship metadata, never in a requirement body.

## Isolation and behavior-only scope

Use only the files frozen by the per-author manifest and an empty quarantine.
Do not inspect repository code, prior corpus source, retired held-out plaintext,
another packet or bundle, synthesis results, baselines, frontiers, rankings,
plans, diagnostics, capabilities, or expected outcomes. Do not run synthesis,
simulation, feasibility, classification, or outcome tools.

Requirements specify externally observable behavior and physical constraints.
They must not prescribe part numbers, footprints, topologies, named circuit
families, coordinates, routes, layers, templates, block families, algorithms,
expected diagnostics, capabilities, or known KiCadAI limitations.

## Structural and electrical validity

Every file strictly follows `PUBLIC_REQUIREMENT_CONTRACT.md`, uses only its
canonical vocabulary, resolves every reference, includes all 14 acceptance
gates, and uses finite, ordered, bounded, physically meaningful values. Each
analysis and assertion must be testable from its conditions and observations.
Interfaces, external energy, loads, events, and assertions must be mutually
coherent. Disclose any uncertainty rather than choosing an implementation or
predicting an outcome.

## Diversity and non-duplication

Within each role, independently vary port quantity/direction/dependency, supply
count/polarity/sequence/range, analyses, events, sweeps, corners, loads,
environment, metrics, observation scopes, behavior shape, board limits, and
safety context. Include meaningful single-output, multi-output, controlled,
autonomous, and cross-domain behavior across the bundle.

Do not repeat a normalized electrical behavior signature within a role. A
discovery and held-out requirement must not be paired variants. The complete
corpus mechanically limits the primary analysis to twelve uses, the primary
metric to nine uses, and any identifier-normalized port/supply shape to six
uses. Each safety category must occur between six and twelve times globally.
Authors should therefore avoid homogeneous primary analyses, metrics, and
interface shapes.

## Return and correction boundary

Return exactly twelve assigned JSON files plus `AUTHORSHIP.json`, with source
hashes in assignment order. The validator may issue at most three
outcome-neutral correction packets naming rule IDs and assigned paths. A
correction may alter only those assigned files and corresponding provenance.
It may not expose other authors, historical plaintext, synthesis behavior,
expected outcomes, diagnostics tied to capabilities, or implementation advice.
