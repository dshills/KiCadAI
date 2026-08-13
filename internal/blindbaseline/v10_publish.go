package blindbaseline

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"kicadai/internal/atomicdir"
	"kicadai/internal/externalkey"
)

// PublishV10 independently encrypts exactly 24 opaque baseline records and
// atomically publishes only ciphertext and non-revealing commitments.
func PublishV10(request V10Request) (result V10Result, returnErr error) {
	if request.Random == nil {
		request.Random = rand.Reader
	}
	if err := validateRequestV10(request); err != nil {
		return V10Result{}, err
	}
	if err := validateDestination(request.RepositoryRoot, request.DestinationRoot); err != nil {
		return V10Result{}, err
	}
	paths := append([]string{request.KeyPath}, request.ReservedKeyPaths...)
	if err := externalkey.Distinct(request.RepositoryRoot, paths...); err != nil {
		return V10Result{}, err
	}
	key, err := externalkey.Create(request.RepositoryRoot, request.KeyPath, request.Random)
	if err != nil {
		return V10Result{}, err
	}
	defer clear(key)
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			if err := externalkey.Remove(request.RepositoryRoot, request.KeyPath); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("remove unpublished V10 held-out baseline key: %w", err))
			}
		}
	}()
	ciphertext, manifest, err := sealRecordsV10(key, request.Binding, request.Records, request.Random)
	if err != nil {
		return V10Result{}, err
	}
	opened, err := OpenV10(key, manifest, ciphertext)
	if err != nil {
		return V10Result{}, fmt.Errorf("verify V10 held-out baseline record seals: %w", err)
	}
	if !equalRecordsV10(opened, request.Records) {
		return V10Result{}, fmt.Errorf("verify V10 held-out baseline record seals")
	}
	if err := atomicdir.Publish(request.DestinationRoot, func(root string) error {
		manifestBytes, err := marshalStable(manifest)
		if err != nil {
			return err
		}
		audit := baselineAuditV10(manifest)
		artifacts := map[string][]byte{CipherFileV10: ciphertext, ManifestFile: manifestBytes, AuditFile: audit}
		for path, data := range artifacts {
			if err := os.WriteFile(filepath.Join(root, path), data, 0o644); err != nil {
				return err
			}
		}
		return writeChecksums(root, artifacts)
	}); err != nil {
		return V10Result{}, err
	}
	keyCommitted = true
	return V10Result{Manifest: manifest}, nil
}

func validateRequestV10(request V10Request) error {
	if request.RepositoryRoot == "" || request.DestinationRoot == "" || request.KeyPath == "" {
		return fmt.Errorf("V10 held-out baseline repository, destination, and key paths are required")
	}
	if len(request.Records) != expectedCasesV10 {
		return fmt.Errorf("V10 held-out baseline requires exactly %d records", expectedCasesV10)
	}
	total := 0
	for _, record := range request.Records {
		if len(record) == 0 || len(record) > maximumPayloadSize-total {
			return fmt.Errorf("V10 held-out baseline record set exceeds bounds")
		}
		total += len(record)
	}
	return validateBindingV10(request.Binding)
}

func validateBindingV10(binding V10Binding) error {
	for _, field := range binding.fields() {
		valid := false
		switch field.kind {
		case bindingCommit:
			valid = commitPattern.MatchString(field.value)
		case bindingHash:
			valid = hashPattern.MatchString(field.value)
		case bindingIdentifier:
			valid = policyIdentifierPattern.MatchString(field.value)
		case bindingPlatform:
			valid = platformPattern.MatchString(field.value)
		case bindingVersion:
			valid = versionPattern.MatchString(field.value)
		}
		if !valid {
			return fmt.Errorf("V10 held-out baseline binding %s is invalid", field.name)
		}
	}
	return nil
}

func validateManifestV10(manifest V10Manifest) error {
	if manifest.Schema != ManifestSchemaV10 || manifest.Version != ManifestVersionV10 || manifest.Algorithm != AlgorithmV10 || manifest.NonceBytes != nonceBytesV10 || manifest.CaseCount != expectedCasesV10 || len(manifest.RecordCiphertextSHA256) != expectedCasesV10 || !hashPattern.MatchString(manifest.CiphertextSHA256) || !hashPattern.MatchString(manifest.PlaintextAggregateSHA256) || !hashPattern.MatchString(manifest.AADAggregateSHA256) || !hashPattern.MatchString(manifest.Hash) {
		return fmt.Errorf("V10 held-out baseline manifest is invalid")
	}
	for _, digest := range manifest.RecordCiphertextSHA256 {
		if !hashPattern.MatchString(digest) {
			return fmt.Errorf("V10 held-out baseline record commitment is invalid")
		}
	}
	if err := validateBindingV10(manifest.Binding); err != nil {
		return err
	}
	if hashBytes(manifestCommitmentV10(manifest)) != manifest.Hash {
		return fmt.Errorf("V10 held-out baseline manifest hash mismatch")
	}
	return nil
}

func baselineAuditV10(manifest V10Manifest) []byte {
	return []byte(fmt.Sprintf("# V10 Held-Out Baseline Seal Audit\n\nThe isolated baseline custodian sealed %d independently authenticated held-out evidence records after the frozen public generation-zero selection. Public artifacts disclose no case mapping, outcome bucket, gap, anchor, path, diagnostic, membership, timing, or promotion detail.\n\n- manifest hash: `%s`\n- selection hash: `%s`\n- ciphertext hash: `%s`\n- algorithm: `%s`\n", manifest.CaseCount, manifest.Hash, manifest.Binding.SelectionSHA256, manifest.CiphertextSHA256, manifest.Algorithm))
}

func equalRecordsV10(left, right [][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !bytes.Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}
