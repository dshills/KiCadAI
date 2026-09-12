# Interface v1: evaluation complete, gate not met

The approved campaign completed on September 11, 2026. Source-bound local review
found faithful outcomes in **1/4 ready cases, 2/2 refusals and 1/2 complete
clarification workflows**. This is not a passing interface gate and does not
demonstrate a generated schematic or board.

The sealed replay command failed on an omitted empty issue list. A separately
recorded, post-run offline checker matched all 15 compiler results and rejected
all eight tampered answer bindings, using unchanged production compiler inputs.
This supplementary check does **not** make the sealed replay pass. No live
requests were repeated.

## Complete results

| Case | First response | Final reviewed outcome | Clauses | POSTs | Estimated USD |
|---|---|---|---:|---:|---:|
| I01 | invalid | Faithful after correction | 8/8 | 2 | 0.293330 |
| I02 | invalid | Failed: coverage reference | 8/8 | 2 | 0.299980 |
| I03 | invalid | Failed: coverage/endpoints | 7/8 | 2 | 0.331220 |
| I04 | invalid | Failed: external output source | 6/8 | 2 | 0.288995 |
| R01 | unsupported | Valid refusal | 5/5 | 1 | 0.101980 |
| R02 | unsupported | Valid refusal | 6/6 | 1 | 0.097525 |
| C01 | needs_clarification | Faithful clarification | 7/7 | 2 | 0.246300 |
| C02 | needs_clarification | Failed: follow-up contract | 5/7 | 3 | 0.405860 |

“Faithful” means the appropriate compiler status plus every frozen case clause,
supported by raw evidence authentication and supplementary offline checks.
It is an observed interface outcome, not sealed-replay certification or board
acceptance. I02 preserves all eight semantic clauses in a rejected proposal;
correct values without legal compiled state do not pass.

First-attempt faithful outcomes: ready **0/4**, refusal **2/2**, complete
clarification **1/2**. Both initial clarification questions were valid (**2/2**),
which is different from successfully completing their follow-ups. There were
no exclusions, unrun cases, provider failures or extra retries.

## Findings and review

- **I01:** a simple external analog low-pass requirement became faithful after
  the permitted compiler correction.
- **I02:** both attempts referenced the project name as a requirement entity in
  coverage. The compiler rejected that reference; the numeric and functional
  content of the selected proposal was otherwise faithful.
- **I03:** the correction resolved two invalid whole-circuit observation IDs but
  left five source-coverage reference errors. The local audit additionally
  does not infer an input-to-ADC measurement endpoint from generic whole-circuit
  observations. This stricter endpoint judgment is separate from the decisive
  compiler failure; accepting that clause would not change the case outcome.
- **I04:** the compiler accepted the correction, but the regulated output supply
  is labeled `source: "external"` instead of derived from a declared supply
  signal. Frozen clauses 2 and 6 fail. This is a **compiler-accepted semantic
  counterexample**, not proof of an unsafe fabricated board. The required
  zero-unsafe-accepted gate cannot be claimed met.
- **R01/R02:** the first responses faithfully refused the unsupported radiation
  calibration/certification and stateful event-control guarantees.
- **C01:** the minimal cutoff/tolerance question received its exact prewritten,
  hash-bound answer. The first follow-up was ready and preserved all seven
  clauses; supplementary validation checked the bindings and tamper controls.
- **C02:** the initial load-current question was correct. After its answer and
  one correction, the proposal still duplicated a constraint identity, used an
  unresolved operating-condition target, and lost material ground-source
  coverage. Its regulated supply was also labeled external, but unlike I04
  this proposal was not accepted.

All **57 frozen clauses** have exact raw-file SHA-256/JSON-pointer bindings in
[audits.json](audits.json). **52 pass and 5 fail**; clause totals never override
case-level failure. Review was performed locally by Codex, not by an independent
reviewer or another provider. No Gemini review was requested in this scope.

## Evidence, budget and limitations

[authentication.json](authentication.json) verifies **304 files / 16,140,341 bytes**,
all **110 frozen input files**, exact request/schema/context bytes, response
terminal streams, provider usage receipts and the outer inventory. No existing
API credential or key-shaped string was present in retained evidence.

The run made **15 of 20 allowed POSTs** and used **USD 2.065190** in conservative
estimated/reserved cost against **USD 25**. All 15 records had complete accepted
usage; actual invoiced cost remains unknown. Frozen pricing is in
[pricing.json](../live-v1/pricing.json). This is not an account-wide spend report.

Elapsed live time was **803.041 seconds (13m 23s)**. There were **1,622 resource
samples**, no sampling errors, peak sampled process-tree RSS **59,506,688 bytes**
and sampled evidence size **16,063,565 bytes**. Sampled limits were met; sampling
does not prove instantaneous peak memory. Final retained bytes exceed the last
sample because terminal metadata/inventory are written afterward.

SHA-256 checks establish local retained-byte and source consistency, not
independent provider-signed attestation. This public-frozen eight-case corpus is
not blind, randomized or representative of all boards. No causal improvement
estimate is justified against the different earlier practical-board corpus.

### Frozen replay defect

[frozen-replay-failure.json](frozen-replay-failure.json) retains the command,
exit code 1 and `unexpected end of JSON input`. The frozen
`canonicalCompilation` unconditionally unmarshals the `issues` member;
`behavioralintent.Result` uses `omitempty`, so a no-issue result has no member.

[replay-audit/main.go](replay-audit/main.go) is a separate offline-only audit,
not a replacement evaluator. It authenticates every frozen input and the
original sealed binary, strict-decodes retained proposals and calls the
unchanged production compiler. It preserves every typed value and all array
orders except sorting full diagnostic objects, preserving duplicates. Its only
comparison accommodation is handling an absent issue list without inventing it.
[supplementary-replay.json](supplementary-replay.json) records **15/15 matches**
and **8/8 rejected tampered bindings**, with no provider calls.

Post-run tests reproduce the sealed bug and exercise omitted issues, status
differences, full diagnostic fields, diagnostic ordering and duplicates.
The supplementary audit and production provider/behavioral-intent packages
passed race-enabled tests. All three frozen Node authenticator tests passed.
The report's read-only SQLite aggregation independently matches the Node-derived
request, cost, clause and outcome rows.

## Provenance and retention

- Reviewed source: `008491ddee707d739c86a083ddb35a093d38f927`.
- Sealed build source: `909c6a0290be26c7f13b96ea0c7cb81c60c58756`.
- Freeze SHA-256: `fafd5c8e381fc51e7633e5677b583ca6f041db9c3753151e0d51dc1109fef57a`.
- Evaluator SHA-256: `6d75691600d14771ae860480a08c77b441552debfa0ec608657ce986ae6a402f`.
- Raw inventory SHA-256: `fada814dd6f4c8e32dc5797ead08455bb86eb977283e411e026b4a5fbfb330d9`.

The terminal tree remains unchanged at
`/tmp/kicadai-ai-requirement-interface-v1`. Local durable copies of raw evidence
and both binaries are in
`.cache/ai-requirement-interface-evidence-v1-909c6a0/`, described and hashed by
[archives.json](archives.json). These archives are **local and git-ignored**;
a repository checkout alone is not the complete raw evidence package.
[prerun-approval.json](prerun-approval.json) records the existing-key approval,
restricted LuLu rule and no-probe preflight.

The earlier practical-board baseline was reauthenticated after this run:
472 files / 101,237,999 bytes, its local archive and all 108 clause audits
still verify. No historical journal, result, source freeze or support boundary
was changed.

## Reproduce offline checks

Run from the repository root. Authentication uses the existing key only for a
local exact-secret scan; it does not call the API.

```sh
node specs/ai-requirement-contract-integration/publication-live-v1/verify-publication.mjs
sqlite3 -json :memory: ".read specs/ai-requirement-contract-integration/publication-live-v1/report-query.sql"
```

The publication verifier reauthenticates raw evidence, checks all clause links,
recomputes results and report rows, scans publication text for the current
credential, verifies the publication inventory and extracts both local archives
into a new temporary verification directory for byte comparison. It never edits
the terminal tree or calls a provider.

To rerun supplementary compiler verification, use the retained audit binary with
all provider keys removed and a **new** output filename outside the raw tree.
Do not rerun the live evaluator. The original sealed replay failure remains
documented even if a future software revision fixes it.

## Decision and next step

**Do not advance to a practical-board campaign on these results.** The frozen gate
requires 4/4 ready, 2/2 refusals, 2/2 complete clarifications and no unsafe accepted
output, missing evidence or resource violations. Ready/clarification targets,
the accepted semantic counterexample and the sealed replay failure prevent
that conclusion.

Recommended next scope, requiring a new decision: an offline repair and
regression-test pass for power-source modeling, valid coverage identifiers,
participant measurement endpoints, operating-condition targets and empty-issue
replay. Then propose a separately frozen evaluation on new held-out cases.
Do not tune and rerun this exposed corpus as a new success claim.

No schematic/PCB synthesis, simulation, ERC/DRC, routing, manufacture or physical
validation was performed in this interface run. **Zero new complete boards**
were demonstrated; the original six-board/two-uplift goal remains unachieved.
No further API campaign, additional provider review, merge, release or
fabrication is authorized by this publication.
