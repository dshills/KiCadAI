# Implementation plan

1. Define the nonlinear workflow and reviewed diode/NPN/PNP primitive contracts, parameter bounds, catalog claims, and strict provider schema.
2. Refactor primitive selection to use unique compatible catalog claims while retaining the linear workflow's fail-closed boundary.
3. Implement deterministic source/gmin continuation, Newton stamping, convergence evidence, operating-limit validation, and actionable diagnostics.
4. Add unit and integration tests covering diode, NPN, PNP, determinism, nonconvergence, operating limits, tamper resistance, schema injection, and linear compatibility.
5. Add a held-out catalog-backed transistor fixture with no topology hint; prove simulation, routing, KiCad ERC/DRC/connectivity, writer correctness, replay, and round-trip stability.
6. Update capability documentation and golden evidence, run all repository gates, review staged changes with Prism, commit, push, and verify GitHub Actions.
