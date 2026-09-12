# Recorded publication recipes

These text files retain the exact Node helper source used from the repository's ignored `.cache` directory to construct the publication drafts, authenticate prior phases and build/stream-check the new archive. Original names were `practical-completion-archive-work.mjs`, `practical-completion-audit-work.mjs` and `practical-completion-history-all.mjs` under `.cache/`.

They are provenance records, not new-run instructions. They expect their original location, existing local evidence, an in-memory approved key for exact scanning, and exclusive output paths; rerunning would collide with retained outputs. The audit recipe shown is the corrected depth-first serialization version; the earlier failed ordering check is disclosed in `../audit-attempt-1.json`. The archive recipe's Python subprocess streams and hashes every member without full extraction; credentials travel only through private stdin for exact scanning, never command arguments or printed output.

Use `../../verify-publication.mjs` for current read-only reauthentication. No recipe contacts a provider or runs a new board evaluation. Committed reports and these recipes are separate from the 456-file raw archive and are retained in Git.
