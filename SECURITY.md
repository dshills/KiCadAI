# Security And Electrical Safety

## Reporting a vulnerability

Please report security vulnerabilities through GitHub's private security
advisory interface for this repository. Do not include credentials, private
provider responses, unpublished held-out corpus content, or exploit details in
a public issue.

Include the affected KiCadAI version, platform, reproduction steps, expected
behavior, and the smallest non-sensitive artifact needed to reproduce the
problem. Maintainers will acknowledge a complete report, assess affected
versions, and coordinate a fix and disclosure.

## Maintenance release baseline

v1.0.1 is the security maintenance baseline for the v1 release line. Users of
v1.0.0 should upgrade to its replacement binaries; old tags and assets remain
immutable. v1.0.1 uses Go 1.26.8, `golang.org/x/text` 0.39.0, and
`golang.org/x/sys` 0.44.0. Source builds require Go 1.26.8 or newer. Updating
system Go alone does not update a previously compiled KiCadAI binary.

Vulnerability scans describe the database and artifacts checked at release
time, not a permanent absence of vulnerabilities. New findings require a new
maintenance release, never replacement of an existing tag's assets.

## Protected boundaries

Run `make security-check` before a maintenance release. This networked gate
uses a pinned `govulncheck` version and the current official Go vulnerability
database; unlike frozen circuit evaluations, its findings can change over time.
CI runs it separately from offline gates. Keep the exact Go patch in `go.mod`
on a supported security-maintained release and update affected dependencies.
Local Make targets and release CI share that toolchain pin.

Security-sensitive defects include:

- path traversal or unintended mutation outside an authorized output root;
- disclosure of provider credentials, held-out corpora, source keys, or
  encrypted evaluation content;
- acceptance of unauthenticated model, catalog, toolchain, or provenance data;
- bypass of execute, overwrite, imported-project, safety, or fail-closed gates;
- artifact or checksum substitution in a release or promotion bundle.

KiCadAI treats AI/provider output as untrusted input. Never place secrets in a
prompt, recorded response, generated project, issue, or review artifact.

## Electrical safety boundary

A security-clean result is not an electrically safe or fabrication-ready
design. KiCadAI's checks cover only the explicitly recorded behavioral,
electrical, simulation, physical, writer, and KiCad gates. They do not replace
qualified review of shock, fire, thermal, mechanical, regulatory,
manufacturing, or application-specific hazards. v1 deliberately refuses mains,
high-energy, and other unsupported safety envelopes.
