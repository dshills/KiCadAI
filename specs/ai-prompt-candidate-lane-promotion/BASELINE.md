# AI Prompt Candidate Lane Baseline

Date: 2026-07-03

Command:

```sh
kicadai --text "make a simple LED indicator board" --output examples/.generated/ai_led_prompt_candidate --overwrite intent create
```

Observed status:

- CLI exit: non-zero
- `data.ai_status.status`: `blocked`
- `data.ai_status.stage`: `placement`
- `data.ai_status.issue_code`: `PLACEMENT_OUTSIDE_BOARD`
- `data.ai_status.repair_category`: `placement_outside_board`
- `data.ai_status.retry_allowed`: `true`

Observed workflow stages:

- `block_planning`: `ok`
- `component_selection`: `warning`
- `schematic`: `ok`
- `schematic_electrical`: `ok`
- `pcb_realization`: `ok`
- `placement`: `blocked`
- `routing`: `skipped`
- `project_write`: `skipped`
- `writer_correctness`: `skipped`
- `validation`: `skipped`
- `kicad_checks`: `skipped`

Observed issues:

- `component_selection.power_header.connector`: warning,
  `block component has no component_id or component_query`
- `component_selection.connector.connector`: warning,
  `block component has no component_id or component_query`
- `design.inter_block_routing.connections[0].to`: warning,
  `connection endpoint does not resolve to a generated PCB pad`
- `components.J148c456e001.position`: error,
  `fixed placement is outside usable board area`

Interpretation:

- The natural-language draft and planner already reach a supported simple LED
  design.
- The schematic path is healthy enough for this lane.
- The concrete blocker is generated PCB placement for a fixed connector
  footprint outside the board usable area.
- The implementation should fix generated placement/board adaptation rather
  than weaken placement validation.

