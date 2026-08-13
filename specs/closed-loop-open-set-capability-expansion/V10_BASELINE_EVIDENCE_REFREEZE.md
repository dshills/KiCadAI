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
