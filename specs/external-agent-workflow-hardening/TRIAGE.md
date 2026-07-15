# External Agent Patch Triage

## Baseline

The repository baseline after patch triage passes:

- `go test ./... -count=1`
- `make lint`

The independent agent report is prose only. It does not include the original
provider envelope or strict-decoded graph, so no replay artifact can be safely
reconstructed from it. Later phases use checked-in sanitized recorded fixtures
instead of inventing provider evidence.

## Classification

| Reported change | Classification | Resolution |
| --- | --- | --- |
| MMBT3904/MMBT3906 `Device` symbol nicknames | Accepted after revision | Corrected all catalog, pinmap, and verification identities to the symbols verified under `Transistor_BJT`; added catalog consistency tests. |
| Implicit `power_neg` schematic lane | Accepted after revision | Phase 4 requires an explicit nullable provider field, permits one warning-backed legacy recorded inference, and rejects ambiguous implicit rails. |
| Multi-unit waypoint direct fallback | Superseded | Phase 5 moved transform, connection-grid snapping, and collision-free symbol placement into one shared canonical anchor contract. Uniform and nonuniform stale waypoints now both fail closed. |
| Global 32,768 provider output-token constant | Rejected | Restored the bounded 8,192 default. Phase 6 introduces profile-aware, bounded configuration and structured incomplete-response evidence. |
| Metadata-only output after failed `--overwrite` | Unproven | Phase 1 covers provider/preflight preservation. Phase 9 expands fault injection across all commit boundaries and changes production code only if loss is reproduced. |

## KiCad Evidence

The checked-out KiCad symbol library defines `Q_NPN_BEC` and `Q_PNP_BEC`
under `Transistor_BJT`; those names are absent from `Device`. Runtime catalog
preflight remains necessary because installed library roots and nickname tables
can differ from the development checkout.

## Reproduction Matrix

| Boundary | Current evidence | Next action |
| --- | --- | --- |
| Input conflict | Existing test proves no output is created. | Retain. |
| Malformed provider record, new output | Existing test proves no output is created. | Retain. |
| Malformed provider record, existing output with `--overwrite` | Phase 1 test proves core files and manifest remain byte-identical. | Include in Phase 9 matrix. |
| Catalog, schematic, routing, writer, artifact, timeout, cancellation | Not yet covered as one preservation matrix. | Phase 9 fault injection. |

No production overwrite change is justified by Phase 1 evidence.
