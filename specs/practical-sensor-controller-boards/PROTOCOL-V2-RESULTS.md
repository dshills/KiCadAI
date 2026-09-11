# Practical-board baseline: no complete designs demonstrated

## Technical summary

The approved protocol-v2 baseline completed all 16 frozen cases on September 11,
2026. It produced **0/8 complete primary boards**, **1/4 valid refusal workflows**,
**0/2 complete clarification workflows** and **0/2 complete paraphrases**.
No compiler-ready design, circuit, schematic, board or downstream replay was
produced. The practical-board capability-growth milestone is **not achieved**.

The decision is to [stop before production implementation and request a new
scope](SCOPE-DECISION.md). The six non-reserved positive cases fail at requirement
compilation. That evidence does not support a credible promise of two complete
fresh-plus-same-input uplifts under at most three generic changes. There is no
final or paired campaign; their comparison metrics are unavailable, not zero
values suggesting a completed growth evaluation.

## What was measured

The primary denominator is exactly P01–P08. N01–N04 are separate refusal criteria;
C01–C02 test clarification; W01/W02 paraphrase P01/P05 and are not independent
boards. P07/P08 are public-frozen reserved cases, not blind tests, and their
individual details did not select implementation changes.

This is the merged-V23 production baseline
`87c411b7a13bdeb0efc6ff36b35e9a69c4e2706d`, with isolated experimental evaluator
and documentation additions. The clean evaluated build is
`91cce8324a5eaa4d79008e4580dc91c92c0a2d37`. The sole approved v2 behavior change
was a five-minute evaluator HTTP deadline; model, attempts, acceptance and other
resource limits were unchanged. The run lasted from 12:18:11.737 to 13:11:05.130 UTC
(07:18:11.737–08:11:05.130 America/Chicago).

The eight gates remain requirement interpretation, component/model qualification,
electrical analyses, schematic/readability, placement, routing/connectivity,
native validation/writer and deterministic replay. Provider-completed JSON is
not an accepted requirement, and a zero worker exit code is not a design pass.

## Per-case outcomes and resource observations

Every frozen case is listed in original order. “Attempts” counts the initial
provider invocation plus its permitted diagnostic correction, not extra designs.
No clarification-answer turn ran. RSS is sampled process-tree memory, in MiB;
case evidence excludes shared snapshot/journal files. The JSON sources retain
exact byte counts. All physical/electrical stages remain not run.

| Case | Audited outcome | Attempts | Worker seconds | Peak sampled RSS MiB | Case evidence MiB |
|---|---|---:|---:|---:|---:|
| [P01](protocol-v2-audits/P01.audit.json) | Fail — compilation | 2 | 299.411 | 42.02 | 3.75 |
| [P02](protocol-v2-audits/P02.audit.json) | Fail — compilation | 2 | 275.300 | 42.02 | 3.77 |
| [P03](protocol-v2-audits/P03.audit.json) | Fail — compilation | 2 | 266.965 | 40.11 | 3.62 |
| [P04](protocol-v2-audits/P04.audit.json) | Fail — compilation | 2 | 340.561 | 42.69 | 4.21 |
| [P05](protocol-v2-audits/P05.audit.json) | Fail — compilation | 2 | 295.169 | 39.89 | 3.88 |
| [P06](protocol-v2-audits/P06.audit.json) | Fail — compilation | 2 | 362.678 | 39.25 | 3.81 |
| [P07](protocol-v2-audits/P07.audit.json) | Fail — response-size limit | 1 | 177.772 | 33.69 | 2.12 |
| [P08](protocol-v2-audits/P08.audit.json) | Fail — compilation | 2 | 310.460 | 38.75 | 4.02 |
| [N01](protocol-v2-audits/N01.audit.json) | Fail — refusal record invalid | 2 | 42.657 | 33.34 | 0.45 |
| [N02](protocol-v2-audits/N02.audit.json) | Pass — corrected refusal | 2 | 32.567 | 32.66 | 0.38 |
| [N03](protocol-v2-audits/N03.audit.json) | Fail — wrong/invalid workflow | 2 | 103.544 | 34.64 | 0.77 |
| [N04](protocol-v2-audits/N04.audit.json) | Fail — refusal record invalid | 2 | 53.888 | 33.55 | 0.52 |
| [C01](protocol-v2-audits/C01.audit.json) | Fail — incomplete correction | 2 | 91.874 | 34.63 | 0.79 |
| [C02](protocol-v2-audits/C02.audit.json) | Fail — clarification bindings | 2 | 119.583 | 36.92 | 1.20 |
| [W01](protocol-v2-audits/W01.audit.json) | Fail — response-size limit | 1 | 223.729 | 33.50 | 2.18 |
| [W02](protocol-v2-audits/W02.audit.json) | Fail — response-size limit | 1 | 166.064 | 34.36 | 2.12 |

Source: [machine-readable results](protocol-v2-results.json),
[full authentication](protocol-v2-authentication.json) and linked per-case audits.
The table is exact case lookup, not a statistical generalization or a ranking
of design difficulty.

### Compilation failures are not electrical failures

P01–P06 and P08 never passed requirement compilation. The accepted JSON shape
still contains invalid version-specific fields, domain/source values, endpoint
bindings, units/relations, coverage or uncertainty records. Seven downstream
gates are explicitly not run; no measured electrical quantity is inferred.
The retained audits preserve every clause, including temperature, startup/fault,
thermal, board-size, routing, readability and replay requirements.

### Refusal prose and valid refusal workflows differ

N02 correctly rejects incompatible unsplit-rail bounds after one correction,
without changing the bounds or generating a project. N01/N04 explain the expected
mains/RF limitations, but invalid capability identifiers prevent a valid refusal
workflow. N03 asks to change the protection/guarantee choice instead of producing
the predetermined unsupported-high-energy refusal, and its structured bindings
are invalid. None emitted an accepted unsafe executable design.

### Clarification content alone does not complete the workflow

C01's first proposal asks for the required supply facts, but compilation finds
45 issues. Its correction ends without a terminal response. C02 asks the correct
load facts and retains `requirement=null`; its narrow question-content clause
passes, but 17 structured-binding/coverage issues prevent `needs_clarification`.
Neither establishes a valid bound-answer workflow, so neither counts as a
complete clarification success. No fixed answer was injected manually.

### Complete wire responses can still be rejected by the client

P07, W01 and W02 have complete retained HTTP-200 streams that exceed the production
client's 2,097,152-byte response limit. They fail before an accepted proposal or
client usage record exists. Their raw output is not repaired, compiled, replayed
or promoted by this audit. W01/W02 reproduce a different first failure from their
primary prompts, but neither wording produced a complete design.

C01's correction instead lacks a terminal response. The frozen supervisor treats
`ai_output_json_invalid` and `ai_provider_incomplete` as case failures; its
explicit campaign-stop codes are authentication, rate limit, transport, timeout
and configuration errors. It continued according to those unchanged rules.
No request was added after a failed case.

## Budget and assistance accounting

The phase took **3,173.393 seconds (52m 53s)**. All 16 workers stayed within the
sampled supervisor wall/RSS limits; no sampling errors or supervisor cap kills
were recorded. Peak sampled case RSS was **44,761,088 bytes (42.69 MiB)**.
The retained tree contains **472 files / 101,237,999 bytes**, below the 10 GiB cap.
These observations do not erase provider/client failures or establish exact
instantaneous memory peaks.

There were **29 new requests: 16 initial and 13 permitted corrections**, plus
three carried reservations from separately preserved interrupted v1 cohorts.
Cumulative reservations are **32/36 baseline and 32/72 total**. The frozen journal
estimate is **USD 6.646416**, comprising USD 5.278128 new and USD 1.368288 carried;
**USD 43.353584** remains under the original USD 50 ceiling. No further spend is
authorized merely because budget remains.

Usage remains unreconciled in the original journal for requests
001, 002, 003, 016, 028, 031 and 032. Complete rejected streams expose supplemental
usage for 016/031/032, but the conservative receipts are not rewritten.
Actual account billing is unknown; this is not an invoice or an account-wide
spending guarantee.

Manual component/net/placement/routing repairs: **zero**. Fixed clarification
answers used: **zero**. Assistance included corpus/feasibility authoring,
credential reuse consent, separately recorded recovery/protocol approvals,
two approvals for the restricted LuLu hostname-and-port rule, and this read-only
self-review. Human-active minutes are unavailable. Per-case review intervals
include shared/overlapping work and must not be summed as exclusive labor.
Build, preflight, regression and publication costs are separate from case costs.

## Evidence authentication and its limits

[The outer inventory](protocol-v2-evidence-inventory.json) binds every case
inventory, resource log, request/usage receipt, snapshot, authorization and raw
response. All 16 separate audits resolve **108 exact acceptance clauses** and
their SHA-256/JSON-location bindings. Reverification compares against this pinned
outer inventory, so replacing a case inventory cannot hide changed raw files.

The three earlier interrupted v1 cohorts remain separate and byte-authenticated.
Their receipts were carried exactly once; partial outputs and copied receipts
are not additional designs or successful baseline inputs. Run their original
authentication scripts from the preserved v1 checkout as documented in
[PROTOCOL-V2.md](PROTOCOL-V2.md).

The exact existing credential and key-shaped strings were absent from all
retained files, including the decompressed native-library index. The large-index
scan uses bounded overlapping chunks to avoid JavaScript's single-string limit.
No authorization headers or plaintext credentials are published.

Opaque-field redactions occur in P04 (1), P05 (1), P06 (1), P07 (1), N03 (2), C01 (1), W01 (2). Every redaction in v2
response streams is confined to encrypted-content fields, not design text.
The recorder's field named `retained_bytes` actually reports pre-redaction size;
the authentication record separately reports actual retained size and exact
redacted JSON locations. Stream sequence, final output text and retained intent
are cross-checked where the client accepted output. This authenticates retained
redacted bytes, not unavailable original ciphertext or external timestamps.

[The local archive](protocol-v2-archive.json) is 102,119,424 bytes with SHA-256
`92982ba1bfe6bb8e067981e7777b5870005785f0541683947632049fbf51da54`. All 472 extracted payloads match the original tree.
It is retained in the repository's ignored `.cache` directory, not uploaded with
the PR. **Remote reviewers need the archive to rerun full raw-evidence checks.**
The PR contains source, results, audits, inventories and reproduction instructions;
local retention is not remote publication or a guarantee against cache cleanup.

There are no new native projects or renders to deliver because generation never
reached those stages. Historical fixture projects/previews are not substituted
for missing experimental evidence. This is single-agent self-review and local
hash authentication, not independent certification or manufactured-board testing.

## Verification and interpretation

The prior bounded repository suite passed all 162 listed packages (148 with tests,
14 without), lint reported zero issues, and the existing native reference control
passed two clean downstream runs. These are separately retained controls in
[OFFLINE_REVIEW.md](OFFLINE_REVIEW.md), not new corpus successes. The v2 timeout
change also passed focused race/protocol checks in
[protocol-v2-verification.json](protocol-v2-verification.json).

Publication verification is recorded in
[PROTOCOL-V2-REVIEW.md](PROTOCOL-V2-REVIEW.md). The result is ready to share as a
**negative baseline with explicit limitations**, not as a completed board-growth
demonstration. No production behavior, stable-support boundary, historical
V18–V23 evidence, frozen acceptance file, model or execution budget was changed
during this run.

## Reproduce the review without another provider request

Run from the repository root with the original raw root/archive and previously
authorized credential available only for the local exact-secret scan:

```sh
node --test specs/practical-sensor-controller-boards/publication-v2/*.test.mjs
node specs/practical-sensor-controller-boards/publication-v2/verify-publication.mjs --verify-local-archive
node specs/practical-sensor-controller-boards/publication-v2/verify-audits.mjs
node specs/practical-sensor-controller-boards/publication-v2/authenticate.mjs
```

These commands are read-only and make no provider calls. The last command refuses
a live campaign and compares all retained bytes to the published outer inventory.
Do not rerun `run-campaign-v2.mjs` to improve this result.

## Next decision

Approve or revise the separate AI-facing integration scope in
[SCOPE-DECISION.md](SCOPE-DECISION.md) before any production implementation or
new live protocol. Reliable requirement translation is the observed immediate
gap; further electrical/physical capability needs remain unmeasured. No merge,
release, fabrication approval or stable support expansion is requested.
