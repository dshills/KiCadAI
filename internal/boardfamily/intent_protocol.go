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
)

func (p extractionProtocol) admissionVersion() string {
	if p == ownedProtocol {
		return OwnedEvidenceVersion
	}
	return ReferenceIntentVersion
}

func (p extractionProtocol) journalVersion() string {
	if p == ownedProtocol {
		return "owned-evidence-journal-1"
	}
	return referencedJournalVersion
}

func (p extractionProtocol) auditVersion() string {
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
	default:
		return Decision{}, errors.New("unknown extraction protocol")
	}
}
