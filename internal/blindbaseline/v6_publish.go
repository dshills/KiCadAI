package blindbaseline

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"kicadai/internal/atomicdir"
	"kicadai/internal/externalkey"
)

func PublishV6(request V6Request) (V6Result, error) {
	if request.Random == nil {
		request.Random = rand.Reader
	}
	if err := validateRequestV6(request); err != nil {
		return V6Result{}, err
	}
	if err := validateDestination(request.RepositoryRoot, request.DestinationRoot); err != nil {
		return V6Result{}, err
	}
	paths := append([]string{request.KeyPath}, request.ReservedKeyPaths...)
	if err := externalkey.Distinct(request.RepositoryRoot, paths...); err != nil {
		return V6Result{}, err
	}
	key, err := externalkey.Create(request.RepositoryRoot, request.KeyPath, request.Random)
	if err != nil {
		return V6Result{}, err
	}
	defer clear(key)
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			_ = externalkey.Remove(request.RepositoryRoot, request.KeyPath)
		}
	}()
	payloadHash := hashBytes(request.Payload)
	aad := additionalDataV6(request.Binding, payloadHash, request.CaseCount)
	ciphertext, nonceBytes, err := sealV6(key, request.Payload, aad, request.Random)
	if err != nil {
		return V6Result{}, err
	}
	manifest := V6Manifest{Schema: ManifestSchemaV6, Version: ManifestVersionV6, Algorithm: AlgorithmV6, Binding: request.Binding, PayloadSHA256: payloadHash, CiphertextSHA256: hashBytes(ciphertext), AADSHA256: hashBytes(aad), NonceBytes: nonceBytes, CaseCount: request.CaseCount}
	manifest.Hash = hashBytes(manifestCommitmentV6(manifest))
	if err := atomicdir.Publish(request.DestinationRoot, func(root string) error {
		manifestBytes, err := marshalStable(manifest)
		if err != nil {
			return err
		}
		audit := baselineAuditV6(manifest)
		artifacts := map[string][]byte{CipherFile: ciphertext, ManifestFile: manifestBytes, AuditFile: audit}
		for path, data := range artifacts {
			if err := os.WriteFile(filepath.Join(root, path), data, 0o644); err != nil {
				return err
			}
		}
		return writeChecksums(root, artifacts)
	}); err != nil {
		return V6Result{}, err
	}
	keyCommitted = true
	return V6Result{Manifest: manifest}, nil
}

func validateRequestV6(request V6Request) error {
	if request.RepositoryRoot == "" || request.DestinationRoot == "" || request.KeyPath == "" {
		return fmt.Errorf("V6 held-out baseline repository, destination, and key paths are required")
	}
	if request.CaseCount <= 0 || request.CaseCount > maximumCases {
		return fmt.Errorf("V6 held-out baseline case count must be between 1 and %d", maximumCases)
	}
	if len(request.Payload) == 0 || len(request.Payload) > maximumPayloadSize {
		return fmt.Errorf("V6 held-out baseline payload must be between 1 and %d bytes", maximumPayloadSize)
	}
	return validateBindingV6(request.Binding)
}

func validateBindingV6(binding V6Binding) error {
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
			return fmt.Errorf("V6 held-out baseline binding %s is invalid", field.name)
		}
	}
	return nil
}

func validateManifestV6(manifest V6Manifest) error {
	if manifest.Schema != ManifestSchemaV6 || manifest.Version != ManifestVersionV6 || manifest.Algorithm != AlgorithmV6 || manifest.NonceBytes != nonceBytesV6 || manifest.CaseCount <= 0 || manifest.CaseCount > maximumCases || !hashPattern.MatchString(manifest.PayloadSHA256) || !hashPattern.MatchString(manifest.CiphertextSHA256) || !hashPattern.MatchString(manifest.AADSHA256) || !hashPattern.MatchString(manifest.Hash) {
		return fmt.Errorf("V6 held-out baseline manifest is invalid")
	}
	if err := validateBindingV6(manifest.Binding); err != nil {
		return err
	}
	if hashBytes(manifestCommitmentV6(manifest)) != manifest.Hash {
		return fmt.Errorf("V6 held-out baseline manifest hash mismatch")
	}
	return nil
}

func baselineAuditV6(manifest V6Manifest) []byte {
	return []byte(fmt.Sprintf("# V6 Held-Out Baseline Seal Audit\n\nThe isolated baseline custodian evaluated and sealed %d held-out cases after the public causal rank-one selection was committed. This artifact discloses no requirement, outcome, gap, diagnostic, bundle membership, or promotion detail.\n\n- manifest hash: `%s`\n- selection hash: `%s`\n- algorithm: `%s`\n", manifest.CaseCount, manifest.Hash, manifest.Binding.SelectionSHA256, manifest.Algorithm))
}
