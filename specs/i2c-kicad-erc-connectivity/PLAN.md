# I2C KiCad ERC Connectivity Closeout Implementation Plan

## Phase 1: Baseline ERC Evidence Capture

### Goals

- Freeze the current I2C KiCad ERC blocker shape.
- Make the failing generated schematic objects easier to inspect.

### Tasks

- Add or update a focused I2C design-example test that extracts the
  `kicad_checks` stage issues for `i2c_sensor_breakout_candidate`.
- Assert the current blocker family without overfitting to every KiCad wording
  detail:
  - disconnected labels;
  - unconnected pins;
  - off-grid connection points;
  - unconnected wire endpoints.
- Persist or expose generated schematic artifact paths in the test failure
  output.
- Add helper assertions that distinguish candidate-blocking ERC findings from
  warning-only writer-correctness/library gaps.
- Record whether the evidence came from fake KiCad, real KiCad, or internal
  structural checks.

### Acceptance

- The test fails if the fixture blocks for stale route-tree/project-write
  reasons instead of ERC.
- The test fails if ERC evidence disappears without candidate promotion.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|ERC|Promotion|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Capture I2C KiCad ERC blocker shape`.

## Phase 2: Generated Schematic Geometry Audit

### Goals

- Map each ERC connectivity finding back to generated schematic objects and
  writer decisions.

### Tasks

- Add a schematic inspection helper or test utility that summarizes:
  - labels and their connected components;
  - wire endpoints not on symbol pins, labels, junctions, or no-connect markers;
  - off-grid symbol pin anchors, label positions, and wire endpoints;
  - symbol pins with no connected wire/label/no-connect marker.
- Run the helper against the generated I2C schematic in tests.
- Add targeted diagnostics to the design-example failure formatting when
  generated schematic connectivity fails.
- Identify whether failures come from:
  - label stubs;
  - block-local generated labels;
  - connector symbol pin geometry;
  - I2C sensor pin geometry;
  - power symbols/no-connect behavior;
  - long-net alias routing.

### Acceptance

- Diagnostics name specific generated object classes and references.
- No route-tree or PCB writer behavior changes in this phase.
- Focused tests pass:

  ```sh
  go test ./internal/schematicrules ./internal/designworkflow -run 'I2C|Schematic|ERC|Connectivity' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Diagnose I2C schematic ERC geometry`.

## Phase 3: Grid-Safe Label Stub And Wire Emission

### Goals

- Ensure generated schematic labels and stubs are valid KiCad ERC geometry.

### Tasks

- Review the design API `Connect` and label-stub emission path.
- Ensure generated label stubs:
  - start at exact pin anchors or existing wire points;
  - end on the schematic connection grid;
  - do not cross unrelated net geometry;
  - do not leave dangling endpoints unless the endpoint is the label location;
  - add junctions where a stub intentionally meets an existing wire segment.
- Add unit tests for:
  - long labeled net between two symbols;
  - generated `io_` alias net;
  - adjacent connector pins with separate labels;
  - vertical and horizontal pin orientations;
  - no off-grid endpoints for 1.27 mm and 2.54 mm pin grids.
- Keep schematic readability rules intact.

### Acceptance

- Internal schematic rules no longer report off-grid label/wire endpoints for
  generated I2C labels.
- No schematic label conflicts are introduced.
- Focused tests pass:

  ```sh
  go test ./internal/kicadfiles/designapi ./internal/schematicrules ./internal/designworkflow -run 'Label|Stub|I2C|ERC|Schematic' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Make generated schematic label stubs ERC safe`.

## Phase 4: Pin Connectivity And Intentional No-Connects

### Goals

- Remove KiCad ERC `Pin not connected` findings for required generated I2C
  schematic pins without hiding real required nets.

### Tasks

- Audit generated symbol pin specs for:
  - connector breakout;
  - generic I2C sensor;
  - decoupling capacitor;
  - pull-up resistors;
  - power symbols.
- Confirm every required pin has one of:
  - a connected wire;
  - a connected net label;
  - an intentional no-connect marker;
  - metadata declaring it not emitted/required.
- Add block-level pin intent metadata where missing.
- Add no-connect markers only for genuinely unused pins.
- Add tests proving no-connect markers sit on exact symbol pin anchors.

### Acceptance

- Generated I2C schematic has no unintentional disconnected pins in structural
  schematic checks.
- Required VCC/GND/SDA/SCL pins remain connected.
- Focused tests pass:

  ```sh
  go test ./internal/blocks ./internal/schematicrules ./internal/designworkflow -run 'I2C|Pin|NoConnect|ERC' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Close I2C schematic pin connectivity gaps`.

## Phase 5: Fake KiCad ERC Promotion Gate

### Goals

- Prove candidate promotion behavior with deterministic KiCad ERC evidence
  without requiring local KiCad in default tests.

### Tasks

- Update fake KiCad CLI fixtures for the I2C candidate:
  - clean ERC path;
  - ERC connectivity failure path;
  - optional missing-KiCad path if still relevant.
- Assert promotion gate behavior:
  - clean ERC supports `candidate`;
  - dirty ERC remains `expected_fail`/blocked with precise issue codes;
  - missing optional evidence is warning-only only when the metadata policy
    allows it.
- Ensure report artifact paths are preserved in promotion reports.

### Acceptance

- Candidate readiness cannot be claimed without required ERC evidence.
- Clean fake ERC promotes I2C as far as metadata allows.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|KiCad|ERC|Promotion|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Prove I2C KiCad ERC promotion policy`.

## Phase 6: Metadata And Documentation Update

### Goals

- Align checked-in fixture metadata and docs with the achieved readiness.

### Tasks

- If KiCad ERC is clean enough for candidate:
  - update `i2c_sensor_breakout_candidate.metadata.json` readiness to
    `candidate`;
  - remove ERC connectivity known gaps;
  - document remaining pass-level DRC and broader rich-board coverage gaps.
- If ERC still blocks:
  - keep readiness as `expected_fail`;
  - update known gaps with exact remaining findings and source object classes.
- Update:
  - `examples/design/kicad-backed/README.md`;
  - `README.md`;
  - `docs/layout-routing.md`;
  - `specs/ROADMAP.md`.
- Remove stale claims that route-tree contact proof, project-write,
  writer-correctness, or structural validation are active I2C blockers.

### Acceptance

- Metadata matches promotion report behavior.
- Docs identify the current blocker precisely.
- Focused tests pass:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|Promotion|DesignExamples' -count=1
  ```

### Review And Commit

- Run `prism review staged`.
- Commit: `Update I2C ERC promotion metadata`.

## Phase 7: Full Regression

### Goals

- Confirm the repo remains stable after schematic ERC closeout work.

### Tasks

- Run:

  ```sh
  go test ./internal/designworkflow ./cmd/kicadai -run 'I2C|ERC|ProjectWrite|WriterCorrectness|Validation|KiCad|Promotion|DesignExamples' -count=1
  go test ./...
  ```

- Check `git status --short`.
- Run Prism on staged changes if any remain.

### Acceptance

- Focused and full tests pass.
- Worktree is clean except intentional follow-up specs.
- The current I2C readiness state is documented and defensible.

### Review And Commit

- Run `prism review staged`.
- Commit: `Run I2C ERC connectivity regression`.
