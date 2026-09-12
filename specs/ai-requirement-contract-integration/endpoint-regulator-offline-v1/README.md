# Exact pin measurements and standalone regulator outputs

September 11, 2026. Approved offline implementation phase on
`codex/endpoint-regulator-contracts`, starting at
`7f2752c0462370909273a161c00195c50240a2cb`.

The two missing contract forms now work through catalog search, physical
pin/pad/net resolution, schematic IR and writer-request construction, and
trusted simulation with deterministic replay. This is implementation evidence,
not evidence of improved AI performance or completed circuit-board promotion.
Repository-wide verification is recorded separately in [verification.json](verification.json).

The final uncached short suite passes all 165 packages: 151 with passing tests
and 14 without tests. Final-source race checks pass four complete affected
packages plus the new composition integration test; the full composition race
suite was not completed. Repository vet and affected-package lint pass.

## Supported forms

- Observe an exact scalar participant port with
  `{"kind":"participant_port","id":"controller.adc"}`. The qualified ID
  resolves the declared participant and port; it is not an arbitrary component
  pin number, net name, coverage reference, or operating-condition target.
- Declare a standalone generated supply with `"source":"port:regulated_output"`.
  Its same-domain public power output must be source-direction and have exactly
  one declared `output`-role producer and one selected physical producer.
  Actual internal consumers can share that rail. No dummy signal sink or
  fictitious participant is needed.
- Both additions require behavioral v3+ and exactly one reference domain.
  Bundled bus/differential observations and multi-reference routing fail closed.
  Existing signal-sourced domains remain supported; v1/v2 do not accept the new
  syntax. The power-tree check also rejects a self-powered input cycle.

## Offline examples and retained rejection

These are three named in-memory test fixtures, not a frozen benchmark or three
independent board successes. They read a checked-in regulator fixture without
editing it. The 100 mA controller fixture is a separately labeled design
variation of the retained 150 mA rejection, not a changed acceptance limit for
that rejected case and not a qualifying baseline-to-pass design.

| Offline example | Concrete realization | Trusted simulation result |
|---|---|---|
| Standalone regulator | 8 components, 3 nets; regulator VOUT pin/pad 3 on generated rail | 3.3 V; worst reported temperature 103.0495 °C; both requested behaviors pass |
| Regulator/filter/controller, 150 mA load | 21 components, 10 nets; ADC PA0 pin/pad 7 and VDD pin/pad 4 resolve correctly | Rejected: predicted regulator temperature 125.0495 °C exceeds catalog maximum 125 °C |
| Regulator/filter/controller, 100 mA load | 21 components, 10 nets; ADC PA0 pin/pad 7 is measured at `composition_net_005` | 923.3613 Hz cutoff, 3.3 V rail, 114.0495 °C; all three requested behaviors pass |

The controller ADC is distinct from the raw analog input. Its power pin and the
regulator output share the actual generated rail (`composition_net_003`). The
tests resolve symbol pins and footprint pads, check every requested measurement
against its bounds, compare repeated lowering byte-for-byte, and compare trusted
simulation evidence hashes on replay. No generated KiCad project or routed PCB
is claimed by these examples. See [results.json](results.json) and the retained
[focused test transcript](focused-tests.log).

## Safety, provider boundary, and preservation

All Go tests run with provider keys and the live-test switch unset, using the
pre-existing cached Go toolchain with network module access disabled. No live
OpenAI/Gemini requests, model changes, new evaluations, pushes, PRs, merges,
releases, or fabrication actions occur in this phase. The existing key is used
only by read-only historical authenticators for local exact-secret scans.

The provider schema uses closed required-field objects and nested unions in
line with the [official Structured Outputs guide](https://developers.openai.com/api/docs/guides/structured-outputs).
Offline captures of 16 initial/correction forms have a largest request of
126,382 bytes against the unchanged 131,072-byte limit (4,690 bytes of headroom).
That is not a bound on every possible follow-up, a provider schema acceptance
probe, or evidence that the model will reliably choose the new forms.

The source-bound historical authentication rechecks 304 interface raw files,
110 frozen inputs, 8 cases/57 clauses, both interface archive contents, 472
practical-baseline raw files, 16 cases/108 clauses, prior recovery inventories,
and sealed binaries. SQL and Node-derived interface rows agree. The historical
sealed replay remains a failure; the supplementary 15-attempt replay and eight
tamper rejections do not convert it into a sealed pass. See
[preservation.json](preservation.json).

## Goal status and next decision

The [historical practical baseline](../../practical-sensor-controller-boards/PROTOCOL-V2-RESULTS.md)
still has 0/8 complete primary boards. No new full-board score or paired
improvement score is available from this offline phase. The six-complete-board
and two-materially-different-design milestone remains unmet.

Next useful scope is explicit reference routing for participants/generated
outputs, followed by bounded end-to-end schematic/PCB/KiCad promotion evidence.
Any new live evaluation or external review needs its own scope and authorization;
this report does not authorize another campaign. The current limitations and
self-review are in [REVIEW.md](REVIEW.md).

## Reproduction

Run `node specs/ai-requirement-contract-integration/endpoint-regulator-offline-v1/verify.mjs`
to check the recorded source and evidence hashes. Add `--run-focused` to rerun
the endpoint regressions with keys unset and cached offline dependencies. The
receipt includes exact environment settings and commands for broader checks.
Historical authentication remains the existing read-only command:
`node specs/ai-requirement-contract-integration/offline-repair-v1/verify-preservation.mjs`.
Run it without redirecting into historical records; its output is a new receipt.
