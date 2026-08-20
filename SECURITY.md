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

## Protected boundaries

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
