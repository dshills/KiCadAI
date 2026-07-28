package capabilityexpansion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxArtifactBytes = 16 << 20

func DecodePlan(reader io.Reader) (ExpansionPlan, error) {
	var value ExpansionPlan
	if err := decodeStrict(reader, &value); err != nil {
		return ExpansionPlan{}, err
	}
	return value, ValidatePlan(value)
}

func DecodeCandidate(reader io.Reader) (CandidateRegistry, error) {
	var value CandidateRegistry
	if err := decodeStrict(reader, &value); err != nil {
		return CandidateRegistry{}, err
	}
	return value, ValidateCandidate(value)
}

func DecodeBundle(reader io.Reader) (PromotionBundle, error) {
	var value PromotionBundle
	if err := decodeStrict(reader, &value); err != nil {
		return PromotionBundle{}, err
	}
	return value, ValidateBundle(value)
}

func DecodeSupportedRegistry(reader io.Reader) (SupportedRegistry, error) {
	var value SupportedRegistry
	if err := decodeStrict(reader, &value); err != nil {
		return SupportedRegistry{}, err
	}
	return value, ValidateSupportedRegistry(value)
}

func DecodeApproval(reader io.Reader) (PromotionApproval, error) {
	var value PromotionApproval
	if err := decodeStrict(reader, &value); err != nil {
		return PromotionApproval{}, err
	}
	if !validSHA256(value.BundleHash) || !validSHA256(value.ReviewSHA256) {
		return PromotionApproval{}, fmt.Errorf("approval hashes are invalid")
	}
	return value, nil
}

func decodeStrict(reader io.Reader, destination any) error {
	if reader == nil {
		return fmt.Errorf("reader is required")
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxArtifactBytes+1))
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > maxArtifactBytes {
		return fmt.Errorf("artifact is empty or exceeds %d bytes", maxArtifactBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("artifact contains trailing JSON")
	}
	return nil
}

func WriteArtifact(path string, value any) error {
	data, err := MarshalJSONStable(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".capability-expansion-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
