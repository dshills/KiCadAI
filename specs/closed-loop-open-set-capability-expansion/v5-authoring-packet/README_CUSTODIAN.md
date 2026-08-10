# V5 Corpus Authoring Packet Set

This directory defines three disjoint, independently distributable author
packets. It is custodian-facing and must not be sent wholesale to an author.

For author slot `author_N`, provide only:

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

Each `AUTHOR_N_PACKET.sha256` freezes exactly the six author inputs other than
the checksum itself. `PACKET_SET.sha256` freezes the complete public packet set
and its three distribution manifests. The custodian must verify both before
distribution and record the selected per-author checksum in the returned
authorship record.

Every author returns only their twelve requirement files and completed
`AUTHORSHIP.json` into a separate quarantine. Authors must never share a context
or see another author's returned content.
