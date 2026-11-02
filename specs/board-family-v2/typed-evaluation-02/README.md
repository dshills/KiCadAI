# Successor typed-intent evaluation

This separate evaluation is being prepared for the unchanged, offline-qualified
selector at `8913e2b6e6a013b86db7641911881e2b5ffba5f6`. It does not rewrite the
failed final-01 evaluation or create new spending authority. Read
[PROTOCOL-02.md](PROTOCOL-02.md) and [cases-02.json](cases-02.json).

The proposed batch is 14 requests / USD 1.00, existing key only, pinned model and
qualified typed-requirement contract. Raw extraction, final decisions and native
artifacts are evaluated separately. Automatic passes require source-bound meaning
review; an unnecessary clarification on a clear supported request fails.

Offline preparation and authentication:

```sh
node --test specs/board-family-v2/typed-evaluation-02/acceptance.test.mjs
node specs/board-family-v2/typed-evaluation-02/freeze.mjs --freeze
node specs/board-family-v2/typed-evaluation-02/prepare.mjs .cache/board-family-v2/typed-runtime-02-01
node specs/board-family-v2/typed-evaluation-02/run.mjs --check
```

Freeze/preparation refuse existing destinations. Preparation checks the complete
compiler-selected nonstandard source/embed closure against prior qualification,
copies the tested binary byte-identically, and verifies the exported payload.
It reruns new runner tests and unchanged evidence safeguards, not all unchanged
native builds. Logs and source provenance are preserved in the new runtime record.
CI has a separate credential-free runner-safeguard workflow; production CI and
its unchanged coverage floor still apply.

Only after actual new user approval of the exact hashes and scope may a separate
`APPROVAL-02.json` be recorded and `run.mjs --live` be used. No such record is
provided by preparation. The record must bind the runtime/freeze hashes, 14/$1
limits, existing-key-only choice and actual user message/date. Optional
`allow_unattempted_resume: true` must reflect explicit user-approved continuation
scope; it never permits retrying attempted cases. A required firewall change is
separate authority. Neither an approval file nor a budget file is a substitute
for obtaining the actual approval.

After collection, `audit.mjs --check` reauthenticates each output, ledger snapshot
and recomputed score. Passing source-bound semantic-review JSON may be supplied
as its second argument. The review must bind state/runtime/freeze hashes, each
prompt/selection hash and every requirement to specific raw clause/fact indices.
False or missing semantic judgments never become passes. Preserve every failure
and disclose implementing-agent review; this is not an independent holdout,
statistical reliability claim, raw HTTP archive or bench certification.
