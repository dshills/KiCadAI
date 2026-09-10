# Electrical blockers after V21 topology completion

Date: 2026-09-09. Status: diagnostic report complete; generic correction and
successor evaluation are not yet implemented. No new complete circuit pass is
claimed. V18/V20/V21 history and the supported v1 surface remain unchanged.

## Finding

The four advanced public cases do **not** share a missing model or unavailable
solver as their first failure. One stops at analysis-observation preparation,
two reach numerical assertion failures, and one is rejected by the selected
linear model's positive-feedback stability check. The shared implementation
gap is that V21 ends at structural completion: it does not follow a complete
certificate with bounded, behavior-driven electrical repair.

A complete structural path does not prove appropriate differential control,
negative feedback, output-drive compatibility, thresholds, or acceptable noise.
The observed candidates demonstrate those distinctions concretely. The next
implementation should repair and numerically evaluate control-terminal bindings
after topology completion, rather than add more structural completion templates.
This recommendation is not a claim that one terminal edit will solve all four.

## First failed gates

All numbers below are from the selected certified candidate, not an arbitrarily
chosen earlier search candidate. Case IDs identify public discovery records only.

| Public case | First failed gate | Observed evidence | Structural operations |
| --- | --- | --- | ---: |
| 004 | Pre-simulation preparation | Circuit-level phase-margin observation does not resolve to one feedback loop; no numerical report for this attempt | 0 |
| 017 | Electrical assertion | DC output is 2 V; required 2.45–2.55 V | 1 |
| 018 | Electrical assertion | Output noise is 0.005277330782123311 V RMS; maximum 0.002 V RMS | 0 |
| 021 | Model/solver preparation within evaluation | Positive feedback rejected by the linear model; reported DC loop gain 6294627.05897 | 1 |

For 021 the first failed attempt is number 2: an earlier attempt passed. The
reported loop gain is a stability diagnostic, not a measured output voltage or
a claim that a valid steady operating solution was obtained for that attempt.

The original 017 trace records stage `solver` because historical code normalized
an assertion error into `simulation_invalid`. Its exact report-bound structured
diagnosis is `assertion_below_minimum`, with the same actual and bounds. This
report uses that stronger evidence; the original trace bytes are preserved.
The recovery projector fixes this distinction without changing numerical code.
The same original-label limitation affects 004's secondary overshoot failure.

## Candidate-specific causes

### 004 — observation binding is the immediate blocker

The public requirement requests a circuit-level stability observation. The
selected graph has a TLV9061 whose noninverting input is the measured input,
inverting input is the externally driven gate, and output is the measurement
port. A 49.9-ohm resistor connects output to the gate input.

Source-confirmed mechanism: `ExternalNodeForObservation` accepts only `port`
and `domain`; a `circuit` observation therefore becomes an empty observed node.
`simulationStabilityObservationNode` refuses that empty node before identifying
a loop. See [graph.go](../../internal/opentopologysynthesis/graph.go) and
[simulation.go](../../internal/opentopologysynthesis/simulation.go).

This is not proof that the supplied graph has a valid negative-feedback loop:
the alleged feedback node is also externally driven. Merely resolving the
observation cannot be claimed as an electrical repair. A secondary transient
diagnosis already reports 4.98 V overshoot against a 4.8 V maximum. Gated transfer
behavior and all other requirements remain unproven.

### 017 — numerical output failure with an uncontrolled parallel driver

The selected graph places an LM4562 output directly on the voltage-excitation
port, also connected to a power transistor's emitter. Both inputs of that
amplifier are tied to reference. It therefore supplies no differential control
derived from the required output voltage. The selected model declares a 2 V
low-output rail margin, consistent with the observed 2 V result under the
nonlinear DC workflow. See [the catalog model](../../data/components/audio_opamps.json).

This is a source-supported clamping explanation, **not yet an isolated causal
experiment**: the compact trace does not retain active-set internals or a
counterfactual evaluation with the driver removed. The candidate has several
other coupled active stages. Removing that driver or altering one terminal has
not been shown to satisfy either output's full requirements.

The structural branch mechanism explains why such a candidate can be selected.
`topologyStageProposalsV21` uses role-complete stage connection maps; when a supply
is the upstream node, `causalStageConnectionMapsV19` can power an active device
and tie its remaining signal inputs to reference. The planner returns the first
structurally complete candidate without comparing electrical outcomes. See
[topology completion](../../internal/opentopologysynthesis/topology_completion_v21.go)
and [connection maps](../../internal/opentopologysynthesis/causal_operations_v19.go).
The trace's one-operation count alone does not prove that a particular instance
was the added device; that attribution is not required for the observed defect.

### 018 — cancelled differential input and absent feedback

Both inputs of the OPA992 connect to the same external signal. Its output has a
470 nF shunt capacitor but no output-to-input feedback connection. The intended
signal therefore cancels in the differential control equation; structural
input-to-output reachability still exists.

The trusted noise implementation injects input-referred noise into the amplifier
equation, with feedback determining closed-loop noise gain. With no feedback,
that noise is amplified by the open-loop response. The solver produces the
reported 5.277 mV RMS result. See `solveNoiseAnalysis` in
[mna_advanced.go](../../internal/simmodel/mna_advanced.go). This is the strongest
small candidate for an independent feedback/control-terminal repair regression.
It does not justify changing noise parameters, bounds, or the requirement.

The inherited synthesis report uses the legacy inventory digest, while this
certificate and its exact evaluation use the successor inventory digest. Both
are retained. Conflating them caused the initial diagnostic-projector abort;
it was an instrumentation assumption, not a new synthesis failure.

### 021 — positive feedback plus a separate threshold error

The high-indication amplifier receives the monitored signal at its inverting
input. Its noninverting input is connected through equal resistors to its own
output and the monitored input. The selected linear model rejects this positive
feedback in the `high_output_inactive_level` operating case. A separate DC sweep
reports a falling threshold of 1.8872549019607843 V against 1.1–1.3 V.

Positive feedback is not universally an invalid circuit: it may intentionally
provide switching hysteresis. Here, however, the selected analysis/model path
cannot certify the candidate's required behavior. The evidence does not justify
removing that stability gate or assuming that reversing polarity alone will
satisfy the requested rising/falling thresholds and inactive output levels.

## Ranking and bounded correction scope

Counts below distinguish confirmed shared implementation gaps from different
physical mechanisms; diagnostic leaves are never counted as distinct circuits.
Order is affected cases, then distinct reporting domains, then stable key.

| Rank/key | Public cases / domains | Confidence and implication |
| --- | --- | --- |
| 1: `post_certificate_electrical_repair_absent` | 4 / 4 | Confirmed control flow: zero-operation certificates return failed; changed certificates receive one evaluation and then terminate. Add a bounded electrical continuation. |
| 2: `active_control_binding_not_behavior_validated` | 3 / 3 (017, 018, 021) | Graph/model evidence; distinct mechanisms are uncontrolled drive, missing feedback, and positive feedback. The 017 clamping attribution remains a hypothesis until an isolated experiment. |
| 3: `circuit_stability_observation_unresolved` | 1 / 1 (004) | Confirmed representation gap; resolving it alone cannot establish a pass. |

The first-ranked mechanism is directly visible in
[repair_v21.go](../../internal/opentopologysynthesis/repair_v21.go): it never
invokes a numerical repair search after certifying topology. Existing
[repair_v20.go](../../internal/opentopologysynthesis/repair_v20.go) has numerical
repair, but its generators emphasize value/passive-edge changes and polarity
swaps; equal differential inputs cannot be corrected by swapping them.

**Selected implementation direction:** a version-isolated, bounded numerical
continuation that proposes control-terminal rebindings from catalog terminal
roles and graph nodes, authenticates the new graph/model/solver combination,
and accepts only fully evidenced improvements under preserved safety gates.
No component-name, circuit-family, fixture-ID, coordinate, or desired-answer
branches are permitted. Independent fixtures must include tied controls,
feedback polarity, incompatible domains, cancellation, exhausted budgets,
tampered provenance, and a previously passing assertion that must not regress.

Prove causal improvement on those independent fixtures before freezing the
successor's exact scope and limits. Preserve V21 byte-for-byte by using a new
entry point. The later 24-case/two-replay successor must measure **additional
complete simulation-and-installed-KiCad passes**. A renamed error, a better
structural score, or a lower numerical error by itself does not meet the goal.

## Timing and evidence size

Measurements are single diagnostic executions, not statistical benchmarks.
Times are wall-clock seconds. Raw bytes are measured by streaming the complete
historical JSON representation through SHA-256; that representation was not
written to a multi-GB spool. Trace bytes retain the selected graph, certificate,
compact failure evidence, and identities, not all historical search waveforms.

| Case | Synthesis seconds | Projection seconds | Complete replay bytes | Compact trace bytes |
| --- | ---: | ---: | ---: | ---: |
| 004 | 36.370 | 13.136 | 760,075,794 | 9,347 |
| 017 | 252.501 | 1.763 | 144,574,696 | 16,251 |
| 018 | 2.616 | 0.330 | 19,365,810 | 8,599 |
| 021 | 508.438 | 79.174 | 4,829,192,142 | 14,989 |

Together the four traces are 49,186 bytes, versus 5,753,208,442 bytes of complete
replay representation. These are different evidence scopes, not lossless
compression of all search records. The complete replay digests match history
exactly in all four cases; compact traces bind to those identities.

Projection is material for 021 (about 13.5% of synthesis-plus-projection time),
but these data do not identify a numerical hot function or justify concurrency
changes. The projector makes two complete streaming hash passes, and runtime
allocation is still substantial. Keep subsequent profiling/performance work
separate from the electrical correction; do not increase search limits as a
substitute for correcting candidate construction.

## Execution provenance and limitations

- Initial source: `676e3f18da98864d8900e42047e4b685b129e66b`. Cases 004 and 017
  completed with matching replays; 018 synthesized but its projection failed;
  021 did not start. The partial run took 307.12 s, with 4,359,094,272 bytes peak
  resident memory reported by macOS. This is not a four-case memory measurement.
- Recovery source: `ed3580b98b538930b39d2d16594a6539c9389eb1`, under the committed
  [recovery configuration](DIAGNOSTIC_RECOVERY_1.json). Only 018 and 021 executed.
  Both traces and the final receipt were published after unchanged-source checks.
  The diagnostic program completed; `/usr/bin/time -l` subsequently returned
  nonzero because macOS denied `sysctl kern.clockrate`. Its 591.71 s elapsed,
  805.15 s user, and 16.69 s system measurements are retained; aggregate recovery
  peak-memory statistics are unavailable. No case was rerun for that wrapper
  error. A subsequent read-only check confirmed the same clean source commit.
- Effective diagnostic execution counts are **004:1, 017:1, 018:2, 021:1**.
  The extra 018 execution was instrumentation recovery, not outcome tuning.
  These executions are not a replacement or repetition of the frozen evaluation.
- Environment: Go 1.26.8, darwin/arm64, unchanged default synthesis policy,
  authenticated public corpus and model/catalog environment. No held-out data,
  keys, external corpus, solver settings, or requirement bounds were changed.
- Original trace and timing bytes are retained under [evidence](evidence/).
  Per-case measurements carry source requirement/replay/file hashes. The
  recovery receipt binds its exact source commit and execution environment.
  In measurement files, `requirement_sha256` hashes the original public JSON
  file bytes; in synthesis traces it hashes the normalized semantic requirement.
  These intentionally different identities must not be conflated. Publication
  tests authenticate both against the same public source file and history.
- Instrumentation race tests, lint, unchanged historical seals, and authorized
  Prism review passed or were explicitly dispositioned before recovery. This is
  not the final milestone's full coverage, release, or installed-KiCad validation.
- No physical promotion was performed by these diagnostic runs. The experimental
  corpus still has only its previously established **one full pass out of 24**.
