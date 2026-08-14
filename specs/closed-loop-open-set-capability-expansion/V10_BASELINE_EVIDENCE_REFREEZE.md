# V10 Baseline Evidence Corrective Refreeze

Before corpus validation or any real evaluation, executor design review found
that the evidence envelope committed identical replay bytes but did not commit
the distinct clean roots required by `V10_BASELINE_PROTOCOL.md`.

The corrective refreeze adds exactly two canonical replay-root SHA-256 values
to every discovery case and rejects missing, malformed, or reused roots. It
does not change the four outcomes, gate semantics, frontier semantics,
promotion rules, selection policy, corpus, author packets, or any expected
result. No V10 requirement was evaluated and no key or held-out record was
opened before this correction.

During the later public-only evaluator run, the host process lifetime ended
before the 24-case cohort could finish. A second outcome-neutral correction
therefore exposes the existing single-case canonical validator and marshaller
for authenticated evaluator checkpoints. It does not change any case field,
hash algorithm, gate, outcome, frontier, promotion, or aggregate validation
rule. The correction was selected from process-lifetime evidence only; no
completed V10 report, held-out key, held-out record, or held-out outcome was
available.
