# V22 publication review

The complete staged publication diff was reviewed through the authorized Prism
Gemini provider. Evaluator and implementation reviews remain recorded in their
separately frozen review documents; this review covers retained evidence,
independent publication audits, and current-status documentation.

## Review 1

Run: `31f4b3c7d4e12acdd73551a8840b173e`, model `gemini-3-flash-preview`.
Prism returned five low-severity findings, with no medium/high findings.
The raw response is retained locally at
`/tmp/kicadai-v22-publication-prism-1.json`.
Its SHA-256 is `6b2f4c5d1533e1f4e02de6f3ac40f090302c11dbbc895af2c5139c1001a948ea`.

- `3b78de2644e77014`: accepted. Split population file-read and JSON-decode errors
  so the audit reports the specific cause.
- `eefc9969a6ec683b`: accepted. Check every synthetic-fixture digest error using
  the correct parent/subtest testing handle rather than discarding errors.
- `74e41719341cedd8`: accepted. Inventory failures now identify missing and extra
  paths in deterministic order, without adding a comparison dependency.
- `d987465662efcc5e`: not a valid finding. The configured Go 1.26.8 toolchain is
  installed, used by the frozen run, and authenticated in its actual executable
  metadata. The evaluator was reproduced byte-for-byte from a clean clone. The
  provider's assumption that this version is unavailable contradicts direct
  evidence; changing it would violate the frozen environment.
- `4cd34900ebb560f7`: not a defect in this test contract. `go test` runs tests
  from their package directory; this audit intentionally binds the fixed
  repository-relative public population, consistent with its other sealed-input
  paths. Repository-root invocations and clean-checkout validation passed.

An independent staged whitespace check identified trailing empty fields in the
raw `go env` output. The publication now names each of the same eight values and
records the original output hash. No frozen input, outcome, replay record, or
environment value changed. The publication checksum commitment and exact byte
count were updated before final review. Missing/changed/extra/link and rehashed
manifest tamper checks remain fail-closed.

## Review 2 and final disposition

Run: `7913424411737c170491550fda820f02`, same authorized provider/model, reviewing
the complete 619,634-byte remediated staged diff. Raw response:
`/tmp/kicadai-v22-publication-prism-2.json`, SHA-256
`5477d60829c02c1ab67ca06b5ad8c88cf0e3c4f08c31b1130a06bbe8743edc09`.
Prism returned two findings, both contradicted by the verified code:

- `555caf8bfa57d02c` (high): not a valid finding. `auditCases` calls
  `capabilitybaselinev10.ValidateCase` and terminates the subtest on error before
  indexing replay data. `validateCase` at `internal/capabilitybaselinev10/validate.go`
  requires exactly two replay-root hashes (lines 116–118), and exactly two
  promotions for passing cases (lines 126–128). The existing missing-root and
  missing-promotion regression cases were rerun and passed.
- `1e0267fbb9196a86` (medium): not a valid finding. This code uses the
  `encoding/json` v1 API, whose `Marshal` contract explicitly sorts map keys.
  The installed toolchain documentation and its legacy-semantics implementation
  confirm deterministic map ordering. Replacing the codec would change the
  authenticated historical serialization contract, not fix this alleged issue.

Full local formatting/vet/lint and focused V22 race checks passed. After the
accepted review-1 remediation, both publication/frozen-audit race suites passed
again and scoped lint reported zero issues. The staged whitespace check passed.
No implementation, audit code, or retained evidence changed after review 2;
only this review receipt and documentation of the completed checks were updated.
There are **no unresolved valid Prism findings**. This review does not change
the negative electrical-improvement assessment or authorize capability admission.
