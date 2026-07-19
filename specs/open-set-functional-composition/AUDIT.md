# Open-Set Functional Composition Acceptance Audit

Audit date: 2026-07-19

## Result

The milestone satisfies the bounded envelope in `SPEC.md`. Five frozen,
behavior-only requirements select catalog-backed architectures, lower through
the normal circuit graph and design workflow, and pass deterministic offline
and real-KiCad promotion. This is evidence for the registered capabilities and
checked-in catalog only; it is not a claim of universal or unbounded circuit
generation.

## Frozen Corpus

The corpus is
`internal/circuitgraph/testdata/open_set_composition_corpus`. Its manifest hash
is `f3a9b341d64186b6063b9f308ff9bdcf66f2421bf12068eba7f9a1a3ac3580b1`.
`manifest.sha256` verifies the manifest, and the freeze test verifies each
fixture hash, membership, sorted identity, strict schema, acceptance gates, and
absence of implementation details.

| Requirement | Selected capability | Real KiCad result |
| --- | --- | --- |
| `adjustable_regulated_supply` | voltage regulation | pass |
| `fourth_order_active_lowpass` | frequency filter | pass |
| `hysteretic_threshold_detector` | threshold detection | pass |
| `protected_inductive_load_switch` | load switch | pass |
| `translated_sensor_controller` | logic-level translation | pass |

Production Go packages contain no corpus path or frozen fixture identity. The
only corpus references are in freeze, architecture-search, lowering, and
promotion tests.

## Requirement Evidence

| Specification obligation | Evidence |
| --- | --- |
| One strict behavior-only schema | `architecturesearch.DecodeStrict`, normalization/validation tests, and the frozen-corpus identity-neutrality test reject unknown fields, topology, parts, pins, nets, coordinates, and routes. |
| Typed ports and electrical compatibility | `architecturesearch.PortContract`, compatibility reports, and contract matrix tests cover kind, direction, voltage, current, logic levels, domains, protocol, traits, and evidence confidence. |
| Reusable generic providers | `architecturesearch.CatalogProvider` supplies capability-selected threshold, load-switch, regulator, filter, translator, MCU, and sensor expansions. Synthetic mutation and catalog-order tests exercise providers without corpus identity. |
| Bounded deterministic search | The fixed search policy enforces state, depth, component, obligation, provider-expansion, alternative, and tolerance-corner budgets. Search tests prove registration/request/expansion order neutrality and fail-closed budget exhaustion without partial selection. |
| Rejections, scoring, alternatives, rationale | Search results retain stable rejection codes and bounded samples, lexicographic scores, distinct fingerprints, ranked alternatives, and the first differentiating score field. Tests cover deterministic electrical rejection, a real filter alternative, and tied user-visible ambiguity. |
| Values and tolerance evidence | Preferred-value, divider, RC/pole, hysteresis, gate-drive, rating, and active-filter solvers publish formula inputs, candidates, nominal results, worst-case corners, bounds, and margins. Replay and tamper tests reject missing or altered evidence. |
| Complete lowering | `compositionlowering.Lower` converts selections and bindings to a normal function-level circuit graph, preserves synthesis evidence, rejects duplicate/lost bindings, and produces byte-identical replay. All five resolve to concrete catalog identities, schematic IR, and design requests. |
| Fail closed | Strict decode, invalid contracts, unsupported capabilities, missing evidence, failed calculations, exhausted budgets, and tied material choices return structured failures and emit no partial selected architecture. |
| Deterministic artifacts | Each held-out workflow runs twice and compares normalized `.kicad_sch` and `.kicad_pcb` bytes. Search, normalization, realization, and lowering also have byte-stability tests. |
| Writer, routing, connectivity, ERC, and DRC | The optional promotion configures strict unrouted validation, required DRC, required ERC/DRC checks, required KiCad round trip, and strict diffs. Every schematic, electrical, placement, routing, project-write, writer-correctness, validation, and KiCad-check stage must be `ok`. |

## Verification Evidence

The following gates passed from the committed-intent worktree on 2026-07-19:

- `go test ./...`: complete offline repository suite.
- `TestFrozenOpenSetCorpusOptionalKiCadPromotion`: all five frozen requirements;
  clean workflow stages, clean ERC, strict DRC, complete routing/connectivity,
  writer correctness, strict zero-diff round trip, and deterministic replay.
- `TestDesignExamplesOptionalKiCadBackedTier`: all 13 checked-in KiCad-backed design
  fixtures, including Class A, protected Class AB headphone and 10 W speaker,
  ESP32-WROOM-32E, sensors, connector/LED, and both protected USB-C fixtures.
- `shasum -a 256 -c manifest.sha256`: `manifest.json: OK`.
- `git diff --check` and repository Go formatting check: clean.

The GitHub Actions result and published commit are recorded by repository
history after push; they are intentionally not predeclared in this audit.

## Remaining Boundary

Architecture search is still bounded by registered capability providers,
catalog evidence, solver families, board budgets, and the existing placement
and routing envelope. The next expansion should broaden electrically distinct
provider families and use adversarial held-out requirements to measure coverage
without weakening fail-closed safety or KiCad promotion gates.
