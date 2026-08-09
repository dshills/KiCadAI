# Closed-Loop Open-Set Capability Expansion V2 Validation Audit

Status: discovery uplift failed; V2 closed fail-closed; held-out evidence retired without decryption

Date: 2026-08-09

## Frozen experiment

V2 used the committed, independently authored 24-case behavior-only corpus and
the discovery-only rank-one selection. The attempted implementation did not
change the frozen corpus, policy, catalog, model registry, work ceilings,
baseline, ranking, or acceptance gates.

- production starting commit:
  `8bdc31e668152b7324066bd75182d86d7320d3f8`
- V2 corpus freeze commit:
  `cea6040301230d16372aa1c390acb36903a0e711`
- discovery baseline report SHA-256:
  `33f660a711236f657e8fda52994326dcb5b54b0a657a7dd71919a9df77f96f23`
- selected discovery cluster:
  `exhausted:topology_search:topology:complete_topology:OPEN_TOPOLOGY_SEARCH_EXHAUSTED`
- affected discovery cases: `v2_case_002`, `v2_case_005`, `v2_case_006`,
  `v2_case_011`, and `v2_case_012`

The V2 held-out baseline remained encrypted throughout implementation and
diagnosis. No held-out requirement bytes, per-case results, gaps, cluster
membership, or diagnostics were revealed.

## Discovery result

Three generic changes were evaluated together as inseparable prerequisites for
the selected complete-topology capability:

1. deterministic materialization of a required electrical reference when a
   reference domain had no explicit external port;
2. use of a declared current range, rather than voltage compliance, for a DC
   current-input sweep; and
3. recognition that an absolute decision threshold under a swept single supply
   needs an independent reference.

These changes moved cases beyond graph completion and exposed their downstream
simulation and composition limits, but produced no strict pass:

| Scope | Baseline pass count | Candidate pass count | Required result | Verdict |
| --- | ---: | ---: | --- | --- |
| All discovery | 0 | 0 | strict increase | fail |
| Rank-one-affected discovery | 0 | 0 | strict increase | fail |

The zero-pass baseline is the reproduced measurement for the newly authored,
previously unsolved V2 discovery corpus; it is not a missing or failed baseline
run.

Because discovery uplift is a mandatory prerequisite to blind validation, the
candidate was rejected and fully removed. No outcome-changing production code
from the attempt was released.

## What the attempt exposed

The selected failure label grouped requirements that did not share one
sufficient implementation gap:

- `v2_case_002` needs generation of a bipolar output domain from a positive
  supply in addition to current-input conversion.
- `v2_case_005` requires an output-high floor above the minimum declared supply,
  so a direct single-rail comparator implementation cannot satisfy all corners;
  an additional energy domain or a revised contract is required.
- `v2_case_006` combines direction-specific current thresholds, rail-loss
  behavior, and output-drive obligations not covered by the current decision
  synthesis path.
- `v2_case_011` combines conditional analog gain, off-state isolation, and a
  signed output range from a positive supply.
- `v2_case_012` requires coordinated power regulation, sensing, notice output,
  transient load behavior, and thermal acceptance. A graph can be structurally
  complete while still lacking a valid source-to-load power path.

This is not evidence that the three generic corrections are intrinsically
wrong. It is evidence that they are insufficient for the frozen V2 success
contract, and therefore cannot be promoted as the selected capability.

## Trust and release decision

The held-out final run was not started. Its sealed baseline commitments remain
unchanged, but the V2 held-out set is retired and must not be decrypted, tuned
against, or reused as blind evidence. Retiring it prevents a later partial fix
from silently converting this stopped experiment into a claimed V2 pass.

V2 is not eligible to write final reports, comparison artifacts, a promotion
matrix, or completion evidence. Installed-KiCad promotion was not applicable
because no new discovery case passed. The committed V1 evidence and protected
USB-C regression evidence remain unchanged.

## Required mitigation before V3

The next experiment must begin with a generic behavioral-contract feasibility
and realizability gate, before cluster selection or production work:

1. derive conservative voltage, current, power, and reference-domain envelopes
   for every operating corner;
2. distinguish direct-domain behavior from behavior that requires explicit
   voltage or energy-domain creation;
3. reject or classify contracts whose output envelopes cannot be supported by
   any declared energy path under the allowed physical model;
4. require source-to-load causal paths for power obligations, not only complete
   graph bindings;
5. separate single-obligation topology gaps from multi-output and
   multi-physics composition gaps when clustering;
6. report feasibility uncertainty explicitly instead of forcing it into
   `complete_topology`; and
7. validate the gate with public, hand-checkable boundary and adversarial
   contracts before authoring a fresh, independently isolated V3 corpus.

V3 must use a newly authored held-out set. Neither V1 nor V2 held-out material
may serve as blind validation evidence again.
