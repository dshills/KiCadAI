# Creation Evidence Contract

Successful `design create`, `intent create`, and `circuit create` commands emit
the same lane-neutral evidence set under `<output>/.kicadai/`:

| Path | Go representation | `schema_version` | Generation stage |
| --- | --- | --- | --- |
| `design-request.json` | `creationevidence.DesignRequestDocument` | `kicadai.design-request-artifact.v1` | `parse_request` |
| `transaction.json` | `provenance.TransactionProvenance` | `1` (with `schema: kicadai.transaction.provenance.v1`) | `project_write` |
| `workflow-result.json` | `creationevidence.WorkflowResultDocument` | `kicadai.workflow-result.v1` | `feedback` |
| `validation-summary.json` | `creationevidence.ValidationSummary` | `kicadai.validation-summary.v1` | `validation` |
| `design-promotion.json` | `creationevidence.DesignPromotionDocument` | `kicadai.design-promotion.v1` | `feedback` |
| `manifest.json` | `manifest.Manifest` | `kicadai.manifest.v2` | final evidence indexing |

The documents embed the existing design request, workflow result, and promotion
report representations at the JSON root. Existing consumers can therefore
continue decoding those types while automation that needs an explicit contract
can decode the `creationevidence` types.

`manifest.json` records the creation lane and one sorted `evidence` entry for
every non-manifest core artifact. Each entry contains the artifact kind,
normalized project-relative path, SHA-256 of the exact file bytes, schema
version, and generation stage. The manifest does not hash itself because a
self-hash cannot be represented without changing the bytes being hashed.
Evidence outside the project is not represented as a broken relative path or
silently discarded. It is indexed under `external_evidence` with a
content-addressed `external-evidence://` URI, full SHA-256, kind, and stage.

Skipped workflow stages remain visible as gates with `status: "skipped"` and a
rationale. When promotion is not applicable, `design-promotion.json` uses an
explicit `applicability.status: "inapplicable"` and rationale instead of
omitting the file or implying that gates ran.
If promotion evaluation itself fails, the same field is `"error"`, the
rationale carries the failure, and the validation summary is blocked.

Lane-specific provider responses, intent planning records, retry state,
rationale, simulation, and KiCad reports remain additive. They do not replace
the six core paths.

All core document bytes are prepared before the shared writer starts updating
files. Individual files and the manifest are replaced atomically, and a
failure before writing preserves the previous complete evidence. Project
generation continues to use the existing transactional project writer.

An incompatible representation requires a new `schema_version`. Adding an
optional field that older decoders safely ignore does not by itself require a
version change.
