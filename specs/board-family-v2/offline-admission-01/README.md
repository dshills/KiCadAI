# Offline admission correction 01

This is a successor development correction, not a new live evaluation. The
published `evaluation/*-01` inputs, `evidence/final-01`, RESULTS, REVIEW, WORK,
examples, qualified runtime-01 executable and exhausted ledgers stay unchanged.
The September 14 evaluation still failed: 4/14 raw-contract passes, 8/14
application passes, and one incorrectly generated heater-on request.

## Approved scope

The user approved: "implement the requirement-checking fix offline, preserving
the frozen evidence and making no additional API calls." This authorizes local
implementation, replay and adversarial regression, not a new live batch,
recovery baseline, external model review, model change, firewall change or
physical qualification. Existing credential preferences are unchanged. The
verification harness removes provider keys and disables live-test opt-ins.

## Design and limits

`DecodeDecision` now uses `bounded-requirements-01` local admission. The provider
response is an untrusted proposal, not proof that every user requirement was met.
The provider model, payload schema, capability context, transport and accounting
are unchanged; export equality is checked during verification.

1. Validate the response structure. Errors return no usable configuration.
2. Parse the **original** request with a bounded, fully consumed local grammar.
   Accumulate sensing, profile, clock, resistance and numeric operating limits.
   Handle complete exclusion phrases before positive capability phrases. Do not
   discard negation, conditions or unfamiliar words as filler.
3. Refuse recognized conflicts; clarify missing choices or unrecognized wording.
   Heater-off only at startup does not authorize heater use later. Unknown heater
   paraphrases cannot pass simply because they evade a keyword blacklist.
4. Derive a configuration only from locally recognized requirements. Require an
   exact match from an overall-supported provider proposal. Never promote an
   overall provider refusal/question into a board, invent a family, substitute a
   profile or weaken a numeric limit.
5. Construct one consistent application decision containing the complete original
   request. Retain the raw provider object separately, unchanged. Duplicated or
   inconsistent model clause annotations cannot authorize anything on their own.

This is **not a general English interpreter or a proof of arbitrary natural-language
semantics**. It intentionally trades breadth for conservative admission. Unknown
wording returns a request to restate the specific unrecognized terms; even a valid
request can need clarification. The model can still falsely refuse or propose an
incorrect configuration. The old requirement to reproduce verbatim model clauses
remains failed historical evidence, not a success credited to this policy.

Examples of reviewed input forms:

```text
Build BMP280 with the fast profile, 400 kHz bus, 2.2k pull-ups and 100 pF total capacitance. No wireless or USB power.
Use SHT31 standard with 70 pF total capacitance. Keep the heater off. Use the supported default power and ambient limits.
Use SHT31 fast with 3.25 to 3.35 V, 1500 mA source capacity, 15 to 30 C ambient, and 65 pF total loading.
```

These examples describe grammar only. Running `--prompt` still requires a
separately authorized request and ledger allowance. The offline correction does
not silently bypass the provider or create new budget. Explicit configuration
generation remains available without an API call. All catalog engineering limits
and physical-qualification caveats continue to apply.

## Verification

From the repository root, run the bounded verification recipe with a **new**
cache output directory (it refuses existing output):

```sh
node specs/board-family-v2/offline-admission-01/verify.mjs .cache/board-family-v2/offline-admission-01-run-03
```

It authenticates historical evidence before and after; runs seen-response replay,
adversarial and positive grammar tests, race checks, a bounded fuzz exercise,
full short Go regression, full lint and the existing Node safeguard tests; builds
a new binary outside runtime-01; compares the provider contract; and checks fresh
explicit BMP280/SHT31 standard bundles against the reviewed examples. No ledger,
frozen output, key, API or firewall is modified. Every subprocess is bounded.

`verification/receipt.json` and its hash-bound logs record the completed local
run. Native smoke artifacts live in the named cache directory, with their complete
hashes and normalized comparisons in the receipt; they are not republished as new
hardware qualification. Frozen reference artifacts remain the durable native
source. Later changes must not rewrite this receipt or the original failed run.

## Release status

PR #14 remains draft. Seen-response regression and synthetic tests are development
evidence, not an independent holdout, a new first-shot result, or a statistical
reliability claim. A future live acceptance run requires a new explicitly approved
request/dollar budget, new runtime qualification and an evaluation protocol that
identifies this admission policy separately. No remaining original request slot
exists. Fabrication, assembly, firmware and bench testing are not included.
