# Indexed-quantity experiment — 2026-09-14

Disposition: **local component checks pass; production adoption and end-to-end
language acceptance remain unproven**. This is a continuation of the source-only
prototype, not a new live run. The previous goal turn made concrete progress by
implementing and testing that source-reference boundary. The current turn moves
recognized numeric values and conversions out of model-authored output.

## Changes under test

- Experimental protocol version is now `3-indexed-quantities-experimental`.
  Facts cite source-clause IDs and quantity-occurrence IDs. No model-authored
  numeric magnitude is accepted, including one that happens to be correct.
- `PrepareReferencedRequest` creates exact byte-addressed quantities and compatible
  normalized field values from original source text. The decoder regenerates
  this table; it does not trust a supplied or subsequently modified table.
- Required numeric facts use application-normalized values. Not-required values
  do not configure the board. Forbidden clock/pull-up values exclude matching
  profiles when the catalog proves the mapping. Other forbidden/uncertain
  numeric bounds ask a targeted question rather than becoming positive bounds.
- Non-configurable numerical requirements retain explicit source IDs and
  requirement states. An uncertain additional requirement produces a question,
  not an unsupported verdict. Feature/other/unclear facts cannot silently cover
  unrelated or repeated quantity occurrences merely by citing the whole clause.
- The new splitter retains leading decimals; the historical splitter, decoder,
  provider path, frozen binaries, ledgers, and evaluation files are unchanged.

The application-owned table proves source location and supported literal unit
conversion, not the truth of a semantic label or the completeness of language
understanding. Configurations remain subject to the existing admission/electrical
checks. No native outputs were generated, changed, or repaired in this experiment.

## Completed local checks

- All 14 original provider selections replay with their original decisions and
  the same six original validation errors. They are never migrated or rescored.
- All 14 known prompts with separately authored **synthetic** indexed facts
  produce the expected local outcomes. This is not a new model evaluation.
- The 35-case normalization table covers scalar/range magnitudes, mixed unit
  scaling, leading decimals, scientific notation, Unicode microfarad spelling,
  `between … and` ranges, mismatched dimensions, reversed bounds, overflow and
  underflow. Additional tests cover exact source bytes, identifier boundaries,
  repeated occurrences, wrong IDs/roles, invented numeric fields/defaults,
  missed quantities, distinct requirement states, and table ownership.
- Full target-package race checks passed with `-short -count=1`:
  `internal/boardfamily` **5.451 s**, `cmd/kicadai-board-family` **1.321 s**.
- Targeted `golangci-lint` completed with **zero issues**.
- Two separately bounded 15-second fuzz runs passed:
  source/quantity integrity **81,727 executions** (16.508 s command duration);
  decision/configuration invariants **119,567 executions** (16.318 s). The latter
  started with 165 cached seeds from earlier development, not an unseen corpus.
  These are not performance benchmarks or proofs over all possible inputs.
- Final-02 publication authentication passed after the edits: 203 original
  publication files plus five CI-addendum files remain intact; all 14 attempts
  remain recorded, with raw 5/14, application 7/14, complete 5/14, and the two
  original native bundles matching 81 reviewed deliverables. Acceptance remains
  failed, and the old budget remains exhausted.

All checks removed provider credentials, disabled live-provider tests, and used
the existing repository Go caches with module-network access disabled. No live
API request, additional paid review, firewall change, or screen interaction was
performed. These new experimental files remain local; no new PR CI result is
claimed. The previous broad-suite interruption remains disclosed in
[the earlier verification record](VERIFICATION.md); it was not rerun or promoted
to a pass for this component experiment.

## Review and next boundary

Implementing-agent review, not independent review. The explicit indoor-project /
custom-geometry semantic counterexample still demonstrates a wrong user outcome
despite structurally valid references. A numeric source ID likewise cannot prove
that an accuracy tolerance is an ambient bound, that a current is source capacity,
or that the model classified negation correctly. The retained narrow role checks
are defenses, not a complete classifier. The scanner recognizes a bounded set of
literal forms, not all English quantities or mathematical expressions; unrecognized
forms and further numeric-role interactions still need evaluation.

Do not switch the production command, claim reduced live failure rates, or spend
on another batch based only on these checks. The next offline decision is a
compact model-facing contract and semantic-failure review that preserves all user
requirements and truthful targeted outcomes. Payload bounds, in-memory provider
integration, complete-outcome collection, whole-command qualification, and
exact-head CI are still required before a separately approved/frozen live run.
Physical operation remains a separate authorization. The full goal is not met.
