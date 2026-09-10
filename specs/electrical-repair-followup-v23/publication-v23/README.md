# V23 retained-evidence audit

This package checks the compact publication independently of the frozen
evaluator. It does not execute synthesis, invoke KiCad, alter selection, or
change a capability outcome. Its implementation was added while the original
frozen process was running, in a separate untracked package; all evaluated
sources remained identical to commit
`16013c7d3f4e44ae8cfbda18be655c853ef8f111`.

`TestPublishedV23ReplayEvidence` requires a complete authenticated 24-case
report and both retained replay directories for every case. It checks:

- Exact case and public requirement identities, clean-root commitments, and
  the frozen selected population.
- Byte-identical electrical sidecars across both replays, authenticated repair
  and wrapper hashes, requirement binding, exact predecessor binding to the
  frozen V22 sidecars, unchanged repair limits, exact V23 solver-policy identity,
  and binding of that policy in both the repair and synthesis wrappers.
- Positive measured synthesis/hash durations, canonical evidence byte counts,
  and zero synthesis-spool bytes. Measurements are not deterministic outcomes.
- For passing cases, both promotion identities, exact synthesis/project
  bindings, passing status, and project replay identity.
- If an electrically passing synthesis instead fails physical promotion,
  retain and authenticate both failed promotion records and their diagnostics.
  Positive promotion timing requires the corresponding record; it never counts
  as a complete pass or permits a missing physical failure artifact.

`TestPublishedV23Inventory` also requires the exact report-derived file inventory
and canonical `SHA256SUMS`: the report, assessment, invocation summary, source
commit, Go environment, executable digest, process measurements, and both compact
replay directories. It rejects missing, extra, changed, linked, or nonregular
files and malformed checksum manifests. The manifest binds measurements and
machine-local metadata as recorded bytes, not as deterministic simulation output.
Its SHA-256 is pinned in the publication audit, so changing evidence and
recomputing the manifest does not satisfy the publication commitment. The
completed 154-file / 521,456-byte inventory is pinned to
`4d3c8dc8c0f07f068631f0c3427e095226104a5614cb2ee1006d9f3c56117182`.
All 24 cases and 48 replays pass the final audit. See [results](RESULTS.md).
`GO_ENVIRONMENT.txt` names all eight originally recorded values, including empty
ones, and retains the raw invocation output's SHA-256. This presentation avoids
ambiguous trailing blank lines without changing any environment value.

The promotion identity projection intentionally mirrors the sealed production
projection: machine-specific paths and descriptive messages are not included
in its deterministic hash. Publication file checksums separately bind the full
retained bytes. The audit does not reconstruct the full synthesis digest from
the smaller sidecars; that digest is produced and replay-checked by the frozen
evaluator and authenticated in the case/report evidence.

The audit does not rerun numerical analyses or replace the frozen producer's
full electrical-certificate validation. Solver-policy self-hashes are checked
against the exact sealed implementation's declared policy, not trusted merely
because a sidecar is internally self-consistent.

The separate frozen `public-evaluation-v23` tests remain authoritative for the
20 unselected predecessor comparisons, preservation of historical pass/unsafe
outcomes, all final gates, and the additional-complete-pass criterion. This
package supplements those checks; it cannot replace them.

`TestRetainedNativeV23Projects`, enabled by the scratch variable below, also
rehashes the original `.kicad_pro`, `.kicad_sch`, and `.kicad_pcb` files using
the frozen producer's exact relative-path/NUL/bytes/NUL framing. It authenticates
the recorded promotion first, checks both clean projects in each outer replay,
and rejects missing, changed, or linked project files. Like the frozen project
hash, it excludes `.evidence` and does not claim to hash library or auxiliary
files. This is a read-only check, not a new KiCad execution. Native project
directories remain outside Git; compact publication tests do not depend on
their machine-specific paths.

An optional read-only scratch audit can additionally check completed atomic
checkpoints and the original native project bytes:

```sh
KICADAI_V23_AUDIT_SCRATCH=/absolute/scratch/cases \
  go test ./specs/electrical-repair-followup-v23/publication-v23 -count=1 -v
```

Use the frozen Go 1.26.8 environment and repository caches. An unfinished
scratch audit is not evidence of a completed evaluation. Now that this
publication is frozen, its report and complete retained inventory are required
on every test run, locally and in CI; missing evidence is a failure, not a skip.
Unit tests reject changed sidecar identities,
requirements, selection, repair records, and rehashed predecessor, solver-policy
or limit changes.
