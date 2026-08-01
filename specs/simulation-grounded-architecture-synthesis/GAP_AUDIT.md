# Simulation-Grounded Architecture Synthesis Gap Audit

Date: 2026-07-31

This document records the pre-implementation gap. The completed result and
reproducible receipts are in [AUDIT.md](AUDIT.md) and
[PROMOTION_MATRIX.json](PROMOTION_MATRIX.json).

## Existing Foundation

KiCadAI already has a bounded, provider-independent open-topology lane. It
accepts behavior-only requirements, derives a reviewed primitive inventory,
generates canonical terminal graphs, searches catalog-backed values, runs
trusted operating-point/AC/transient/noise/distortion/stability/thermal/SOA
analyses, applies graph-changing repair, and promotes passing graphs through
the standard KiCad workflow.

The first frozen corpus proves six complete installed-KiCad passes from eight
requirements. This is the correct foundation for architecture synthesis; a
parallel template or block-family selector would weaken the evidence.

## Gaps Against This Goal

| Goal clause | Current evidence | Gap |
| --- | --- | --- |
| Generate multiple architectures | Search retains diverse topology hashes and evaluates them round-robin | Synthesis returns the first physically ready pass instead of ranking all passing topologies |
| Op-amp, BJT, MOSFET, passive, and protection combinations | Reviewed primitive/model families exist | Search relationships do not yet construct enough materially different amplifier output/bias/protection structures from requirements |
| Derived component values | Preferred-series domains and several analytic seeds exist | Amplifier bias, standing current, output swing, emitter/source resistance, compensation, and protection equations need explicit provenance |
| Full SPICE evidence | Trusted analysis registry covers the required analysis kinds | Held-out amplifier requirements do not yet prove all analysis kinds and corners through the primitive-only lane |
| Rating, bias, thermal, and performance rejection | Static ratings, thermal RC, SOA, and assertion rejection exist | Amplifier-specific headroom, safe bias, dissipation, load drive, and crossover/stability rejection need held-out proof |
| Explain winning architecture | Candidate/attempt evidence is retained | Selection summary currently states only that the first deterministic pass won |
| Readable KiCad output | Generic topology-aware schematic layout and standard physical promotion exist | New amplifier structures must prove readable lowering without fixture coordinates |
| Three held-out demonstrations | Existing corpus covers eight analog/power/mixed-signal behaviors | It has no independently frozen Class A, Class AB, and non-amplifier trio for this milestone |

## First Remediation

The first production change replaces first-pass selection with deterministic
ranking of every physically ready passing topology reached inside the declared
budgets. Ranking uses, in order:

1. worst normalized requirement margin, descending;
2. topology-repair count, ascending;
3. component count, ascending;
4. internal-node count, ascending;
5. topology, evaluation, and value hashes as stable tie-breakers.

The report retains every distinct passing topology, its score inputs, and an
explicit selection explanation. Repairs run only when the initial topology and
value evaluation produces no physically ready pass, preventing repair work from
preempting comparison of already viable architectures.

## Non-Goals

This milestone does not claim unrestricted parts, arbitrary vendor SPICE
models, RF/high-speed design, mains/high-energy safety, mechanical thermal
design, or fabrication release. Missing reviewed evidence must continue to
fail closed.
