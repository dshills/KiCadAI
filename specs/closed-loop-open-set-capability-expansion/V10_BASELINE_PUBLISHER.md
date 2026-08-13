# V10 Public Baseline Publisher Freeze

The V10 public baseline publisher is frozen before corpus publication or any
real V10 requirement evaluation. It accepts only an already validated 24-case
discovery report and immutable commit, manifest, obligation, evaluator, and
environment bindings. Its API has no held-out record, key, decryption,
synthesis, simulation, selection, ranking, or implementation input.

Publication writes exactly one canonical report, 24 canonical per-case
evidence files, one manifest, one public audit, and one canonical checksum
manifest. A same-parent no-replace atomic directory operation prevents partial
or replacement publication. The publisher invokes its independent read-only
verifier before returning success.

The verifier rejects noncanonical JSON, unknown fields, map- or interface-
backed artifact types, malformed commit/digest bindings, case or outcome
drift, unexpected files or directories, symlinks, special or oversized files,
path traversal, byte/hash mismatch, non-reproducing audit/checksums, and
manifest/report/evidence disagreement. It reads opened regular file handles
under a fixed size bound and verifies identity against the inspected path.

Every case contains exactly two identical evaluation replay commitments. Every
pass must already carry all 14 gates and two distinct clean-root installed-
KiCad promotions. Nonpasses carry no promotions, retain deterministic replay
and fail-closed gates, and expose a complete typed root frontier. The frozen
V10 baseline-evidence validator enforces those rules before publication and
again during verification.

`V10_BASELINE_PUBLISHER.sha256` binds the complete production implementation,
its synthetic tests, and this protocol. No real V10 corpus, external key,
outcome, or held-out artifact was accessed while preparing this freeze.
