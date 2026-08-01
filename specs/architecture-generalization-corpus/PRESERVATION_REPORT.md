# Architecture Generalization Preservation Report

## Prior Architecture Promotions

The existing Class A, Class AB, and notch architecture promotions remain clean
under the installed KiCad toolchain. The authoritative test recalculates and
compares their complete synthesis, topology, physical, project, and promotion
evidence hashes on every run rather than treating copied log prefixes as a
separate golden interface.

## First Held-Out Open-Topology Corpus

The complete eight-case held-out corpus passes its authoritative local test.
Six requirements complete installed-KiCad promotion and two unsupported
requirements (`adjustable_current_output` and `hysteretic_detector`) reproduce
stable fail-closed outcomes. The promoted cases include adjustable voltage
regulation, audio mute, ground-referenced load control, powered low-pass,
sensor conditioning, and voltage-window monitoring.

The voltage-window case specifically confirms the generic board copper-edge
clearance correction with these deterministic hashes:

- synthesis: `61bf948a435feeafdf097f4dff9cf9698bbb1bed78bb1b438aceb7167d1b8221`
- topology: `af32286ef646dae9a54d7de6de35ba7f5bd3788f36aec0edb2ad49aebe4fd449`
- physical: `0d7abf643a4986d0b7978bfbf04c31e36c0d10826d59aa0d505ffda2a8c5b38b`
- project: `4a03a0015c64676f7942c17721e15974af1a2220edcefc7958af70e809c5855b`

## Simulation-Grounded Composition Corpus

The complete simulation-grounded offline workflow passes all ten frozen
requirements, including active-filter, Class A, Class AB, protection,
hysteretic load, sensor, mixed-function, regulated-interface, and split-supply
cases. The final run completed locally in 226.11 seconds.

The separate six-case dynamic-electrothermal corpus is not a completion gate
for this frozen goal. A diagnostic run remains non-green in its Class AB,
step-down, and dual-rail physical workflows; the focused Class AB case reaches
trusted electrothermal synthesis but cannot route two of thirty nets on its
existing dense board. This is recorded as a future generic physical-routing
boundary, not presented as passing preservation evidence.

## Reproduction

```sh
KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 go test ./internal/opentopologysynthesis -run '^TestFrozenArchitecturePromotionOptionalKiCad$' -count=1 -timeout 70m -v
KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 go test ./internal/opentopologysynthesis -run '^TestFrozenHeldOutCorpusOptionalKiCadPromotion$' -count=1 -timeout 70m -v
go test ./internal/compositionlowering -run '^TestFrozenSimulationGroundedCorpusPassesOfflineWorkflow$' -count=1 -timeout 30m -v
```

All commands are local by project policy; GitHub Actions is not required as a
completion gate.
