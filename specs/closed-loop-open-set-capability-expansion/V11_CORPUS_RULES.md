# V11 Immutable Corpus Reuse Rules

V11 uses exactly the published V10 corpus at
`internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus`.

The V11 contract records and verifies the V10 corpus manifest SHA-256
`0ec3834c832246e659b417dcef4aaae6d1634cbcd19c734518990280b124dc94`
and checksum-file SHA-256
`24541ffe0f4ee372e0f1db5508eb22b102343f4eca2f376cacbca51cf275dcdf`.
Every corpus file must continue to match the published checksum manifest.

No V11 authoring, correction, replacement, adjudication, or republishing phase
exists. The encrypted held-out partition remains opaque outside an authorized
blind custodian. The external V10 source key is not a V11 public-evaluation
input and must not be opened, copied, or replaced during discovery evaluation.

The 21 V10 checkpoint files are not corpus artifacts. V11 must neither read nor
reuse them. A V11 working root must be new, absent, and bound to the V11
evaluator and environment commitments.
