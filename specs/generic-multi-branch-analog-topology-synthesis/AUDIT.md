# Generic Multi-Branch Analog Topology Synthesis Audit

Status: complete
Date: 2026-08-03
Frozen baseline: `8f6ac90426b308b69cfcccbda9146be9bf6cc5f0`

## Result

The primitive-only open-topology lane now passes all eight original frozen
behavior requirements. Both independently frozen neutral multi-branch
requirements pass twice with byte-identical synthesis evidence. The neutral
corpus selects a real graph-changing series feedback repair, and both selected
designs pass two clean installed-KiCad project generations.

## Acceptance Evidence

| Clause | Result | Evidence |
| --- | --- | --- |
| Original frozen benchmark | Passed exactly 8/8 | `TestFrozenHeldOutCorpusSimulationPromotion` |
| Previously failing behaviors | Passed | Adjustable current output and hysteretic detector pass without requirement changes; adjustable voltage and voltage window remain green |
| Graph-changing repair | Passed | `TestGenericGraphChangingRepairAddsMissingPassiveAndReplays` plus the neutral corpus-wide selected-repair assertion |
| Independent neutral corpus | Passed 2/2 twice | `TestMultiBranchAnalogNeutralCorpusPromotion` requires materially multi-branch graphs, empty diagnostics, matched consumption evidence, and byte-identical JSON replay |
| Analysis coverage | Passed | Required DC, sweep, startup, stability, transient, thermal, SOA, and fault assertions are evaluated by trusted model evidence before selection |
| Physical promotion | Passed 2/2 | `TestMultiBranchAnalogNeutralCorpusOptionalKiCadPromotion` executes two clean roots per design and requires routing, connectivity, route completion, writer correctness, ERC, strict DRC, zero normalized round-trip differences, and identical project hashes |
| Milestone regression suite | Passed locally | `internal/simmodel` and `internal/opentopologysynthesis` pass; exact 8/8, neutral replay, and optional installed-KiCad promotions pass explicitly |
| Repository-wide comparison | No new failures | 121 `internal/compositionlowering` top-level tests ran in five bounded shards. Two assertion failures reproduce identically at the frozen baseline; the contended Class-AB shard reached its timeout, then its single case passed in isolation in 782.6 seconds. Every other repository package passed locally |
| Production neutrality | Passed | Production scan found no original/neutral fixture IDs, generated primitive IDs, target-specific values, coordinates, block substitutions, or case-specific repair rules |
| External review | Passed | Successive staged Prism findings were remediated where actionable and affected package tests passed after every code change. The review confirmed immutable graph cloning and deterministic inventory selection; split repairs now carry their generated node identity without positional lookup. The op-amp active-set fallback remains cycle-triggered, participant-only, and explicitly capped at 64 solves. |

## Deterministic Physical Evidence

| Neutral behavior | Synthesis hash | Topology hash | Physical hash | Two-run project hash |
| --- | --- | --- | --- | --- |
| Outside-window supply guard | `130bdfa83cd815fd7bd5d99b40f6f9ded3bfa4e740f1635b10c97dcde571fd5c` | `b8809fcefc5855d318486cb8fc61a8fa26ed0d5c2af7fd4e856c8f2dd8a35de8` | `d2de4144c92df297e04022bca08aaed03d644ff6c1f96dbcfeb4cf49f84c39aa` | `f5616c39a76c5250b87fc9d899694e84b67b6963202cccbc56010a65b17bc598` |
| Precision low-voltage rail | `b2ac82226a0916283c1f76ff317544ec8cd2d331520e591b28a6c9225646b9b6` | `d5296dff2a49df602cb181404be621a62db26daa0d7a9ad4ce0b79af97675f07` | `7e5139a89ef331c93b25d1f6a23da31b743fe0b4aee0d1c12c55f9b6dda3a4cc` | `b97c47fbeb9841dc40d746800b7baff72b5aa38d650bbb0606044e5b617e2d4e` |

## Repository Baseline Exceptions

The sharded local run exposed two deterministic failures that are outside this
milestone and reproduce unchanged from a clean checkout of frozen baseline
`8f6ac904`:

- `TestProtocolAwareBusCorpusPassesOfflineWorkflow/segmented_smbus.json`
  exhausts the existing two-layer router on one of 17 nets near R6/U3.
- `TestHierarchicalMultiDomainCorpusPassesOfflineWorkflow/current_limited_switched_load_system.json`
  produces a constant overload waveform where the frozen assertion requires
  an edge-time measurement.

The milestone also repaired a third baseline failure generically: resolved
simulation plans retained stale parameter indexes after continuation cloned and
scaled intrinsic model parameters. Indexed source, threshold, and op-amp gain
scaling now stays coherent, and
`TestOpenWorldCapabilityPromotionCorpusPassesOfflineWorkflow/heldout_power_protected_isolated_12v.json`
passes locally. No fixture identity or value exception was introduced.

## Scope Boundary

This closes a bounded, independently evidenced multi-branch analog envelope;
it does not claim unrestricted arbitrary-circuit generation. New families must
enter through frozen behavior requirements, trusted model coverage, bounded
generic search/repair, and the same physical promotion gates.
