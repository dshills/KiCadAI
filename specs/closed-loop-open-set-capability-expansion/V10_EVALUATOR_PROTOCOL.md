# V10 Production Discovery Evaluator Protocol

Status: production evaluator implemented and corpus-independent tests passing.
No real V10 requirement has been synthesized and no held-out key or plaintext
has been opened by this implementation phase.

`kicadai-discovery-baseline-v10` is the sole generation-zero public evaluator.
It requires a clean committed repository, an independently authenticated V10
publication, a fresh working root, a fresh report path, the checksum-frozen
evaluator manifest, the embedded component catalog and model registry, and the
locked installed-KiCad promotion toolchain.

The command has no key argument and loads only the 24 public discovery records
and their public obligation anchors. Publication authentication runs both
before and after loading; manifest, requirement, and obligation commitments
are checked again while loading. Symlinks, path escapes, malformed JSON,
unknown fields, trailing JSON, changed files, wrong order, missing obligations,
and noncanonical stable paths fail closed.

The evaluator fixes `opentopologysynthesis.DefaultPolicy()` exactly. Its common
environment commitment covers primitive inventory, catalog, model registry,
the complete search policy, the exact installed `kicad-cli` bytes, and the
content-addressed toolchain/library environment without host-specific paths.
Policy drift, registry drift, missing libraries, or an inconsistent catalog is
an evaluator error rather than a circuit capability outcome.

Cases execute through a fixed four-worker queue, with at most four cases
running concurrently and evidence retained in manifest order. Every case executes
exactly twice sequentially within its worker. Each replay receives a new
no-replace root containing a deterministic commitment to the corpus,
requirement, case, replay slot, environment, and evaluator. Full synthesis
evidence must be byte-identical. Invalid, canceled, timed-out, or drifting
execution aborts the cohort without publishing a baseline.

Every synthesis pass executes physical promotion once from each outer replay
root. Each promotion itself performs the production two-run clean-root KiCad
workflow. The two outer promotion hashes, statuses, and project hashes must
match. Only two fully passing outer promotions can produce `pass`; a stable
physical failure becomes a typed nonpass without a promotion claim.

Production capability observation converts authoritative synthesis and
promotion evidence to exactly `pass`, `unsupported`, `unsafe`, or `exhausted`.
Public obligation anchors are expanded into sorted one-leaf root frontiers in
topology, component, model, simulation, physical design, or verification.
Unknown selectors, unmapped gaps, missing diagnostics, and pass/frontier
contradictions fail closed.

The complete 24-case report is rebuilt by `capabilitybaselinev10`, including
the fourteen gates, replay hashes, distinct root commitments, promotion
evidence, outcome counts, per-case hashes, common environment/evaluator
bindings, and aggregate hash. The report is installed through a same-directory
temporary file and atomic no-replace link. Public baseline publication remains
a separate already-frozen boundary.

Example invocation after authenticated corpus publication:

```text
go run ./cmd/kicadai-discovery-baseline-v10 \
  --repository-root . \
  --working-root /private/tmp/kicadai-v10-generation-zero \
  --report /private/tmp/kicadai-v10-generation-zero-report.json
```
