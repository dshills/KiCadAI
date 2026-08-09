# V2 Corpus Authorship Draft

This draft records the independent authorship of the 24 requirements in this quarantine directory.

- I worked from only the authorized behavior contract: sections 4–5 of `SPEC.md`, `CORPUS_RULES.md`, the Corpus and Blind validation sections of `V2_CONTINUATION_PROTOCOL.md`, and the public JSON schema and validation vocabulary in `internal/opentopologysynthesis/model.go` and `internal/opentopologysynthesis/validate.go`.
- I did not inspect V1 corpus material, V1 baseline or selection evidence, V1 validation evidence, existing requirement examples, existing test data, staged changes, repository history or differences, or production synthesis, search, simulation, or repair implementation.
- Every requirement was independently conceived for V2. No requirement was copied, paraphrased, or tuned from another corpus, an observed evaluator result, a known diagnostic, or a predicted implementation limit.
- Discovery and held-out membership was fixed during authorship. The held-out source bytes were not informed by discovery execution or any observed result; no synthesis was run.
- The manifest contains 12 discovery and 12 held-out entries. Each role contains exactly two entries in each reporting domain: analog, power, digital, mcu, sensor, and mixed_signal.
- Requirement documents state only external interfaces, bounded operating conditions and events, observable behavioral assertions, safety bounds where appropriate, manufacturing-neutral board limits, and the complete strict acceptance profile.
- Project names, titles, descriptions, case IDs, and source IDs are opaque. The requirements do not intentionally prescribe a topology, component, manufacturer, part, model, pin, net, block, coordinate, layer, track, route, via, repair action, expected result, issue identity, or capability identity.
- The corpus is neutral with respect to current KiCadAI support. I did not optimize for, predict, or investigate current support, and I did not inspect outcomes.

This document remains a draft until an independent reviewer verifies the quarantine corpus and records the freeze artifacts required by the normative process.
