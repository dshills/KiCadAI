# Simulation-Guided Open-Topology Synthesis Completion Audit

Date: 2026-07-30

## Outcome

KiCadAI now has a separate primitive-component synthesis lane that accepts
strict behavior-only requirements, constructs canonical terminal-level graphs,
searches catalog-backed values, evaluates every declared operating case through
the trusted simulator, applies bounded generic graph repair, and promotes
passing results through the normal KiCad production workflow.

The frozen eight-case corpus advances from the untouched baseline of 0/8 to
6/8 complete installed-KiCad promotions. The two remaining cases terminate
deterministically with `OPEN_TOPOLOGY_REPAIR_EXHAUSTED`. No held-out identity,
filename, hash, expected value, topology label, coordinate, provider, or block
family appears in production search logic.

This proves discovery inside a bounded primitive/model/search envelope. It does
not prove unrestricted circuit synthesis, arbitrary parts or SPICE models,
RF/high-speed design, mains or high-energy safety, dense-board routing, or
fabrication release.

## Requirement Audit

| Requirement | Production evidence | Verification evidence | Result |
| --- | --- | --- | --- |
| Behavior-only strict input | `decode.go`, `normalize.go`, `validate.go` reject implementation-bearing fields and hash canonical requirements | corpus freeze and model/decode tests | Pass |
| Evidence-backed primitive inventory | `inventory.go` derives accepted terminal, package, rating, value, and model contracts from the catalog and model registry | inventory and missing-evidence tests | Pass |
| Resistors, capacitors, inductors, diodes, op-amps, comparators, BJTs, MOSFETs, and regulators | catalog/model additions plus `mna_registry.go` and primitive descriptors expose the bounded families | default-inventory, simulator applicability, and descriptor tests; promotion matrix uses three active-family groups | Pass |
| Deterministic topology search | `graph*.go`, `search.go`, and `search_model.go` use canonical graphs, generic operations, best-first ordering, deduplication, and count budgets | graph permutation/adversarial tests; two complete corpus runs compare byte-for-byte | Pass |
| Deterministic value search | `value_domains.go` derives bounded preferred domains and trial order from catalog evidence and behavioral scales | value-domain, tolerance, rating, ordering, and exhaustion tests | Pass |
| Trusted nominal/corner/event/fault simulation | `simulation.go` lowers generated connectivity into the reviewed simulator; simulator additions cover aggregate, thermal, startup, noise, stability, and nonlinear measurements | all three simulation-grounded shards pass; held-out consumption records 1,040–4,287 corner evaluations per case | Pass |
| Stable diagnosis | `simulation.go` emits topology-neutral diagnosis records with assertion, case, metric, observed/required bounds, cone, and hash | failure, replay, unsupported-model, and exhaustion tests | Pass |
| Genuine graph-changing repair | `repair.go` adds/removes/redirects passive edges or substitutes compatible primitives and rejects value enumeration that undoes the graph delta | active filter and sensor conditioner each pass after six recorded topology repairs; before/after topology hashes differ | Pass |
| Complete budget accounting | `synthesis.go` rolls nested repair simulations, corners, values, and topology repairs into the top-level report | `assertSynthesisConsumptionMatchesEvidence` reconciles retained child evidence exactly | Pass |
| Frozen identity/topology-neutral corpus | `testdata/held_out_corpus/manifest.json` pins eight behavior-only requirements at base commit `8965a304` | `manifest.sha256` is `90a530d0…95835`; freeze tests reject prohibited fields | Pass |
| At least six complete passes | `Synthesize` and `PromoteSynthesisRun` are the production APIs | 6/8 pass simulation and physical promotion; 2/8 exhaust stably | Pass |
| Multiple topologies and post-failure selection | search retains materially distinct canonical graphs and schedules candidates round-robin | at least two cases retain multiple topologies and at least two selections follow failed simulation | Pass |
| Physical integration | `physical_lowering.go` maps the selected primitive graph into the standard design request; `physical_promotion.go` runs the standard workflow twice | all six passing cases clear schematic electrical, placement, routing, connectivity, writer, ERC, strict DRC, and round trip | Pass |
| Deterministic physical replay | `PromoteSynthesisRun` hashes raw `.kicad_pro`, `.kicad_sch`, and `.kicad_pcb` files outside evidence directories | each passing row has one identical clean-root project hash | Pass |
| Public CLI | `kicadai open-topology create` loads strict requirements, catalog/model/library evidence, synthesizes, promotes twice, and writes full evidence artifacts | direct installed-KiCad smoke succeeds from a nonexistent root with compact JSON and project hash `57d4cf61…0499e` | Pass |
| Stable fail-closed diagnostics | report/model contracts define the required open-topology failure codes and retain strongest candidates plus consumption | both unsupported held-out cases replay with `OPEN_TOPOLOGY_REPAIR_EXHAUSTED` | Pass |
| No held-out leakage | production search depends on semantic ports, terminal contracts, registered metrics, diagnoses, and generic compatibility only | exact ID/hash/filename/path scan over non-test `cmd` and `internal` Go returns no match | Pass |
| Existing USB-C, MCU, amplifier, bus, and onboarding evidence | provider-backed paths are unchanged except catalog hashes/goldens | installed-KiCad design tier: 13/13; MCU: 3/3; bus: 4/4; onboarding: 7/7; bounded full suite passes | Pass |

## Benchmark

The machine-readable [promotion matrix](PROMOTION_MATRIX.json) records the
frozen requirement hashes, explicit policy, work consumption, synthesis,
topology, physical-lowering, and raw project hashes for all eight cases.

Passing cases:

- adjustable current output;
- audio mute;
- ground-referenced load control;
- hysteretic detector;
- powered low-pass response; and
- sensor conditioner.

Stable bounded exhaustions:

- adjustable voltage regulation; and
- voltage-window monitoring.

The active filter and sensor conditioner are the strongest topology-repair
evidence. Their initially complete feedback-free graphs fail trusted
simulation, a generic passive-edge repair changes the topology hash, and the
fully re-evaluated repaired graph passes.

## Fresh Local Commands

Focused and bounded suites:

```sh
GOCACHE=/tmp/kicadai-go-cache go test -short ./... -count=1 -timeout=20m
KICADAI_OPEN_TOPOLOGY_PROMOTION=1 GOCACHE=/tmp/kicadai-go-cache \
  go test ./internal/opentopologysynthesis \
  -run '^TestFrozenHeldOutCorpusSimulationPromotion$' -count=1 -v
```

Installed-KiCad open-topology promotion:

```sh
KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 \
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
GOCACHE=/tmp/kicadai-go-cache \
go test ./internal/opentopologysynthesis \
  -run '^TestFrozenHeldOutCorpusOptionalKiCadPromotion$' -count=1 -v
```

Preservation lanes:

```sh
go test ./internal/designworkflow \
  -run '^TestDesignExamplesOptionalKiCadBackedTier$' -count=1 -v
go test ./internal/compositionlowering \
  -run '^(TestNeutralMCUSynthesisCorpusOptionalKiCadPromotion|TestProtocolAwareBusCorpusOptionalKiCadPromotion)$' \
  -count=1 -v
go test ./internal/componentonboarding \
  -run '^TestHeldOutCorpusOptionalKiCadPromotion$' -count=1 -v
```

The installed-KiCad commands require the three `KICADAI_*` paths shown above.
The simulation-grounded closed-loop corpus was run in the same `0/3`, `1/3`,
and `2/3` `KICADAI_PROMOTION_SHARD` split used by CI; all ten cases passed.

A monolithic non-short `go test ./...` is not the authoritative local command
for this repository's promotion corpora: it serializes several intentionally
long matrices into one package and can exceed Go's package timeout. The
bounded suite plus named/sharded long lanes preserves complete case coverage
and matches the repository's required workflow structure.

## Delivery

The complete staged diff must receive Prism review before commit. Per standing
project policy, local evidence is authoritative for this milestone; GitHub
Actions are not used as the primary test loop.
