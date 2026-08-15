# V11 Production Discovery Evaluator Protocol

`kicadai-discovery-baseline-v11` is the sole V11 public generation-zero
evaluator. It consumes only the authenticated public discovery partition of the
immutable V10 corpus. It has no held-out key parameter, held-out decryption
surface, baseline input, selection input, or capability-update input.

The evaluator requires a clean committed repository, the frozen
`V11_EVALUATOR.sha256` manifest, the locked installed-KiCad promotion toolchain,
a fresh absent report path, and either a fresh absent working root or an exact
authenticated V11 resume root. Its root marker uses the V11 schema and version;
V10 checkpoint or root-marker bytes are never accepted.

All 24 public cases execute with four workers and exactly two sequential clean-
root replays per case. Each complete synthesis run is streamed to a read-only,
no-replace canonical JSON spool while its replay digest is computed. Provisional
promotion, observation, and gate evidence is derived from replay one before its
complete object graph is released. Replay two then runs. No worker retains two
complete synthesis runs or a full marshaled replay buffer.

Replay and physical-promotion equality are mandatory before a case checkpoint
is accepted. The checkpoint is written atomically and authenticated before its
replay spool files are removed. Resume accepts only complete, strictly decoded,
hash-bound checkpoints and matching V11 clean-root markers; incomplete case
roots are discarded and rerun.

The final report preserves the frozen V10 evidence schema so semantic results
remain comparable. V11 root commitments and resulting hashes are version-
separated. Only a complete 24-case deterministic report may be published.
Failure, timeout, resource exhaustion, nondeterminism, installed-KiCad failure,
or incomplete evidence retires V11 without accepting partial checkpoints.

Canonical invocation after the freeze is committed:

```text
go run ./cmd/kicadai-discovery-baseline-v11 \
  --repository-root . \
  --working-root /private/tmp/kicadai-v11-generation-zero \
  --report /private/tmp/kicadai-v11-generation-zero-report.json
```
