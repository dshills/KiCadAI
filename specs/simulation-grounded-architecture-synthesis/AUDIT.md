# Simulation-Grounded Circuit Architecture Synthesis Completion Audit

Date: 2026-07-31

## Outcome

KiCadAI now converts bounded behavior-only requirements into multiple
materially different primitive circuit architectures, derives catalog-valid
values with equation provenance, evaluates required cases with trusted
simulation, and deterministically ranks every physically ready pass. The
selected Class A, complementary Class AB, and 60 Hz notch architectures each
complete two installed-KiCad runs with identical project and path-independent
evidence hashes.

This closes the milestone's bounded architecture-synthesis contract. It does
not claim unrestricted arbitrary circuit generation, arbitrary vendor models,
RF/high-speed design, mains or high-energy safety, mechanical thermal
qualification, dense-board autorouting, or fabrication release.

## Clause Audit

| Requirement | Evidence | Result |
| --- | --- | --- |
| Behavior-only frozen inputs | Three requirements and their manifest are SHA-256 pinned; freeze tests reject component identities, topology names, internal nets, values, coordinates, providers, and repair hints | Pass |
| Multiple materially different architectures | Generic active-stage, controlled-device, complementary-follower, bias/feedback/protection, and balanced-bridge relationships retain distinct canonical topology hashes; Class AB also retains distinct active-family signatures | Pass |
| Op-amp, BJT, MOSFET, passive, diode, and protection combinations | Candidate grammar uses reviewed primitive terminal contracts and catalog/model compatibility rather than named circuit templates | Pass |
| Equation-proven values | Gain, bias, standing/idle current, headroom, load drive, ballast/sense resistance, coupling, compensation, and frequency-selective scales retain derivation, inputs, units, and source requirement | Pass |
| Full required simulation and corners | The analysis registry and synthesis evaluation cover every declared operating case and critical assertion using the applicable operating-point, AC, transient, distortion, noise, stability, thermal/electrothermal, and SOA contracts | Pass |
| Fail-closed safety rejection | A catalog-valid low-sense-resistor Class A alternative fails standing-current, safe-temperature, or SOA evidence while the safer analytic realization passes and lowers | Pass |
| Ranked explainable winner | Synthesis retains the best pass per topology and ranks alternatives by worst normalized requirement margin, repairs, component/internal-node counts, and stable hashes; the report records selection reason and alternatives | Pass |
| Readable generic schematic | Role/stage/lane placement, passive-lane spreading, standard title-block reservation, SI engineering values, recovered symbol geometry, canonical outward endpoint labels, constraint-preserving overlap repair, and orthogonal routing are exercised without held-out coordinates | Pass |
| Standard physical gates | Every case passes schematic electrical validation, route completion, connectivity, writer correctness, installed-KiCad ERC, strict DRC, and zero normalized round-trip differences | Pass |
| Deterministic clean-root replay | Each case is generated twice from isolated roots; raw project and normalized evidence hashes are identical | Pass |
| Path-independent promotion receipts | Canonical promotion hashes retain input/project hashes, acceptance, stage status, artifact kinds, and diagnostic classes while excluding machine-local artifact paths | Pass |
| Held-out Class A, Class AB, and non-amplifier demonstrations | `continuous_conduction_audio_stage`, `efficient_audio_power_stage`, and `mains_notch_filter` all pass synthesis and installed-KiCad promotion | Pass |
| Preservation | The complete bounded suite and all three simulation-grounded composition shards pass locally | Pass |

## Promotion Evidence

The machine-readable [promotion matrix](PROMOTION_MATRIX.json) binds the frozen
manifest and requirement hashes to synthesis, physical, raw project, and
evidence hashes. KiCad 10.0.3 produced six clean-root runs: two per case, with
no replay or round-trip differences.

Installed-KiCad command:

```sh
KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 \
KICADAI_OPEN_TOPOLOGY_ARTIFACT_ROOT=/tmp/kicadai-architecture-promotion \
GOCACHE=/tmp/kicadai-go-cache \
go test ./internal/opentopologysynthesis \
  -run '^TestArchitectureCorpusOptionalKiCadPromotion$' -count=1 -v
```

Preservation commands:

```sh
make GO_TEST_FLAGS='-short -count=1' test

for shard in 0/3 1/3 2/3; do
  KICADAI_PROMOTION_SHARD="$shard" GOCACHE=/tmp/kicadai-go-cache \
    go test ./internal/compositionlowering \
      -run '^TestFrozenSimulationGroundedCorpusPassesOfflineWorkflow$' \
      -count=1 -timeout 55m
done
```

The authoritative bounded suite passed in full. The long preservation corpus
passed in the same three-shard structure used by the repository workflow. A
monolithic default `go test ./...` is not authoritative because it serializes
the intentional long-running promotion corpus into one package and can exceed
Go's package timeout.

## Scope Boundary And Next Measurement

The evidence supports AI-directed architecture generation inside the reviewed
primitive, catalog, model, simulation, and physical-design envelope. The next
goal should freeze a second independently authored, out-of-distribution corpus
before changing production grammar. New failures should drive only reusable,
identity-neutral extensions and should retain the same fail-closed safety and
two-run KiCad contract.

## Delivery

The completed diff still requires Prism review and commit when explicitly
requested. Per project policy, local evidence is the primary verification loop;
GitHub Actions are not used here.
