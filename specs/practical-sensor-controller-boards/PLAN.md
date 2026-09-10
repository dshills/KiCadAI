# Implementation plan

Status at acceptance freeze: preparation complete; baseline not yet run. No
production behavior changes are authorized by baseline evidence yet.

1. Verify merged main, isolate a `codex/` branch, and record API-key reuse consent.
2. Prepare eight practical positive briefs, four refusals, two clarifications,
   two paraphrases and fixed clarification answers. Review feasibility and
   distinguish public frozen reserved cases from genuine blind cases.
3. Implement an isolated evaluator using existing provider/compiler/search/
   synthesis/workflow entry points. Add synthetic tests for attempt capture,
   fail-closed gates, resource/spend limits, immutable destinations, normalized
   identity and evidence seals. Do not test corpus outputs before freeze.
4. Pin environment and evaluator; commit/seal the acceptance set. Verify the
   baseline production tree differs from merged main only by evaluator/docs.
5. Run and authenticate one baseline campaign. Perform read-only faithfulness
   and readability audits. Publish first failures and a maximum-three-fix scope
   decision before production changes. Stop for new scope if uplift is not
   credible under the frozen requirements and budget.
6. Implement the approved-in-scope reusable corrections with independent unit
   tests. Do not use P07/P08 details for tuning. Run synthetic and historical
   supported regression lanes; do not touch historical inputs or evidence.
7. Freeze candidate source. Run one final fresh-provider campaign and paired
   replay of the retained baseline proposals/requirements. Authenticate native
   checks and both clean downstream replays. Grade each acceptance clause.
8. Publish capability outcome and limitations, account for assistance, review
   implementation and evidence, address only publication/authentication issues
   that do not change evaluated behavior, and open a PR. Do not merge or release.

Required regression families: provider/schema/clarification; MCU selection,
power integrity and clock/programming; protocol-aware buses; composition
lowering; schematic/placement/routing/writer; scoped real-KiCad MCU, power and
sensor promotions; repository bounded test suite and lint. Exact commands and
toolchain are part of the pre-evaluation freeze, not retroactive pass criteria.

Prepared artifacts and progress will be recorded here. An unchecked phase is
not evidence of completion.
