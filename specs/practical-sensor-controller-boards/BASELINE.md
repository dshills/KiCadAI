# Baseline interrupted before a provider response

Date: 2026-09-10. Status: **incomplete; no capability score**.

The acceptance set and evaluator were sealed in commit `eb78476c` before any
live request. The production tree still matches merged V23
`87c411b7a13bdeb0efc6ff36b35e9a69c4e2706d`. No production capability was changed.

The first case, P01, attempted a TCP connection to the OpenAI endpoint and
failed after 31.019 seconds with `dial tcp 172.66.0.243:443: i/o timeout`.
There was no HTTP response, model response, proposal or compiled requirement.
The run used elevated execution, so it was not merely an un-escalated sandbox
retry awaiting approval. This evidence identifies a connection failure; it
does **not** establish invalid credentials, exhausted credits, model access,
or an OpenAI service outage. The existing key has not been authenticated by
this test and does not need replacement based on this evidence.

The frozen supervisor stopped on the terminal timeout. No automatic retry,
model change, budget change, replacement case, final run or paired run occurred.

## Observed outcomes

| Inputs | Outcome | Electrical/native result |
|---|---|---|
| P01 | One connection attempt; provider timeout | All eight acceptance gates `not_run` |
| P02–P08 | `not_run` after campaign stop | No design evidence |
| N01–N04 | `not_run` after campaign stop | Refusal behavior not tested |
| C01–C02 | `not_run` after campaign stop | Clarification behavior not tested |
| W01–W02 | `not_run` after campaign stop | Paraphrase consistency not tested |

The primary denominator remains eight. There are zero observed complete board
candidates, but a complete baseline success rate is **unavailable**, not a
measured 0/8 design-engine result. The two-uplift and six-board targets cannot
be assessed from this interrupted campaign. Existing V23 results are unchanged.

## Resources and assistance

- P01 worker wall time: 31.104 s; recorded generation-call time: 31.019 s.
- Peak sampled P01 process-tree RSS: 30,785,536 bytes; 1 s sampling interval;
  zero monitor errors and no worker/campaign resource-cap kill.
- Campaign wall time including the environment snapshot: 43.640 s.
- Complete retained tree: 25 files, 61,899,789 bytes, including the full
  compressed installed-library index and every failed-attempt artifact.
- One append-only request reservation; zero correction/clarification turns.
  USD 0.456096 remains conservatively reserved because usage was unavailable.
  This is **not** an observed charge; billed amount and token usage are unknown.
- One user credential-reuse approval; zero manual circuit/layout repairs.
  Human-active minutes and separately timed acceptance-review minutes were not
  measured. No passing-design claim depends on those unavailable quantities.

## Authentication and limitations

`baseline-evidence-inventory.json` binds every retained file to byte count and
SHA-256, including the campaign start/end, worker inventory, request journal,
resource sample summary and full environment snapshot. P01's internal inventory
also matches its twelve payload files exactly. The recorded 28,008-byte request
matches its append-only receipt and uses the pinned model, strict JSON schema,
streaming, output cap, `store=false`, and `background=false`.

The worker verified the freeze; the supervisor verified a clean current-source
build, unchanged production baseline, native tool identities, library content,
catalog/model/capability identities and the fixed closed-loop policy. Two
preparation snapshots also matched after removing only collection time from
the library identity. The raw collection timestamps remain retained.

An exact-secret scan of all retained files, including the decompressed library
index, found no environment API-key value. Authorization headers are never
captured. This is a disclosed self-review and local hash authentication, not
third-party certification. No actual schematic, board or electrical result was
produced for the new corpus. Native plumbing controls remain separate and are
not counted as corpus successes.

`baseline-audits.json` accounts for all 16 inputs and all 106 numbered
case/common clauses with explicit missing evidence and `not_run` gates. Each
started-case source, resource file and campaign outcome is hash-bound. There
are no inferred electrical observations, bounds or model results.

The raw evidence remains at
`/tmp/kicadai-practical-sensor-controller-public-1`; it has not been uploaded or
published through a PR. These local paths are not a remote reproducibility
guarantee, and the retained tree must not be deleted while deciding recovery.

## Baseline-driven scope decision

**No production fixes are selected.** There is no compiler, electrical or
physical failure evidence on which to justify two complete reusable uplifts.
Changing hardware logic or relaxing acceptance would be unsupported.

Recovery first needs working outbound connectivity to `api.openai.com:443`.
Because this live phase is already terminal under the frozen no-retry rule,
a separately recorded recovery baseline requires a new user scope decision.
The original freeze and interrupted evidence must remain immutable; recovery
must explicitly state its relationship to them and its remaining request/spend
budget. Do not silently reuse or overwrite this campaign directory.

The overall milestone, final evaluation, reviewed PR, and any capability-growth
claim remain incomplete. No merge, release, fabrication approval or support
boundary expansion is authorized by this report.
