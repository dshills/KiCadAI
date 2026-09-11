# Recovery baseline 1: terminal connection failure

Date: 2026-09-11. Status: **incomplete; no design-capability score**.

The user approved one separately recorded recovery baseline. It ran with the
existing OpenAI key and the unchanged frozen corpus, model, evaluator,
acceptance criteria and cumulative limits. The clean worker was built from
`4341df799c25c811cd05e80f5d860c009ea50585`; production still matches merged V23.
Authorization is retained in `RECOVERY-1.md` and
`recovery-1-authorization.json`, including a byte-identical copy in the raw tree.

The first input, P01, again failed before an HTTP response:
`dial tcp 172.66.0.243:443: i/o timeout`. The generation-call duration was
31.023405041 seconds. The supervisor stopped the phase at
2026-09-11T09:43:28.637Z. Its successful process exit means the terminal record
was written, **not** that the case or evaluation passed; the case worker exited 1.

## Results and remaining budget

- P01: one recovery connection attempt; no HTTP/model response, proposal,
  compiled requirement, electrical measurement, schematic or board.
- P02–P08, N01–N04, C01–C02 and W01–W02: all 15 inputs `not_run` after the
  terminal provider timeout. Refusal, clarification and paraphrase behavior
  remain untested. No correction, final phase or paired replay occurred.
- The eight-positive denominator is unchanged. Complete baseline success rate
  is unavailable, not a measured 0/8 design-engine result. The six-board and
  two-uplift targets cannot be assessed. No production fixes are selected.
- Recovery worker wall time: 31.108 s; campaign including snapshot: 41.731 s.
  Peak sampled process-tree RSS: 30,294,016 bytes at 1 s intervals, with zero
  sampling errors and no resource-cap kill.
- Recovery request 002 retained USD 0.456096 of estimated/reserved spend because
  usage was unavailable. The cumulative journal also contains the original
  request 001, copied byte-for-byte, not counted as a new recovery request.
  Across both interrupted campaigns there are two reservations totaling
  USD 0.912192. Actual billing and token usage are unknown, not asserted zero.
- Original caps remain 36 baseline / 72 total requests and USD 50 estimated
  spend. Remaining headroom is 34 baseline / 70 total requests and
  USD 49.087808. Headroom does not itself authorize another recovery attempt.
- There were no manual circuit/layout repairs. Human-active time and separate
  review-active time were not measured. Existing offline regression/native
  controls remain separate from these unexecuted corpus design gates.

## Evidence authentication

`recovery-1-evidence-inventory.json` binds all 28 retained files (61,902,602
bytes) to SHA-256 and byte counts. P01's internal inventory matches its twelve
payload files. The 28,008-byte request matches receipt 002 and is byte-identical
to the original failed request. Its model, strict schema, output cap, streaming,
`store=false` and `background=false` settings were verified.

All 18 frozen files and all 25 original evidence files authenticated again.
The original tree, receipts and reports remain unchanged. An exact-secret scan
of every recovery file, including the decompressed library index, found no
environment API-key value. No authorization headers were captured.

`recovery-1-audits.json` accounts for all 16 inputs and all **108** frozen
acceptance clauses, each with explicit `not_run` disposition and hash-bound
evidence. The original interrupted audit used six nonpositive category labels
and collapsed each two-clause clarification into one label, producing 106 rows.
This recovery audit expands the full frozen text and retains those prior labels
for traceability. This corrects reporting granularity, not acceptance criteria
or any outcome; the original report remains an immutable historical record.

The read-only `authenticate-recovery-1.mjs` verifies this terminal state and
emits the inventory and audit to stdout. It makes no provider calls and writes
no files. Its only credential access is the local exact-secret scan. These are
disclosed self-review and local hash checks, not external certification.

The raw tree remains at
`/tmp/kicadai-practical-sensor-controller-public-1-recovery-1`.
`recovery-1-archive.json` records the separate ignored project-local archive:
61,962,752 bytes; SHA-256
`1ee06d33868fe77e3599b425d62c00a827bb24ee5d405be3ce01af3091f114fd`.
All 28 archived payloads verified exactly, with no duplicate, extra or missing
payloads. The archive is not uploaded or Git-tracked, so this is local retention,
not a remote reproducibility guarantee.

## Credential-free network diagnosis

`recovery-1-network-diagnostics.json` separates observations from inference.
All TCP probes were elevated, with the API key removed, and sent no HTTP/API
requests. Native `nc`, Node's direct TCP socket and `nc` spawned by Node could
connect to the same IPv4 address and port. Go 1.26.8's normal TCP dial timed out;
a raw blocking Go socket hit its watchdog. Go DNS resolution succeeded and no
proxy environment variables were present.

LuLu's outbound extension and Tailscale were active. The Apple application
firewall was disabled, which does not disable LuLu. Initially there was no
saved rule matching the recovery worker. A later read-only LuLu view showed an
Allow rule for `/tmp/kicadai-practical-board-eval-recovery-1`, created at
2026-09-11T10:17:50.797Z, after the campaign stopped. Codex did not add or modify
any firewall rule or setting; it only viewed and filtered existing rules.

This pattern is consistent with the earlier Go connections awaiting an
application-specific firewall decision. It is not a captured causal proof:
the narrow log query returned no matching deny/pause event, and the exact
recovery worker has not been retested after the Allow rule appeared. It does
not establish an invalid key, exhausted quota, unavailable model or provider
outage. Replacing the key is not justified by these observations.

## Next authority boundary

Recovery 1 is terminal under its recorded stopping rule. No automatic retry,
transport substitution, firewall bypass or model/budget change is authorized.
If the user approves another separately recorded recovery now that LuLu shows
the worker allowed, preserve both interrupted cohorts and carry forward the
two canonical journal reservations exactly once. All frozen criteria and
original cumulative limits must remain intact.

The overall milestone, final evaluation, baseline-driven production fixes and
reviewed PR are still incomplete. No branch was pushed, PR opened, merge made,
release published, fabrication approved or support boundary expanded by this
recovery record.
