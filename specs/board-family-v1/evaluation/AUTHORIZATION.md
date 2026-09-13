# Recorded request-limit extension

On September 13, 2026 the user replied **“approved”** to the request to raise this goal's request ceiling from 20 to **35 total**, keeping **$10 total**, the existing OpenAI key, and the same persistent ledger.

Approved scope: up to two recorded development checks (`nl-03` and `ambiguous-02`), then one new 16-case acceptance set (10 supported, 4 unsupported, 2 ambiguous), using these exact previously proposed files:

- `LIVE_CONTRACT_REVISED_PROPOSED.json`: SHA-256 `76cf6b0a3965c18b81504ac67a1e5dbf3b9eec86b10e4a8ba8581da2e7229a9c`.
- `language-holdout-proposed.json`: SHA-256 `1b2daa0c91e8a76ce1c6cb2524d247696af3ef7f0033da9a5bf4b403588f97e4`.

The files retain their historical proposed names/status text to preserve the bytes the user approved; this record provides the subsequent authorization. Only each literal prompt, the bounded contract/schema and generic JSON instructions go to `api.openai.com`. Expected answers and other cases are not sent. No new provider, key, firewall change or fabrication is authorized.

At approval there were 17 retained requests, including one unknown response billed conservatively with its $0.05 reserve. The two development checks and sixteen holdout cases fit exactly in the eighteen remaining slots. No prior entry is removed or overwritten. Original failed acceptance remains failed regardless of corrected-version results.

Before the first new call, the acceptance policy is explicit: at least 9/10 correctly selected and fully validated supported cases on their first attempts; 4/4 unsupported dispositions and 2/2 clarifications without native outputs; median of all ten supported attempt durations below 300 seconds and maximum below 600 seconds. Only correctly accepted, fully validated projects may be delivered. The ten deterministic configuration cases and three clean replays retain their separate existing qualification evidence. Development retries do not count as unseen first attempts.

## Subsequent six-call extension

The user subsequently replied **“approved”** to six targeted refusal/clarification checks, increasing the ceiling from 35 to **41 total physical requests**, with the same **$10 total**, key, ledger and approved contract. This authorization does not waive earlier failures, change decision/generation logic, authorize a second PR or authorize a merge.

- Exact predefined file: `guardrail-followup-proposed.json`, SHA-256 `ccd2806424b811d24f13b7e2cb91b4ed92c8673d691096ca3dce64ba338a208a`.
- Contract remains SHA-256 `76cf6b0a3965c18b81504ac67a1e5dbf3b9eec86b10e4a8ba8581da2e7229a9c`.
- Starting ledger: 35 entries, SHA-256 `3a90e509d6a6ee37fc0a657f51d2f086004cb62ce9807632d08c02f0cc18b3d6`. All entries remain immutable; only indices 36–41 may be appended.
- Exactly one request for each of the six literal prompts. No retry, extra development call or replacement ledger. Only the literal prompt and approved contract/schema are sent; expected answers stay local.
- Pass requires **4/4 unsupported** and **2/2 targeted clarifications**, null configurations, successful non-design command dispositions and no native project outputs. This is a separate targeted follow-up, not a reclassification of the 13/16 live suite.
- Production changes for the run are limited to the request-cap literal and CLI help text. Existing source identities and unchanged deterministic/native evidence remain recorded. The old 35-request comment records the earlier extension; this section records the later one.

The proposed file keeps its historical status text so the exact approved bytes remain verifiable. Its authorization is supplied by this later record, not by rewriting the proposed file.
