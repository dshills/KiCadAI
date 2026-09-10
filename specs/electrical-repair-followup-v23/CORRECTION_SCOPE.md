# V23 correction scope: revalidate obsolete amplifier clamp states

Selected after residual report commit `9c9dd380`. No production correction or
successor corpus evaluation has run at this scope freeze.

## Independent proof and limits of the diagnosis

`TestV23ReproducerObsoleteOpAmpRailClamp` uses independent model evidence: a
negative-feedback amplifier with 9 V supply, 0.2 V rail margins, 22 kΩ load,
finite open-loop gain 100,000, and 2.4 V excitation. The unclamped operating point
matches the analytical finite-gain solution `2.4 * 100000 / 100001`. Starting the
historical solver from either obsolete rail clamp instead produces a repeated
state/nonconvergence refusal. The reproducer passed against unchanged production.

This establishes a genuine generic stale-active-state defect, not permission to
accept a numerical nonconvergence result. V22's case 018 exposes the same bounded
linear operating-point failure class in a sweep that carries clamp history.
The exact corpus cause and later electrical assertions must still be tested by
the separately frozen successor. Case 017 uses a nonlinear solver with its own
release logic; do not assume that changing the linear path fixes it.

## Allowed correction

1. Provide an explicitly versioned experimental solver/evaluation path. Preserve
   the historical numerical paths, v1 defaults, provenance identities, and seals.
2. When the existing bounded active-state solve cannot converge, permit at most
   one deterministic alternative seed that releases obsolete op-amp clamps.
   Preserve comparator/hysteresis state and unrelated device states; do not reset
   circuits indiscriminately or enumerate an unbounded active-set space.
3. Rebuild the exact finite-gain equations, solve them, and require the ordinary
   residual, state-consistency, supply, operating-range, and stability checks.
   An alternative seed is not evidence of convergence until these checks pass.
4. Preserve the historical diagnostic when the bounded alternative is inadmissible
   or fails. Do not suppress positive-feedback instability, current ambiguity,
   real bound failures, critical failures, or previously passing-corner regression.
5. Record the exact solver revision/selection and authenticated numerical evidence.
   Do not silently label corrected execution as the historical solver. Reuse
   trusted primitive models without editing their parameters or substituting parts.
6. Keep public synthesis/repair budgets unchanged. The extra numerical seed has
   a separately recorded finite work bound; no tolerance relaxation, random seed,
   concurrency increase, time-based search decision, or outcome-driven tuning.

## Verification before corpus evaluation

Require independent tests for both rail-to-linear transitions, chained and
resistive negative feedback, finite-gain accuracy, unchanged legitimate saturation,
preserved comparator state, rejected unstable feedback, invalid supply/model
parameters, deterministic replay, provenance tampering, and bounded exhaustion.
Keep the historical reproducer intact as an isolation check. Nonlinear-load
failures remain unchanged unless an independent reproducer establishes the same
specific defect and the correction remains inside this frozen boundary.

The follow-up evaluator must preserve V18/V20/V21/V22 artifacts, historical passes
and safety outcomes, and the v1 support surface. No claim of additional capability
is allowed until complete numerical and installed-KiCad acceptance succeeds under
the committed successor protocol. If the fix only exposes real later electrical
failures, publish that outcome honestly; do not widen this correction to force a
pass.
