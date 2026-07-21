# Simulation-Grounded Closed-Loop Synthesis

KiCadAI can synthesize and promote circuits from behavior-only requirements
inside a bounded, reviewed envelope. The workflow generates deterministic
architecture alternatives, resolves catalog-backed components and models,
runs registered analyses over declared operating cases, diagnoses failed
assertions, applies bounded generic repairs, and reruns every required gate.

Promoted evidence can include DC operating point, AC response, noise,
stability, transient/startup behavior, distortion, thermal limits, and
explicit supply/load/temperature/tolerance/model corners. Model identity,
source revision, immutable hash, review status, applicability, decisions,
repairs, budgets, and final selection are retained in replayable artifacts.

The provider may request behavior and operating conditions. It may not provide
equations, solver settings, model files, arbitrary expressions, executable
content, route coordinates, or repair code. Missing trust, unsupported
analysis, ambiguity, nonconvergence, exhausted budgets, failed assertions, or
failed physical/KiCad gates block promotion.

The frozen ten-circuit corpus includes low-noise analog, active filtering,
regulated and protected interfaces, mixed control/power, Class-A, and
Class-AB designs. The measured Class-A and Class-AB lanes pass behavioral
assertions, complete routing/connectivity, writer correctness, deterministic
replay, zero normalized round trips, and installed-KiCad ERC/strict DRC.

This evidence does not establish arbitrary topology, arbitrary parts,
unrestricted SPICE compatibility, parasitic-accurate simulation, RF/high-speed
layout, mains safety, or fabrication release. New capability should be added
through reviewed catalog/model coverage and new neutral held-out failures.

See the [specification](../specs/simulation-grounded-closed-loop-synthesis/SPEC.md)
and [completion audit](../specs/simulation-grounded-closed-loop-synthesis/AUDIT.md).
