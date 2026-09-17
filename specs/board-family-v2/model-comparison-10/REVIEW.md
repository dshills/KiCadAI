# Local implementation review

Reviewer: the implementing Codex agent, **not an independent reviewer**.
Scope: full-model selection, reservations/settlement, historical compatibility,
immutable journal replay and command failure boundaries. No live responses were
obtained or scored, and no remote CI pass is claimed for these local changes.

The first offline run passed 72 focused test/subtest events, short regression,
targeted race checks, vet, lint, whitespace and formatting. It is superseded by
the subsequent review corrections; its receipt describes the earlier source.

Review corrections made before final verification:

1. Added direct replay of all 14 retained candidate08 journals with the modified
   old-model inspector, plus rejection by the new full-model inspector. Historical
   request bytes are checked against the exact new provider wire modulo model.
2. Added coherently rehashed accounting attacks: absent/foreign profile, old ledger
   version, mini-model prices, wrong model, changed reservation and changed budget.
   These must fail both journal audit and the next spend reservation.
3. Added a full-model-only guard against continuing past unresolved, failed,
   duplicated-response or cost-disputed ledger entries. Legacy behavior stays
   unchanged. Added exact 16,000-byte and no-retry transport boundary tests.
4. Corrected contract-export prose to require the distinct full-model journal and
   ledger, without modifying anything in the provider input.
5. Batched Git inventory queries in the offline runner to avoid a subprocess for
   every Go source file. Test coverage and source-inventory checks are unchanged.

The full model may still make the same semantic errors. Keeping the failed
evaluation's schema, prompts, decoder and scoring frozen makes a later approved
trial informative; these software tests cannot establish model reliability.
Native generation/qualification code and reviewed native artifacts are unchanged.
Independent review, remote CI/release preparation and fresh live authority remain
separate requirements before treating this as a completed goal.
