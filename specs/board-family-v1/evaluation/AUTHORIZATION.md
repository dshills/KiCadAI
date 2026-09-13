# Recorded request-limit extension

On September 13, 2026 the user replied **“approved”** to the request to raise this goal's request ceiling from 20 to **35 total**, keeping **$10 total**, the existing OpenAI key, and the same persistent ledger.

Approved scope: up to two recorded development checks (`nl-03` and `ambiguous-02`), then one new 16-case acceptance set (10 supported, 4 unsupported, 2 ambiguous), using these exact previously proposed files:

- `LIVE_CONTRACT_REVISED_PROPOSED.json`: SHA-256 `76cf6b0a3965c18b81504ac67a1e5dbf3b9eec86b10e4a8ba8581da2e7229a9c`.
- `language-holdout-proposed.json`: SHA-256 `1b2daa0c91e8a76ce1c6cb2524d247696af3ef7f0033da9a5bf4b403588f97e4`.

The files retain their historical proposed names/status text to preserve the bytes the user approved; this record provides the subsequent authorization. Only each literal prompt, the bounded contract/schema and generic JSON instructions go to `api.openai.com`. Expected answers and other cases are not sent. No new provider, key, firewall change or fabrication is authorized.

At approval there were 17 retained requests, including one unknown response billed conservatively with its $0.05 reserve. The two development checks and sixteen holdout cases fit exactly in the eighteen remaining slots. No prior entry is removed or overwritten. Original failed acceptance remains failed regardless of corrected-version results.

Before the first new call, the acceptance policy is explicit: at least 9/10 correctly selected and fully validated supported cases on their first attempts; 4/4 unsupported dispositions and 2/2 clarifications without native outputs; median of all ten supported attempt durations below 300 seconds and maximum below 600 seconds. Only correctly accepted, fully validated projects may be delivered. The ten deterministic configuration cases and three clean replays retain their separate existing qualification evidence. Development retries do not count as unseen first attempts.
