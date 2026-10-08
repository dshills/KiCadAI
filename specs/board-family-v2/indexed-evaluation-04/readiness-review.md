# Request revision 04 — readiness review

Date: 2026-09-15. Reviewer: implementing Codex agent; not an independent engineer,
provider attestation service or hardware-certification authority.

Decision: **ready for one newly approved final evaluation; overall goal not achieved**. No new
provider request, recovery run, ledger settlement or firewall change is authorized
by this review. The existing draft PR must not be promoted as live-accepted.

## Exact candidate

- Source commit: `7130f4db199d72468409d2d8d3d6a2f365fb9257`.
- Request revision: `indexed-request-04`; intent format remains experimental
  indexed-v3 and the default remains typed-v2.
- Model remains `gpt-4.1-mini-2025-04-14`.
- Runtime directory: `.cache/board-family-v2/indexed-runtime-03-02`.
- Manifest SHA-256:
  `d3996a549ae1995f933179f8f73e93e21e23cb559465830ec5291b9ecc8322e4`.
- Production executable SHA-256:
  `9a92ea9846f0ada034e4c2a23e1ae0f275a05dff40b39cb1f4de190b33c89072`.
- Qualification SHA-256:
  `75c8a748267ab780efc142a902b8dd83bcb55ce0606f3e6e4b455cc47943ef09`.
- Contract SHA-256:
  `4fbe0c4dbe31b70f634ce0dd9adcdea6777b7a9d3b39e39dba21c163788a9bf9`.

The qualification began at 10:51:42.648 UTC and finished at 10:52:25.484 UTC.
All 11 recorded child processes reached an observed terminal exit 0. The
manifest binds 2,536 files, including the four captured-response fixtures read
by the new tests. Independent readback of the manifest and qualification passed
after preparation. These are local byte-consistency checks, not signatures.

## Requirement-by-requirement status

| Goal requirement | Evidence inspected | Status |
| --- | --- | --- |
| Restore green CI | Current-head main CI run 34960209267 and three evaluation workflows; source checkout clean and PR #14 head matches 7130f4db. | All 25 main jobs and all three evaluation workflows successful. |
| Genuinely distinct second board | Fresh BOMs: BMP280 board has 17 parts and C6; SHT31 board has 16 parts, different U2 sensor/footprint and no C6. Reviewed SHT31 pin map, sensor-specific restrictions, mask/paste correction and electrical/assembly review remain unchanged. | Proven for the bounded software-qualified designs, not working physical hardware. |
| One reliable natural-language command | Explicit indexed-v3 path uses the shared command and deterministic admission/generator. Fourteen synthetic cases pass; measurement-only requests still select either family. Captured invented sensor cannot fit the new request schema. | Integration proven offline; live semantic reliability unproven. Default remains the earlier, failed typed-v2 path. |
| Consistent complete output bundles | Fresh production binary generated both standard families; each passed all 14 gates including ERC, strict DRC/parity, native round trips, previews, manufacturing exports and input immutability. Compared 39 BMP280 + 42 SHT31 deliverables to reviewed examples. Five-profile unchanged generator replay compared 111 generated files. | Proven within declared software limits; prior complete five-profile native/export qualification reused. No manual output repair. |
| Final evaluation and review | The unchanged plan requires all 14 raw extractions and decisions correct, all five useful native bundles verified, per-fact/source-bound review, useful-case median below 60 s and maximum below 120 s. | Not performed for request revision 04; no current spending authority. |
| Physical bring-up | Existing electrical/assembly review explicitly separates rail/bus measurements, firmware, calibration, assembly, fabrication and bench testing. | Separate milestone, not claimed or authorized. |

Current-source CI completion was verified at 11:04 UTC. The final quality job
merged 10 coverage shards covering 6,722 tests and 174 packages; generated-code-
excluded coverage was 79.30%, above the unchanged 75.00% threshold. This is the
existing bounded CI selection, not a claim that every possible test was run.
Coverage retains its declared `^TestClosedLoopV8Round1` exclusion.
[Main CI](https://github.com/dshills/KiCadAI/actions/runs/34960209267),
[indexed safeguards](https://github.com/dshills/KiCadAI/actions/runs/34960209181),
[typed safeguards](https://github.com/dshills/KiCadAI/actions/runs/34960209366),
[historical typed evidence](https://github.com/dshills/KiCadAI/actions/runs/34960209216).

## Scope and residual risks

Request-specific sensor choices reuse the decoder's existing literal identity
rule. They do not infer a positive requirement from a mention, filter general
English requests, discard contradictions or repair a provider response. Negative,
uncertain and background mentions remain available; their meaning still needs
interpretation. Measurement-only requests do not require the user to name a
sensor. The model still receives the complete original request and source tables.

Descriptions and polarity examples are guidance only. The schema can still
permit a wrong wireless/geometry feature, omitted requirement, wrong negation,
quantity role or temporal interpretation. A permitted sensor name can still be
given the wrong state or source scope. These risks have not been measured with
the revised model request. Synthetic inputs and the known 14-case corpus are
not an unseen holdout or a statistical claim about arbitrary natural language.

The recorded native validations used KiCad 10.0.3. Each manufacturing manifest
includes nine Gerber layers/job metadata, plated and nonplated drill outputs,
maps/report and placement CSV, and checks their native-board correspondence.
These are software-validated exports, not production or fabrication approval.
The supported power/environment, heater-off, radio-off, no-extra-load and other
family restrictions remain in force. Software checks do not demonstrate bench
performance or assembled-board ambient accuracy.

## Historical evidence and spending boundary

The original failed indexed batch remains immutable: one physical request,
13 unattempted cases, zero complete passes/native bundles and a USD 0.05
reservation with unverified usage. Its 31 archived files, 1,155 Git source inputs
and 1,375 original cache files reverified; the original pinned auditor reproduced
the original failure. Nothing was rescored, refunded, retried or resumed.

The new runtime reuses the same frozen benchmark plan/corpus identity. It has a
different request revision, executable, manifest and qualification. Reusing the
benchmark name is not authority to reuse the old approval, batch or ledger.

A possible next step requires **new explicit approval** for one fresh, separately
recorded final batch of at most 14 physical requests and USD 1.00, using the
existing OpenAI key and this exact manifest. No retry, probe or recovery is
included. The old batch's unused request slots provide no authority.

If a firewall rule is needed, the only proposed scope is Allow for
`/Users/dshills/Development/projects/KiCadAI/.cache/board-family-v2/indexed-runtime-03-02/kicadai-board-family`
to `api.openai.com:443`. LuLu's available restriction is hostname and port, not
a separate TCP-only selector; the evaluator itself uses HTTPS over TCP. No
blanket allow, firewall disablement or other destination is proposed.

This review is outside the frozen runtime directory and does not alter it.
