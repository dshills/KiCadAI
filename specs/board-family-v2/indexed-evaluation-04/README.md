# Request revision 04: complete evaluation, acceptance not met

September 15, 2026. **9/14 complete passes, 11/14 correct application outcomes,
and 3/5 useful native board bundles.** All 14 first attempts finished in one
59.48-second batch; none were retried or omitted. The natural-language workflow
is still not ready for unattended use. PR #14 remains draft and the overall
two-family reliability goal is not complete.

The experiment evaluated source `7130f4db199d72468409d2d8d3d6a2f365fb9257`,
request revision `indexed-request-04`, experimental indexed-v3, and the unchanged
`gpt-4.1-mini-2025-04-14` model. The default typed-v2 path was not changed.
The unchanged benchmark identity is `board-family-v2-indexed-final-03`; this
fresh batch is **indexed-final-04**, with its own actual approval and ledger.

## Results

| Measure | Verified result | Acceptance requirement |
| --- | --- | --- |
| Raw extraction: all facts, source/quantity scope and completeness | 9/14 | 14/14 |
| Application decision and required native bundle | 11/14 | 14/14 |
| Both raw and application pass | 9/14 | 14/14 |
| Useful native bundles | 3/5 | 5/5 |
| Targeted clarification cases | 3/3 complete | 3/3 |
| Unsupported cases | 3/6 complete; 5/6 application | 6/6 complete |
| All-five-useful median / maximum | 6.439 / 8.282 seconds | Below 60 / 120 seconds |
| Requests / retries / unattempted | 14 / 0 / 0 | At most 14; no retries |
| Verified token usage | 43,291 input + 1,697 output | Frozen accounting limits |
| Ledger-estimated cost | USD 0.020037 | At most USD 1.00 |

The timing denominator includes the two failed useful requests. It does **not**
mean that five successful boards were produced in those times. The three actual
successful board commands took 8.282, 6.439 and 6.744 seconds. Their 39 + 42 + 42
native/export comparisons passed, with no manual output repair. Native checks
include the existing electrical, ERC, strict DRC/parity, round-trip, preview and
manufacturing gates. They do not prove fabricated or assembled hardware works.

The cost is calculated from verified response usage at the frozen rate, not an
account invoice. Fourteen historical USD 0.05 reservations total USD 0.70, but
all entries are completed; those reservations are not extra settled charges.
The low estimated cost does not authorize request 15 or a recovery run.

## Case-by-case review

| Case | Raw | Application | Finding |
| --- | --- | --- | --- |
| useful-01 | Fail | Fail | Invented wireless requirement causes false refusal of a wired pressure monitor. |
| useful-02 | Pass | Pass | BMP280 fast, 2.2k / 400 kHz / 100 pF; both exclusions retained; native bundle verified. |
| useful-03 | Fail | Fail | Power/battery exclusions cite a pull-up quantity from an uncited clause; extraction rejected. |
| useful-04 | Pass | Pass | Temperature/humidity selects SHT31 standard, 70 pF; pressure exclusion retained; bundle verified. |
| useful-05 | Pass | Pass | SHT31 fast, 100 pF; explicit heater prohibition retained; bundle verified. |
| choice-01 | Pass | Pass | Asks which measurement is needed, without inventing a family. |
| choice-02 | Pass | Pass | Both family alternatives remain uncertain; asks the relevant choice. |
| choice-03 | Pass | Pass | Selected SHT31 retained; asks about the explicitly undecided profile. |
| refuse-01 | Pass | Pass | Correctly refuses a mandatory combined-sensor board without substitution. |
| refuse-02 | Pass | Pass | Correctly refuses SHT31 standard at the fixed out-of-range 100 pF. |
| refuse-03 | Fail | Pass | Requested low_current becomes forbidden; 10k incompatibility still yields a correct refusal. |
| refuse-04 | Pass | Pass | Preserves startup-off and later-on heater scope; refuses the later demand. |
| refuse-05 | Fail | Fail | Invalid wireless quantity citation and invented GPIO-load prohibition; generic error is not the required refusal. |
| refuse-06 | Fail | Pass | Correct accuracy refusal despite five invented prohibitions and tolerance misread as ambient bounds. |

See the full [source-bound review](review.json), [recomputed scores](results.json),
[usage/timing and per-response receipts](metrics.json), and [unchanged raw batch](batch).
No scored response, source clause, gold requirement, output or ledger was repaired.

## Evidence authentication and validation review

**Assessment: share with caveats; release acceptance failed.** The original
frozen collector reached observed terminal exit 0, recorded all 14 child
terminal states, and ran a successful credential-free journal audit for every
response. All 14 HTTP responses were 200 with observed EOF and no recorded
transport, truncation, read or close error. Response IDs are unique; each case
has exactly one completed ledger entry. Two model extractions were rejected,
not transport failures or unattempted cases.

After collection, the frozen scorer authenticated the manifest, qualification,
approval, exact request and response bytes, reconstructed request/source tables,
raw extraction, derived decision, ledger snapshots, native comparisons and full
415-file batch inventory. The published archive is byte-identical to that batch.
Every emitted fact and every required gold fact received explicit implementing-
agent review. Scores were recomputed under the unchanged full 14-case denominator.
That review covers 64 emitted facts and 47 required gold facts. All 12 portable
archive tests pass, including changed review/results/approval/qualification,
ledger bytes, raw response and native output, plus missing/extra/symlink evidence.
The ledger-whitespace mutation initially exposed a missing byte-level pin in the
new publication checker; the checker was corrected and all tests rerun. This
post-run checker correction did not alter the frozen evaluator or any evidence.
The evidence-quality and validation checks keep format validity, semantic
correctness, safe withholding and useful board delivery separate.

Reproduce complete local runtime authentication without credentials:

```sh
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY \
  -u GOOGLE_API_KEY -u KICADAI_LIVE_PROVIDER_TESTS \
  node specs/board-family-v2/indexed-evaluation-04/record-results-04.mjs --check
```

This requires the original pinned macOS runtime/cache, source inputs and private
local journals. The portable archive check below verifies preserved bytes,
recorded Git source identity and score arithmetic, but does not run KiCad or
the original macOS auditor:

```sh
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY \
  -u GOOGLE_API_KEY -u KICADAI_LIVE_PROVIDER_TESTS \
  node specs/board-family-v2/indexed-evaluation-04/authenticate-archive-04.mjs
```

The [readiness review](readiness-review.md) is preserved as pre-run history,
including its then-pending approval statement. Actual subsequent authority is
recorded separately in [approval.json](approval.json). The [execution observation](execution-observation.json)
records the terminal tool result and exact LuLu hostname/port readback. The
approved evaluator-only rule was saved; no other firewall rule was changed.

## Release blockers and limits

1. Five raw extractions were unfaithful or structurally invalid. Source IDs,
   request-specific sensor enums and numeric IDs did not eliminate semantic
   errors, polarity errors or incorrect quantity roles.
2. Only three of five supported requests produced complete native bundles.
   Safe refusals/error withholding do not satisfy practical generation success.
3. Correct final refusals in refuse-03 and refuse-06 mask incorrect raw facts;
   application-only accuracy would overstate end-to-end reliability.

The 14 cases are known regression prompts seen during development, not an unseen
holdout, production sample or statistical reliability estimate. Reviewer:
implementing Codex agent, not an independent engineer. Hashes and saved-byte
replay establish local consistency, not provider-signed attestation or protection
against an author able to rewrite all evidence. The evaluated source's main CI
run [34960209267](https://github.com/dshills/KiCadAI/actions/runs/34960209267)
and all three evaluation workflows were green before launch. Evidence-only
publication does not change the evaluated runtime.

This run demonstrates bounded software-qualified family construction, not
arbitrary schematic/PCB synthesis, fabrication readiness or measured physical
performance. Existing heater/radio/power/environment restrictions and separate
assembly, firmware, calibration and bench obligations remain in force.

No additional API request is authorized. The earlier stopped indexed-final-03
batch still has one request, 13 unattempted cases and its unchanged unknown-cost
reservation; this fresh successful accounting does not settle or rehabilitate it.
Do not promote the draft PR, switch the default or claim the overall goal met.
The next development decision should address semantic interpretation (including
quantity roles and negation), not merely add another retry or loosen acceptance.
