# Live interface evaluation authorization

Approved September 11, 2026 in the current task. The user answered "I approve"
to: "Do you approve preparing, freezing, and running it with the existing OpenAI
key, capped at 20 requests and $25?" The referenced protocol is
`../LIVE-PROTOCOL-PROPOSAL.md` at commit
`906cf2ab16998bfb3347a89e3382e7b9eedec2bc`.

This authorizes one new eight-case interface evaluation, including offline
preparation, corpus/answer review, freeze, execution and evidence publication.
It does not authorize a practical-board recovery/final campaign, an additional
run after failure, Gemini review, model substitution, raised response/attempt
limits, merge, release, fabrication or stable-support expansion. Historical
evaluation artifacts and journals must not change. Existing-key reuse is
explicitly approved; no key creation or credential-file write is needed.

There are four ready, two refusal and two clarification cases. The maximum is
20 generation POSTs and USD 25 of conservative estimates/reservations, with one
correction per initial/follow-up leg and no uncertain transport retries.
The model is `gpt-5.6-sol`; output 16,384 tokens; encoded request 131,072 bytes;
production stream cap 2,097,152 bytes; evaluator transport deadline five minutes;
case ceiling 20 minutes; campaign ceiling 90 minutes; one in-flight request;
sampled process-tree RSS 16 GiB; retained evidence 1 GiB.

No API calls or probes precede the freeze. The first frozen case also establishes
actual account access; an access/configuration failure stops the campaign.
Any firewall/security-rule modification requires its own explicit approval.
