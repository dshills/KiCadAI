# Architecture Generalization Completion Audit

## Frozen Boundary

- Base commit before production synthesis changes:
  `346fcdd4ffb5c99aa3e3945c76110a0652722428`.
- Corpus manifest SHA-256:
  `fd23189af2ef0471d4e69f0b4693b3bb05a10081a7d7ed15964ae6f98df08f86`.
- Untouched baseline report SHA-256:
  `77aea2c19ad22976fd3722a14d5384365c91cf69fee878bfbdfca31cb91229cc`.
- The manifest, requirements, baseline report, and baseline checksum were not
  changed while implementing production support.

## Acceptance Trace

| Requirement | Evidence | Status |
| --- | --- | --- |
| Frozen behavior-only six-design and four-adversarial corpus | Manifest and checksum tests | pass |
| At least five of six designs pass deterministic electrical acceptance | Five passing rows in `PROMOTION_MATRIX.md`; two-run aggregate acceptance | pass |
| Passing designs evaluate multiple topology hashes with explainable values and ranking | Aggregate acceptance assertions | pass |
| Installed-KiCad ERC, strict DRC, connectivity, routes, writer, round trip, and replay | Five two-run physical promotions | pass |
| Unsafe and unsupported cases fail closed | Four adversarial evidence hashes plus protected current-output stable non-pass | pass |
| Existing Class A, Class AB, and notch promotions remain green | Installed-KiCad architecture promotion | pass |
| First held-out corpus remains green | Six promoted and two stable fail-closed cases | pass |
| Simulation-grounded composition corpus remains green | Ten-case local offline workflow | pass |
| No requirement identity, fixture coordinate, allowlist, schema, or named block shortcut in production | Production-source content audit and diff review | pass |

## Generic Changes

The implementation extends behavior-derived graph relationships and value
domains for feedback/reference, full-wave transfer, window comparison,
transimpedance, and band-selective behavior. It also adds trusted measurements,
conservative thermal inference, bounded event duration, topology-independent
supply ordering, schematic text containment, and generated-board copper-edge
clearance alignment. These decisions depend on declared electrical or physical
facts, not corpus identities.

## Remaining Boundary

Protected programmable current drive is intentionally not promoted. The
bounded generic search still cannot establish a fully safe, trusted solution
for its requested control, dissipation, protection, and SOA envelope. It exits
before physical lowering with stable evidence rather than emitting an unsafe
project. This is the one allowed non-pass within the five-of-six target and is
the clearest input to a future diagnosis-driven repair goal.

## Completion Commands

```sh
go test $(go list ./... | grep -v '/compositionlowering$') -count=1 -timeout 20m
go test ./internal/compositionlowering -run '^TestFrozenSimulationGroundedCorpusPassesOfflineWorkflow$' -count=1 -timeout 30m -v
KICADAI_VERIFY_ARCHITECTURE_GENERALIZATION=1 go test ./internal/opentopologysynthesis -run '^TestArchitectureGeneralizationAcceptance$' -count=1 -timeout 20m -v
KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 go test ./internal/opentopologysynthesis -run '^TestArchitectureGeneralizationCorpusOptionalKiCadPromotion$' -count=1 -timeout 70m -v
KICADAI_OPEN_TOPOLOGY_KICAD_PROMOTION=1 go test ./internal/opentopologysynthesis -run '^TestFrozenHeldOutCorpusOptionalKiCadPromotion$' -count=1 -timeout 70m -v
```

The corpus-specific promotion and preservation reports contain the remaining
authoritative local commands and deterministic hashes.
