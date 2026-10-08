# Full GPT-4.1, unchanged v7 extraction: offline candidate

This local candidate implements an explicit `partitioned-full-v7` command mode
for a controlled model-only comparison. It is **not** a claim of improved model
accuracy, a replacement for the failed candidate08 results, or spending approval.
The default command and all earlier protocol/model identities remain unchanged.
The separate candidate09 keyed-quantity prototype is not included.

The only intended provider-request change is `model` from
`gpt-4.1-mini-2025-04-14` to `gpt-4.1-2025-04-14`. The 14 frozen prompts, original
v7 schema/instructions, 1600-token output limit, source tables, parser, decoder,
acceptance rubric and deterministic generation remain unchanged. A focused test
joins each new request to the actual retained candidate08 request and requires
byte equality except for the model name. Synthetic responses used in tests are
never presented as model-generated evidence.

## Accounting and failure boundaries

- New ledger version 3 has immutable profile
  `gpt-4.1-full-standard-2026-09-16`; old ledgers cannot be reused in either
  direction. New journal/audit identities prevent cross-model certification.
- Full standard rates are $2/$8 per million input/output tokens, without caching
  discounts, as documented by [OpenAI](https://developers.openai.com/api/docs/models/gpt-4.1)
  and checked on 2026-09-16. Legacy mini rates and rounding are unchanged.
- The full-model request bound is **16,000 bytes**, smaller than the mini path's
  bound. Treating each input byte as a token plus 1600 output tokens gives
  $0.0448, leaving $0.0052 overhead inside the existing $0.05 reservation.
  Oversized bodies fail before reservation/dispatch. These are conservative
  client estimates, not a provider-side account spending cap. Unexpected
  over-reserve usage is recorded and halts further requests.
- The original 14 bodies range from 6,828 to 9,582 bytes after model replacement,
  so the tighter bound does not drop or alter an evaluation case.
- Full mode rejects the legacy/default budget, requires an explicit separate
  goal policy, never retries, forbids redirects, and authenticates the returned
  snapshot before native generation. Unknown/failed outcomes retain evidence.
  A previous unresolved, failed or accounting-disputed entry prevents any next
  request in this full-model ledger; there is no automatic recovery path.
- Journal replay is local byte/contract/accounting verification, not provider
  attestation, semantic correctness, independent review or physical board proof.

At the old run's exact token counts, full-model repricing would be **$0.058308**.
This is not a forecast; target access, usage, quality and latency remain unknown.
The original result remains **6/14 complete cases and 4/5 native bundles**.

## Offline verification

`check-offline.mjs` runs focused synthetic/journal/budget/CLI tests, three-package
short regression, targeted race detection, vet, lint, whitespace and formatting
checks. The synthetic CLI tests run the real generator and compare reviewed
native bytes; their validation stub is not new native qualification. The script
inherits no real provider credentials, disables Go network dependency lookup,
rechecks the original closure, and freezes source/evidence hashes. Its check
mode rejects changed or newly added nonignored source files.

Run from this worktree with the preinstalled offline toolchain/cache variables:

```sh
node specs/board-family-v2/model-comparison-10/check-offline.mjs --run \
  /Users/dshills/Development/projects/KiCadAI/.cache/board-family-v2/development-08 \
  /absolute/path/to/a/fresh/output
```

Use `--check` with that output for read-only verification. Do not rerun the old
live collector. A reviewed release, fresh separately approved budget and any
required executable-specific network approval are still necessary before a live
trial. Do not broaden firewall rules or silently fall back to another model.
