# Historical indexed-evaluation verification

This versioned addendum removes an integration constraint: evaluation 03's
original checker requires development-source paths to remain byte-identical.
The new checker instead verifies those source bytes from their recorded Git
commit. It does **not** edit the frozen manifest, original checker, qualification,
approval, ledger, review, results, or captured HTTP bodies.

The exact references are:

- Evaluated source: `8744d7fc032452b2683afac0e5f9082f963de164`.
- Published failed-batch snapshot: `a0d719cce11c51fa6c510058a73869900a5bae7f`.
- Original manifest SHA-256:
  `429f02b613038d22adaacc1cc9ef054e8b18703afe748312c3e925b59c977023`.

## Two explicitly different scopes

Run with provider credentials and live-test flags unset:

```sh
env -u OPENAI_API_KEY -u ANTHROPIC_API_KEY -u GEMINI_API_KEY \
  -u GOOGLE_API_KEY -u KICADAI_LIVE_PROVIDER_TESTS \
  node specs/board-family-v2/history-verification-04/authenticate-indexed-03.mjs --check-archive
```

This portable mode requires the two recorded Git commits locally. It verifies
the complete 31-file publication inventory against its recorded commit, checks
all 1,155 frozen repository-source inputs against SHA-256 digests of Git blob
bytes, and verifies the original failed outcome, approval/result/selection
bindings, process receipts, per-case byte inventory, ledger, and review/score
identity. It reports `runtime_verified: false`: a clean CI checkout does not
prove that the original macOS compiler, executable, Node, or KiCad still exists.
The offline CI job uses full history explicitly; the checker never fetches it.

For local runtime verification, replace `--check-archive` with `--check-runtime`.
This additionally checks all **1,375 frozen cache files**, the original compiler,
executable, Node and KiCad hashes, and retained qualification process records.
It invokes only the pinned original executable's **read-only journal audit**,
with a minimal credential-free environment, and requires the same terminal exit
and failure logs as the original audit. It must reproduce the old rejection,
not a successor parser's success. No provider call, native generation, retry,
recovery, ledger settlement, or release of budget is performed.

Git does not preserve 0700 directory / 0600 file permissions. Therefore the
runtime check stages a byte-checked journal copy in an owned private temporary
directory, audits that copy, then removes only that scratch directory. It never
changes permissions or bytes in the committed publication. The initial check
against a normal Git checkout correctly failed the old auditor's private-path
preflight; private staging fixes replay location, not historical response data.

## Evidence and tests

Both scopes passed from the isolated correction worktree, where `intent.go`
and the provider-envelope validator differ from the evaluated source. Full
runtime mode verified the original 2,530-file frozen closure through Git and
the unchanged local cache, and reproduced the original auditor failure.

Tests cover exact-length binary Git parsing, traversal/request-injection paths,
missing/non-blob/truncated/trailing/duplicate/oversized batch data, rejection of
provider credentials, successful verification with changed development source,
and tampered review/body/missing/extra publication files. Every tamper fixture
first verifies unchanged, so an unrelated Git or staging error cannot be
mistaken for successful tamper detection.

All 15 tests passed locally. Adding the positive fixture precheck initially
exposed four failures because `git ls-tree` scoped paths to the scratch
subdirectory. The checker now requests `--full-tree`; the unchanged scratch
copies verify and all four subsequent mutations are rejected. Both verification
scopes then passed again, including exact original-auditor failure reproduction.

The failure remains **one physical request, 13 unattempted cases, zero complete
passes and native bundles, and a USD 0.05 reservation with unverified usage**.
No actual paid outcome is rescored using successor code. The original strict
checker remains available for the original unchanged source checkout; this new
verifier is explicitly the versioned historical/integration path.

These checks prove local byte consistency and reproducible failure, not provider
signatures, independent review, invoice accuracy, semantic reliability, or
tamper resistance against someone who controls the verifier and every record.
The referenced Git objects and local runtime/cache still need to be retained;
archive-only mode is not a portable package of the original runtime.

The envelope correction can now be integrated without keeping the development
checkout frozen at the evaluated source. Live semantic acceptance and the
overall two-family goal remain incomplete. No additional live work is authorized
by this addendum.
