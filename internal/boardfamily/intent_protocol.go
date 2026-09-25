package boardfamily

import (
	"errors"

	"kicadai/internal/aiprovider"
)

// A closed internal choice, never inferred from a response, journal or model.
// Zero preserves every legacy caller's indexed-v3 contract and byte bounds.
type extractionProtocol uint8

const (
	indexedProtocol extractionProtocol = iota
	ownedProtocol
	connectionProtocol
	directProtocol
	groundedProtocol
	groundedFullProtocol
	sourceEligibleProtocol
	sourceAddressedProtocol
	semanticBoundaryProtocol
	coverageProtocol
	fidelityProtocol
)

func (p extractionProtocol) admissionVersion() string {
	if p == fidelityProtocol {
		return FidelityEvidenceVersion
	}
	if p == coverageProtocol {
		return CoverageEvidenceVersion
	}
	if p == semanticBoundaryProtocol {
		return SemanticBoundaryVersion
	}
	if p == sourceAddressedProtocol {
		return SourceAddressedVersion
	}
	if p == sourceEligibleProtocol {
		return SourceEligibleVersion
	}
	if p == groundedProtocol || p == groundedFullProtocol {
		return GroundedEvidenceVersion
	}
	if p == directProtocol {
		return DirectEvidenceVersion
	}
	if p == connectionProtocol {
		return ConnectionEvidenceVersion
	}
	if p == ownedProtocol {
		return OwnedEvidenceVersion
	}
	return ReferenceIntentVersion
}

func (p extractionProtocol) journalVersion() string {
	if p == fidelityProtocol {
		return "requirement-fidelity-evidence-journal-1"
	}
	if p == coverageProtocol {
		return "requirement-coverage-evidence-journal-1"
	}
	if p == semanticBoundaryProtocol {
		return "semantic-boundary-evidence-journal-1"
	}
	if p == sourceAddressedProtocol {
		return "source-addressed-evidence-journal-1"
	}
	if p == sourceEligibleProtocol {
		return "source-eligible-evidence-journal-1"
	}
	if p == groundedFullProtocol {
		return "partitioned-full-evidence-journal-1"
	}
	if p == groundedProtocol {
		return "partitioned-evidence-journal-1"
	}
	if p == directProtocol {
		return "direct-evidence-journal-1"
	}
	if p == connectionProtocol {
		return "connection-evidence-journal-1"
	}
	if p == ownedProtocol {
		return "owned-evidence-journal-1"
	}
	return referencedJournalVersion
}

func (p extractionProtocol) auditVersion() string {
	if p == fidelityProtocol {
		return "requirement-fidelity-journal-audit-1"
	}
	if p == coverageProtocol {
		return "requirement-coverage-journal-audit-1"
	}
	if p == semanticBoundaryProtocol {
		return "semantic-boundary-journal-audit-1"
	}
	if p == sourceAddressedProtocol {
		return "source-addressed-journal-audit-1"
	}
	if p == sourceEligibleProtocol {
		return "source-eligible-journal-audit-1"
	}
	if p == groundedFullProtocol {
		return "partitioned-full-journal-audit-1"
	}
	if p == groundedProtocol {
		return "partitioned-journal-audit-1"
	}
	if p == directProtocol {
		return "direct-journal-audit-1"
	}
	if p == connectionProtocol {
		return "connection-journal-audit-1"
	}
	if p == ownedProtocol {
		return "owned-journal-audit-1"
	}
	return "indexed-journal-audit-1"
}

func (p extractionProtocol) requestLimit() int64 {
	switch p {
	case indexedProtocol:
		return 24000
	case ownedProtocol:
		return OwnedEvidenceMaxRequestBytes
	case connectionProtocol:
		return ConnectionEvidenceMaxRequestBytes
	case directProtocol:
		return DirectEvidenceMaxRequestBytes
	case groundedProtocol:
		return GroundedEvidenceMaxRequestBytes
	case groundedFullProtocol:
		return GroundedFullMaxRequestBytes
	case sourceEligibleProtocol:
		return SourceEligibleMaxRequestBytes
	case sourceAddressedProtocol:
		return SourceAddressedMaxRequestBytes
	case semanticBoundaryProtocol:
		return SemanticBoundaryMaxRequestBytes
	case fidelityProtocol:
		return FidelityEvidenceMaxRequestBytes
	case coverageProtocol:
		return CoverageEvidenceMaxRequestBytes
	default:
		return 0
	}
}

func (p extractionProtocol) prepare(prompt string) (aiprovider.GenerateRequest, ReferencedRequest, error) {
	switch p {
	case indexedProtocol:
		return prepareReferencedGenerateRequest(prompt)
	case ownedProtocol:
		return prepareOwnedGenerateRequest(prompt)
	case connectionProtocol:
		return prepareConnectionGenerateRequest(prompt)
	case directProtocol:
		return prepareDirectGenerateRequest(prompt)
	case groundedProtocol, groundedFullProtocol:
		return prepareGroundedGenerateRequest(prompt)
	case sourceEligibleProtocol:
		return prepareSourceEligibleGenerateRequest(prompt)
	case sourceAddressedProtocol:
		return prepareSourceAddressedGenerateRequest(prompt)
	case semanticBoundaryProtocol:
		return prepareSemanticBoundaryGenerateRequest(prompt)
	case fidelityProtocol:
		return prepareFidelityGenerateRequest(prompt)
	case coverageProtocol:
		return prepareCoverageGenerateRequest(prompt)
	default:
		return aiprovider.GenerateRequest{}, ReferencedRequest{}, errors.New("unknown extraction protocol")
	}
}

func (p extractionProtocol) decode(prompt string, raw []byte) (Decision, error) {
	switch p {
	case indexedProtocol:
		return DecodeReferencedIntent(prompt, raw)
	case ownedProtocol:
		return DecodeOwnedEvidenceIntent(prompt, raw)
	case connectionProtocol:
		return DecodeConnectionEvidenceIntent(prompt, raw)
	case directProtocol:
		return DecodeDirectEvidenceIntent(prompt, raw)
	case groundedProtocol, groundedFullProtocol:
		return DecodeGroundedEvidenceIntent(prompt, raw)
	case sourceEligibleProtocol:
		return DecodeSourceEligibleEvidenceIntent(prompt, raw)
	case sourceAddressedProtocol:
		return DecodeSourceAddressedEvidenceIntent(prompt, raw)
	case semanticBoundaryProtocol:
		return DecodeSemanticBoundaryIntent(prompt, raw)
	case fidelityProtocol:
		return DecodeFidelityEvidenceIntent(prompt, raw)
	case coverageProtocol:
		return DecodeCoverageEvidenceIntent(prompt, raw)
	default:
		return Decision{}, errors.New("unknown extraction protocol")
	}
}
