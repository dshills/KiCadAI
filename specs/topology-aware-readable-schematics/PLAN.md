# Topology-Aware Readable Schematic Implementation Plan

Status: implemented and locally verified. See `EVIDENCE.md` for installed-KiCad
and rendered results.

## Phase 0: Baseline And Contract

- Record the label-heavy current-source failure and trace it to transaction,
  layout, and writer operations.
- Freeze the six-family fixture matrix and quantitative evidence schema.
- Add focused failing tests for local labeled wires, merged net fragments,
  route-tree junctions, topology-derived ranks, and compact paper selection.

## Phase 1: Normalize Electrical Topology

- Merge same-name net fragments into one deterministic hyperedge without
  changing endpoint identity.
- Infer forward, feedback, bias/reference, protection, and supply relationships
  from component roles, pin directions, SCCs, and graph reachability.
- Treat inferred groups as proximity regions, not fixed graph ranks.

## Phase 2: Conventional Placement

- Rank forward paths left to right.
- Put positive power above, returns below, and feedback below/around its stage.
- Place boundary connectors at occupied-drawing edges.
- Keep power flags at rail entry and remove arbitrary synthesized-component
  `near` chains.
- Apply role-derived orientation only when the caller did not fix orientation.

## Phase 3: Visible Local Routing

- Split label annotation from endpoint-only label routing.
- Wire local two-point nets continuously.
- Route multi-point nets as deterministic trees and derive junctions from actual
  branch degree.
- Keep label-only fallback bounded, deterministic, and diagnostic.

## Phase 4: Page Compaction And Metrics

- Try standard paper sizes from smallest to largest for inferred, movable
  layouts while preserving explicitly fixed sheets and coordinates.
- Add occupied-area, whitespace, dispersion, continuous-wire, route-tree,
  connector-edge, and feedback-visibility metrics.
- Make strict failures actionable and deterministic.

## Phase 5: KiCad-Backed Verification

- Generate and render the six-family fixture matrix with installed KiCad.
- Inspect the rendered artifacts and retain a manifest with hashes and metrics.
- Run local ERC, strict DRC where a PCB exists, connectivity, route-completion,
  writer-correctness, replay, and round-trip gates.
- Run the complete local Go suite.

## Phase 6: Review And Delivery

- Review the staged diff with Prism and resolve actionable findings.
- Commit and push the implementation.
- Do not wait for GitHub Actions; the repository owner will report any later CI
  failure.
