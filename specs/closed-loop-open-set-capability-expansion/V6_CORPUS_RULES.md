# Closed-Loop Open-Set V6 Corpus Rules

Status: freeze candidate; no authoring before the contract freeze commit

## Size, roles, and authorship

The corpus contains exactly 36 behavior-only requirements:

- 18 discovery and 18 held-out;
- six reporting domains: analog, power, digital, MCU, sensor, and mixed-signal;
- exactly three independent authors;
- 12 requirements per author: six discovery and six held-out; and
- exactly one discovery and one held-out requirement per domain per author.

Authors receive disjoint ID, source-ID, and stable-path allocations covering
the complete 36-entry corpus. Concrete IDs and source IDs appear only in
assignments, the manifest, and authorship records, never in requirement bodies.

## Isolation

Each author receives only the committed V6 author packet, strict public
requirement contract, canonical vocabulary, assignment, and their own empty
quarantine root. Authors may not inspect the repository implementation, any
V1-V5 requirement source, V5 held-out plaintext, synthesis output, baseline,
ranking, selection, another author's packet, or another author's files.

The corpus custodian and validator may inspect authored requirement meaning but
must not run synthesis, inspect candidate outcomes, or alter electrical intent.
The implementation context receives only the published discovery corpus and
encrypted held-out source after the corpus freeze.

## Requirement contract

Every requirement must strictly decode under the frozen public
open-topology-requirement schema and use only canonical public enums, metrics,
units, references, ports, conditions, events, analyses, assertions, physical
constraints, and acceptance gates.

Requirements describe externally observable electrical behavior and physical
constraints. They must not prescribe a component part number, footprint,
topology, named circuit family, coordinate, trace path, layer transition,
template, block family, implementation algorithm, expected diagnostic, or
known KiCadAI limitation.

All numeric ranges must be finite, ordered, physically meaningful, and bounded.
Every reference must resolve. Every analysis and assertion must have sufficient
conditions and observations to be testable. Every requirement must include the
complete inherited 14-gate acceptance profile.

## Diversity

Within each author bundle and across the complete corpus, the validator must
enforce diversity in:

- port direction and electrical quantity;
- supply count, polarity, and operating range;
- DC, AC, transient, noise, tolerance, thermal, electrothermal, and startup
  analysis coverage;
- events, sweeps, corners, loads, and environmental conditions;
- assertion metrics and observation scopes;
- board limits, placement classes, thermal constraints, and safety impact; and
- semantic connectivity and behavior signatures.

No author may repeat the same normalized electrical behavior signature within
a role. Discovery and held-out cases may not be paired variants. The complete
corpus must not be dominated by one analysis kind, assertion metric, port
shape, supply shape, or safety category.

## Historical exclusion

The validator binds the raw and normalized semantic commitments of every V1
through V5 requirement, including the retired V5 corpus. A V6 requirement fails
if its raw hash, normalized semantic hash, or prohibited near-duplicate
signature matches historical or same-corpus evidence.

Historical held-out comparisons operate on commitments only. No V1-V5 held-out
plaintext may be decrypted or exposed to authors, validators, selectors,
implementers, reviewers, or logs.

## Validation and publication

Validation is mechanical and outcome-neutral: strict JSON shape, canonical
enums, references, numeric bounds, gate completeness, assignments, provenance,
hashes, quotas, diversity, uniqueness, and historical non-overlap. It may not
run synthesis or infer expected outcomes.

After validation, the publisher must:

- preserve exact manifest order and byte hashes;
- publish discovery files in plaintext;
- encrypt held-out sources using AES-256-GCM with a fresh external 0600 key and
  authenticated length-delimited metadata;
- publish only ciphertext and non-revealing held-out commitments;
- use atomic no-replace publication; and
- remove quarantine plaintext only after canonical publication succeeds.

Any schema, provenance, quota, diversity, uniqueness, encryption, commitment,
or publication failure retires the candidate corpus without synthesis.
