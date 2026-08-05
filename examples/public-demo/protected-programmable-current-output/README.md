# Protected Programmable Current Output

This is KiCadAI's featured public demonstration: a behavior-only request for a
0.1 A/V programmable current output, evaluated across supply, load, startup,
temperature, and safe-operating-area constraints.

The input contains no components, topology, internal nets, values, placement,
or routes. KiCadAI derives and ranks physical architectures, selects catalog
parts and values, generates native KiCad files, routes the board, invokes
KiCad ERC and strict DRC, checks connectivity and writer round trips, and then
repeats the complete physical run to prove deterministic output.

![Generated 3D PCB](assets/pcb-3d.png)

## Run It

From the repository root, with KiCad 9 or newer installed:

```sh
make public-demo
```

The script detects the standard macOS or Linux KiCad installation. Override
discovery when necessary:

```sh
KICADAI_KICAD_CLI=/path/to/kicad-cli \
KICADAI_SYMBOLS_ROOT=/path/to/kicad-symbols \
KICADAI_FOOTPRINTS_ROOT=/path/to/kicad-footprints \
  make public-demo
```

The run writes ignored scratch output under
`examples/.generated/public-demo/protected-programmable-current-output/`.
Expect roughly one minute on a current laptop and allow about 1 GB for the
complete topology-search evidence. Open the `run-1/*.kicad_pro` file in KiCad.

To exercise the fail-closed path with an excessive 48 V to 5 V, 4 A,
85 degrees C requirement:

```sh
make public-demo-refusal
```

The underlying `kicadai` command is expected to exit unsuccessfully. The
wrapper verifies that no KiCad project was emitted and then exits successfully
when the refusal contract holds. The public command reports the bounded
search/capability refusal; the frozen nonlinear-switching promotion corpus
additionally checks that this case contains thermal, SOA, or rated-envelope
rejection evidence.

## What Was Proven

| Gate | Result |
|---|---:|
| Candidate architectures | 2 complete, both simulation-passing |
| Search work | 43 graphs, 194 simulations, 6,326 corner evaluations |
| Selected physical circuit | 12 synthesized primitives plus 3 interfaces |
| Placement and routing | 15/15 placed, 12/12 nets routed |
| Connectivity | 0 unconnected endpoints or DRC unconnected items |
| Writer correctness | 10/10 checks passed |
| KiCad 10.0.3 ERC | 0 violations |
| KiCad 10.0.3 strict DRC | 0 violations |
| Round trip | 0 normalized differences |
| Determinism | 2 runs, identical project hash |

The compact, checked-in [evidence receipt](evidence/receipt.json) records the
content hashes, competing architecture ranks, selected parts, and individual
verification gates. Reproduction retains the full raw synthesis and promotion
reports locally in `.kicadai/`; those reports are hundreds of megabytes and
are intentionally not duplicated in Git.

## Inspect Without Running

- [Behavior-only requirement](requirement.json)
- [Compact evidence receipt](evidence/receipt.json)
- [Fail-closed evidence receipt](evidence/refusal-receipt.json)
- [Native KiCad project](generated/protected_programmable_current_output.kicad_pro)
- [Native schematic](generated/protected_programmable_current_output.kicad_sch)
- [Native PCB](generated/protected_programmable_current_output.kicad_pcb)
- [Generated transaction provenance](generated/provenance/transaction.json)
- [60-second video storyboard](VIDEO_STORYBOARD.md)
- [Evidence interpretation and boundaries](EVIDENCE.md)

The generated schematic and PCB are evidence-bearing machine output, not a
hand-redrawn marketing illustration. The board image above is a KiCad render
of the checked-in PCB.

## Boundary

This result is an ERC/DRC-checked deterministic candidate, not a fabrication
release. Human electrical, thermal, mechanical, compliance, and manufacturing
review is still required before building hardware.
