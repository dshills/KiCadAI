# Natural-language usability audit 01

Code under test: `9ef97737b6c6d2576ebadfbc5d2bbdc2f60a3f61`.
Result: **5/25 expected application outcomes**, with **20 unnecessary
clarifications**. All cases use an explicitly named supported sensor and profile;
the injected candidate contains the correct nine-field configuration and the
complete original prompt. No provider was called and no native board generated.

The five unchanged canonical requests pass. Adding "Please", appending "Thank
you", using "I would like", or asking "Could you use ...?" makes each of the five
family/profile requests fail admission. These phrases add no hardware requirement
or genuine ambiguity. The local rule requiring every word to match an allowlist
therefore prevents an ideal AI response from achieving the original goal.

This is an implementing-agent-authored metamorphic development audit, not an
independent holdout, user-traffic sample or statistical model-reliability estimate.
The synthetic candidates are explicitly marked; they are not provider responses.
The 14/14 historical-response regression remains true for its narrow cases. It
does not override this usability failure or the unchanged failed live evaluation.

## Evidence

- `cases.json`: 25 prompts and explicit expected configurations, authored before
  executing the diagnostic helper.
- `main.go`: credential-free helper invoking production `DecodeDecision`; it
  validates every expected configuration, records all outcomes, hashes relevant
  source/evidence files, and refuses to overwrite output.
- `results.json`: every synthetic input and admitted decision, decoder errors,
  source hashes and limitations. SHA-256:
  `4a378b90dd9db912f005aa952c9867ab9ad26bb36c98891084b4d6045e7df5d2`.

Executed with the existing cached Go toolchain, provider credentials and live-test
opt-ins removed, network dependency lookup disabled. Both historical publication
authentication and the correction's current-source/cache check still pass. The
production working tree was not changed. PR #14 and its exact code-head CI were
revalidated: open draft, 25 successful checks.

## Consequence for the full goal

The previous turn made progress by fixing a demonstrated unsafe admission and
publishing verified development evidence. However, a closed-vocabulary parser is
not a substitute for the requested practical natural-language selector. The full
goal is still incomplete for an implementation reason as well as the need for
separately approved live acceptance. Do not mark the goal complete or treat a new
API budget as the only remaining work.

Next implementation should keep the original request in application-owned data,
replace redundant model verdict/copying fields with a small typed requirements
representation, and derive configuration/disposition in code from explicit
capabilities and operating bounds. Retain independent contradiction checks and
all historical failures. Do not merely add these 20 phrases to the allowlist and
call the language problem solved. Preserve the broader ordinary-language goal;
clarifications must resolve real ambiguity, not enforce a command vocabulary.

Before another live run, test complete/contradictory/unknown requirements and
normal paraphrases offline, review the successor payload separately, and keep raw
model correctness distinct from local decisions. Only after that should a new
runtime, frozen evaluation and explicit request/dollar authority be proposed.
Do not reuse either exhausted ledger, overwrite runtime-01 or change firewall
rules. Physical qualification remains separately authorized.

The evaluation methodology follows the distinction between typical, edge and
adversarial cases in [OpenAI's evaluation guidance](https://developers.openai.com/api/docs/guides/evaluation-best-practices).
Schema conformance alone cannot establish semantic correctness; the
[Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs)
also explicitly warns that schema-valid responses can contain mistakes. Neither
source constitutes evidence that any proposed successor implementation will pass.
