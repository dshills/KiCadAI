# 60-Second Demo Storyboard

Record a single terminal and KiCad window at 1920x1080. Do not accelerate the
result output or substitute hand-edited artifacts. Time-compress only the
search wait and label that edit on screen.

| Time | Picture | Narration / overlay |
|---:|---|---|
| 0-5 s | Repository title, then `requirement.json` | “Start with behavior, not a netlist: 0.1 amp per volt across supply, load, startup, temperature, and safe-operating-area corners.” |
| 5-12 s | Highlight the `acceptance` object | “The request also demands complete routing, connectivity, writer round trip, ERC, strict DRC, and deterministic replay.” |
| 12-18 s | Run `make public-demo` | Overlay: “Deterministic bounded search — wait time compressed.” |
| 18-27 s | Compact JSON result | Highlight: 43 graphs, 2 complete architectures, 194 simulations, 6,326 corner evaluations. “KiCadAI derives candidates and ranks two passing physical architectures.” |
| 27-34 s | `evidence/receipt.json`, architecture alternatives and selected parts | “It chooses the stronger margin, fixes real catalog parts and values, and binds the decision to content hashes.” |
| 34-43 s | Open the generated `.kicad_pro`; switch schematic to PCB | “It writes native KiCad—not a screenshot—then places and routes all 12 nets.” |
| 43-50 s | Show receipt physical gates, then 3D viewer | Overlay: “ERC 0 · strict DRC 0 · unconnected 0 · writer 10/10 · round-trip diff 0.” |
| 50-56 s | Show the two identical project hashes | “The complete physical workflow runs twice and produces the same project hash.” |
| 56-60 s | Run `make public-demo-refusal`; show nonzero result and no project | “Outside the reviewed envelope, it stops. AI proposes. KiCadAI proves—or refuses.” |

For a shorter 35-second cut, omit the schematic view and refusal command, then
show the checked-in refusal receipt/audit beside the final title card.
