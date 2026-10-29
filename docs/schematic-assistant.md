# Guided schematic workflow

Generate a voltage divider or RC low-pass filter from an explicitly confirmed specification, without an API key. KiCadAI writes a native schematic and project through its existing schematic IR and transaction engine, checks them internally, and creates an offline HTML explanation.

This is a schematic-only lane. It does not generate a PCB, choose rated manufacturer parts, simulate arbitrary circuits, or approve hardware for fabrication.

## Try an example

Build the current source, then run either example into a new output directory:

```sh
make build
bin/kicadai --output ./out/divider schematic example divider
bin/kicadai --output ./out/filter schematic example rc-filter
```

The divider uses 5 V and two 10 kΩ resistors: its unloaded output is 2.5 V. The filter uses 5 V, 10 kΩ and 100 nF: its ideal unloaded cutoff is approximately 159.155 Hz. Example selection confirms these documented preset values; it is not engineering approval.

Open `report.html` in the output directory. The bundle also contains native `.kicad_pro` and `.kicad_sch` files, the confirmed session, semantic source IR, internal-check results, and a complete SHA-256 artifact inventory.

## Enter your own specification

```sh
bin/kicadai --output ./out/my-session schematic new
```

The wizard asks for a project name, family, maximum input voltage, resistor values, explicit output load, and capacitance where applicable. Enter SI values without a unit suffix: `10k`, `1M`, `100n`. Uppercase M means mega; lowercase m means milli. Load `0` explicitly means unloaded/high impedance, not a zero-ohm short.

The summary shows the exact values, calculated behavior, and ideal-model assumptions. Only typing `yes` authorizes native generation. Type `edit` to change a field, or `cancel` to stop. Answers are saved after each edit. EOF also stops without generating a project.

```sh
bin/kicadai --output ./out/my-session schematic resume
```

The first confirmed output is in `my-session/project/`. Changed specifications generate a separate `project-<specification-hash>/` directory; previous projects are not overwritten. Repeating an existing output is refused.

A confirmed session can also be regenerated without a conversation:

```sh
bin/kicadai --request ./out/my-session/session.json --output ./out/replayed schematic generate
```

Changing any specification field invalidates its confirmation hash and requires reconfirmation. The hash is an integrity binding, not an independent review signature.

## Require native KiCad checks

Internal checks do not count as native ERC. Request installed-KiCad validation explicitly:

```sh
bin/kicadai --output ./out/native-divider --require-erc schematic example divider
```

Use `--kicad-cli /absolute/path/to/kicad-cli` for a nonstandard installation. Native validation includes all ERC severities; a missing executable, tool crash, missing/malformed report, or ERC finding blocks publication. Successful SVG export supplies the native schematic preview. Otherwise the report labels its functional illustration as such.

The report records the actual KiCad version. KiCad 10.0.3 is the project's supported reference; a successful invocation on another version does not establish platform or version qualification. DRC is not applicable because no PCB is generated.

## Read the report

The report explains component purposes, ideal analytical behavior, and each check's status. `checked` means that named check ran successfully; `not_checked` is not a pass. `candidate` means internal checks completed; `erc_checked_candidate` additionally means native ERC completed successfully.

Divider calculations include any specified load. RC calculations include load-dependent DC gain, cutoff, time constant, settling time, and worst initial resistor power for a 0→VIN step with an initially uncharged capacitor. These values are predictions, not physical measurements or numerical simulation results.

The bounded input envelope is 0.1–24 V; resistors are 1 kΩ–1 MΩ; finite loads are 1 kΩ–1 GΩ; capacitance is 1 pF–1 mF. Every estimated resistor dissipation must remain at or below 0.125 W. That budget is not a component rating. Source impedance, tolerances, parasitics, capacitor ESR/leakage, temperature, and real-component derating require additional work.

## Verify a copied bundle

```sh
bin/kicadai --target ./out/native-divider schematic verify
```

Verification rejects changed, missing, unexpected, or non-regular files and checks the confirmed session. It establishes retained-byte integrity, not independent electrical correctness. Native tool reports preserve their original execution paths even after relocation.

## Answer behavioral compiler questions

For an existing `intent compile` result requiring clarification:

```sh
bin/kicadai --target ./out/prior-compilation --output ./out/answered intent clarify
```

The command asks the compiler-owned questions and writes a bound `answers.json` plus a fresh workspace. The original proposal, source, and compilation remain unchanged. Answers are checked against the exact source, capabilities, proposal and compilation before being accepted.

The output gives an explicit `intent compile` resume command. Live provider use remains a separate choice; collecting answers makes zero API requests. Supplying `--provider recorded --provider-record /path/to/response.json` resumes automatically with that offline response. Unsupported results remain unsupported, not silently retried or converted into generated designs.
