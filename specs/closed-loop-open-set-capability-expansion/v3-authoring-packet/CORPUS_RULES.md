# V3 Corpus Rules Supplied to the Isolated Author

Every requirement describes externally observable electrical behavior only.
It may name semantic domains, ports, operating conditions/events, analyses,
metrics, bounds, board limits, and acceptance gates from the public contract.

It must not name or imply a topology, block, component, part, package, model,
footprint, symbol, net, layer, coordinate, route, placement, repair,
implementation algorithm, expected synthesis status, failure code, capability
cluster, rank, expected pass, project fixture, or existing example.

Requirements must be independently conceived and must not be copied,
paraphrased, or transformed from any prior or sibling requirement. Discovery
and held-out files are authored before any evaluation and receive equal care.

The fixed manifest requires exactly two cases per role for analog, power,
digital, MCU, sensor, and mixed-signal reporting. Within each role, the corpus
as a whole must include:

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
analysis kinds. All 14 acceptance fields are present and true.

Avoid mathematical ideals such as a zero-ohm environmental load unless the
behavior explicitly studies a bounded short-circuit event. Perform a public
electrical sanity review for unit consistency, finite bounds, reference use,
energy conservation, and compatible all-corner requirements. Do not run or
consult KiCadAI.
