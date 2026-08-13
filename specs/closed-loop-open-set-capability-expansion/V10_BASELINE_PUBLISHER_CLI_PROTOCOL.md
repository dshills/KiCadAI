# V10 Public Baseline Publisher CLI

Status: production command wrapper implemented with synthetic tests. No real
V10 corpus, evaluator report, outcome, held-out material, or external key was
opened while preparing this freeze.

`kicadai-baseline-publish-v10` is the sole production command that converts a
canonical V10 evaluator report into the already-frozen public baseline
publication. It accepts no key, ciphertext, held-out record, synthesis,
ranking, selection, or implementation input.

The command requires a clean committed repository and authenticates the exact
frozen baseline-publisher manifest before reading inputs. The report and
binding must be bounded regular files containing canonical strict JSON with no
unknown fields or trailing data. The binding commits the corpus, validation,
obligations, evaluator, environment, publisher, and immutable Git boundaries.

Publication delegates to the frozen atomic publisher, which writes one report,
24 per-case evidence files, manifest, audit, and checksums to a fresh repository
child. The command permits only that new untracked destination during its
post-write repository check and then independently verifies the complete
publication before returning its aggregate summary.
