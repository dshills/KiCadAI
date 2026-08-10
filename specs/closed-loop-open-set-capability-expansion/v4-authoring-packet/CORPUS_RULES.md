# V4 Rules Supplied to the Isolated Corpus Author

Every requirement describes externally observable electrical behavior only.
It may name semantic domains, ports, conditions/events, analyses, metrics,
bounds, board limits, and acceptance gates from the public contract.

It must not name or imply a topology, block, component, part, package, model,
footprint, symbol, net, layer, coordinate, route, placement, repair,
implementation algorithm, synthesis status, failure code, gap, capability,
rank, expected pass/failure, project fixture, or existing example.

Requirements must be independently conceived and must not be copied,
paraphrased, or transformed from a prior or sibling requirement. Discovery and
held-out files are authored before any evaluation, receive equal care, and may
not be assigned an expected outcome.

## Fixed balance

The manifest assigns exactly two cases per role for analog, power, digital,
MCU, sensor, and mixed-signal reporting. Within each role, the aggregate must
include:

- single-positive, bipolar, and multiple-supply declarations;
- voltage-, current-, and power-observed behavior;
- at least three requirements with multiple observed source ports;
- at least three requirements with distinct external excitations converging on
  one observed source port;
- DC, AC/noise/stability, transient/startup, and thermal/electrothermal
  analyses;
- load, tolerance/model, temperature, and supply variation;
- input/load/power steps, startup, rail-loss, and short-circuit events; and
- critical assertions in at least three reporting domains.

Each file has at least two operating cases, four behavioral assertions, and two
analysis kinds. Every one of the 14 acceptance fields is present and true.

## Public electrical sanity review

Avoid mathematical ideals such as a zero-ohm environmental load unless the
behavior explicitly studies a bounded short-circuit event. Using electrical
and schema meaning only, review unit consistency, finite physical bounds,
reference use, energy conservation, event envelopes, and compatibility of
simultaneous all-corner requirements.

An adversarial requirement must remain a bounded behavioral safety case and
must not disclose its expected classification. Do not run, consult, or predict
KiCadAI behavior.
