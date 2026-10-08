# Source-addressed extraction: offline prototype 12

This document records the prototype checkpoint at `7d8e2488`. For the subsequent
experimental command integration and offline native checks, see
[INTEGRATION.md](INTEGRATION.md). The historical prototype results below remain
unchanged; they are not live acceptance evidence.

Status: **experimental, offline-only, not integrated into the command**. This
checkpoint is progress toward the two-family natural-language-to-KiCad goal,
not completion of that goal and not permission for a live evaluation.

## Why this change

The closed source-eligible-v8 live evaluation completed 11/14 cases and produced
3/5 required native bundles. The two lost useful cases included invented profile
assertions; another case invented multiple requirements and lost the no-adapter
constraint. Those results remain unchanged. This prototype does not rewrite,
repair, or replay captured responses as new success.

Instead of asking a model to choose known requirement labels and copy source
anchors, the application now supplies a stable mention table. Each `mN` fixes one
lexically present concept and its source clause. The proposed model task is to
classify its state and necessary cross-clause context. Every original clause
also has an `additional.cN` slot for unfamiliar requirements. The existing
quantity inventory and precision/role guards are retained.

The version is `9-source-addressed-evidence-experimental`. There is no provider
adapter, CLI protocol flag, spending ledger, dispatch route, or evidence journal
for this version. The default command and all existing protocols are unchanged.

## Implementation and invariants

`internal/boardfamily/intent_addressed.go` provides preparation, schema export,
an explicitly offline contract, compilation, and engineering admission.

- The complete original source bytes, clause table and quantity table are retained.
- Known sensors, measurements, profiles, connections and features acquire slots
  only where the existing lexicon or a connection lexeme occurs. A sensor name
  does not create measurement slots; USB power does not create wired telemetry.
- Every `mN`, `cN` and `qN` key is mandatory. Missing, invented or renamed keys are
  rejected. The model cannot supply a replacement known label or source anchor.
- Mention states are requested, unnecessary-but-allowed, prohibited, unresolved,
  or context-only. Up to four distinct substantive states can coexist to preserve
  a temporal contrast; context-only must stand alone.
- Owning clauses are supplied by the application. Context references must be
  valid, unique clause IDs. Unknown requirements remain `other`; genuine
  ambiguity can remain `unclear` with a targeted question.
- Compilation makes a separate internal v8 object and uses unchanged v8/v7/v5
  admission. Raw input is not mutated and rejected assertions are not discarded.
- Existing limits remain: valid UTF-8 source of 1–2000 bytes, at most 32 clauses
  and 128 source quantities; extraction at most 65,536 bytes and 64 compiled
  nonnumeric assertions. Over-limit data is rejected, not truncated.

This is a lexical inventory, not a semantic parser. For example, `non-radio`
can expose both wired and radio concepts. State and scope still require faithful
interpretation. One concept per clause is not a span-by-span semantic annotation;
the complete clause remains available as provenance.

## Offline verification

`cmd/kicadai-board-family/addressed_corpus_test.go` converts only the pre-existing
hand-authored **synthetic** fixtures for the 14 frozen cases. It reads no provider
responses. It compares compiled fact **multisets**, including states, quantities,
details and source references, allowing only fact/citation ordering differences.
It also checks all 14 expected dispositions, exact useful configurations and
unchanged targeted clarification messages. The original gold-assertion tests
continue to check 47 assertions and 15 quantity occurrences.

The focused suite passes. It covers source ownership and determinism, profile
and heater exclusions, temporal contrasts, unknown requirements, numeric meaning,
malformed JSON/inventories/states/context, limits and raw-byte immutability.
Full two-package tests, race checks, static checking and bounded fuzzing are
recorded in `VERIFICATION.md`. Native tests requiring an explicit KiCad CLI are
not enabled in this checkpoint: no new native-bundle or live result is claimed.

For the same 14 synthetic fixtures, serialized JSON totals are:

| Component | Existing v8 | Prototype v9 |
| --- | ---: | ---: |
| Input + schema + instructions | 110,986 bytes | 87,221 bytes |
| Synthetic extraction output | 6,494 bytes | 5,472 bytes |

These are approximately 21% and 16% smaller, respectively. They are **not** token
counts, complete HTTP request sizes, API cost estimates, latency measurements,
or evidence of better model accuracy. The byte calculation is reproduced by the
verbose synthetic corpus test.

## Known limits are tested, not hidden

`TestSourceAddressedRetainsSemanticCounterexamples` deliberately shows that a
shape-valid answer can still:

- reverse a heater prohibition, causing a false refusal;
- omit an unfamiliar waterproofing demand, causing false acceptance;
- attach the heater clause's prohibition to the fast-profile slot's state,
  despite the profile identity retaining its correct owning clause.

Those tests pass by **demonstrating known semantic failures**, not by establishing
semantic correctness. An incorrectly chosen context-only state is also not
generally detectable. A separate regression proves the existing prompt-level
heater guard still rejects the particular request “Enable the heater” even when
its extraction is context-only. That narrow guard is not a completeness proof.

The schema was checked using the existing offline schema test helper, not by an
OpenAI dispatch. Provider acceptance, output-token headroom, journal identity and
request accounting have not been qualified for this prototype. The implementing
agent performed the review; no independent review is claimed.

## Boundaries and next work

No API request, retry, recovery, external model review, firewall modification,
PR write, push, merge, primary-checkout change, fabrication or physical test is
part of this checkpoint. Existing key reuse approval does not authorize new
requests. All 14 request slots in the previous v8 live batch are consumed; its
small recorded spend does not create further request authority.

Next, integrate this representation only behind a separately identified
experimental protocol, preserving fail-closed source validation, raw-response
capture, one-attempt transport, accounting and journal isolation. Qualify it
with fake transport and existing deterministic native examples before proposing
any new live evaluation. Reuse unchanged board qualification; do not change the
frozen prompts, gold requirements, thresholds or historical evidence to improve
the result. Any future live batch needs a newly approved payload and budget.

The original completion requirements still include reliable family/configuration
selection or targeted clarification, all 14 complete evaluated cases and all five
useful native/manufacturing bundles with complete-board validation and no manual
output repair. Software validation still does not establish bench performance.
