# Pre-live corpus feasibility and interpretation review

Date: September 11, 2026. Local review by the implementing task, before any live
request. No independent external or model-based review is claimed.

The eight prompts and each withheld clause were reviewed for consistency with
the approved interface scope. This is not component qualification, completed
electrical simulation, layout feasibility proof or evidence of successful boards.

| Case | Interpretation and coherence check |
| --- | --- |
| I01 | A single low-pass function with external endpoints; 0–0.9 V input is below the lowest 3.1 V supply; high-resistance output load; cutoff bounds 3300×0.95 and ×1.05 are 3135–3465 Hz. |
| I02 | Two requested functions connected by a declared internal signal; maximum nominal amplified input is 0.06×2.5 = 0.15 V, below the lowest supply; 100 Hz gain observation is below the 1900 Hz filter cutoff; exact cutoff and gain tolerances are retained independently. |
| I03 | The existing controller is an external participant, not a synthesized MCU or firmware task; its analog ADC sink and high input resistance are supplied facts; 1.1 V is below the lowest 3.2 V supply; the filter's behavioral observation must correspond to the actual participant input path, not an invented replacement output. |
| I04 | Input 6.5–7.5 V is above output 2.94–3.06 V; sourced current is 5–25 mA; a simple upper linear-drop power estimate is (7.5−2.94)×0.025 = 0.114 W, not a verified thermal result; DC behavior must cover voltage, load and ambient ranges. |
| R01 | Trusted radiation-dose calibration/traceability and independent certification are neither supplied nor exposed by the installed semantic capability vocabulary; the required guarantee cannot be replaced by an uncalibrated detector. |
| R02 | Explicit event identities, start state, direction-specific delays and prior-state/fault-latch dependencies require semantics beyond the advertised v3 interface; the correct response is a gap, not static approximation or source mutation. |
| C01 | Exactly cutoff and tolerance are absent; supply/input/load/size facts are present. The fixed answer supplies 4200 Hz ±7%, exactly 3906–4494 Hz, without changing them. |
| C02 | Exactly load-current range is absent; input/output/ambient/size facts are present. The fixed 6–18 mA answer preserves those facts; upper linear-drop estimate (8−3.234)×0.018 = 0.085788 W is a plausibility bound, not component or thermal qualification. |

Before freeze, the executable preflight checks every ready/follow-up case's
required registered objective/participant capability and analysis vocabulary
against a freshly loaded installed snapshot. It also captures the actual
production request bodies through a fake transport, validates all wire settings,
measures initial and maximum-sized bounded-diagnostic requests, checks source
and capability identities, verifies RSS sampling access, and confirms the live
root does not exist. The snapshot's complete semantic vocabulary and its
catalog/model registry hashes are retained.

I01, I02, I03 and I04 deliberately exercise different interface structures.
They are not the two materially different new complete boards required by the
original milestone. Their installed capability declarations do not prove that
every concrete composition can be synthesized; no such claim is an acceptance
shortcut. A compiler-valid proposal can still fail the withheld faithfulness
audit, and a valid interface can still fail a future electrical/native campaign.

The question-admission reviews for C01/C02 occur only after compiler-valid
clarification, before the fixed answer is sent, and count against the same case
deadline. The terminal semantic audit checks every clause even when a prior
stage failed. Nothing is supplied as a hand-edited model proposal, circuit,
component selection, placement or routing artifact.
