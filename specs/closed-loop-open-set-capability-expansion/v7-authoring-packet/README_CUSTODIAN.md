# V7 Corpus Authoring Packet Set

This directory defines three disjoint, independently distributable author
packets. It is custodian-facing and must not be sent wholesale to an author.

For `author_N`, provide only:

- `README.md`;
- `PUBLIC_REQUIREMENT_CONTRACT.md`;
- `CORPUS_RULES.md`;
- `AUTHORSHIP_TEMPLATE.json`;
- `CONTRACT_BINDING.json`;
- `assignments/author_N.json`; and
- `AUTHOR_N_PACKET.sha256`.

Do not provide another assignment, this custodian README, `PACKET_SET.sha256`,
repository access, conversation history, prior corpus material, synthesis
evidence, diagnostics, expected outcomes, or implementation information.

Each author returns only the twelve assigned requirements and `AUTHORSHIP.json`
into a separate quarantine. Authors must never share a context or see another
author's returned content. The custodian verifies both the packet-set manifest
and the selected per-author manifest before distribution.
