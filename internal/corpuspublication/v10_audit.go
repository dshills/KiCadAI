package corpuspublication

import (
	"fmt"
	"strings"
)

func auditBytesV10(manifest ManifestV10, manifestHash string, discovery DiscoveryObligationsV10, heldOut HeldOutObligationCommitmentV10) []byte {
	var output strings.Builder
	output.WriteString("# Closed-Loop Open-Set V10 Corpus Freeze Audit\n\n")
	output.WriteString("Status: frozen; outcome-blind validation passed\n\n")
	output.WriteString("Six isolated authors supplied behavior-only requirements. Publication performed no synthesis, simulation, feasibility, classification, ranking, or outcome inspection.\n\n")
	fmt.Fprintf(&output, "- validated cases: %d\n", manifest.DiscoveryCaseCount+manifest.HeldOutCaseCount)
	fmt.Fprintf(&output, "- discovery plaintext cases: %d\n", manifest.DiscoveryCaseCount)
	fmt.Fprintf(&output, "- held-out record ciphertext cases: %d\n", manifest.HeldOutCaseCount)
	fmt.Fprintf(&output, "- manifest SHA-256: `%s`\n", manifestHash)
	fmt.Fprintf(&output, "- public discovery obligations: %d\n", len(discovery.Obligations))
	fmt.Fprintf(&output, "- held-out obligation count: %d\n", heldOut.ObligationCount)
	fmt.Fprintf(&output, "- held-out obligation aggregate: `%s`\n", heldOut.AggregateSHA256)
	fmt.Fprintf(&output, "- held-out ciphertext SHA-256: `%s`\n", manifest.HeldOutSource.CiphertextSHA256)
	fmt.Fprintf(&output, "- encryption: `%s`\n", manifest.HeldOutSource.Algorithm)
	fmt.Fprintf(&output, "- validation report SHA-256: `%s`\n", manifest.ValidationReportSHA256)
	fmt.Fprintf(&output, "- sanitized authorship attestations SHA-256: `%s`\n", manifest.AuthorshipAttestationsSHA256)
	fmt.Fprintf(&output, "- validator manifest SHA-256: `%s`\n", manifest.ValidatorManifestSHA256)
	fmt.Fprintf(&output, "- publisher manifest SHA-256: `%s`\n", manifest.PublisherManifestSHA256)
	fmt.Fprintf(&output, "- packet-set SHA-256: `%s`\n", manifest.PacketSetSHA256)
	fmt.Fprintf(&output, "- historical commitments SHA-256: `%s`\n", manifest.HistoricalCommitmentsSHA256)
	output.WriteString("\nThe 32-byte V10 held-out source key was created exclusively outside the repository. Held-out records use independently authenticated unique random nonces. Discovery anchors are public; held-out anchors are represented only by their aggregate commitment. All artifacts are bound by the publication checksum manifest.\n")
	return []byte(output.String())
}
