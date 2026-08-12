package blindbaseline

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"kicadai/internal/atomicdir"
	"kicadai/internal/externalkey"
)

func PublishV7(request V7Request) (result V7Result, returnErr error) {
	if request.Random == nil {
		request.Random = rand.Reader
	}
	if err := validateRequestV7(request); err != nil {
		return V7Result{}, err
	}
	if err := validateDestination(request.RepositoryRoot, request.DestinationRoot); err != nil {
		return V7Result{}, err
	}
	paths := append([]string{request.KeyPath}, request.ReservedKeyPaths...)
	if err := externalkey.Distinct(request.RepositoryRoot, paths...); err != nil {
		return V7Result{}, err
	}
	key, err := externalkey.Create(request.RepositoryRoot, request.KeyPath, request.Random)
	if err != nil {
		return V7Result{}, err
	}
	defer clear(key)
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			if err := externalkey.Remove(request.RepositoryRoot, request.KeyPath); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("remove unpublished V7 held-out baseline key: %w", err))
			}
		}
	}()
	payloadHash := hashBytes(request.Payload)
	aad := additionalDataV7(request.Binding, payloadHash, request.CaseCount)
	ciphertext, nonceBytes, err := sealV7(key, request.Payload, aad, request.Random)
	if err != nil {
		return V7Result{}, err
	}
	manifest := V7Manifest{Schema: ManifestSchemaV7, Version: ManifestVersionV7, Algorithm: AlgorithmV7, Binding: request.Binding, PayloadSHA256: payloadHash, CiphertextSHA256: hashBytes(ciphertext), AADSHA256: hashBytes(aad), NonceBytes: nonceBytes, CaseCount: request.CaseCount}
	manifest.Hash = hashBytes(manifestCommitmentV7(manifest))
	if err := atomicdir.Publish(request.DestinationRoot, func(root string) error {
		manifestBytes, err := marshalStable(manifest)
		if err != nil {
			return err
		}
		audit := baselineAuditV7(manifest)
		artifacts := map[string][]byte{CipherFile: ciphertext, ManifestFile: manifestBytes, AuditFile: audit}
		for path, data := range artifacts {
			if err := os.WriteFile(filepath.Join(root, path), data, 0o644); err != nil {
				return err
			}
		}
		return writeChecksums(root, artifacts)
	}); err != nil {
		return V7Result{}, err
	}
	keyCommitted = true
	return V7Result{Manifest: manifest}, nil
}

func validateRequestV7(request V7Request) error {
	if request.RepositoryRoot == "" || request.DestinationRoot == "" || request.KeyPath == "" {
		return fmt.Errorf("V7 held-out baseline repository, destination, and key paths are required")
	}
	if request.CaseCount <= 0 || request.CaseCount > maximumCases {
		return fmt.Errorf("V7 held-out baseline case count must be between 1 and %d", maximumCases)
	}
	if len(request.Payload) == 0 || len(request.Payload) > maximumPayloadSize {
		return fmt.Errorf("V7 held-out baseline payload must be between 1 and %d bytes", maximumPayloadSize)
	}
	return validateBindingV7(request.Binding)
}

func validateBindingV7(binding V7Binding) error {
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
			return fmt.Errorf("V7 held-out baseline binding %s is invalid", field.name)
		}
	}
	return nil
}

func validateManifestV7(manifest V7Manifest) error {
	if manifest.Schema != ManifestSchemaV7 || manifest.Version != ManifestVersionV7 || manifest.Algorithm != AlgorithmV7 || manifest.NonceBytes != nonceBytesV7 || manifest.CaseCount <= 0 || manifest.CaseCount > maximumCases || !hashPattern.MatchString(manifest.PayloadSHA256) || !hashPattern.MatchString(manifest.CiphertextSHA256) || !hashPattern.MatchString(manifest.AADSHA256) || !hashPattern.MatchString(manifest.Hash) {
		return fmt.Errorf("V7 held-out baseline manifest is invalid")
	}
	if err := validateBindingV7(manifest.Binding); err != nil {
		return err
	}
	if hashBytes(manifestCommitmentV7(manifest)) != manifest.Hash {
		return fmt.Errorf("V7 held-out baseline manifest hash mismatch")
	}
	return nil
}

func baselineAuditV7(manifest V7Manifest) []byte {
	return []byte(fmt.Sprintf("# V7 Held-Out Baseline Seal Audit\n\nThe isolated baseline custodian evaluated and sealed %d held-out cases after the public causal rank-one selection was committed. This artifact discloses no requirement, outcome, gap, diagnostic, bundle membership, or promotion detail.\n\n- manifest hash: `%s`\n- selection hash: `%s`\n- algorithm: `%s`\n", manifest.CaseCount, manifest.Hash, manifest.Binding.SelectionSHA256, manifest.Algorithm))
}
