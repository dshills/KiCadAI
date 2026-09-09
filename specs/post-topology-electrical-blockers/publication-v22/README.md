# V22 retained-evidence audit

This package checks the compact publication independently of the frozen
evaluator. It does not execute synthesis, invoke KiCad, alter selection, or
change a capability outcome. Its implementation was added while the original
frozen process was running, in a separate untracked package; all evaluated
sources remained identical to commit
`d3b6088fbaa4f318b5685d62c4fc83eeba3eb4a2`.

`TestPublishedV22ReplayEvidence` requires a complete authenticated 24-case
report and both retained replay directories for every case. It checks:

- Exact case and public requirement identities, clean-root commitments, and
  the frozen selected population.
- Byte-identical electrical sidecars across both replays, authenticated repair
  and wrapper hashes, requirement binding, and unchanged repair limits.
- Positive measured synthesis/hash durations, canonical evidence byte counts,
  and zero synthesis-spool bytes. Measurements are not deterministic outcomes.
- For passing cases, both promotion identities, exact synthesis/project
  bindings, passing status, and project replay identity.

`TestPublishedV22Inventory` also requires the exact report-derived file inventory
and canonical `SHA256SUMS`: the report, assessment, invocation summary, source
commit, Go environment, executable digest, process measurements, and both compact
replay directories. It rejects missing, extra, changed, linked, or nonregular
files and malformed checksum manifests. The manifest binds measurements and
machine-local metadata as recorded bytes, not as deterministic simulation output.
Its SHA-256 is pinned in the publication audit, so changing evidence and
recomputing the manifest does not satisfy the frozen publication commitment.
`GO_ENVIRONMENT.txt` names all eight originally recorded values, including empty
ones, and retains the raw invocation output's SHA-256. This presentation avoids
ambiguous trailing blank lines without changing any environment value.

The promotion identity projection intentionally mirrors the sealed production
projection: machine-specific paths and descriptive messages are not included
in its deterministic hash. Publication file checksums separately bind the full
retained bytes. The audit does not reconstruct the full synthesis digest from
the smaller sidecars; that digest is produced and replay-checked by the frozen
evaluator and authenticated in the case/report evidence.

The separate frozen `public-evaluation-v22` tests remain authoritative for the
20 unselected predecessor comparisons, preservation of historical pass/unsafe
outcomes, all final gates, and the additional-complete-pass criterion. This
package supplements those checks; it cannot replace them.

Before publication, an optional read-only scratch audit can check only completed
atomic checkpoints:

```sh
KICADAI_V22_AUDIT_SCRATCH=/absolute/scratch/cases \
  go test ./specs/post-topology-electrical-blockers/publication-v22 -count=1 -v
```

Use the frozen Go 1.26.8 environment and repository caches. An unfinished
scratch audit is not evidence of a completed evaluation. The publication test
skips only while the final report is absent; once present, missing retained
replay evidence is a failure. Unit tests reject changed sidecar identities,
requirements, selection, repair records, and rehashed limit changes.
