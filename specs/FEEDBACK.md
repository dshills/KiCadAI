# KiCadAI Test Session Feedback — 2026-07-21

Tester context: I ran a six-circuit ladder of increasing complexity, authoring every
request myself (no checked-in fixture replayed as a deliverable). Binary built from
`main` @ `962476be` ("docs: refresh project status and workflows"). KiCad 10.0.3,
stock macOS app libraries. My previous session (2026-07-14) required four source
patches to generate one Class A amp; this session is the comparison point.

## Ladder Results

| # | Circuit | Lane | Result | Evidence |
|---|---------|------|--------|----------|
| T1 | 5 V LED indicator + power header | `design create` (blocks) | PASS at 60×40 (fails 40×25, see F3) | writer ✓, board ✓, ERC 0, DRC warn-only |
| T2 | Two-pole RC low-pass, 6 parts | `circuit create` (explicit graph) | PASS (after symbols-root workaround, see F2) | writer ✓, board ✓, ERC 0, DRC warn-only |
| T3 | USB-C → AP2112K → BMP280 + LED | `intent plan` / `intent create` | plan: **ready, score 100**; create: **FAIL** (see F1) | plan calcs correct (650 Ω LED, 4.7 k I2C pull-ups) |
| T4 | LM358 Sallen-Key LPF, multi-unit A/B/P, two follower self-loops | `circuit create` | **PASS first try** | ERC 0, DRC warn-only, exactly one SOIC-8 on PCB |
| T5 | Split-supply Class A headphone amp (the 2026-07-14 design) | `circuit create` | **PASS first try at `erc-drc` acceptance** | ERC 0, DRC warn-only; legible schematic |
| T6 | Audio-activity LED (LMV321 amp + NPN switch + LED), **function-level**: no pins, no coordinates, no board dims | `circuit create` (synthesis) | **PASS** | ERC 0, **DRC 0 violations, 0 unconnected** — cleanest board of the session |

Artifacts: `requests/*.json` (inputs), `out/t*/` (projects), `out/t5_class_a/render/`,
`out/t6_audio/render/` (PNG/PDF renders).

## What Improved Since 2026-07-14 (all four of my old patches superseded properly)

1. **`circuit create` / `circuit preflight`** is exactly the deterministic authoring lane I
   previously had to fake with a captured `ai-response.json` replay. Iteration is seconds,
   free, and reproducible. This is the single biggest DX win.
2. **Multi-unit waypoint bug fixed at root.** T4 deliberately used two voltage-follower
   self-loops (`A.OUT→A.IN_MINUS`, `B.OUT→B.IN_MINUS`) on one LM358 — the exact trigger of
   the old "waypoints must start and end at connected pin anchors" failure. Clean first try.
   Removing my `reanchorSchematicWaypoints` workaround was safe.
3. **Split supply is first-class.** `schematic.lanes.power_negative` is in the schema (with
   legacy-fixture tolerance in `parse.go`). T5 ran at `erc-drc` acceptance — last time I had
   to patch the lowering code and drop to `connectivity`.
4. **Token cap** became `--ai-max-output-tokens` with per-profile defaults.
5. **BJT catalog fix kept** (`Transistor_BJT:Q_NPN_BEC`).
6. **Schematic readability is transformed.** T5's schematic renders as a reviewable,
   label-based drawing: every ref/value/net legible, correct topology. The 07-14 output was
   overlapping text soup. Remaining nit: the two LM358 unit bodies crowd each other slightly.
7. **Intent planning quality.** T3's plan: status `ready`, score 100, correct engineering
   math applied ((3.3−2.0)/2 mA → 650 Ω; I2C pull-up policy → 4.7 k), regulator headroom
   correctly deferred rather than guessed.
8. **Function-level synthesis (T6) is the standout.** I declared 7 functions, 2 interfaces,
   8 port-level connections — no pins, no coordinates, no board outline, no layout section.
   The synthesizer selected parts, added the support components I never mentioned (LMV321
   decoupling), sized the board, placed, routed, and produced the only **zero-violation DRC**
   board of the session. ERC 0 as well.

## Bugs / Regressions Found

### F1 (P1): `intent create` placement is broken for regulator compositions — including a checked-in fixture
- `examples/intent/regulator_ap2112k_sensor.json` (verbatim, clean worktree at HEAD,
  clean catalog, clean binary) fails `intent create` with 11 errors: regulator-block
  components get `PLACEMENT_OUTSIDE_BOARD` ("fixed placement is outside usable board
  area") and `PLACEMENT_COLLISION` against keepout `usb_power.usb_c_power_entry_placement`,
  then `translate_as_unit` gives up for `regulator.regulator_core` and `sensor.sensor_core`.
- Not board-size dependent: same failure at 45×30 (fixture), 50×35, and 80×60.
- Not USB-C-only: swapping the input to `external` still fails on the regulator block's
  fixed placements.
- Isolation ladder: sensor-only fixture (`sensor_bmp280_breakout.json`) places and routes
  but ends with one "Tracks crossing" validation error; add regulator → placement collapse.
- **Test-coverage gap that hides it:** `TestRunIntentPlanRegulatorEvidenceFixtures` covers
  `intent plan` only. Nothing exercises `intent create` for this fixture, so the regression
  is invisible to the suite (it passed while the CLI failed in front of me).

### F2 (P1): Library-wide symbol lint blocks unrelated designs — quickstart broken on stock KiCad 10 libs
- `circuit create` with `--symbols-root` pointed at the stock KiCad 10.0.3 app libraries
  fails **any** design — including the checked-in `examples/circuit-graph/rc_filter.json` —
  on errors in symbols the design never references: "duplicate pin number" in
  `Video:TDA1950`, `MCU_Texas:TMS320LF2406`, `74xx_IEEE:74LS49` (KiCad-legitimate stacked
  pins), and "hidden power pin requires explicit connectivity policy" in `Converter_DCDC:*`,
  `Regulator_Linear:TPS7A4701xRGW`.
- Workaround I used: a hand-curated symbols dir symlinking only Device, Connector_Generic,
  Amplifier_Operational, Transistor_BJT, power, Diode, Sensor_Pressure.
- Suggested fix: scope symbol-level lint to symbols actually resolved by the design (or
  demote whole-library findings to warnings). Right now the documented flow fails out of
  the box for anyone pointing at their real KiCad install.

### F3 (P2): Placement density is very conservative; block-lane fixed placements don't scale down
- T1 (one 1×02 header + LED + resistor, `skip_routing: false`) fails on a 40×25 board —
  the same size the checked-in LED example uses (that one skips routing) — with
  `PLACEMENT_OUTSIDE_BOARD` on the LED block's fixed offsets and a group-translation
  failure. Works at 60×40. Three components needing 2,400 mm² suggests fixed block
  offsets + group translation give up too early.
- Related observation (T5): on my 80×60 board the placer packed everything into the
  top-left ~25×20 mm and largely ignored my five PCB regions. Functional (DRC-clean),
  but region adherence looks advisory in practice.

### F4 (P3): JSON output hygiene
- `circuit create` / `intent create` interleave large non-JSON output with the JSON
  payload on stdout (one run emitted ~480 k lines; `jq` choked twice — I had to salvage
  with `raw_decode`). Logs should go to stderr, or `--format json` should guarantee a
  single JSON document on stdout.

### F5 (P3): Function-level lane discoverability
- The synthesis lane's validators are precise but sequential — I hit four rounds of
  fail-closed errors (power_domains field shape → explicit constraints → explicit policy
  → GND external-interface binding) before the first success, each round revealing one
  more requirement. All shapes exist only in `internal/circuitgraph/testdata/function_corpus/`.
  A documented complete minimal example (or a `capability generation` excerpt with the
  synthesis contract) would collapse that loop to one round.
- Minor CLI nits: `kicadai circuit --help` errors instead of printing usage (must learn
  subcommands from the error text); the `usage` vocabulary for functions
  (`noninverting_amplifier`, `low_side_switch`, …) is only discoverable by grepping testdata.

### F6 (P3): Evidence-artifact asymmetry between lanes
- The provider lane writes `.kicadai/design-promotion.json`, `validation-summary.json`,
  `workflow-result.json`; `circuit create` writes only `manifest.json` + `transaction.json`.
  Skill/agent workflows keyed on promotion artifacts can't treat the lanes uniformly.

## Overall Assessment

Night-and-day versus a week ago. Last session I spent the day patching the tool to produce
one amplifier at `connectivity` acceptance with warnings; today the same amplifier passed
`erc-drc` acceptance on the first attempt, a harder multi-unit analog design passed on the
first attempt, and the new function-level lane produced a DRC-perfect board from a
port-level description. The explicit-graph lane is now a genuinely usable deterministic
EDA interface for an agent: author JSON → preflight → create → check, all in seconds.

The two P1s are the priorities: F1 because a checked-in intent fixture no longer survives
its own create path (and the test suite doesn't know), F2 because it breaks the
out-of-box experience against a stock KiCad install. Neither touches the new lanes'
core quality, which is excellent.
