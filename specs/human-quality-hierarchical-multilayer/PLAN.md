# Human-Quality Hierarchical Schematics And Dense Multi-Layer PCB Plan

## Phase 0: Freeze Contract And Baseline

- Freeze the four-category behavior-only corpus and its hashes before changing
  production hierarchy or PCB generation.
- Run each case twice through the public CLI at the base commit.
- Record electrical outcome, hierarchy mode/sheets/interfaces, board layers,
  planes/zones, placement regions, routing/return-path evidence, KiCad gates,
  artifact hashes, and replay identity.
- Cluster failures by reusable capability.

Exit: immutable corpus and baseline evidence identify the current generic gaps.

## Phase 1: Semantic Functional Partitioning

- Derive functional groups from graph connectivity, roles, power domains,
  feedback edges, protection boundaries, and coupling weights.
- Add deterministic low-coupling partitioning with multi-unit atomicity.
- Produce explicit hierarchical interfaces and global power/reference scope.
- Bind the partition plan and rationale into physical evidence.

Exit: every frozen design produces a complete deterministic hierarchy plan.

## Phase 2: Hierarchical Writer And Readability

- Carry the derived hierarchy through physical lowering, schematic IR,
  transactions, and project writing.
- Lay out each child independently with conventional stage flow and local
  feedback/protection context.
- Validate root/child syntax, flattened connectivity, strict readability, and
  zero-diff round trips.

Exit: all corpus schematics are human-readable multi-sheet KiCad projects.

## Phase 3: Four-Layer Stackup And Plane Planning

- Replace the two-layer physical-lowering constant with policy-derived stackup.
- Emit `F.Cu`, `In1.Cu`, `In2.Cu`, and `B.Cu` plus complete dielectric,
  thickness, material, mask, and fabrication evidence.
- Derive continuous ground and bounded power-plane zones from domain/net roles.
- Fail closed when stackup or zone-fill evidence is insufficient.

Exit: all corpus boards contain an auditable fabrication-ready four-layer plan.

## Phase 4: Functional And Thermal Placement

- Derive PCB regions for boundaries, sensing, control, power, protection, and
  connectors from graph semantics.
- Add generic thermal-edge/heatsink placement and keepout rules from catalog
  package and thermal-path evidence.
- Preserve unrelated placement during bounded corrections.

Exit: placement evidence proves grouping, clearance, and thermal obligations.

## Phase 5: Layer-Aware Routing And Return Paths

- Extend routing search and endpoint access across four copper layers.
- Select vias and layer transitions by congestion, net role, plane reference,
  clearance, and deterministic cost.
- Validate plane continuity and require nearby return transitions when changing
  reference context.
- Report per-net layer, via, return-net, and return-distance evidence.

Exit: all required endpoints route with controlled return paths and zero strict
DRC/connectivity failures.

## Phase 6: Promotion And Preservation

- Run every corpus case twice with installed KiCad and retain evidence hashes.
- Require hierarchy/readability, placement, routing, connectivity, zone fill,
  writer correctness, ERC, strict DRC, round-trip, and replay gates.
- Re-run all existing promoted corpora and protected USB-C fixtures.
- Update capability documentation only from passing evidence.

Exit: the frozen corpus and all preservation lanes pass locally.

## Phase 7: Review And Commit

- Run the full local suite.
- Stage only milestone files and review with authorized Prism.
- Resolve actionable findings, rerun affected gates, and commit.
- Do not run GitHub Actions unless explicitly requested.
