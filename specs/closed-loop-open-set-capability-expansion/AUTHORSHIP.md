# Closed-Loop Open-Set Corpus Authorship Record

Status: version-2 corpus authored and frozen; production outcomes have not been
recorded

Date: 2026-08-09

## Separation Record

The 24 requirements were independently composed for this milestone rather than
selected, copied, or paraphrased from an existing KiCadAI fixture corpus. The
authoring inputs were limited to:

- the frozen corpus rules in `CORPUS_RULES.md`;
- the public open-topology requirement schema;
- the public port, condition, event, analysis, metric, and unit vocabulary; and
- ordinary externally observable electrical behavior.

No expected outcome, stop reason, diagnostic, gap, rank, implementation,
component identity, model identity, geometry, or route was supplied during
authoring. No corpus synthesis or physical-promotion run was performed before
all 24 requirement definitions, roles, domain labels, safety labels, order,
policy, impact registry, budgets, and environment fields were frozen.

The generic evidence adapter was committed separately as `520dd4c2` before the
corpus bytes were materialized. The version-1 normative contract was already
frozen in `dde76b3b`.

## Corpus Composition

Version 2 contains 12 discovery and 12 held-out cases. Within each role there
are exactly two cases in each reporting domain:

- analog;
- power;
- digital/control;
- MCU/interface;
- sensor; and
- mixed-signal.

Case identities and filenames are opaque sequential labels. Reporting role,
domain, and safety metadata exist only in the manifest, not in requirement
documents. Discovery and held-out requirements use distinct ports, assertions,
conditions, and events; normalized requirement hashes are unique.

## Review Record

The freeze tests verify:

- strict schema decoding and complete acceptance gates;
- manifest order, role/domain quotas, filenames, file bytes, and SHA-256 values;
- deterministic regeneration from the reviewed behavior-only seed definitions;
- unique normalized requirement content;
- absence of prohibited implementation language in requirement string values;
- absence of role, domain, safety, outcome, and ranking metadata in requirement
  bytes;
- at least four multi-function, dynamic, startup/fault, and safety-evidence
  cases per role;
- at least two bipolar/below-reference and switching/time-varying cases per
  role; and
- an acyclic content-addressed impact registry and finite synthesis budget.

The held-out files are checked in so the freeze is auditable, but the evaluator
contract prevents held-out gaps from producing clusters or influencing the
rank-one expansion plan. Implementation work must use only the sealed discovery
selection record created by the untouched baseline.

## Freeze Boundary

After the version-2 corpus-freeze commit, any change to a requirement byte,
entry order, role, reporting domain, safety level, evaluator policy, impact
registry, synthesis policy, or environment contract creates corpus version 3.
Expected outcomes are deliberately absent from this record and from the
manifest.

## Version 2 Budget Correction

Corpus version 1 was committed in `3813489d` with the production default search
policy. The first untouched execution reached the Go test runner's ten-minute
ceiling during the first replay of discovery case 3 while evaluating bounded
electrothermal corners. It produced no baseline report, selection, expected
outcome file, or capability implementation.

Version 2 preserves all 24 requirement files byte-for-byte, their order, roles,
domains, safety declarations, evaluator policy, impact registry, and reference
environment. It changes only the explicit finite synthesis work budget to make
the required 48 synthesis runs locally reproducible. This is recorded as a new
corpus version rather than a silent budget change, as required by
`CORPUS_RULES.md`. Version 1 is superseded and must not be used for baseline or
uplift claims.
