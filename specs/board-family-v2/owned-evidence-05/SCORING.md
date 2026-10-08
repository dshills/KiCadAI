# Owned-v4 raw scoring and source-bound review

Status: **implemented, verified offline, and bound to the retained rehearsal**.
The overall two-family goal remains active. This extends
[the collection checkpoint](COLLECTION.md), not its live authority.

## Results and limits

The evaluator preserves raw owned-v4 facts and reports extraction, application
decisions and complete outcomes separately. Each reviewed fact retains its raw
JSON plus resolved clause text, quantity occurrences, numeric choice and owner.
Resolution annotations are not a rewritten model response.

The retained synthetic rehearsal received an implementing-agent review of all
48 facts and all 47 frozen gold requirements. It scored:

| Measure | Result |
| --- | --- |
| Raw fact fidelity/completeness | 14/14 |
| Application decisions and required artifacts | 14/14 |
| Complete cases | 14/14 |
| Acceptance | Offline only; cannot establish live acceptance |

These are synthetic responses from the test transport, not new AI answers.
The last live batch remains **9/14 complete and 3/5 required native bundles**.
The known corpus is neither an independent holdout nor a statistical estimate.
The recorded reviewer is the implementing agent, not an independent reviewer.

All five useful-case times remain included: median 4.018 seconds and maximum
4.329 seconds, measured in the earlier synthetic command rehearsal. They are
not model-latency measurements. Failures and unattempted cases cannot shrink the
14-case denominator or silently disappear from timing calculations.

## Safeguards

37 pure scoring tests and one retained-evidence integration test passed. They
cover source/contract changes, wrong polarity, malformed references, dimensional
choice errors, numeric omissions, duplicate alias set semantics, altered review
facts and owners, missing native evidence, unattempted/invalid cases and timing.
A valid-looking invented feature still needs an explicit semantic failure even
when all minimum gold requirements match.

The template approves no judgment. Editing it cannot mutate the authenticated
input objects. Completed review requires per-fact meaning and source-scope
judgments, explicit completeness, requirement-to-fact support, and targeted or
truthful decision judgments. Every fact is reviewed, including extra facts
outside the minimum gold list. A forged live mode cannot make the pure scorer
grant live acceptance.

Separating automated measurements from explicit semantic judgment is consistent
with [OpenAI's evaluation guidance](https://developers.openai.com/api/docs/guides/evaluation-best-practices).
No external model judge was used.

## Evidence reuse

No Go application source, native design, frozen collector, archived answer or
reviewed example was modified. No compilation or native regeneration was needed
for this checkpoint. The existing journal auditors and source-specific contract
exports run offline; all previously generated native files are compared again
without invoking KiCad.

The evaluator is a separate append-only record at:

```text
.cache/board-family-v2/owned-review-05-01/
  freeze.json
  review-template.json
  review.json
  results.json
  verification.json
  scorer-check.process.json
  scorer-check.stdout.log
  scorer-check.stderr.log
```

Its freeze binds 15 evaluator, test, workflow and shared-source files, plus the
unchanged corpus, 14 per-request contracts, selections, original manifest and
original result. It does not rewrite or replace the rehearsal manifest.

- Evaluator freeze: `8202be378f17c8b3a497ecf67f209c043cc1e91f0531e8f0df9e14555e4f8de4`
- Completed review: `346e010d35854427be672459c8fe0c827602fc8e5f952097a8e7c5b0db3ab388`
- Scored results: `abc6f10df0248b13f00dd9f915a448ab040190ea1c226f13ee9ac225d45bb0db`

The unchanged rehearsal also passed its full read-only package authentication.
Checksums and replay establish local consistency, not independent provider
attestation. Source, evidence or evaluator changes require a separately bound
review; old results are never silently recomputed under new inputs.

Read-only review verification from this worktree:

```sh
node specs/board-family-v2/owned-evidence-05/review-runner.mjs --check \
  .cache/board-family-v2/owned-review-05-01 \
  .cache/board-family-v2/owned-review-05-01/review.json
```

## Next gates

Finish exact-source code review and remote CI, then prepare a final live
collector that reuses unchanged qualification and these scoring invariants.
The current collector and review extension are offline-only. Their status
cannot be changed into spending authority.

Fresh specific user budget approval is still required before any actual model
evaluation. No API requests, external model reviews, firewall changes,
fabrication or bench actions occurred. The primary checkout and PR remain
unchanged; the new scoring CI workflow is local and has not run remotely.
