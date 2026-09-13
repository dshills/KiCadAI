# Historical lint compatibility

Merged base `101a96fd1bf095a4b82727508f0a1e72177bc3f7` fails CI on exactly two `errcheck` findings. They are in checksum-frozen historical experiment programs, not the board-family production command:

| File | Finding | Historical commitment |
|---|---|---|
| `specs/ai-requirement-contract-integration/publication-live-v1/replay-audit/main.go:258` | Cleanup `f.Close()` after an already-failed write ignores a secondary error; the write error is returned. | `publication-live-v1/publication-inventory.json` pins the complete file. |
| `specs/practical-board-completion-v1/engine/main.go:61` | Deferred close of read-only input ignores a cleanup error. | The frozen development batch binds the common adapter; the published branch inventory also retains the file identity. |

The repository already has exact-path historical lint exclusions for frozen V9/V10 evidence. This change extends that policy to only these two file/error combinations. **It is a disclosed historical lint exception, not a source-code repair or a claim that cleanup errors are handled.** All original source and evidence bytes stay unchanged. No directory-wide exclusion, disabled linter, skipped test, reduced coverage floor or `continue-on-error` is added.

Before lint, CI runs `TestFrozenHistoricalLintExceptions*`. It checks both published SHA-256 identities, requires the exact scoped rule and guard ordering, and tests changed/missing source and successor-path rejection. Changed source fails the hash gate; successor paths do not inherit the exceptions. A future maintained replacement must correctly handle cleanup errors instead of modifying or resealing the historical programs.

Full lint, static contracts and the dependent coverage merge/floor must actually execute successfully before green CI is claimed. Source preservation alone does not prove that outcome.
