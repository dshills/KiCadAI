# Existing schematic review and local integrations

These workflows are available in source builds containing the new commands
and in future release builds that include them. Older published binaries do
not gain these commands automatically. None requires an API key.

## Review an existing schematic

```sh
make build
bin/kicadai --target /path/to/project.kicad_sch --output ./out/review review schematic
bin/kicadai --target ./out/review review verify
```

The target can be a root `.kicad_sch`, a `.kicad_pro`, or an unambiguous project
directory. Open `report.html` for component identities and possible purposes,
symbol/wire/label counts, supported electrical checks, findings and unsupported
constructs. These purposes are conservative reference-prefix descriptions,
not inferred functional correctness. `report.json` preserves the structured
inspection/evaluation results. The bundle also contains the exact reviewed
schematics and an integrity inventory.

The source is read into a temporary snapshot, not rewritten. Every captured
source hash is checked again before publication. The output must be outside
the source project, including through symlinked parents. Literal child-sheet
paths must remain inside the selected root; absolute, symbolic, variable and
case-aliased paths are rejected. Limits are 32 distinct sheets, 128 child
instances per sheet, 2048 total symbols, 2 MiB per source file and 16 MiB total.
Ambiguous projects need an explicit root schematic rather than a guessed file.

Internal checks run per sheet. For cross-sheet native ERC, explicitly request:

```sh
bin/kicadai --require-erc --target /path/to/project.kicad_sch --output ./out/native-review review schematic
```

KiCad runs on the snapshot, outside the source project. Project-specific
rule overrides, ERC exclusions, external libraries and PCB data are not
imported. A missing requested tool fails without publishing a review; a
completed review with findings saves its report and exits nonzero. Native ERC
results, when produced, are retained in `native-erc.json`. A pass does not
assess unsupported constructs, operating points, tolerances, temperatures,
physical behavior or manufacturing readiness. This command performs no repair.

## Attach regression tests to a catalog candidate

The existing `component onboard` command produces a quarantined candidate.
`component self-test` revalidates that candidate's immutable document excerpts,
pin functions, actual symbol/footprint evidence, ratings, parameter bounds,
model identity and provenance. It then resolves and executes supplied circuit
regressions with the existing Go simulation engine.

Prepare a self-test request beside the existing onboarding request so its
relative document paths remain valid. This example uses `jq` only to assemble
the request; it is not a runtime dependency of KiCadAI:

```sh
jq -n --slurpfile candidate candidate.json --slurpfile request onboard-request.json --slurpfile graph regression-graph.json '
  {schema:"kicadai.component-self-test-request.v1",
   documents:$request[0].documents,
   kit:{schema:"kicadai.component-test-kit.v1",
        candidate_sha256:$candidate[0].hash,
        fixtures:[{id:"dc-regression",graph:$graph[0]}]}}' > self-test.json

bin/kicadai --request candidate.json --file self-test.json --symbols-root /path/to/symbols --footprints-root /path/to/footprints --output ./out/component-tests component self-test
bin/kicadai --target ./out/component-tests component self-test verify
```

Each fixture is an existing circuit-graph document, not an executable model or
script. It must include explicit simulation analyses and nonempty assertions,
execute at least one candidate component, and collectively cover every
proposed component with a passing regression. The analysis kinds must fit the
candidate model's provenance allowance. Synthesis is disabled in this harness.

The bounded kit supports 1–8 fixtures, up to 16 components and 32 nets per
fixture, 1–4 explicit analyses, DC operating point, up to 256 AC points or
1024 transient steps. It accepts 1–8 source documents of at most 2 MiB each.
Cancellation is checked between bounded resolver/solver calls and before
publication; the existing numerical evaluator is not asynchronously killed.
Use small cases, not an open-ended synthesis or physical-promotion campaign.

A diagnostic bundle records candidate/kit/source bytes, resolved plans,
simulation observations, first failing stages and coverage. Failed cases exit
nonzero after saving their diagnostic bundle. Invalid request envelopes or
oversized artifacts can fail before publication. All outputs must be new.

Passing tests **leaves the candidate quarantined**. It does not mutate the
supported catalog, add an unverified manufacturer part, fabricate an approval,
or bypass existing promotion gates. Assertions are authored expectations, not
independent manufacturer-model certification. No ERC/DRC or hardware checks
are run by the self-test command.

## Diagnose a local installation

```sh
bin/kicadai doctor
bin/kicadai --require-erc doctor
```

Doctor checks the embedded catalog and model-provenance registry, confirmed
passive examples, bounded board-family profiles, library directories and the
local KiCad version. Its report separates checked resources, discovered
directories, missing optional dependencies, different native references and
blocked required probes. `--require-erc` requires the supported KiCad 10.0.3
version for environment readiness; doctor still does not run ERC or DRC.

Explicit library flags override the existing `KICADAI_*_ROOT` variables. Empty
roots are probed at standard macOS/Linux locations and the standard KiCad 10
Windows installation location. Directory discovery is not a complete library
content audit. No package installation, downloads, provider calls, persistent
configuration changes or environment/credential dump is performed.

The release smoke test now invokes `scripts/install-experience-smoke.sh`. It
copies a binary to a path containing spaces, clears its environment, changes
to an empty working directory, and tests embedded resources, relocated symbol
libraries, generated/relocated schematic integrity, read-only review, the
adapter and a portable offline board-family bundle. The new CI workflow runs
release-format host binaries on macOS and Linux, then the Rust client tests.
These jobs must actually pass before making platform acceptance claims.

## Use the existing offline board family from the main binary

```sh
bin/kicadai family profiles
bin/kicadai --request examples/install-experience/board-family.json family check
bin/kicadai --request examples/install-experience/board-family.json --output ./out/family family generate
bin/kicadai --target ./out/family family verify
```

This exposes the existing embedded `esp32_bmp280_v1` reference and its
standard/fast/low-current profiles without a second executable or API key.
Generation uses the unchanged engineered reference geometry and publishes a
new portable bundle. It does not add board shapes, component placement,
autorouting or electrical capability. `native_checks_run` and
`hardware_validated` remain false. Verification checks file integrity,
rejects contradictory native/hardware claims and recomputes the bounded
electrical equations with a 1e-9 relative/absolute numerical allowance. This
is not ERC/DRC, round-trip validation or manufacturing approval.

## Connect a NetForm consumer

The [standalone Rust client](../integrations/netform-client/README.md) uses a
versioned, one-request-per-process JSON boundary. It can reuse catalog and
pin-map data, inspect a local schematic, verify a local evidence bundle and
check a bounded board-family configuration. It cannot execute arbitrary CLI
commands, call providers, run native tools, mutate projects or replace
NetForm's live-simulation engine.

NetForm's existing checkout and GUI are unchanged. Add the small client as a
path dependency when integrating it there; invoke it on a worker thread, and
tie the result to the document revision that requested it. A circuit review
is not a NetForm electrical document import, connectivity reconstruction or
live simulation result.
