# Closed-Loop Open-Set V7 Corpus Rules

Status: freeze candidate; no authoring before the V7 contract freeze commit

## Size, roles, and authorship

The corpus contains exactly 36 new behavior-only requirements:

- 18 discovery and 18 held-out;
- six reporting domains: `analog`, `power`, `digital`, `mcu`, `sensor`, and
  `mixed_signal`;
- exactly three independent authors;
- 12 requirements per author: six discovery and six held-out; and
- exactly one discovery and one held-out requirement per domain per author.

Authors receive disjoint ID, source-ID, and stable-path allocations. Concrete
identities appear only in assignments, the manifest, and authorship records,
never in requirement bodies.

The exact cardinality is part of the frozen statistical sample and may not be
reduced after authoring. An invalid pre-publication requirement is corrected in
the bounded outcome-neutral remediation window below or the corpus candidate
retires; it is never silently dropped.

## Isolation

Each author receives only the committed V7 packet, strict public requirement
contract, canonical vocabulary, assignment, and an empty private quarantine.
Authors may not inspect repository implementation, V1-V6 source, any retired
held-out plaintext, synthesis output, baseline, frontier, lineage, ranking,
selection, implementation plan, another author's packet, or another author's
files.

The validator may inspect public electrical and schema meaning but may not run
synthesis, infer expected outcomes, or alter electrical intent. The publisher
may validate and encrypt but may not evaluate. The implementation context
receives only published discovery source and authenticated held-out ciphertext.

## Requirement contract

Every requirement must strictly decode under the frozen public
open-topology-requirement schema and use only canonical public enums, metrics,
units, references, ports, conditions, events, analyses, assertions, physical
constraints, and acceptance gates.

Requirements describe externally observable behavior and physical constraints.
They must not prescribe component part numbers, footprints, topologies, named
circuit families, coordinates, routes, layers, templates, block families,
algorithms, expected diagnostics, capability names, or known KiCadAI limits.

All numeric ranges must be finite, ordered, bounded, and physically meaningful.
References must resolve. Analyses and assertions must have sufficient
conditions and observations to be testable. Every requirement includes the
complete inherited 14-gate acceptance profile.

## Diversity

Within each author bundle and across the complete corpus, the validator enforces
diversity in:

- port direction, quantity, multiplicity, and dependency;
- supply count, polarity, sequencing, and operating range;
- DC, AC, transient, noise, tolerance, thermal, electrothermal, startup, and
  mixed-analysis coverage;
- events, sweeps, corners, loads, and environmental conditions;
- assertion metrics and observation scopes;
- single-output, multi-output, controlled, autonomous, and cross-domain
  behavior shapes;
- board limits, placement classes, thermal constraints, and safety impact; and
- semantic connectivity and normalized behavior signatures.

No author may repeat a normalized electrical behavior signature within a role.
Discovery and held-out cases may not be paired variants. The validator enforces
the following complete-corpus limits mechanically:

- each reporting domain occurs exactly three times per role and six times total;
- each safety-impact category occurs between six and twelve times total;
- the behavioral-requirement object whose `id` has the smallest raw unsigned
  UTF-8 byte sequence (no locale or array-order dependence) is primary; its
  analysis occurs no more than twelve times across the corpus;
- the metric of that same primary behavioral requirement occurs no more than
  nine times across the corpus; and
- no identifier-normalized port/supply shape occurs more than six times.

The four safety-impact weights used by ranking are frozen as `non_safety=0`,
`review_required=1`, `safety_relevant=3`, and `safety_critical=5`.

## Historical exclusion

The validator binds every available V1-V6 raw and neutral semantic commitment,
including V6 discovery and held-out commitments. A V7 requirement fails if its
raw hash, available semantic hash, identifier-normalized semantic hash, or
prohibited near-duplicate signature matches historical or same-corpus
evidence.

Before accepting any author bundle for publication, the validator performs one
global cross-author raw, semantic, identifier-normalized, and near-duplicate
comparison across all current submissions. A collision rejects every involved
file and returns outcome-neutral diagnostics to the owning author contexts.

Historical comparisons use commitments only. Retired held-out plaintext may
not be decrypted or exposed to authors, validators, selectors, implementers,
reviewers, or logs.

Every composed identity and semantic-hash input uses the frozen field order.
Each field is encoded as its UTF-8 bytes preceded by one unsigned 32-bit
big-endian integer containing that field's byte length; the prefixed fields are
then concatenated without separators. A field longer than `2^32-1` bytes is
invalid rather than truncated.

## Validation and publication

Validation is mechanical and outcome-neutral: strict JSON, canonical enums,
references, numeric bounds, complete gates, assignments, provenance, hashes,
quotas, diversity, uniqueness, and historical non-overlap. It may not run
synthesis or generate frontier evidence.

Before publication, each author may receive at most three correction packets
containing only validator rule IDs, affected assigned paths, and
outcome-neutral diagnostics. The author may resubmit only those assigned files
and the corresponding provenance hashes. Corrections remain inside the same
isolated author context, are fully recorded, and receive the complete validator
again. The packet may not disclose synthesis behavior, expected outcomes,
capability names, other authors' content, or historical plaintext. The third
packet is final: any validation failure in the resubmission produced from it
discards the bundle, so an author has at most one initial submission plus three
corrected resubmissions. The custodian may commission at most two sequential
replacement contexts for that assignment. A replacement author receives only
the original frozen packet and assignment, never the rejected files or
diagnostics, and receives the same correction ceiling. Failure of the second
replacement retires the corpus candidate without synthesis.
The published corpus still has exactly three contributing authors.

After validation, the publisher must:

- preserve exact manifest order and full-byte hashes;
- publish discovery files in plaintext;
- encrypt held-out source using AES-256-GCM with a fresh external 0600 key and
  authenticated length-delimited metadata;
- publish only ciphertext and non-revealing held-out commitments;
- use atomic no-replace publication; and
- remove quarantine plaintext only after canonical publication succeeds.

After the bounded correction window, any schema, provenance, quota, diversity,
uniqueness, encryption, commitment, or publication failure retires the
candidate corpus without synthesis.
