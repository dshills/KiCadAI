# V8 Corpus Validator Freeze

The V8 validator is frozen before any aggregate corpus outcome is inspected.
It accepts only the checksum-bound six-author packet, exact V8 assignment and
authorship shapes, and the sanitized V1–V7 historical commitment set.

It validates exactly 36 behavior-only requirements and rejects:

- packet, assignment, authorship, path, timestamp, or byte-hash drift;
- malformed public requirements, legacy vocabulary, implementation language,
  unresolved references, or missing acceptance gates;
- raw, neutral-semantic, normalized-semantic, or normalized-behavior reuse;
- assignment, partition, reporting-domain, circuit-role, or safety imbalance;
- insufficient static/dynamic, multi-output, off-nominal, analysis, noise,
  thermal, stability, or tolerance diversity; and
- safety-impact/critical-evidence mismatches.

The historical commitments extend the previously frozen V1–V6 commitments
using only the 36 hashes already published in the V7 corpus manifest. The 18
sealed V7 requirements remain unopened. The validator imports no synthesis,
simulation, feasibility, classification, ranking, frontier, or outcome path.

`V8_VALIDATOR.sha256` binds the executable validator and all direct semantic
dependencies. `V8_VALIDATOR_FREEZE.json` records its digest. The contract
manifest binds those artifacts, this statement, and the executable freeze test.
