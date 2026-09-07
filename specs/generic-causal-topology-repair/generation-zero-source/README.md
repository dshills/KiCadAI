# Original V21 evaluator source

These byte-exact snapshots preserve changed generation-zero source and seals
from commit `c31f3bc7af70c2787ddb728fb65358a207f71ded`. Unchanged source remains
at the paths in the archived manifests. `TestV21GenerationZeroSourceIsPreserved`
authenticates every historical manifest entry and the original manifests
independently of the active maintenance seal, including in shallow checkouts.

The `.snapshot` suffix keeps archived Go files out of compilation. To reproduce
the original evaluator, use a separate checkout of the recorded commit and its
recorded environment; never copy these files over the maintained implementation.

The published generation-zero report, frozen corpus, selected population,
protocol, and V18–V20 evidence are unchanged. Active V21 seals now identify
maintenance revision 1 with `evaluation_status: not_run_after_maintenance`.
They must not be substituted into the historical report or interpreted as a
new evaluation result. See the [review](../../../docs/project-review-2026-09-06.md).
