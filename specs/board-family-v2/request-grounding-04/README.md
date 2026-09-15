# Indexed request grounding revision 04

2026-09-15. Status: **offline candidate; no live acceptance**. This changes the
experimental `indexed-v3` request contract to `indexed-request-04`. The model,
intent wire shape, typed-v2 default, admission outcomes, family catalog, native
generator, frozen evaluation cases and acceptance thresholds are not changed.

## Proven change

The earlier request schema offered both sensor names even when neither occurred
in the request. Its decoder already rejected invented sensor names. The new
request schema reuses that exact literal-identity predicate before generation:
only mentioned names can appear in a `sensor` fact. With no names, there is no
sensor-fact branch. The maximum-inventory export still shows both names and
explicitly describes the request-specific restriction.

This moves an existing constraint into the model's output choices; it does not
add a general English vocabulary gate, infer positive requirements from mentions,
rewrite input or response text, or require a user to choose a named board.
Pressure and humidity requests without sensor names still select their respective
families through measurement facts. Mentioned alternatives, prohibitions,
exclusions and background references remain available for faithful extraction;
their state and source scope still require interpretation. `other` and `unclear`
remain available for unsupported or genuinely ambiguous requirements.

The exact captured bad extraction is rejected by the new request-specific
schema because it invents BMP280. This is a deterministic schema property, not
evidence that a new model response is correct or that the failed request passed.

## Model guidance, not a correctness guarantee

Schema descriptions distinguish user requirements from catalog capabilities,
and short contrastive instructions explain wireless required/not-required/
forbidden states and a wired request that needs no wireless fact. The official
[Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs#handling-mistakes)
notes that schema-conforming output can still be mistaken and suggests prompt
clarity and examples. These are applied here without changing the pinned model
or adding another request, retry, verifier model, or agent loop.

The wireless improvement is **unmeasured**. The schema still permits an incorrect
feature classification, omission, negation or temporal interpretation. No
independent model evaluation or unseen holdout was run. Development used the
already-known regression corpus and the retained failed response. The overall
natural-language goal and unattended-use acceptance remain unproven.

## Verification and review

New tests first failed on eight request-scoping cases and on the exact captured
invented sensor (0.368 s). After correction, the focused schema/envelope/request
tests passed (0.456 s). The ten request contexts exercise 80 sensor/state pairs
against the existing decoder rule, plus measurement-only selection for both
families and rejection of the captured bad extraction. Negative states and
background mentions are not automatically removed or converted to required.

Using pinned cached Go 1.26.8, with provider credentials and module downloads
disabled:

| Package | Short tests | Race + short tests |
| --- | ---: | ---: |
| `internal/boardfamily` | 9.851 s | 14.805 s |
| `internal/aiprovider` | 0.618 s | 2.569 s |
| `cmd/kicadai-board-family` | 5.657 s | 8.260 s |

Full cmd/internal lint passed with zero issues. The 88 combined Node unit tests
passed. A focused provider/generator replay also passed all 14 synthetic cases;
its five supported configurations reproduced all 111 generated files byte for
byte. Actual encoded request bodies were 14,334–16,966 bytes, below the unchanged
24,000-byte request bound. These are byte sizes, not token or latency estimates.
The schema and test-only validator both understand description annotations; no
validation keyword is silently ignored.

Native circuitry, geometry and export code are unchanged. Existing complete
native/export qualification is reused rather than claimed as a new live-board
result. The implementing agent reviewed the diff and these boundaries; this is
not independent electrical review, fabrication approval or bench validation.

The historical verifier passed after the changes: all 31 publication files,
1,155 Git-source inputs and 1,375 original cache files were unchanged, and the
original pinned auditor reproduced its original failure. The envelope test's
saved-response injection now explicitly distinguishes a current synthetic
request from the historical request/response pair. It is not a repaired trial.

## Execution boundary

No provider call, recovery baseline, ledger settlement, firewall change, new
approval, production-default switch, or merge is part of this work. The stopped
batch remains one physical request, 13 unattempted cases, zero complete passes,
and a USD 0.05 reservation with unverified usage. Old executable, manifest,
approval, journal and scoring records are immutable historical inputs.

Exact-source CI for this request revision must be checked after publication.
The earlier green CI at `b7d79c4c` is not evidence for these successor changes.
Any future paid evaluation needs a new frozen runtime and actual new explicit
request/dollar authorization. Unused slots in the stopped batch do not permit
resumption, and this offline change does not justify marking the goal complete.
