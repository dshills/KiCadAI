# Route-Tree VCC Proof Closeout Spec

Date: 2026-07-02

## Objective

Close the remaining route-tree proof gap in the KiCad-backed I2C sensor
breakout fixture by making VCC branch routing and contact proof precise enough
to either:

- prove all required VCC endpoints through legal same-net graph connectivity; or
- report a narrow, actionable blocker that identifies the exact access pair,
  obstacle, clearance, placement, or net-class issue preventing proof.

The current selected retry for
`examples/design/kicad-backed/i2c_sensor_breakout_candidate.json` proves 11 of
12 required inter-block contacts with three complete route-tree contact-graph
groups and one partial group. The remaining expected-fail blocker is one VCC
contact/branch proof gap after access-driven branch routing.

## Non-Goals

- Do not relax clearance, keepout, board-edge, via, or net-class rules to force
  promotion.
- Do not claim KiCad ERC/DRC-clean readiness unless configured KiCad evidence
  supports it.
- Do not replace the routing engine with a production autorouter.
- Do not broaden component catalog, block library, or amplifier work in this
  project.
- Do not promote the I2C fixture unless all declared promotion gates pass.

## Current Evidence

The latest generated I2C run reports:

- `route_tree_contact_graph.required_endpoints == 12`;
- `route_tree_contact_graph.proven_endpoints == 11`;
- `route_tree_contact_graph.complete_groups == 3`;
- `route_tree_contact_graph.partial_groups == 1`;
- `route_tree_access.local_route_anchors == 16`;
- route-tree-managed nets: `GND`, `SCL`, `SDA`, `VCC`;
- remaining VCC issues include no-legal-two-layer-path failures involving
  synthetic route-tree access refs and one `ROUTE_CONTACT_MISS`.

Access-driven branch routing now exists:

- endpoint access candidates are ranked deterministically;
- synthetic access-pad route requests preserve net metadata;
- branch attempts try bounded candidate pairs;
- selected local-anchor routes bypass post-route pad snapping through
  in-memory `SnapExempt` route metadata;
- branch evidence records access-pair attempts and selected access roles.

The remaining gap is therefore no longer "does access evidence exist?" It is:

1. whether the selected VCC branch route candidate actually reaches the correct
   same-net graph component;
2. whether a better legal VCC access pair exists but is not ranked/tried;
3. whether route request context is over-constrained by fixed non-target nets,
   missing net-class policy, synthetic access geometry, or placement; or
4. whether proof diagnostics are still too coarse to explain the real blocker.

## Required Capabilities

### 1. VCC Branch Evidence Extraction

The workflow must expose enough VCC-specific branch evidence to debug the final
gap without reading raw route operations manually.

Required evidence:

- net name and branch index;
- branch start/end endpoint IDs;
- candidate pair count considered;
- candidate pair count attempted;
- selected source/target roles;
- selected source/target refs/pads when known;
- selected source/target coordinates and layers;
- route status;
- operation count;
- nearest blocking obstacle or validation diagnostic;
- contact proof status for each required VCC endpoint;
- contact graph component IDs for required targets and emitted route endpoints.

### 2. Access Pair Attempt Accounting

Current branch evidence records selected roles but does not fully explain which
candidate pairs were rejected or why.

The closeout must add bounded attempt evidence for failed VCC route-tree branch
pairs while keeping payloads compact.

Attempt evidence should include:

- pair rank;
- source/target candidate roles;
- source/target coordinates/layers;
- route status;
- issue count;
- primary issue code/message;
- primary blocking ref/net/source when available.

This evidence should be available for all route-tree nets, but tests should
focus on VCC.

### 3. VCC Contact Graph Diagnostics

Contact graph proof must distinguish:

- route did not generate copper;
- route copper exists but does not touch the required target;
- route touches a same-net local anchor but that anchor is not connected to the
  required target component;
- route endpoint was snap-exempt and intentionally not moved to a pad center;
- wrong-layer or wrong-net contact;
- graph split after routing.

The I2C fixture should report the remaining VCC gap as one of these explicit
categories.

### 4. Candidate Ranking Review

The VCC failing branch must be analyzed against the available access candidate
set.

The implementation should verify whether:

- VCC local-route anchors are available for both sides of the failing branch;
- access candidates on the same layer as existing local VCC copper are ranked
  before pad centers where useful;
- access pair generation includes connector-side and capacitor/sensor-side VCC
  anchors;
- the bounded candidate limit does not exclude the first legal VCC pair;
- synthetic pad sizes and layers are appropriate for the active route rules.

If the limit excludes a legal pair, adjust ranking or increase only the
route-tree branch candidate cap with evidence.

### 5. Net-Class And Fixed-Net Noise Reduction

The current branch request keeps non-target nets as fixed metadata so the router
retains context. This produces repeated informational "fixed net was preserved
and skipped" issues in route-tree branch diagnostics.

The closeout should preserve useful context without polluting branch blocker
evidence:

- fixed non-target skip messages should not inflate route-tree blocker counts;
- target-net net-class warnings should remain visible;
- power/high-current VCC/GND warnings should produce a clear repair suggestion:
  assign explicit net classes for generated power rails;
- issue summaries should separate informational preserved-net notices from
  actual route blockers.

### 6. Fixture Promotion Decision

After implementation, rerun the I2C fixture.

Allowed outcomes:

- If all route-tree groups become complete and KiCad/writer/validation gates
  support promotion, promote readiness according to fixture policy.
- If internal graph proof reaches 12/12 but KiCad DRC is still missing or
  failing, keep the fixture expected-fail with a KiCad-evidence blocker.
- If VCC remains unproven, keep the fixture expected-fail but update docs and
  promotion evidence with the narrower blocker category and candidate evidence.

## Data Model

Add or extend route-tree branch evidence with compact attempt details. Names may
be adjusted to match existing conventions.

```go
type RouteTreeBranchAccessAttemptEvidence struct {
    PairRank      int    `json:"pair_rank"`
    SourceRole    string `json:"source_role,omitempty"`
    TargetRole    string `json:"target_role,omitempty"`
    SourceLayer   string `json:"source_layer,omitempty"`
    TargetLayer   string `json:"target_layer,omitempty"`
    SourceXMM     float64 `json:"source_x_mm,omitempty"`
    SourceYMM     float64 `json:"source_y_mm,omitempty"`
    TargetXMM     float64 `json:"target_x_mm,omitempty"`
    TargetYMM     float64 `json:"target_y_mm,omitempty"`
    Status        string `json:"status"`
    PrimaryCode   string `json:"primary_code,omitempty"`
    PrimaryMessage string `json:"primary_message,omitempty"`
    PrimaryRef    string `json:"primary_ref,omitempty"`
    PrimaryNet    string `json:"primary_net,omitempty"`
}
```

Existing branch evidence should gain:

- `access_pair_attempts`;
- selected source/target coordinates;
- selected source/target layers;
- selected source/target refs/pads when available;
- `snap_exempt_route` flag where relevant;
- contact graph/proof category if available.

## Acceptance Criteria

- Focused tests lock current VCC failure evidence before changing behavior.
- VCC route-tree branch attempt evidence identifies each attempted pair and the
  primary failure category.
- Informational fixed non-target net notices no longer inflate blocker counts.
- Contact graph diagnostics identify the remaining VCC proof gap precisely.
- The I2C fixture is rerun and docs/metadata reflect the actual outcome.
- `go test ./internal/designworkflow -run 'VCC|RouteTree|I2C|Access' -count=1`
  passes.
- `go test ./...` passes.

## Open Questions

- Does the remaining VCC gap require better access ranking, better placement
  retry, or explicit generated power net classes?
- Should generated VCC/GND rails receive default net classes earlier in the
  design workflow, or should this remain a separate power-net policy project?
- Is the synthetic access pad size too large near dense decoupling geometry, or
  too small for reliable contact proof?
- Should route-tree branch retry consider local fanout stubs before full
  branch routing for power nets?
