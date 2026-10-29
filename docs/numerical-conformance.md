# Numerical conformance and failure reproduction

Use numerical conformance to compare the production Go solver with independent
analytical equations and, optionally, ngspice 47. Use reproduction bundles to
isolate a schematic failure in one command. Neither workflow needs an API key.

## Run the numerical suite

```sh
make build
bin/kicadai --output ./out/analytical conformance run
```

The offline run checks all 12 frozen cases against the production
`simmodel.ResolveWithTopology` and `simmodel.Evaluate` path. Its status is
`analytical_checked`, with external simulation explicitly `not_checked`.

For external comparison, prepare a headless simulator in a new cache directory:

```sh
bash scripts/build-ngspice47.sh .cache/ngspice47
make numerical-conformance NGSPICE="$PWD/.cache/ngspice47/install/bin/ngspice" CONFORMANCE_OUTPUT="$PWD/out/external"
```

Dependency preparation requires network access, a C/C++ compiler, make and
shasum. It verifies the frozen SHA256 before extraction, builds only inside the
selected directory, and does not install or link system software. An existing
ngspice 47 binary can be supplied instead. Its version and executable digest
are recorded; the upstream source digest is a reference pin, not proof of an
arbitrary supplied binary's build provenance. The CI workflow builds from that
verified archive on Ubuntu 24.04. A missing tool or any other release fails.

The suite covers four DC divider cases (equal, loaded, asymmetric and high
resistance), six RC AC cases (0.1, 1 and 10 times cutoff with/without a load), and
two RC step cases. The analytical oracle uses closed-form conductance, complex
transfer-function and exponential equations independently of Go solver output.
It never rewrites expectations based on simulator observations.

Each real and imaginary voltage component must agree within 1e-8 V absolute
plus 1e-7 relative for DC/AC. Transient comparisons allow 0.5% relative because
the Go capacitor uses backward Euler on a fixed time grid. Steps start from
zero energy, rise after 0.1 time constant, and are observed one time constant
after the step; the Go step is ideal while ngspice has a rise of 1e-6 time
constant. AC uses a unit excitation; step/DC use the specified input voltage.

The suite permits at most three minutes overall, 15 seconds per external case,
and no retries. It only sends generated, trusted netlists to ngspice and uses
`-n` to ignore startup files. This is not a sandbox for arbitrary SPICE input.
Environment variables whose names identify credentials are filtered before
launching subprocesses. No provider calls or hardware measurements occur.

## Inspect a discrepancy

Every completed run publishes an atomic directory with a JSON report, exact
fixture inputs, resolved model plans, Go reports, SPICE netlists, external
stdout/stderr and version identity when requested, and SHA256 inventory.
Differences retain the original observations and identify the first failing
stage. A failed comparison exits nonzero **after saving its evidence**.

```sh
bin/kicadai --target ./out/external conformance verify
bin/kicadai --target ./out/external --output ./out/replay conformance replay
```

Replay selects failed cases, or all recorded cases if none failed. It requires
the frozen fixture and current model registry identities to match. External
execution is never inherited from the bundle: explicitly add `--ngspice PATH`
before the command to repeat it. Without that flag, replay only repeats the Go
and analytical comparison. Inspect `external_status` when interpreting results.

Verification also works for an intact historical model/corpus identity that
the current executable cannot replay. It detects missing, changed, extra and nonregular files; it does not
rerun calculations or authenticate an author. Rehashed malicious content is
not trusted evidence. The executable digest identifies a dirty development
build even when its source revision alone cannot. Known compact fixtures need
no minimization; automatic reduction of arbitrary circuits is not implemented.

## Reproduce a schematic failure

```sh
bin/kicadai --request ./session.json --output ./out/repro repro schematic
bin/kicadai --target ./out/repro repro verify
bin/kicadai --target ./out/repro --output ./out/replayed repro replay
```

Capture runs once in a temporary workspace. It retains the exact input bytes,
source/executable and model identities, first failing stage, error, and replay
instructions. Malformed JSON and invalid confirmations are reproducible too.
The command exits nonzero for a captured failure; successful capture also
works as a baseline. Prior inputs and projects remain untouched.

The bundle does not contain a project-folder copy, environment snapshot or
credential configuration. Input bytes and error text can contain private
design data or local paths, so inspect before sharing. Nothing is uploaded.
Native failure replay requires explicit `--require-erc` again; a tool path
from an untrusted bundle is never executed. Prior native tool outputs are not
copied into schematic reproduction bundles; numerical conformance bundles do
retain their external outputs. Each replay creates a new bundle, never an
overwrite. General circuit minimization remains out of scope. A bundle holds
at most 255 payload files plus its manifest (256 total), with a 2 MiB per-file
and 32 MiB total payload limit. The subprocess runner bounds each stdout/stderr
stream to 64 KiB before JSON serialization. Oversized payloads fail publication
instead of silently truncating numerical evidence.

## Discover supported workflows

```sh
bin/kicadai capability explore
bin/kicadai capability explore rc-filter
bin/kicadai capability docs
```

The [generated capability reference](capability-explorer.md) shares the
schematic validator's bound constants and existing generation registries.
Entries include input contracts, limits, required tools, example commands and
evidence levels. The inventory describes supported front-door workflows and
generation profiles, not every experimental CLI operation. It does not detect
installed tools or turn a declared capability into a passing run. A unit test
rejects documentation drift.

The external runner follows ngspice's documented [batch and control language
interface](https://ngspice.sourceforge.io/ngspice-control-language-tutorial.html).
The [official release history](https://ngspice.sourceforge.io/news.html)
identifies release 47. Numerical agreement in this bounded ideal-passive corpus
does not establish nonlinear, switching, thermal or physical accuracy.
