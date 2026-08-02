# Control-State and Sequencing Completion Audit

## Outcome

The V6 control-state milestone is complete for the V3 behavioral-composition
envelope. KiCadAI now represents and validates control function, polarity,
startup/safe state, directed and timed transitions, and stable-state
prerequisites. These semantics survive architecture projection, analysis
planning, lowering, simulation assertions, and bounded repair selection.

The two motivating requirements are deliberately not promoted. Both require a
deenergized output during startup while their only load-switch control is a
fault signal declared deasserted at startup. For a fault-controlled disconnect,
that state means connected. V6 rejects the contradiction with:

```text
ARCHITECTURE_CONTROL_INVALID
startup output requires a deenergized load while the declared load-switch
control starts in its connected state; declare a separate startup enable or
sequencing dependency
```

This is the safe outcome allowed by the specification: neither an unpowered
glitch nor deletion of the startup requirement is accepted as a repair.

## Frozen Evidence

`internal/architecturesearch/testdata/control_behavior_corpus/manifest.json`
freezes six identity-neutral truth-table cases and two V6 contradiction cases.
The raw requirement hashes are:

- `current_sense_protection.json`:
  `c83dd6cc7a95ac3982fd7ff7de4080865686ea179e0a22e31caa58aee2769e83`
- `mixed_function_control_power.json`:
  `8097a52109fdf8127efa2810f2f69bce41eee8fb104f179cf7584083780b6e88`

The promotion harness overlays a newer V6 expectation only when the overlay
manifest explicitly marks it and the baseline contains the same filename. It
cannot add an unknown case. Historical V3 requirement files remain unchanged.

## Implementation Evidence

- V6 is a versioned extension of V3 behavior; it does not inherit V4 hierarchy
  or V5 dynamic-electrothermal semantics.
- Control producer polarity and consumer connect/disconnect action are derived
  from typed semantics, not fixture identity.
- Directed response assertions accept only `rising` or `falling` and preserve
  legacy assertion identities when direction is absent.
- Transition prerequisites resolve to concrete trusted targets during analysis
  planning; a missing binding is a stable planning diagnostic.
- The high-side semantic inverter exposes a bounded preferred-value bias
  variable. Threshold feedback and rail timing retain their bounded repair
  variables. Polarity and required edge remain immutable contract facts.
- Repeated nonlinear and transient active states use bounded, residual-checked
  fallbacks; the solver does not accept an arbitrary oscillating state, and
  accepted fallbacks are identified in solver evidence.
- A behavioral timing requirement may narrow its linked transition envelope,
  but a bound outside that envelope is rejected.

## Local Verification

The following completed on 2026-08-02:

```sh
GOCACHE=/tmp/kicadai-go-cache go test -short -count=1 -timeout 20m ./...
```

Result: pass across every bounded package, including frozen tolerance hashes.

The documented long lanes completed independently:

```sh
go test ./internal/compositionlowering -run '^TestFrozenOpenSetCorpusPassesOfflineWorkflow$' -count=1
go test ./internal/compositionlowering -run '^TestFrozenAdversarialMultiFunctionCorpusPassesOfflineWorkflow$' -count=1
go test ./internal/compositionlowering -run '^TestFrozenSimulationGroundedCorpusPassesOfflineWorkflow$' -count=1
go test ./internal/compositionlowering -run '^TestFrozenBehavioralIntentHeldOutReadyCorpusPassesOfflineWorkflow$' -count=1
```

Results: pass. The simulation-grounded lane executed eight unchanged cases and
verified the two expected V6 rejections. The held-out lane executed its five
unchanged ready cases and the V6 current-sense rejection.

Targeted tests also prove:

- active-high and active-low physical direction;
- contradiction and underconstraint diagnostics;
- deterministic normalization and raw-hash replay;
- bounded active-high disconnect inversion and bias repair;
- resolved transition dependencies and missing-binding rejection;
- wrong-direction transient glitch rejection; and
- legacy tolerance-report identity preservation.

Prism reviewed the staged diff under the standing external-provider
authorization. Its timing-envelope finding was remediated with positive and
negative coverage. Follow-up review prompted explicit solver-fallback evidence,
named generic gate-headroom constants, and indexed transition diagnostics; the
full bounded suite passed afterward.

## Installed-KiCad Promotion

KiCad CLI 10.0.3 and its bundled symbol/footprint libraries ran the unchanged
five-case open-set promotion twice from the same inputs:

```sh
KICADAI_KICAD_CLI=/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli \
KICADAI_SYMBOLS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/symbols \
KICADAI_FOOTPRINTS_ROOT=/Applications/KiCad/KiCad.app/Contents/SharedSupport/footprints \
go test ./internal/compositionlowering \
  -run '^TestFrozenOpenSetCorpusOptionalKiCadPromotion$' -count=2 -v
```

Both runs passed all five cases. The harness requires successful schematic and
electrical generation, placement, routing and connectivity, project writing,
writer correctness, clean ERC, strict DRC, zero-difference round trip, and
deterministic replay.

The simulation-grounded optional KiCad lane was inspected but is not counted as
evidence: its unchanged active-filter case has two KiCad 10.0.3
`wire_dangling` ERC findings (one 0.0254 mm horizontal stub and one 0.0254 mm
vertical stub), while DRC is clean. That pre-existing writer fixture issue is
outside this semantic milestone and remains visible rather than allowlisted.

## Remaining Boundary

Default-off startup followed by normal operation and later fault disconnect
requires two independent semantic controls (for example power-good/enable plus
fault/inhibit) and a provider capable of composing them. V6 now states exactly
why the one-control form is insufficient; implementing and independently
promoting that generic two-control topology is the next goal.

This work does not establish arbitrary state-machine synthesis, arbitrary
sequencing, or unrestricted circuit generation.
