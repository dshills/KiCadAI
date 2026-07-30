package componentonboarding

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxArtifactBytes = 32 << 20

func DecodeRequirement(reader io.Reader) (BehavioralRequirement, error) {
	var value BehavioralRequirement
	if err := decodeStrict(reader, &value); err != nil {
		return BehavioralRequirement{}, err
	}
	return value, ValidateRequirement(value)
}

func DecodeExtraction(reader io.Reader) (Extraction, error) {
	var value Extraction
	if err := decodeStrict(reader, &value); err != nil {
		return Extraction{}, err
	}
	if value.Schema != ExtractionSchema {
		return Extraction{}, fmt.Errorf("extraction schema must be %s", ExtractionSchema)
	}
	if len(value.Claims) == 0 || len(value.Claims) > MaxClaims {
		return Extraction{}, fmt.Errorf("extraction must contain a bounded nonempty claim set")
	}
	if len(value.Candidates) == 0 || len(value.Candidates) > MaxCandidates {
		return Extraction{}, fmt.Errorf("extraction must contain a bounded nonempty candidate set")
	}
	return value, nil
}

func DecodeCandidate(reader io.Reader) (Candidate, error) {
	var value Candidate
	if err := decodeStrict(reader, &value); err != nil {
		return Candidate{}, err
	}
	if value.Schema != CandidateSchema || value.PolicyVersion != PolicyVersion || value.Status != StatusQuarantined {
		return Candidate{}, fmt.Errorf("candidate is not a quarantined %s artifact", PolicyVersion)
	}
	return value, nil
}

func DecodePromotion(reader io.Reader) (Promotion, error) {
	var value Promotion
	if err := decodeStrict(reader, &value); err != nil {
		return Promotion{}, err
	}
	if value.Schema != PromotionSchema || value.PolicyVersion != PolicyVersion || value.Hash == "" {
		return Promotion{}, fmt.Errorf("promotion is not a %s artifact", PolicyVersion)
	}
	expected, err := hashWithoutField(value)
	if err != nil || expected != value.Hash {
		return Promotion{}, fmt.Errorf("promotion hash mismatch")
	}
	return value, nil
}

func DecodeOverlay(reader io.Reader) (SupportedOverlay, error) {
	var value SupportedOverlay
	if err := decodeStrict(reader, &value); err != nil {
		return SupportedOverlay{}, err
	}
	if value.Schema != OverlaySchema || value.PolicyVersion != PolicyVersion ||
		value.Status != StatusSupported || value.Hash == "" {
		return SupportedOverlay{}, fmt.Errorf("overlay is not supported")
	}
	expected, err := hashWithoutField(value)
	if err != nil || expected != value.Hash {
		return SupportedOverlay{}, fmt.Errorf("overlay hash mismatch")
	}
	return value, nil
}

func WriteArtifact(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	clean := filepath.Clean(path)
	if clean == "." || filepath.IsAbs(clean) && filepath.Dir(clean) == clean {
		return fmt.Errorf("artifact path is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(clean), "."+filepath.Base(clean)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, clean)
}

func decodeStrict(reader io.Reader, destination any) error {
	limited := &io.LimitedReader{R: reader, N: maxArtifactBytes + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if limited.N == 0 {
			return fmt.Errorf("artifact exceeds %d bytes", maxArtifactBytes)
		}
		return err
	}
	var trailing any
	trailingErr := decoder.Decode(&trailing)
	if limited.N == 0 {
		return fmt.Errorf("artifact exceeds %d bytes", maxArtifactBytes)
	}
	if trailingErr != io.EOF {
		return fmt.Errorf("artifact contains trailing JSON")
	}
	return nil
}
