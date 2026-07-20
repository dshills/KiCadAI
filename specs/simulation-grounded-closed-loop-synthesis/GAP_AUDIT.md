# Initial Gap Audit

Date: 2026-07-19

Status: pre-implementation baseline. This document is evidence for planning,
not a completion claim.

| Required capability | Current evidence | Gap to close |
|---|---|---|
| Natural-language behavioral translation | AI profiles strict-decode circuit/function intent | Performance statements are not independently accounted for as typed measurable requirements |
| Architecture alternatives | Open-set search retains and ranks deterministic complete candidates | Simulation does not participate in candidate rejection, repair, or final selection |
| DC operating point | Trusted analytic and graph-MNA evaluators | Must bind requested semantic output behavior, not primarily source-node sanity assertions |
| AC response | Deterministic graph-MNA sweep | Needs requirement-derived gain/bandwidth/cutoff assertions and loop-gain planning |
| Transient/startup | Bounded backward-Euler nonlinear transient solver | Startup is not a first-class required analysis with registered initial-condition policy |
| Noise | Catalog op-amp noise evidence and static global estimates | No trusted graph noise analysis or integrated assertion report |
| Stability | Catalog/static phase-margin evidence | No candidate-level registered return-ratio/loop stability analysis |
| Distortion | Amplifier metadata and static measurement runner | No trusted graph-derived THD/clipping assertion used by architecture search |
| Thermal | Catalog ratings and static global junction estimates | No simulation-plan thermal corner/result tied to behavioral promotion |
| Worst-case corners | Deterministic bounded one-at-a-time uncertainty evaluation | Supply, load, temperature, tolerance, and model axes are not explicit named operating cases |
| Model provenance | Catalog identity/hash and registered model IDs | Claims lack a uniform required reviewed source revision, immutable model hash, analysis applicability, and rejected-claim report |
| Diagnosis and repair | Physical placement/routing and persisted project repair loops | Behavioral assertion failures do not produce typed, bounded design-variable repairs |
| Replay | Search, graph, simulation, lowering, writer, and provider replay artifacts | No single closed-loop transcript covers candidates, model decisions, corners, diagnoses, repairs, and final selection |
| Behavioral held-out proof | Function, tolerance, graph-MNA, and adversarial composition corpora | No frozen behavior-only closed-loop corpus with Class-A/Class-AB behavioral promotion |

The existing solvers and physical pipeline remain authoritative foundations.
The milestone must extend them rather than create a parallel success path.
