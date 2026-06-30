# Fabrication Export And Readiness

Fabrication readiness gates, BOM/CPL evidence, and safe KiCad fabrication export commands.

### Fabrication Export And Readiness

The `export` command family evaluates whether a project has enough evidence to
claim fabrication readiness and can produce deterministic package metadata,
BOM, CPL, Gerber, and drill reports. These commands are intended for
machine-to-machine workflows today, so they are dry-run by default and return
JSON by default. Use `--format text` only with command families that expose a
human-readable summary.

```sh
kicadai export preview ./project
kicadai export bom ./project
kicadai export fabrication ./project
kicadai --source-dir ./data/component-sources export bom ./project
```

Fabrication reports now include explicit assembly evidence:

- BOM rows carry component identity status, source, package, component class,
  lifecycle, confidence, and issue/blocking counts.
- When `--source-dir` is supplied, BOM rows are enriched with local
  procurement snapshot fields: `ProcurementSourceID`, `LifecycleSourceDate`,
  `LifecycleFresh`, `AvailabilityStatus`, `AvailabilitySourceDate`,
  `AvailabilityFresh`, and `ProcurementOutcome`. These columns are appended to
  `bom.csv` after the existing readiness columns for backward compatibility.
  Lifecycle values are `active`, `mature`, `nrnd`, `eol`, `obsolete`, or
  `unknown`. Availability values are `in_stock`, `limited`, `backorder`,
  `unavailable`, `unknown`, or `not_checked`.
- CPL rows carry BOM linkage, component identity, normalized side, raw layer,
  raw rotation, normalized rotation, and placement readiness notes.
- BOM/CPL consistency checks block mismatched references, duplicate
  references, missing placements, extra placements, footprint mismatches,
  missing coordinates, and unknown assembly sides.
- Optional manufacturer profiles add local assembly policy checks. The built-in
  `generic_assembly` profile requires exact manufacturer/MPN evidence for
  assembly-critical rows while allowing generic passives.
- Physical-rule fabrication profiles now drive local DFM thresholds for
  annular rings, copper edge clearance, copper feature widths, solder-mask web
  estimates, edge-plating policy, impedance evidence policy, and fabrication
  metadata requirements. Built-in profiles include `generic_assembly`,
  `generic_2layer_economy`, `generic_2layer_standard`,
  `generic_4layer_standard`, and `generic_castellated_review`.
- Local physical-rule profiles can be loaded from trusted JSON snapshots with
  `--manufacturer-profile-dir` or `KICADAI_FABRICATION_PROFILE_DIR`.
  Explicit flags override the environment directory. Local profiles may not
  shadow built-ins, and malformed or duplicate profile IDs are reported as
  structured issues.

Use `--execute` to write files and `--overwrite` to replace existing package
files. KiCad CLI is required for Gerber and drill generation:

```sh
kicadai \
  --execute \
  --overwrite \
  --manufacturer-profile generic_assembly \
  --manufacturer-profile-dir ./profiles \
  --kicad-cli /path/to/kicad-cli \
  export fabrication ./project
```

Profile discovery and validation are available through the `fabrication`
command family:

```sh
kicadai fabrication profile list
kicadai fabrication profile show generic_assembly
kicadai fabrication profile validate ./profiles/my-board-house.json
```

Profile command output includes the profile ID, name, version, source, hash,
selected thresholds, and validation issues. Fabrication readiness reports and
`package-manifest.json` include top-level `manufacturer_profile` provenance
with the resolved profile ID, version, source, and hash, plus
`physical_rules.profile_details` in `physical-rules.json`.

Default package paths are under `<project>/fabrication/`:

- `readiness.json`
- `package-manifest.json`
- `physical-rules.json`
- `bom.csv`
- `cpl.csv`
- `gerbers/`
- `drill/`

Readiness statuses are intentionally conservative:

- `blocked`: required project files, writer/board validation, report data, or
  configured external evidence is missing or failing.
- `candidate`: the project has partial evidence, but not enough to claim ready.
- `ready`: all modeled required evidence passes. KiCadAI can now generate and
  validate Gerber/drill evidence through `kicad-cli`, but readiness remains
  blocked or candidate when any modeled evidence is missing or failing.

KiCad CLI evidence is policy-driven. Without `--kicad-cli`, preview and export
stay deterministic and do not invoke external tools. With `--kicad-cli` and
`--execute`, `export fabrication` invokes KiCad CLI to generate Gerber and drill
outputs, validates required copper, mask, silkscreen, Edge.Cuts, and drill
files, and records generated file lists in `package-manifest.json`. Missing
`ready`-level evidence keeps the status at `candidate` or `blocked`, never
`ready`. Physical fabrication checks now run during `export preview`,
`export fabrication` without `--execute`, and `export fabrication` execution.
The generated
`physical-rules.json` report covers stackup, net classes, solder-mask/paste pad
policy, solder-mask web estimates, annular rings, copper feature widths,
Edge.Cuts containment, courtyard overlap/presence, silkscreen board clearance,
mounting-hole geometry/edge clearance, edge-plating/castellation policy,
controlled-impedance and differential-pair evidence gaps, and fabrication
metadata for board finish, panelization, and fabrication notes. Physical-rule
blockers are included in readiness status and package manifests. With
`--require-drc`,
missing or failing external fabrication evidence is blocking. `design create`
now runs a dry-run fabrication preview only when the input request JSON sets
`validation.acceptance` to
`fabrication-candidate`, which is the highest current design acceptance level
and functions as a request to prove fabrication readiness. That input value is
an enum value; the output field `acceptance.fabrication_ready` is a JSON field
name and boolean. In the output workflow result, partial readiness status
(`candidate` or `blocked`) downgrades the achieved acceptance and leaves
`acceptance.fabrication_ready` false. The `fabrication_readiness` workflow stage
also exposes a compact `physical_rules` summary with status, blocker count,
warning count, active physical-rule/manufacturer profile, and report path
relative to the project root when available.

Lifecycle and availability evidence in fabrication exports is local snapshot
evidence. Unknown, not-checked, backorder, unavailable, or stale availability
appears as review evidence; KiCadAI does not call live distributor APIs or
claim live stock. Use a current, reviewed source snapshot before treating BOM
procurement fields as fabrication release evidence.

This is still not a manufacturer acceptance guarantee. KiCadAI validates the
presence, identity consistency, and local profile compatibility of modeled
fabrication outputs, but manufacturer-specific DFM interpretation, exact
polygonal copper/mask sliver proof, field-solver impedance proof, live part
availability, and procurement readiness remain separate gates.
