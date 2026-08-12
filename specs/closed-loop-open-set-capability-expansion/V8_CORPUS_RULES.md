# V8 Fresh Behavior-Only Corpus Rules

## 1. Corpus shape and independence

- Exactly six independently isolated authors participate.
- Each author creates exactly six requirements: three discovery and three
  held-out, for exactly 18 cases per role and 36 total.
- Each author sees only its checksum-verified frozen packet and writes only to
  its designated empty quarantine.
- Authors may not inspect KiCadAI source, tests, catalogs, inventories, prior
  corpus source, prior outcomes, baselines, gaps, frontiers, rankings, plans,
  another author bundle, or synthesis/simulation/feasibility output.
- The assignment and author packet are verified before the assignment content
  is used. A procedural isolation violation invalidates that context.
- At most two fresh replacements are allowed for an invalid assignment bundle.

## 2. Frozen assignment dimensions

The publisher freezes assignments before dispatch. Across each author's six
requirements there is exactly one case in every reporting domain and exactly
one in every circuit role.

Reporting domains:

1. analog signal path;
2. power and energy conversion;
3. digital and control;
4. mixed-signal and data conversion;
5. sensing and instrumentation; and
6. protection and power integrity.

Circuit roles:

1. source or bias;
2. amplification or conditioning;
3. conversion or regulation;
4. sensing or measurement;
5. interface or control; and
6. protection or supervision.

The six author assignments use a frozen Latin schedule so domain/role pairings
do not repeat across authors. Each domain and role therefore has exactly three
discovery and three held-out cases globally. Every author receives exactly
three discovery and three held-out assignments.

The four safety impacts are `non_safety`, `review_required`,
`safety_relevant`, and `safety_critical`. The assignment set balances them
globally as 9 each and 4/5 or 5/4 across discovery/held-out. No author chooses
its own domain, role, safety impact, partition, or case ID.

## 3. Behavior-only requirement boundary

Allowed content describes:

- named electrical power, signal, control, data, and reference ports;
- external supply and source envelopes;
- load, environment, timing, bandwidth, accuracy, noise, thermal, and stability
  limits;
- operating cases and externally observable assertions;
- safety behavior and bounded fault cases; and
- educationally meaningful names and descriptions of externally visible
  behavior.

Prohibited content includes:

- component manufacturer, orderable part, symbol, footprint, package, or pin
  number;
- topology, schematic arrangement, block family, stage count, or prescribed
  implementation technique;
- component values unless they are external source/load/environment contracts;
- PCB dimensions, coordinates, layers, routes, zones, placement, or templates;
- KiCadAI identifiers, files, APIs, diagnostics, capabilities, or expected
  outcomes;
- corpus, manifest, author, selection, ranking, frontier, or held-out identity
  tokens inside behavior fields; and
- prose that encodes an implementation through synonyms or illustrative part
  choices.

## 4. Semantic identity and obligation inputs

Every requirement has unique normalized IDs for case, port, output, operating
case, assertion, and observation. References resolve exactly. IDs describe
behavior, not implementation.

Every assertion names exactly one operating case, one observation, and one
output or explicit whole-circuit observation. Whole-circuit observations use
the frozen `@circuit` output sentinel. The publisher—not the author—
derives the immutable V8 obligation anchor from the frozen manifest hash, role,
case ID, operating-case ID, assertion ID, observation kind/ID, and output ID.
Authors neither see nor supply anchor hashes.

Assertions use canonical quantity names and units. Bounds are finite and
ordered, count limits are integers, all physical magnitudes obey the public
contract, and directional behavior such as rising/falling thresholds remains
distinct when electrically meaningful.

## 5. Required behavioral diversity

Per author bundle:

- all six domain and role assignments are present exactly once;
- at least two requirements have multiple externally meaningful outputs, with
  at least one in each role partition;
- at least four distinct analysis kinds occur;
- at least two requirements have a static primary behavior and at least two
  have a dynamic primary behavior;
- at least two include a bounded fault or off-nominal operating case;
- at least one includes noise, thermal, stability, or tolerance behavior; and
- no two normalized structural/behavior signatures are identical.

Across the complete corpus:

- all canonical analysis kinds required by the public contract appear in both
  discovery and held-out;
- at least six cases exercise multi-output behavior in discovery and at least
  six in held-out;
- at least six cases exercise bounded fault behavior in discovery and at least
  six in held-out;
- every reporting domain and every circuit role has at least one
  safety-relevant or safety-critical case in discovery and in held-out; and
- discovery and held-out have distinct normalized signature multisets while
  satisfying the same aggregate quotas.

Cosmetic renaming, reordered arrays, unused ports, duplicated assertions, and
mathematically unbounded ideal sources do not count toward diversity.

## 6. Acceptance profile

Every requirement requests all 14 validation gates:

1. requirement-contract validity;
2. complete topology;
3. electrical connectivity;
4. component/catalog realization;
5. model coverage;
6. simulation execution;
7. behavioral assertion satisfaction;
8. safety evidence;
9. clean ERC;
10. strict DRC;
11. route completion;
12. writer correctness;
13. zero round-trip diffs; and
14. deterministic replay.

An author may strengthen behavior assertions but may not remove, downgrade, or
conditionally disable a validation gate.

## 7. Validation, adjudication, and correction

Packet-local checks cover exact JSON shape, enums, units, references, quotas,
semantic IDs, diversity, forbidden tokens, assignment order, authorship shape,
source hashes, and exact bundle paths. Authors may run only those checks.

Public adjudication may correct an objective public-contract or electrical
meaning error without synthesis, feasibility, classification, ranking, or
outcome inspection. It updates only the affected requirement and the exact
authorship timestamp/hash fields permitted by the template. Every correction
is recorded by category. A correction that changes assigned domain, role,
safety impact, partition, or intended observable behavior invalidates the
bundle and requires a fresh replacement.

## 8. Publication and secrecy

The publisher validates all six bundles together, derives obligation anchors,
rejects cross-author duplicates, and atomically publishes discovery plaintext
plus held-out ciphertext and commitments. Held-out records use AES-256-GCM with
unique 96-bit nonces and authenticated frozen metadata. The external source key
is 32 random bytes, mode 0600, created atomically without replacement outside
the repository and synchronized workspace.

After authenticated publication and independent verification, all author
quarantine plaintext is removed. No held-out plaintext, case mapping, outcome,
gap, frontier, ranking, or diagnostic is disclosed to the implementation
context. Any uniqueness, encryption, commitment, provenance, or cleanup failure
retires the corpus candidate before synthesis.
