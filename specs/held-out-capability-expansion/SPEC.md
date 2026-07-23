# Held-Out Capability Expansion Specification

Date: 2026-07-23

## 1. Purpose

This milestone measures and expands KiCadAI's ability to generate unfamiliar
circuits from observable behavior rather than from prescribed implementations.
A versioned benchmark is frozen before its unsupported behaviors change any
production provider, component catalog, solver, simulator, lowering, placement,
routing, writer, or promotion code.

The benchmark spans analog, power, digital, MCU, sensor, and mixed-signal
designs. It contains both downstream controls and unsupported behavioral
families so the baseline distinguishes missing architecture capability from
regressions in already-supported physical realization.

## 2. Benchmark Trust Boundary

Each benchmark row contains:

- a domain and electrically meaningful family label used only for reporting;
- an independently worded behavioral request;
- a strict `kicadai.open-set-requirement.v3` file;
- SHA-256 hashes for the request text and requirement bytes; and
- a fixture role of `control` or `held_out`.

Requests and normalized requirements may state:

- external interfaces and direction;
- voltage, current, frequency, impedance, load, and environmental ranges;
- observable transfer, timing, startup, thermal, tolerance, and safety limits;
- protocol behavior; and
- manufacturing-neutral board size and component-count limits.

They must not state or imply:

- component, manufacturer, part, symbol, footprint, unit, pin, or pad choices;
- circuit topology, internal nets, schematic groups, block families, or
  provider identities;
- coordinates, regions, layers, tracks, vias, routes, zones, or keepouts;
- solver equations, primitive models, repair actions, expected evidence, or
  expected implementation values; or
- fixture-specific schemas, allowlists, production branches, or exceptions.

The manifest and all referenced requirement bytes are immutable for version 1.
Any membership, prompt, requirement, category, or hash change requires a new
benchmark version and a new frozen baseline.

## 3. Version 1 Coverage

Version 1 contains twelve cases, two in each required domain.

| Domain | Control | Held-out pressure |
| --- | --- | --- |
| analog | protected Class-A line amplification | bipolar precision rectification |
| power | regulated and protected sensor supply | bounded constant-current output |
| digital | protected level-translated interface | standalone clock generation |
| MCU | regulated MCU and sensor subsystem | constant-current peripheral drive |
| sensor | low-noise sensor decision chain | constant-current bridge excitation |
| mixed-signal | sensed protected load control | rectification followed by ADC drive |

The repeated pressures are intentional but the circuits are not duplicates:
constant-current regulation is exercised as a power output, MCU-controlled
peripheral, and sensor excitation source; precision rectification is exercised
alone and as one stage of a mixed-signal acquisition chain.

## 4. Stage Taxonomy And Baseline

Every case is evaluated through the same ordered stages:

1. `integrity` — manifest membership and byte hashes;
2. `schema` — strict decode and public requirement validation;
3. `intent` — behavioral neutrality and complete acceptance gates;
4. `architecture` — provider coverage, global contracts, deterministic search,
   alternatives, and evidence;
5. `component_evidence` — verified catalog identity, ratings, values,
   lifecycle, symbols, units, pins, footprints, and pads;
6. `simulation` — applicable trusted analyses, all declared corners, model
   provenance, assertions, and closed-loop evidence;
7. `lowering` — complete semantic bindings and deterministic circuit graph;
8. `schematic` — generated schematic and readability validation;
9. `placement` — complete, rule-valid placement;
10. `routing` — endpoint access, connectivity, and complete required-net
    routing;
11. `writer` — writer correctness and byte-stable project emission;
12. `erc` — clean installed-KiCad ERC;
13. `drc` — clean installed-KiCad strict DRC;
14. `round_trip` — zero normalized schematic and PCB differences; and
15. `replay` — byte-identical project replay.

The frozen baseline records the earliest authoritative blocker for every case,
using a stable stage, code, semantic capability, requirement path, and root
cause. A skipped external tool, missing model, unverified part, partial route,
or merely writable file is not a pass.

The report also records cumulative reach for every stage. Thus a case blocked
at `architecture` contributes passes only through `intent`, while a fully
promoted case contributes a pass to every stage.

## 5. Failure-Cluster Selection

Production work starts only after the baseline report and checksum are frozen.
Failure clusters are ranked deterministically by:

1. number of benchmark cases sharing the root capability and stage;
2. number of distinct benchmark domains affected;
3. safety-critical case count; and
4. lexicographic capability ID as the final tie-break.

At least the first two electrically distinct unsupported families must be
closed. A family is electrically distinct when its energy flow, transfer
behavior, and verification obligations are not merely parameter variants of
the other family.

## 6. Generic Closure Rules

A correction is acceptable only when it is reusable outside this corpus and is
expressed through public behavioral vocabulary, provider contracts, reviewed
component evidence, bounded deterministic solvers, trusted simulation
primitives, or generic physical-design algorithms.

Production code may not inspect benchmark IDs, paths, request text, hashes,
project names, family labels, expected results, or fixture roles. It may not
introduce benchmark-specific coordinates, block families, component lists,
allowlists, or fallback passes.

Provider selection, component evidence, calculations, search alternatives,
simulation plans, lowering, placement, routing, writer output, and reports must
be deterministic under semantically irrelevant input, catalog, and provider
reordering.

## 7. Promotion Requirements

Every newly supported family must have at least two independent proofs:

- a focused synthetic/mutation suite showing generic bounds, rejection, and
  deterministic behavior; and
- all held-out benchmark cases containing that family.

Every promoted held-out case must pass:

- strict decode and internal validation;
- deterministic architecture selection and component evidence;
- applicable trusted simulation and all declared corners;
- complete lowering, schematic generation, placement, and routing;
- writer correctness;
- clean installed-KiCad ERC and strict DRC;
- connectivity and route completion;
- zero schematic and PCB round-trip differences; and
- byte-identical project replay.

The existing promotion matrix, simulation-grounded corpus, adversarial corpora,
and protected `usb_c_led_indicator_protected` and
`usb_c_i2c_sensor_3v3_protected` fixtures must not regress.

## 8. Measurable Improvement

The final report is generated by the same evaluator and gate profile as the
baseline. Completion requires:

- more full passes than the frozen baseline;
- strictly greater cumulative stage reach;
- no new blocker in a control case;
- at least two electrically distinct newly supported families;
- no reduction or weakening of any acceptance gate; and
- preserved unsupported classifications for remaining genuine gaps.

## 9. Clean-Checkout Evidence

Each newly supported family must be included in a versioned promotion matrix
that can run from a clean checkout with only documented KiCad installation
inputs. The resulting bundle must include the immutable request, normalized
requirement, architecture/evidence hashes, simulation evidence, generated
KiCad project, ERC/DRC reports, connectivity/routing/writer/round-trip results,
replay comparison, tool identities, manifest, and checksums.

The bundle verifier must reject missing, modified, duplicated, unmanifested, or
path-escaping artifacts.

## 10. Completion Rule

This milestone is complete only when the frozen corpus, baseline, failure
ranking, generic implementation history, final report, promotion matrices,
clean-checkout bundles, and regression evidence are checked in and
reproducible; Prism has no unresolved high or medium findings; the exact commit
is pushed; and GitHub Actions is green.
