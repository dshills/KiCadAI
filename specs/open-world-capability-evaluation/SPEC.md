# Open-World Capability Evaluation And Gap-Driven Expansion Specification

## 1. Purpose

Move KiCadAI from manually chosen capability expansion toward a deterministic
feedback loop that evaluates unfamiliar behavior-only requests, identifies
reusable capability gaps, ranks them by measured value, implements the
highest-value families, and proves improvement on untouched held-out requests.

This milestone expands the verified capability envelope. It does not weaken
fail-closed behavior or claim unrestricted arbitrary-circuit generation.

## 2. Required Outcomes

The completed milestone must:

- freeze a novel discovery corpus and a separately frozen held-out corpus
  before production support is added for the selected gaps;
- cover analog, power, MCU, sensor, digital, and mixed-signal domains;
- accept behavior, interfaces, operating conditions, faults, safety limits,
  and manufacturing-neutral board bounds without prescribing implementation;
- measure `ready`, `needs_clarification`, `unsupported`, `ambiguous`, and
  `budget_exhausted` terminal outcomes;
- cluster failures by normalized semantic capability rather than fixture
  identity, prompt wording, topology, or expected implementation;
- rank clusters deterministically by frequency, safety impact, and downstream
  reuse;
- retain complete case-to-cluster and cluster-to-required-evidence
  traceability;
- implement the highest-ranked reusable families, including clock fanout and
  loading, MCU programming/debug electrical loads, bus buffering and level
  translation, and additional converter and isolation primitives;
- prove improvement on held-out cases that did not participate in gap
  selection or implementation decisions;
- promote every newly supported positive case through simulation, physical
  realization, installed-KiCad validation, and deterministic replay; and
- preserve stable unsupported, ambiguity, clarification, and budget outcomes
  for requests that remain outside the trusted envelope.

## 3. Corpus Contract

Two corpora are required:

1. **Discovery corpus**: used exactly once to establish the baseline and rank
   reusable gaps.
2. **Held-out corpus**: frozen at the same time but excluded from ranking,
   family selection, component/model selection, repair policy, and production
   tuning.

Each corpus must contain all six required domains. Every record must include:

- a stable opaque case ID;
- domain;
- behavior-only source text;
- declared safety impact;
- source SHA-256;
- a corpus membership role; and
- no expected topology, provider, component, model, net, pin, coordinate,
  layer, route, issue code, gap code, or expected outcome.

The corpus manifest and source bytes must be SHA-256 pinned. Tests must reject:

- post-freeze byte changes;
- duplicate or reordered membership;
- implementation nouns or identities;
- fixture paths, expected codes, and expected outcomes;
- missing domains or terminal-outcome coverage in evaluated reports; and
- discovery/held-out source duplication or paraphrase leakage.

## 4. Terminal Outcome Model

The evaluator owns this closed set:

- `ready`: executable requirement and all required trusted qualification passed;
- `needs_clarification`: one or more user-owned choices block a safe result;
- `unsupported`: trusted installed capability or evidence is unavailable;
- `ambiguous`: multiple materially different complete candidates remain tied
  or an authoritative semantic interpretation cannot be selected;
- `budget_exhausted`: configured bounded work ended before completeness was
  proven.

Invalid provider data, tool failures, malformed evidence, and evaluator
contract violations are not capability outcomes. They must fail the evaluation
run and cannot be counted or ranked as product gaps.

Every non-ready case must carry at least one normalized observation containing:

- semantic capability;
- terminal outcome;
- blocking stage;
- stable issue code;
- evidence path;
- reason; and
- required evidence needed to close the gap.

Observation identity must exclude case IDs and source wording.

## 5. Generic Clustering

The canonical cluster key is derived from:

```text
outcome + stage + semantic capability + issue code
```

Clustering must:

- normalize and validate all fields;
- deduplicate repeated observations within one case;
- count one case at most once per cluster;
- retain sorted case and domain membership;
- retain the union of sorted required-evidence statements;
- remain identical under case, observation, registry, and map reordering; and
- never inspect prompt text after the terminal evidence is produced.

Production cluster keys may not contain fixture IDs, paths, topology names,
component identities, or benchmark-specific aliases.

## 6. Ranking Policy

Ranking inputs are authoritative evidence, not provider claims.

For each cluster:

- `frequency_score` is the number of distinct discovery cases;
- `safety_score` is the sum of corpus-declared safety weights for those cases;
- `reuse_score` is the count of unique downstream capabilities in the reviewed
  capability-impact registry;
- `domain_count` is the number of distinct discovery domains; and
- `required_evidence` is the stable union of evidence needed by member cases.

Safety weights are versioned by evaluator policy. Downstream reuse edges are
checked-in, acyclic, semantic capability relationships and cannot be supplied
by prompts, providers, or corpus cases.

Clusters sort by:

1. descending frequency score;
2. descending safety score;
3. descending reuse score;
4. descending domain count;
5. normalized semantic capability; and
6. canonical cluster key.

Reports must record the policy version, registry hash, corpus hash, rank,
component scores, and exact tie-break evidence. Reordering inputs must not
change report bytes.

## 7. Baseline And Improvement

The untouched baseline must be generated before implementing selected
capabilities. It records:

- outcome counts and ratios;
- outcome counts by domain;
- blocking stage, code, and capability counts;
- ranked discovery clusters;
- search and solver consumption;
- corpus, registry, catalog, model, and policy hashes; and
- per-case terminal evidence.

The final evaluator must run against the identical corpus bytes, policy, and
acceptance gates.

Improvement requires:

- a strict increase in `ready` discovery cases;
- a strict increase in `ready` held-out cases;
- no discovery or held-out case changing from a safe non-ready outcome to an
  unqualified `ready`;
- no regression in previously ready cases;
- no reduction in required safety evidence;
- stable remaining gap identities under corpus reorder; and
- evidence that each implemented family improves at least one held-out case
  not used to rank that family.

## 8. First Expansion Families

The initial expansion must implement generic, reviewed support for:

### 8.1 Clock Fanout And Loading

- source/receiver electrical compatibility;
- fanout and capacitive loading;
- edge-rate and source-termination evidence;
- optional reviewed buffering;
- startup, enable, and jitter limits where required.

### 8.2 MCU Programming And Debug Loads

- programming/debug interface discovery from MCU records;
- reset, boot, pull, level, and contention constraints;
- programmer/debugger loading and isolation;
- shared-pin arbitration; and
- stable capability gaps for unsupported tools or voltage domains.

### 8.3 Bus Buffering And Level Translation

- whole-bus voltage, direction, topology, pull-up, capacitance, speed, and
  fanout evidence;
- catalog-backed buffer or translator selection;
- bidirectional/open-drain versus push-pull correctness;
- enable/default-state safety; and
- fail-closed unsupported mixed-voltage combinations.

### 8.4 Converter And Isolation Primitives

- at least one additional reviewed converter primitive and one reviewed
  isolation primitive;
- electrical, dynamic, thermal, startup, shutdown, fault, and protection
  applicability;
- isolation working-voltage/clearance evidence where applicable; and
- stable unsupported outcomes outside reviewed energy and safety bounds.

No family may be selected through fixture identity or implemented as a
benchmark-specific block.

## 9. Production Integration

Open-world evaluation must be a reusable internal package with deterministic
JSON artifacts. The CLI or promotion tooling must be able to:

- evaluate a manifest;
- emit the aggregate report;
- compare baseline and final reports;
- explain cluster rank;
- list cases affected by a capability; and
- fail verification when hashes, policy, corpus membership, or required
  evidence differ.

The evaluator must consume normal compiler, architecture-search, closed-loop,
workflow, and promotion evidence. It may not introduce a parallel success path.

## 10. Physical And KiCad Promotion

Every newly ready held-out case must pass locally:

- strict decode and normalization;
- deterministic architecture search and complete alternatives;
- reviewed catalog/model selection;
- all applicable simulation, corner, event, and safety assertions;
- complete lowering and traceability;
- internal validation and connectivity;
- route completion;
- writer correctness;
- clean installed-KiCad ERC;
- strict installed-KiCad DRC;
- zero normalized schematic and PCB round-trip differences; and
- byte-identical deterministic replay.

Two clean-checkout promotion roots must produce identical content-addressed
bundles for the newly supported held-out set.

## 11. Regression And Safety

The milestone must preserve:

- the 12/12 frozen held-out benchmark;
- six hierarchical multi-domain systems;
- six dynamic electrothermal/control-loop circuits;
- behavioral-intent, amplifier, MCU, ESP32, sensor, power/protection, and
  protected USB-C lanes;
- routing, placement, writer, repair, and fabrication evidence suites; and
- all stable unsupported and negative-corpus outcomes.

Production code may not contain corpus IDs, corpus paths, expected outcomes,
fixture-specific coordinates, allowlists, specialized provider schemas, or
conditional success paths.

## 12. Closeout

Closeout requires:

- frozen discovery and held-out corpus manifests;
- untouched baseline and final reports with checksums;
- a requirement-by-requirement audit;
- two identical local promotion bundles;
- Prism review with every high and medium finding resolved;
- a clean commit and push; and
- no GitHub Actions execution or monitoring as part of the development loop.

A later GitHub failure reported by the repository owner reopens the milestone.
