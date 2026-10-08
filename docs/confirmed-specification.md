# Confirm a specification, then generate a KiCad bundle

This workflow uses the existing deterministic BMP280 and SHT31 board families.
It does not require AI, an API key, or network access to review, confirm, build,
or validate a board. AI can optionally propose an editable specification, but
cannot confirm it or generate a board in the drafting step.

The delivered capability is **human-confirmed specification → validated files**,
not reliable unattended interpretation of arbitrary English. Previous failed
language evaluations remain failed and unchanged.

## Quick start (offline)

Run from the repository root with Go and the reviewed KiCad 10.0.3 installed:

```sh
go build -o ./kicadai-board-family ./cmd/kicadai-board-family
./kicadai-board-family spec new \
  --config examples/board-family-v2/sht31-standard/configuration.json \
  --output draft.json
./kicadai-board-family spec review --input draft.json
```

Read the entire review: the proposed configuration, original request if present,
unresolved items, both families' capabilities and restrictions, electrical
calculations, and acknowledgement. Edit `draft.json` if necessary, then review
again. Copy the **latest** `sha256` from that review:

```sh
./kicadai-board-family spec confirm --input draft.json \
  --accept-sha256 COPY_THE_REVIEW_SHA256_HERE --output confirmed.json
./kicadai-board-family spec build --input confirmed.json \
  --output ./generated-sht31 \
  --kicad-cli /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
```

`--accept-sha256` explicitly acknowledges that you compared the original request
with the actual supported configuration and accepted all catalog conditions.
Do not blindly automate copying the fingerprint for unreviewed AI proposals.
Confirmation binds the specification, catalog, electrical calculations and
acknowledgement. Any relevant edit or catalog change requires confirmation again.
This receipt detects changes; it is not a digital signature or proof of who
reviewed the design.

All output paths must be new. Existing files, including confirmation receipts,
are never overwritten. Rebuild into a new directory; do not repair output files
to force validation to pass.

## Supported starting points

Use any of these configuration files with `spec new --config`:

| Family | Profiles | Example directory under `examples/board-family-v2/` |
|---|---|---|
| BMP280 pressure | standard, fast, low_current (pull-ups only) | `bmp280-standard`, `bmp280-fast`, `bmp280-low_current` |
| SHT31 temperature/humidity | standard, fast | `sht31-standard`, `sht31-fast` |

Each directory has `configuration.json`. You may edit declared operating limits
within the reviewed bounds. Supply voltage, current capability, ambient range,
bus capacitance, family and profile are explicit; confirmation never silently
fills missing configuration fields. Geometry, circuitry and routing remain
fixed. For a written request, pass `--request-file request.txt` to `spec new`
so it appears alongside the configuration during review.

## Optional AI-assisted drafting

This reuses the existing typed selector as an **untrusted suggestion**, not a
new parser protocol. It makes at most one provider request per invocation with
the existing ledger accounting and no retries. The original request is retained
verbatim in the draft. Gold answers, repository files and board geometry are
not sent. A key is needed only for this command.

After obtaining spending/payload permission, provide an approved budget file,
for example:

```json
{"goal":"confirmed-spec-draft","max_requests":1,"max_micro_usd":50000}
```

```sh
./kicadai-board-family spec draft --prompt-file request.txt \
  --ledger draft-ledger.json --live-budget approved-budget.json \
  --output ai-draft.json
./kicadai-board-family spec review --input ai-draft.json
```

The budget file is a spending limit, not authorization. Reuse the ledger; do not
delete it to evade limits. Provider failures are not retried. A failed or
unsupported proposal has no configuration and contains unresolved items. Edit
the **specification**, reconcile the full request, and resolve those items
before reviewing and confirming. An unsupported request may need to be declined
instead of weakened to fit a family.

Even a supported AI proposal can omit a requirement. Before confirmation check
every original requirement—including prohibitions, substitutions, combined
sensors and physical guarantees—against what the chosen family actually does.
`review_notes` and the original request are review context, not executable
constraints. Only the explicit configuration and fixed catalog conditions are
implemented. Do not confirm unless that is the design you intend.

## Delivered bundle and failure behavior

A successful build contains the native `.kicad_pro`, `.kicad_sch` and
`.kicad_pcb`, local libraries, BOM CSV/JSON, previews, configuration, electrical
calculations, validation reports and the existing manufacturing export bundle.
It also includes:

- `confirmed-specification.json`: the exact accepted input receipt.
- `bundle-manifest.json`: SHA-256 hashes of every other delivered file,
  confirmation fingerprint and successful 14-gate validation status.

Generation and validation reuse the existing code without modifying a circuit
to make it pass. A failed gate returns a nonzero exit status, retains diagnostic
output, and produces no successful bundle manifest. Never use a partial failed
directory as a qualified output. Provider credentials are removed before native
tools run. Existing legacy command flags remain compatible.

## Verification and limits

Unit tests cover all five configurations, changed confirmations, invalid or
unresolved specifications, duplicate JSON fields, output reuse, and optional
AI drafting with a fake provider. Run the real native integration suite with:

```sh
KICADAI_OFFLINE_NATIVE_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
  go test ./cmd/kicadai-board-family -run '^TestConfirmedSpecificationNative$' -count=1 -v
```

The suite generates all five configurations, runs all 14 gates, compares native
hashes with the reviewed examples, and checks the complete output manifests.
Synthetic AI tests establish wiring and failure handling, not model accuracy.

These boards require external regulated 3.3 V and the catalog's firmware,
assembly and operating restrictions. Neither physical bring-up, EMC, fabricated
hardware reliability, delivered firmware, arbitrary circuitry, nor guaranteed
ambient accuracy is established by this workflow. Confirmation does not remove
those limitations. See [electrical and assembly review](../specs/board-family-v2/ELECTRICAL_ASSEMBLY_REVIEW.md).
