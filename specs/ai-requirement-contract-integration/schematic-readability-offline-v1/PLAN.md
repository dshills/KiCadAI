# Offline schematic readability and sealed replay

Approved on 2026-09-11 after the preceding phase's retained readability failure. Base commit `b9d48213aa0431d5cf19824287c6403a4aea5529`; local branch `codex/schematic-readability`.

## Scope and invariants

Fix reusable schematic generation/layout behavior responsible for unreadable native output, and seal generated-project inventories before rendering from disposable copies. Preserve the preceding phase and all historical frozen inputs, negative outcomes, raw artifacts and seals. Do not manually edit generated schematics, parts, net connectivity, PCB placement or routes. No live provider call, new live campaign, external review, push, PR, merge, release or fabrication is authorized.

The standalone regulator and separately declared 100 mA controller integration requirements remain electrically unchanged. Preserve the original 150 mA thermal rejection. These examples are development integration tests, not new frozen benchmark passes.

## Readability acceptance

- Inspect every generated schematic sheet at a documented usable print/zoom scale, including full-page overview and dense component regions.
- Functional blocks and signal flow are apparent; power and reference intent is visible.
- Component bodies, pin labels, reference/value text and net labels do not overlap or obscure one another.
- Connector purpose and pin identities can be followed without guessing; labels attach visibly to the intended endpoint.
- Keep ERC, strict DRC, exact pin/pad connectivity, trusted simulation and writer round-trip gates enabled. Passing ERC alone is not a readability pass.
- Use existing native layout machinery where it supports these requirements; add regression tests for the observed failure and avoid fixture-specific coordinates or renamed electrical nets to obtain a pass.

## Verification and stopping

Run focused regressions, applicable race checks, lint/vet and the repository short suite (12-minute per-package bound). Native generation is limited to 20 minutes per example and is repeated once for replay. Seal and compare every generated schematic, PCB, project file and project-local library/table before export; run native rendering only on copies and reverify original seals afterward. Retain negative runs and all diagnostic artifacts, including failed preflights.

Publish a source-bound local report with visual evidence, exact replay inventories, native stage results and historical-preservation checks. Record any further downstream failure without beginning an unapproved subsequent repair/evaluation phase. The complete six-positive/two-paired-case milestone is not redefined by this offline work.
