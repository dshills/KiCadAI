# Closed-Loop Open-Set Capability Expansion Specification

Status: version-1 contract frozen; corpus and baseline not yet frozen

## 1. Purpose

KiCadAI can already rank front-end capability observations, quarantine proposed
capability packages, synthesize several held-out mixed-domain architectures,
and promote supported designs through installed KiCad. Those pieces do not yet
form one production feedback loop. In particular, the current open-topology
synthesis path does not emit a corpus-wide, causally normalized expansion
report spanning topology, component, model, simulation, physical, routing, and
verification failures.

This milestone closes that loop. It must let frozen unfamiliar behavior-only
requirements reveal the highest-impact missing reusable capability, implement
only capabilities selected by that evidence, and prove coverage improvement on
untouched held-out requirements without weakening any trust gate.

This is systematic expansion of a bounded capability envelope. It is not a
claim of unrestricted arbitrary-circuit or fabrication-release support.

## 2. Relationship To Existing Systems

The milestone extends rather than replaces:

- `internal/capabilityevaluation`, whose existing report, clustering, ranking,
  comparison, and query contracts cover behavioral-intent and architecture
  search evidence;
- `internal/capabilityexpansion`, whose plans, candidates, source records,
  promotion bundles, approvals, and supported registries remain the only
  reviewed capability-promotion path;
- `internal/opentopologysynthesis`, whose bounded search, trusted simulations,
  physical lowering, and fail-closed reports are authoritative design evidence;
- `internal/designworkflow`, whose connectivity, routing, writer, ERC, DRC,
  round-trip, and replay evidence remains authoritative for physical promotion;
  and
- all previously frozen corpora and promotion lanes.

No parallel success path, fixture-aware evaluator, or benchmark-only provider
may be introduced.

## 3. Required Outcomes

The completed milestone must:

1. freeze a diverse discovery corpus and an independently held-out validation
   corpus before any selected capability is implemented;
2. baseline every case as exactly `pass`, `unsupported`, `unsafe`, or
   `exhausted` from normal production evidence;
3. convert authoritative failures into typed reusable gaps in `topology`,
   `component`, `model`, `simulation`, `physical`, `routing`, or
   `verification` scope;
4. preserve the causal relationship between terminal outcome, earliest root
   blocker, downstream symptoms, affected requirements, and raw evidence
   hashes;
5. cluster gaps without inspecting case identity, project name, prompt text,
   expected implementation, or corpus role;
6. rank clusters deterministically by the number and electrical diversity of
   affected discovery cases, with safety and reviewed downstream reuse as
   explicit tie-break evidence;
7. create an aggregate expansion plan for the highest-ranked actionable
   cluster through the existing quarantined capability-expansion contracts;
8. implement only the highest-ranked reusable capability or the smallest set
   of inseparable generic prerequisites proven by the frozen report;
9. rerun identical corpus bytes and demonstrate strict discovery and held-out
   improvement without regressing any previously passing case;
10. promote every newly passing case through all applicable electrical,
    physical, writer, installed-KiCad, round-trip, and replay gates; and
11. preserve stable fail-closed behavior for every case that remains outside
    the reviewed envelope.

## 4. Corpus Structure

Version 1 consists of two simultaneously frozen roles:

- **discovery**: used once to establish the baseline clusters and select the
  capability to implement;
- **held_out**: excluded from cluster scoring, selection, implementation
  choices, component choices, model choices, tuning, and repair-policy design.

The target is 24 requirements: 12 discovery and 12 held-out. Each role contains
two cases in each reporting domain:

- analog;
- power;
- digital/control;
- MCU/interface;
- sensor; and
- mixed-signal.

Each requirement is a strict behavior-only open-topology document. It may state
external ports, domains, operating cases, observable transfers, accuracy,
frequency, timing, startup, fault, environmental, thermal, SOA, isolation, and
manufacturing-neutral board bounds. It may not prescribe topology, component,
manufacturer, part, model, pin, net, block, coordinate, layer, track, via,
repair, expected outcome, issue code, or capability identifier.

`CORPUS_RULES.md` is normative. The manifest, membership order, requirement
bytes, source hashes, role split, domain labels, and safety declarations become
immutable at freeze. Any change requires a new corpus version and baseline.

## 5. Terminal Outcome Contract

The evaluator owns this closed result set:

- `pass`: synthesis and every requirement-selected acceptance gate pass;
- `unsupported`: required reviewed architecture, component, model, analysis,
  physical rule, router behavior, or verifier capability is unavailable;
- `unsafe`: the behavior is contradictory, violates a proved electrical or
  safety bound, lacks mandatory safety evidence, or every bounded candidate is
  rejected by fault, thermal, SOA, isolation, or other critical evidence;
- `exhausted`: bounded search or repair ends without proving either a passing
  design or a stable unsupported/unsafe conclusion.

Malformed corpus input, invalid evaluator output, tool crashes, nondeterminism,
missing installed tooling in a required run, and artifact corruption are
evaluation failures. They may not be counted as product outcomes.

Outcome precedence is deterministic:

1. invalid evaluation aborts the report;
2. a proved contradiction or safety rejection is `unsafe`;
3. a typed unavailable capability is `unsupported`;
4. bounded work ending without either proof is `exhausted`;
5. only complete qualified evidence is `pass`.

## 6. Evidence Adapter And Causal Gaps

A production adapter must consume the immutable requirement and the normal
synthesis/promotion evidence. It may not infer a gap from diagnostic prose when
a structured code, stage, capability, requirement path, model dependency,
route operation, or gate identity exists.

Every non-passing case records:

- terminal outcome and stop reason;
- earliest authoritative root stage and code;
- normalized semantic capability;
- one typed gap scope;
- affected behavioral requirement IDs and operating cases;
- required evidence needed to close the gap;
- downstream symptom codes that were causally suppressed from ranking;
- search, value, simulation, repair, and promotion consumption;
- requirement, inventory, catalog, model-policy, evaluator-policy, and toolchain
  hashes; and
- hashes of the raw evidence artifacts from which the observation was derived.

The seven gap scopes map to existing capability-expansion needs as follows:

| Gap scope | Expansion need |
| --- | --- |
| `topology` | `architecture` |
| `component` | `component` |
| `model` | `model` |
| `simulation` | `verification` plus a reviewed model when required |
| `physical` | `physical_rule` |
| `routing` | `routing` |
| `verification` | `verification` |

A downstream symptom cannot outrank its root cause in the same case. Multiple
independent root blockers may each contribute once.

## 7. Identity-Neutral Clustering

The canonical cluster key is:

```text
terminal outcome + root stage + gap scope + semantic capability + stable code
```

Clustering must:

- deduplicate one case within one cluster;
- retain sorted discovery and held-out membership separately;
- retain sorted domains, behavioral analysis kinds, safety levels, required
  evidence, and raw evidence hashes;
- remain byte-identical under input, diagnostic, map, and filesystem ordering;
- exclude case IDs from cluster identity while retaining them as traceability;
- reject identities containing project names, fixture paths, corpus roles,
  component selections, coordinates, or expected implementations; and
- preserve stable unsupported, unsafe, and exhausted identities across replay.

## 8. Deterministic Ranking

Only discovery cases participate in selection. Held-out cases are evaluated
and frozen at baseline but their cluster membership is sealed until after the
implementation choice is recorded.

Clusters sort lexicographically by this versioned tuple:

1. descending distinct discovery-case count;
2. descending distinct reporting-domain count;
3. descending distinct behavioral-analysis-kind count;
4. descending safety score using the frozen policy weights;
5. descending reviewed downstream-consumer count from the existing acyclic
   capability-impact registry;
6. ascending normalized semantic capability; and
7. ascending canonical cluster key.

The report records every tuple element, policy version, impact-registry hash,
and exact tie-break. Prompt wording, provider suggestions, implementation cost,
and held-out results cannot influence rank.

A cluster is actionable only if the existing expansion planner can express its
required sources, artifact type, promotion gates, risks, and mutation boundary.
If rank 1 is not actionable, the report must fail closed with that planner gap;
it may not silently choose rank 2.

## 9. Baseline Freeze

`BASELINE_PROTOCOL.md` is normative. Before capability implementation begins,
the repository must contain:

- the frozen manifest and requirement files;
- a corpus SHA-256 file;
- the frozen evaluator and ranking policy versions;
- two byte-identical clean-root baseline reports;
- raw per-case evidence with content hashes;
- a sealed rank-1 selection record;
- a baseline checksum; and
- tests that reproduce all bytes and reject corpus, policy, registry, or
  evidence drift.

Infrastructure required to evaluate and rank the corpus may be implemented
before the baseline. Architecture, catalog, model, simulation, physical,
routing, or verification support capable of changing a corpus outcome may not.

## 10. Expansion Selection And Implementation

Production work is limited to rank 1 and generic prerequisites that satisfy all
of these conditions:

- the prerequisite is explicitly listed in rank-1 required evidence;
- rank 1 cannot be promoted without it;
- it is reusable outside the corpus;
- it is represented by an existing capability-expansion need and source type;
- it does not inspect corpus identity; and
- it does not weaken a gate or convert missing evidence into a pass.

If rank 1 requires external engineering evidence that is not locally available,
the candidate remains quarantined and the milestone reports the missing source;
another cluster is not substituted merely because it is easier.

## 11. Improvement Contract

Final comparison uses identical corpus, policy, registry, catalog baseline,
toolchain major/minor version, acceptance gates, and work budgets unless a
budget increase was itself selected as the rank-1 generic capability.

Success requires:

- at least one additional discovery `pass`;
- at least one additional held-out `pass` not used for ranking or tuning;
- rank-1 affected discovery pass count to increase;
- no baseline `pass` to regress;
- no `unsafe` case to become `pass` without new reviewed safety evidence;
- no `unsupported` or `exhausted` case to become `pass` by reclassification,
  skipped work, relaxed assertions, or increased hidden budgets;
- stable remaining cluster identities and deterministic report bytes; and
- a machine-verifiable baseline-to-final causal attribution to promoted generic
  artifacts and evidence hashes.

## 12. Promotion Gates

Every newly passing case must satisfy all applicable gates:

- strict decode, normalization, and behavior-only validation;
- bounded topology and value search with complete alternatives;
- catalog-backed component selection and pin/footprint fidelity;
- required DC, AC, transient, distortion, noise, startup, fault,
  electrothermal, SOA, isolation, and tolerance evidence;
- readable hierarchical schematic lowering;
- deterministic multilayer placement and routing;
- complete connectivity and required routes;
- writer correctness;
- clean installed-KiCad ERC and strict DRC;
- zero normalized schematic and PCB round-trip differences; and
- two clean-root byte-identical projects and evidence artifacts.

## 13. Regression And Non-Circumvention

The milestone preserves every currently passing frozen corpus and the protected
USB-C LED/I2C, ESP32, amplifier, MCU, sensor, power, routing, writer, and repair
lanes affected by the implementation.

Production code may not contain new corpus IDs, corpus paths, project names,
fixture hashes, expected outcomes, fixture coordinates, allowlists,
benchmark-only schemas, specialized block families, or conditional success
paths. Tests must scan for these shortcuts outside testdata and test files.

## 14. Deliverables And Closeout

Closeout requires:

- `SPEC.md`, `PLAN.md`, `CORPUS_RULES.md`, and `BASELINE_PROTOCOL.md`;
- frozen corpus, manifest, and checksums;
- untouched baseline and final reports plus checksums;
- sealed selection record and aggregate expansion plan;
- source-backed candidate, promotion bundle, and approval evidence when the
  selected capability changes a supported registry;
- promotion matrix and requirement-by-requirement completion audit;
- complete relevant local regression evidence;
- Prism review with high and medium findings resolved or disproved from direct
  repository evidence;
- clean commit and push; and
- no manual GitHub Actions execution or monitoring during development.
