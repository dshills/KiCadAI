package corpuspublication

import (
	"fmt"
	"slices"
	"strings"
)

func auditBytesV7(manifest Manifest, manifestHash string) []byte {
	var output strings.Builder
	output.WriteString("# Closed-Loop Open-Set V7 Corpus Freeze Audit\n\n")
	output.WriteString("Status: frozen; outcome-blind validation passed\n\n")
	output.WriteString("The corpus contains behavior-only public requirements from three isolated authors. No synthesis, simulation, feasibility, classification, ranking, or outcome inspection occurred during authoring, validation, or publication.\n\n")
	fmt.Fprintf(&output, "- validated cases: %d\n", len(manifest.Entries))
	fmt.Fprintf(&output, "- discovery cases published as plaintext: %d\n", manifest.DiscoveryCaseCount)
	fmt.Fprintf(&output, "- held-out cases published only as authenticated ciphertext: %d\n", manifest.HeldOutCaseCount)
	fmt.Fprintf(&output, "- manifest SHA-256: `%s`\n", manifestHash)
	fmt.Fprintf(&output, "- validation report SHA-256: `%s`\n", manifest.ValidationReportSHA256)
	fmt.Fprintf(&output, "- held-out payload SHA-256: `%s`\n", manifest.HeldOutSource.PayloadSHA256)
	fmt.Fprintf(&output, "- held-out ciphertext SHA-256: `%s`\n", manifest.HeldOutSource.CiphertextSHA256)
	fmt.Fprintf(&output, "- encryption: `%s`\n", manifest.HeldOutSource.Algorithm)
	fmt.Fprintf(&output, "- validator manifest SHA-256: `%s`\n", manifest.ValidatorManifestSHA256)
	fmt.Fprintf(&output, "- publisher manifest SHA-256: `%s`\n", manifest.PublisherManifestSHA256)
	fmt.Fprintf(&output, "- contract manifest SHA-256: `%s`\n", manifest.ContractManifestSHA256)
	fmt.Fprintf(&output, "- packet-set SHA-256: `%s`\n", manifest.PacketSetSHA256)
	fmt.Fprintf(&output, "- historical commitments SHA-256: `%s`\n", manifest.HistoricalCommitmentsSHA256)
	output.WriteString("\n## Aggregate role/domain counts\n\n")
	for _, role := range sortedKeys(manifest.Counts) {
		domains := make([]string, 0, len(manifest.Counts[role]))
		for domain := range manifest.Counts[role] {
			domains = append(domains, domain)
		}
		slices.Sort(domains)
		for _, domain := range domains {
			fmt.Fprintf(&output, "- %s / %s: %d\n", role, domain, manifest.Counts[role][domain])
		}
	}
	output.WriteString("\nThe 32-byte V7 held-out source key was created exclusively outside the repository and is not named or committed here. The freeze-parent commit is recorded in the manifest; the corpus-freeze commit is the first Git commit containing this exact manifest checksum. Any later source, policy, validator, assignment, authorship, or key-binding change requires a new experiment version and fresh baseline.\n")
	return []byte(output.String())
}
