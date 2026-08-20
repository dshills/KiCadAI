# V19 Generation-Zero Retirement Audit

The bounded V19 Phase 6 public evaluation completed on 2026-08-20 from clean
commit `5ba364e25f8d8c05c546fda21848717973d8c1b1`. It authenticated the frozen
V19 contract and evaluator, reused the immutable 24-case V10 public discovery
corpus, evaluated every case in manifest order exactly twice with a serial case
limit of one, and opened no held-out key.

The two replays were deterministic. The aggregate result was 0 pass, 12
unsupported, 1 unsafe, and 11 exhausted. V19 therefore failed the frozen
five-of-five advancement gate. It also changed the single V18 admitted pass to
unsupported, so it failed the required preservation gate. V18 remains the
latest admitted public capability.

No correction run was used. Turning this zero-pass result into an admissible
extension would require capability implementation or a changed evaluation
contract, both of which are outside the frozen Phase 6 correction boundary and
the v1 release freeze. The V19 extension is permanently retired and excluded
from the v1 supported surface. Its version-isolated code remains historical
and regression evidence; it does not authorize a supported capability claim.

Authoritative artifacts:

- `internal/capabilityfeedback/testdata/closed_loop_open_set_v19_generation_zero/report.json`
- `internal/capabilityfeedback/testdata/closed_loop_open_set_v19_generation_zero/report.sha256`
- `specs/closed-loop-open-set-capability-expansion/V19_GENERATION_ZERO_RETIREMENT.json`

The report file SHA-256 is
`0bc7c0880e390a8f0cc7c74e3535ccc81be8ebc674c85060a4bfd35d516df09a`;
its stable internal report hash is
`711409f3706eea8734cf1cb9df060e188a48a7c3aab09d0e0f5121a7499c9981`.
