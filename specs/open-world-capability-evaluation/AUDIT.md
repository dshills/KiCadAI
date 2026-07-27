# Open-World Capability Evaluation Completion Audit

Date: 2026-07-27

## Outcome

The milestone is complete. The frozen held-out corpus improves from 1/12 ready
to 6/12 ready without changing corpus membership, source hashes, baseline
evidence, ranking policy, or impact registry. Five identity-neutral
requirements advance through the normal production path:

- analog clock generation with bounded fanout, load, edge, startup, and jitter;
- translated MCU debug with reset entry, target-off loading, and shared-pin
  arbitration;
- unidirectional push-pull sensor translation with partial-power-down behavior;
- asymmetric push-pull functional isolation with safe-state and
  clearance/creepage evidence; and
- protected isolated 12 V conversion with bounded startup, inrush, shutdown,
  thermal, isolation, and transient behavior.

Every promoted case passes architecture/component selection, applicable
simulation and safety assertions, lowering, placement/routing, connectivity,
route completion, writer correctness, clean installed-KiCad ERC, strict DRC,
zero normalized schematic/PCB round-trip differences, and deterministic
replay.

This is measured breadth improvement inside the reviewed catalog and model
envelope. It is not a claim of arbitrary-circuit or fabrication-release
support.

## Frozen Evaluation Result

| Held-out outcome | Baseline | Final |
| --- | ---: | ---: |
| Ready | 1 | 6 |
| Needs clarification | 1 | 1 |
| Unsupported | 8 | 5 |
| Ambiguous | 1 | 0 |
| Budget exhausted | 1 | 0 |

The final largest gap remains
`unsupported:architecture_search:bus_buffering_level_translation:CAPABILITY_UNSUPPORTED`.
It covers three cases in the digital, mixed-signal, and sensor domains. The
other stable gaps are user-owned isolation bounds, broader MCU debug electrical
loading, and broader clock-fanout loading. No remaining case is silently
promoted or converted to a generic error.

## Requirement Audit

| Requirement | Evidence | Result |
| --- | --- | --- |
| Frozen discovery and held-out membership | SHA-256-pinned corpora under `internal/capabilityevaluation/testdata/open_world_corpus/`; reproduction tests reject membership or source drift | Pass |
| Deterministic clustering and ranking | `internal/capabilityevaluation`; baseline/final reports and impact registry under this directory | Pass |
| Reusable production integration | `cmd/kicadai-capability-eval` supports evaluate, compare, explain, and affected-case modes | Pass |
| No benchmark-specific production path | Promotion requirements enter normal architecture search, closed-loop synthesis, lowering, design workflow, and promotion evidence | Pass |
| Generic capability expansion | Catalog-backed clock fanout, MCU debug assignment, push-pull translation/isolation, protected conversion, and trusted simulation primitives | Pass |
| Fail-closed model and safety evidence | Reviewed model provenance, bounded continuation/transient behavior, startup/UVLO, thermal, SOA, isolation, and partial-power-down assertions | Pass |
| Physical library fidelity | Installed-KiCad-exact pad templates for the promoted translator, isolated converters, and protected-power package | Pass |
| Final route correctness | Pad-aware stale-via pruning, physical drilled-pad instance preservation, deterministic drill-clearance relocation, and post-relocation via deduplication | Pass |
| Offline promotion | All five held-out promotion fixtures pass twice with byte-identical recorded outputs | Pass |
| Installed-KiCad promotion | KiCad 10.0.3 reports zero ERC and strict DRC findings for all five cases | Pass |
| Connectivity, writer, and round trip | Required-net connectivity, route completion, writer correctness, and zero normalized schematic/PCB differences are required by every matrix row | Pass |
| Clean-root reproducibility | Two isolated local source roots produce independently verified identical content-addressed bundles using `make open-world-capability-promotion-bundle` | Pass |
| Regression preservation | Complete local short suite plus focused architecture, component, circuitgraph, simulation, routing, writer, and open-world corpora | Pass |
| External review | Staged diff reviewed with Prism; high/medium findings resolved before commit | Pass |
| Local-only development loop | No GitHub Actions run or monitoring was started | Pass |

## Reproduction

Run the complete local physical promotion from an unmodified checkout:

```sh
make open-world-capability-promotion-bundle
```

Run the direct offline and installed-KiCad corpus tests:

```sh
go test ./internal/compositionlowering \
  -run '^TestOpenWorldCapabilityPromotionCorpusPassesOfflineWorkflow$' \
  -count=1

KICADAI_KICAD_CLI=/path/to/kicad-cli \
KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols \
KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints \
go test ./internal/compositionlowering \
  -run '^TestOpenWorldCapabilityPromotionCorpusOptionalKiCadPromotion$' \
  -count=1
```

The checked-in `FINAL_*_EVIDENCE.json` and `FINAL_*_REPORT.json` files reproduce
from the frozen inputs in `TestFinalOpenWorldReportsReproduceFromFrozenInputs`.

## Remaining Boundary

The next breadth milestone should attack the largest remaining cluster:
protocol-aware bus buffering and level translation. It must add reviewed
per-signal direction/default-state behavior, partial-power-down evidence,
segmented bus loading, branch isolation, pull-up and speed qualification, then
repeat the same frozen evaluation and physical promotion process. Dense/RF
layout, unregistered active topologies, broad component discovery, high-energy
systems, and fabrication release remain outside the current claim.
