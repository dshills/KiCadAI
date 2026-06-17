Major Gaps
1. Component Intelligence
We need a real component database, not just symbol names and footprint strings.
Needed:
verified symbol-to-footprint mappings for many components;
pin function maps, electrical types, package variants, and polarity rules;
manufacturer part numbers and availability metadata;
passive defaults: resistor/capacitor packages, voltage ratings, tolerances;
connector families and pin numbering conventions;
power components, regulators, op-amps, MCUs, sensors, crystals, USB, RF modules, etc.
Current state:
basic pinmap registry exists;
only a small built-in set is verified;
hierarchy is blocked for pinmap validation.
2. Circuit Design Knowledge
The AI needs domain-specific circuit templates and design rules.
Examples:
regulator circuits: input/output caps, enable pins, thermal constraints;
op-amp circuits: gain, biasing, stability, rails, output drive;
MCU breakouts: decoupling, reset, boot pins, programming headers;
USB/UART/I2C/SPI/CAN circuits;
sensor front ends;
protection: ESD, fuses, reverse polarity, level shifting;
analog layout constraints.
Current state:
LED and breakout flows exist;
no broad library of reusable circuit blocks yet.
3. Schematic Quality Rules
We need stronger schematic-level validation.
Needed:
ERC-like checks independent of KiCad GUI;
power net sanity;
unconnected required pins;
missing decoupling capacitors;
inconsistent labels;
duplicate references across hierarchy;
incorrect symbol unit handling;
component value/rating sanity;
net naming conventions;
hierarchical sheet traversal.
Current state:
readers and validators catch some structural issues;
generated connectivity validation exists in parts;
full ERC-equivalent behavior is not there.
4. PCB Placement Intelligence
Autonomous PCB quality depends heavily on placement.
Needed:
component grouping by circuit function;
connector edge placement;
power path placement;
decoupling cap proximity rules;
crystal/oscillator proximity rules;
analog/digital separation;
thermal placement;
keepout zones;
mounting holes and mechanical constraints;
board outline constraints from user intent.
Current state:
footprints can be placed;
generated examples use simple deterministic placement;
no general placement optimizer yet.
5. Routing Intelligence
Simple route segments are not enough for serious boards.
Needed:
net class rules;
trace width/current calculations;
differential pairs;
length matching;
via selection;
layer stack awareness;
copper pours and thermal relief rules;
clearance rules;
routing around obstacles;
autorouting or guided routing engine;
DRC feedback loop.
Current state:
routes, vias/zones in writer model are partly supported;
no real router or iterative DRC-driven repair loop yet.
6. Full Footprint Library Handling
We need to consume actual KiCad footprints instead of generating minimal footprints.
Needed:
parse .kicad_mod;
load global/project footprint libraries;
resolve fp-lib-table;
instantiate real footprint pads, graphics, courtyard, fab, silk, 3D models;
preserve pad geometry exactly;
support footprint variants.
Current state:
writer can create footprint objects;
imported footprint mutation is conservative;
no full footprint-library expander yet.
7. Full Symbol Library Handling
Same issue for symbols.
Needed:
parse .kicad_sym;
resolve sym-lib-table;
use real pin geometry, units, alternate bodies;
support power symbols and multi-unit parts correctly;
preserve embedded symbols.
Current state:
schematic writer can write symbols;
reader handles symbols enough for inspection/mutation;
no comprehensive symbol-library resolver yet.
8. Round-Trip Preservation Completeness
For modifying existing user projects, we need broader preservation.
Needed:
preserve every unknown KiCad node;
preserve ordering-sensitive sections;
preserve formatting where practical;
support child sheets;
preserve advanced PCB objects;
detect unsupported objects before mutation;
avoid rewriting unrelated files.
Current state:
preservation work exists;
unsupported content is often blocked rather than rewritten;
still incomplete for complex real projects.
9. KiCad CLI Validation Loop
Autonomy needs a closed loop: generate, run KiCad validation, read errors, fix, repeat.
Needed:
automatic schematic ERC through KiCad CLI where available;
PCB DRC;
netlist export/compare;
gerber/drill generation checks;
BOM generation checks;
round-trip diff checks;
parse KiCad reports into actionable issues;
repair planner.
Current state:
round-trip infrastructure exists;
some KiCad CLI checks are opt-in;
no autonomous repair loop yet.
10. Intent-to-Design Planner
This is the actual AI orchestration layer.
Needed:
parse user requirements into constraints;
select circuit blocks;
choose components;
calculate values;
generate schematic transaction;
assign footprints;
place board;
route board;
validate;
revise;
produce final explanation and artifacts.
Current state:
CLI and transaction substrate exists;
no full agent planner yet.
11. Manufacturing Readiness
A board is not “done” until fabrication outputs are correct.
Needed:
net classes;
board stackup;
design rules;
solder mask/paste settings;
edge cuts validation;
mounting holes;
silkscreen cleanup;
courtyard/collision checks;
BOM/CPL export;
Gerbers, drill files, position files;
fabrication package manifest.
Current state:
export command is still a placeholder;
PCB writer has many object correctness tests, but not full fab packaging.
Most Important Next Projects
Symbol and Footprint Library Resolver
This unlocks real KiCad-native components instead of approximations.

Circuit Block Library
Build reusable verified blocks: LED, regulator, MCU minimal system, USB-C power, I2C sensor, op-amp gain stage, connector breakout.

ERC/DRC Feedback Loop
Run KiCad CLI checks, parse results, and convert them into repairable structured issues.

Placement Engine
Start deterministic and rule-based before attempting optimization.

Routing Engine
Begin with simple two-layer Manhattan routing plus net classes and obstacle checks.

Fabrication Export
Generate and validate Gerbers, drills, BOM, CPL, and a readiness report.

AI Planner
Only after the above pieces exist should the AI be allowed to go from “I need a board that does X” to full schematic + PCB without human intervention.

Most Important Next Projects
Symbol and Footprint Library Resolver
This unlocks real KiCad-native components instead of approximations.

Circuit Block Library
Build reusable verified blocks: LED, regulator, MCU minimal system, USB-C power, I2C sensor, op-amp gain stage, connector breakout.

ERC/DRC Feedback Loop
Run KiCad CLI checks, parse results, and convert them into repairable structured issues.

Placement Engine
Start deterministic and rule-based before attempting optimization.

Routing Engine
Begin with simple two-layer Manhattan routing plus net classes and obstacle checks.

Fabrication Export
Generate and validate Gerbers, drills, BOM, CPL, and a readiness report.

AI Planner
Only after the above pieces exist should the AI be allowed to go from “I need a board that does X” to full schematic + PCB without human intervention.
------
Footprint Library Expansion (DONE)
Build real footprint geometry from /Users/dshills/Development/external/kicad-footprints instead of relying on transaction payload/default pad summaries. This unlocks accurate pads, holes, courtyard/clearance, layers, and better routing/DRC.

Schematic to PCB Transfer (DONE)
Add a workflow that takes schematic symbols + assigned footprints + nets and produces an initial PCB transaction automatically. This is the bridge from “schematic writer works” to “AI can generate a whole board.”

Connectivity-First Board Validation (DONE)
Strengthen validation around net-to-pad correctness, unrouted nets, route completion, zone connectivity, and DRC evidence. The goal is to catch “looks fine but electrically wrong” boards.

Circuit Block PCB Realization (DONE)
Take the verified circuit blocks we built and make them produce schematic + PCB fragments: placements, footprints, routed local connections, and required constraints.

AI Design Workflow CLI
Add a higher-level command like:
kicadai design create --request request.json --output ./out/project
It should orchestrate block selection, schematic generation, footprint assignment, placement, routing, ERC/DRC, and feedback.
