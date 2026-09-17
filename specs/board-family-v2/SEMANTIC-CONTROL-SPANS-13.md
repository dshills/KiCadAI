# Compound control spans: offline follow-up

Status: **locally verified prototype, not live acceptance**. Parent checkpoint:
`be49235d09af5261752a3dc34111a6220942d995` on
`codex/semantic-boundaries-13`. The unfrozen `10-semantic-boundaries-offline`
contract now supports compound defaults. No runtime/provider/CLI integration is
added, and no historical v9 response is rewritten or rescored.

## Change

The application recognizes exact UTF-8 byte ranges for affirmative default
phrases, such as “reviewed electrical defaults” within a larger request.
Everything outside those ranges becomes an application-owned residual span.
Controls and residuals partition every original byte exactly once. There is no
rewritten prompt, and neither a source quantity nor the remainder of a compound
sentence can disappear into a control.

Additional facts now require an `rN` residual anchor belonging to their owning
clause. Clauses consisting entirely of a recognized control allow no additional
fact. Other clauses retain the unknown/unclear requirement fallback. Validation
checks the new span field before internally lowering the unchanged facts into
the existing v9 admission path; caller-owned response bytes are not mutated.

“Standard electrical defaults” is not a named-profile demand. When every
occurrence of “standard” in a clause belongs to a recognized default span, its
mention slot requires `context_only`. An actual “standard profile” occurrence
outside that span prevents this constraint. Explicit fast/standard selections,
electrical values, and unsupported hardware demands remain intact.

Quotation, conditional and negation cues conservatively disable recognition.
Unicode word-boundary checks prevent a default phrase from being recognized
inside a longer non-ASCII word. This is a bounded grammar, not general semantic
understanding.

## Verification

All checks used a key-free `env -i` environment, Go 1.26.8, cached dependencies,
`GOPROXY=off`, and `GOSUMDB=off`. No API requests or firewall changes occurred.

- Focused checks: **19 top-level test/fuzz-seed groups and 127 subtests**, zero
  failures. Counts were computed from the complete JSON test stream locally,
  not truncated tool output. These are not independent reliability samples.
- Three-package race tests passed: `internal/boardfamily` (53.061 s),
  `cmd/kicadai-board-family` (48.541 s), `internal/aiprovider` (2.826 s).
- After adding the complete frozen-corpus fixture, focused race tests passed
  again (4.123 s). No production source changed after the three-package run.
- Repository-wide `golangci-lint run ./...`: zero issues, including its enabled
  govet/staticcheck checks.
- Source-partition fuzzing: 49,863 mutation executions, passing in 10.443 s.
- Decoder fuzzing: 17,614 mutation executions, passing in 11.355 s. This includes
  no partial configuration after validation error and schema/decoder agreement.

New contrasts cover hardware requirements before, after and between default
spans; preserved electrical violations; real profiles versus default adjectives;
invalid/cross-clause residual anchors; conditional/quoted wording; and Unicode
boundaries. The partition fuzzer checks byte completeness, clause ownership,
unchanged residual text, valid UTF-8 boundaries and zero overlap with quantities.

All **14 unchanged evaluation prompts are representable** by handcrafted
responses and reach their expected decisions in local tests. The five supported
cases select the exact expected family/profile/capacitance, across both families;
the three choices still clarify; the six refusal cases remain unsupported.
Substantive explanations are checked for the 70 pF SHT31-standard limit, later
heater operation, and both USB power and wireless demands.

This is synthetic representation/admission evidence, **not a model test or new
live score**. It does not prove extraction completeness or raw semantic accuracy.
No native generator/exporter changed, and unchanged board qualification is
reused rather than rerun. Review was by the implementing agent, not independent.

## Preserved boundaries and next work

- The v9 live result remains **7/14 complete passes, 2/5 useful native bundles**.
  The evaluation corpus remains byte-identical (SHA-256
  `90bbc1f728374a9d3ba55d1fa7ff5583ede74bf6107f833eb51758ae5ebee867`).
- The new residual anchor proves location, not the truth of a model's detail or
  state. Free-form additional details can still narrow a constraint incorrectly,
  particularly “no external adapter or change to either requirement.” Complete
  source remains available, but preservation of that meaning needs a stronger
  contract/review before another live consideration.
- A model can still omit an unfamiliar additional requirement or misclassify a
  mention outside the bounded grammar. These limitations must not be hidden by
  synthetic fixture success. Consider application-owned text for residual
  requirements to remove unnecessary model paraphrasing, with source-scope
  contrasts rather than a frozen-prompt lookup.
- Before a live candidate exists, finish those semantic boundaries and integrate
  the reviewed contract with isolated request/journal/accounting identities,
  key-free preview and rehearsal checks. Do not change the production default.
- Publishing changes, another paid trial, any new firewall rule, merging, and
  physical bring-up remain outside this checkpoint. A consumed v9 request
  allowance or previously approved executable grants no authority for them.

The [official Structured Outputs guidance](https://developers.openai.com/api/docs/guides/structured-outputs#handling-mistakes)
also distinguishes structural conformance from mistakes in generated content;
the checks here deliberately do not equate schema validity with acceptance.
