# AI-Controlled Generation Lane Baseline

Date: 2026-07-03

## Summary

The prompt-driven intent path already exists, but the first-lane natural
language prompts do not yet map to supported design blocks. The current command
pipeline can draft, plan, persist rationale, and fail closed, but a prompt such
as "make a simple LED indicator board" currently becomes a generic
`custom_structured` intent with no blocks.

## Commands Checked

```sh
kicadai --json --text "make a simple LED indicator board" \
  --output examples/.generated/ai_lane_led_baseline \
  --overwrite intent create
```

Current result:

- status: blocked;
- draft kind: `custom_structured`;
- generated request has no blocks;
- blocking issue: `blocks`: "at least one block is required";
- blocking issue: `intent`: "intent did not map to any supported circuit
  blocks";
- rationale artifact is persisted under `.kicadai/`.

```sh
kicadai --json --text "make a connector breakout with power LED" \
  --output examples/.generated/ai_lane_connector_led_baseline \
  --overwrite intent create
```

Current result:

- status: blocked;
- draft kind: `breakout`;
- generated request has no blocks;
- blocking issue: `blocks`: "at least one block is required";
- blocking issue: `intent`: "intent did not map to any supported circuit
  blocks";
- rationale artifact is persisted under `.kicadai/`.

## Existing Strengths

- `intent draft` can persist:
  - `intent-source.txt`;
  - `intent-draft.json`;
  - `intent-extraction.json`;
  - `intent-clarifications.json`.
- `intent create --text` can persist prompt-driven artifacts under
  `.kicadai/`.
- blocked runs fail closed instead of inventing unsupported circuits.
- rationale output already includes known limits and next actions.

## First True Gap

The shortest next implementation step is deterministic prompt-to-intent mapping
for first-lane design families:

- LED indicator;
- connector breakout with LED;
- I2C sensor breakout.

The AI-facing status summary and artifact manifest should then wrap the
existing plan/create evidence so external agents can decide whether to proceed,
retry, or ask for clarification.
