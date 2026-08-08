# Human-Quality Hierarchical Multilayer Completion Audit

## Outcome

The frozen four-case corpus passes the generic behavior-to-KiCad workflow. Each
case produces functional child sheets and an exact four-copper-layer board, then
passes twice from clean roots through routing, connectivity, writer correctness,
installed-KiCad ERC and strict DRC, normalized zero-difference round trip, and
raw-project replay.

The corpus was frozen at base commit
`47f7a018adb292766f0b2dcf324dc5da40df051e`; its immutable requirement hashes
and physical contract remain in
`internal/opentopologysynthesis/testdata/human_quality_physical_corpus/manifest.json`.
No behavior input contains topology, part, sheet, layer, coordinate, zone, via,
route, or architecture expectations.

## Final Promotion Evidence

All cases used policy hash
`845eac6effae6f369bdf7a3eb1832ca37e3db248f2d454687b58f3d3c129274a`.

| Case | Synthesis hash | Physical hash | Project hash | Promotion evidence hash |
|---|---|---|---|---|
| `mixed_signal_environment_control` | `ecb60c7099827f7c138870f7c48c742c6d0926915fed9001b16e93f06c3cd1c7` | `6d45413a25b0b84a6bcbe3a2f3cbce0204b6af1f5310617160dcfc751976010e` | `e9f895638795a62ae4dc2b2694a712ad23116371ac39ced4b14e413a9a42b90c` | `ccc664b12fabd73bf321a5cf5b4ee83b271a298c02cd34401fa722b6c7cb5308` |
| `precision_audio_power_driver` | `b23431e7d48b03c84f1b6e2d76d6ce57304006671aa6388f8c6bc6b02a1cefea` | `d30bcb51f74afa3929671482c17c8212c27b3c532fc9a6df9b855f022e1d8cd2` | `fe99fdc70559e095742b9cc811c57bfb9027e04efdf70c3e8c21220b3a7e6c6a` | `65e3a06de8b789fafe309fc2345ac895c8d1b16a7c4598c79af8ff0e03148d91` |
| `protected_inductive_actuator` | `96b6b5b1614406c747cf7d927535bf33bc0fdb88c6e762878b2f4db676899105` | `32a39991a3e8aaefa74da5f804d35dec1d6bb928daf4733a474a2da4895c26c9` | `3c3e634ee8a601437d6ccf025c0f25580a8d968599682556aa3983402e87e99a` | `e5ce1cb4c26fac35a2526589f864f9433d3a4d8b854d7050512179de8e7844b0` |
| `regulated_enabled_power_stage` | `1821994e48757a827274fcb4cf86bf5a83e0c3f6e323b5751ec18b6ce416feec` | `2bc7e0e320c0679cee99ef16078441b37fec4c44f80615dd00baaaff9dd822a6` | `907b8745d45e44fc5b2131569547bb7b61538f188701b85c03357afa9f970838` | `b52f7e7deb854c7f13b47ac64a098fec77586784e2f745563955e1b0491d4808` |

The initial complete local promotion run completed in 647.29 seconds. After
the final Prism-driven placement performance cleanup, the exact staged
worktree reran all four cases: the first three passed in the aggregate run and
the final regulated-power case passed in an isolated run with an explicit
20-minute Go test timeout. All hashes below remained unchanged. Across the
eight retained clean-root projects:

- ERC inspected 32 sheet instances and reported zero violations;
- strict DRC and post-zone-refill DRC reported zero violations and zero
  unconnected items;
- all declared zones were present with filled polygons;
- 40 root, child, and PCB normalized writer diff files were present and empty;
- all required schematic, schematic-electrical, placement, routing,
  project-write, writer-correctness, validation, and KiCad stages were `OK`;
- both projects for each case had the same raw project hash; and
- the harness verified functional sheet names and ownership, hierarchical versus
  global net scope, parent-pin/child-label connectivity, conventional stage
  flow, exact copper layers and stackup, dielectric evidence, placement-region
  membership, thermal-edge evidence, layer transitions, paired return vias, and
  bounded return-path distances.

Prism's final review of the staged implementation reported zero findings. The
installed KiCad CLI was run outside the desktop command sandbox because KiCad
10's zone filler registers with macOS application services even in CLI mode;
the same command inside that sandbox aborts before DRC with no design finding.

## Preservation And Repository Gates

The final worktree passed:

```text
go test ./... -count=1
go vet ./...
golangci-lint run ./cmd/... ./internal/...
```

The installed-KiCad preservation run also passed both
`usb_c_i2c_sensor_3v3_protected` and `usb_c_led_indicator_protected`. The
function-level capability report was regenerated through its authoritative test
path; only generated-file hashes changed, while input, lowered-graph,
resolution, request, selection, simulation, and pass evidence remained stable.

## Boundary

This audit promotes only the frozen mixed-signal, amplifier, protected-control,
and regulated-power physical envelope. It does not claim arbitrary dense-board
autorouting, arbitrary parts, RF/high-speed signoff, mains/high-energy safety,
or automatic fabrication approval.
