# Pre-maintenance dependency and audit-test source

Byte-exact snapshots from `c31f3bc7af70c2787ddb728fb65358a207f71ded` preserve
the dependencies and test harness pinned by the V5–V17 historical manifests.
Security maintenance updates the live dependencies; it does not rewrite any
old manifest, evaluation, corpus, or provenance claim.

Only historical audit tests may resolve these snapshots. The resolver cannot
substitute production source, protocols, manifests, or corpus files. Existing
manifest hashes still authenticate the snapshot bytes; new tests pin the old
dependency hashes and verify that production rejects the changed environment.
Historical evaluator reproduction requires the original checkout and recorded
environment, not these maintained binaries. No new historical evaluation is
admitted by this archival change.
