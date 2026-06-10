# KiCad Round-Trip Validation

This package validates generated KiCad files by copying them to a temporary
artifact workspace, running KiCad CLI on the copy, and comparing the KiCad-saved
result against the original.

Run the commands below from the repository root.

Normal tests do not require KiCad:

```sh
go test ./...
```

Run KiCad-backed round-trip tests explicitly:

```sh
KICADAI_RUN_KICAD_CLI=1 go test ./internal/kicadfiles/roundtrip
```

Use a specific KiCad CLI binary:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KICAD_CLI="/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli" \
go test ./internal/kicadfiles/roundtrip
```

Preserve generated, round-tripped, diff, and summary artifacts:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_KEEP_ROUNDTRIP_ARTIFACTS=1 \
KICADAI_ROUNDTRIP_ARTIFACT_DIR=/tmp/kicadai-roundtrip \
go test ./internal/kicadfiles/roundtrip
```

`KICADAI_ROUNDTRIP_ARTIFACT_DIR` selects the parent directory. It does not imply
preservation by itself; set `KICADAI_KEEP_ROUNDTRIP_ARTIFACTS=1` to keep the run
directory after tests finish.

Adjust the external command timeout:

```sh
KICADAI_RUN_KICAD_CLI=1 \
KICADAI_ROUNDTRIP_TIMEOUT=2m \
go test ./internal/kicadfiles/roundtrip
```

The default timeout is 60 seconds.

## Failure Artifacts

When artifact preservation is enabled, each run writes into a unique directory.
Artifacts may include:

- The original checked-in or generated source file path in test output.
- The copied file passed to KiCad CLI, modified in place by KiCad as the
  KiCad-saved result.
- `raw.diff`.
- `normalized.diff`.
- `summary.txt`.

Tests never run KiCad CLI against checked-in fixtures directly.

## Debugging Failures

Use the first normalized diff hunk to identify the immediate rewrite. Use
`summary.txt` to see whether top-level PCB section counts changed.

When a failure reveals a writer bug, add one of:

- A writer unit test for the specific emitted object or setting.
- A round-trip integration test fixture.
- A narrow allowlist entry only when the rewrite is understood and deliberately
  deferred.

Allowlist entries must include a reason and a narrow matcher such as category,
section, message text, or diff hunk content.

Example Go allowlist entry:

```go
roundtrip.AllowlistEntry{
    FileType:     roundtrip.FileTypePCB,
    FixtureName:  "pcb-roundtrip",
    Category:     "normalized-diff",
    DiffContains: "(setup",
    Reason:       "KiCad expands setup defaults; remove after writer emits them",
}
```
