# V16 Plan

1. Authenticate and permanently retire the incomplete V15 generation-zero run.
2. Replace only top-level synthesis-run whole-buffer hashing with
   byte-identical streaming canonical hashing and fail closed on encoding
   errors.
3. Freeze the V16 single-worker evaluator and bind the corrected production
   synthesis source without changing any V15 search or output semantics.
4. Run local equivalence, resource, regression, lint, and external staged
   review, then commit the freeze.
5. Execute a fresh 24-case discovery baseline under the committed environment.
6. Authenticate and publish only a complete deterministic report.
7. Cluster typed failures, rank generic gaps, and implement only the selected
   generic capability before public and authorized blind held-out comparison.
