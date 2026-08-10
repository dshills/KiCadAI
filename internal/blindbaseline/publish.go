package blindbaseline

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"kicadai/internal/atomicdir"
	"kicadai/internal/externalkey"
)

var commitPattern = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var policyIdentifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,127}$`)

func Publish(request Request) (Result, error) {
	if request.Random == nil {
		request.Random = rand.Reader
	}
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	if err := validateDestination(request.RepositoryRoot, request.DestinationRoot); err != nil {
		return Result{}, err
	}
	paths := append([]string{request.KeyPath}, request.ReservedKeyPaths...)
	if err := externalkey.Distinct(request.RepositoryRoot, paths...); err != nil {
		return Result{}, err
	}
	key, err := externalkey.Create(request.RepositoryRoot, request.KeyPath, request.Random)
	if err != nil {
		return Result{}, err
	}
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			_ = externalkey.Remove(request.RepositoryRoot, request.KeyPath)
		}
	}()
	payloadHash := hashBytes(request.Payload)
	aad, err := additionalData(request.Binding, payloadHash, request.CaseCount)
	if err != nil {
		return Result{}, err
	}
	ciphertext, nonceBytes, err := seal(key, request.Payload, aad, request.Random)
	if err != nil {
		return Result{}, err
	}
	manifest := Manifest{Schema: ManifestSchema, Version: ManifestVersion, Algorithm: Algorithm, Binding: request.Binding, PayloadSHA256: payloadHash, CiphertextSHA256: hashBytes(ciphertext), AADSHA256: hashBytes(aad), NonceBytes: nonceBytes, CaseCount: request.CaseCount}
	manifest.Hash, err = manifestHash(manifest)
	if err != nil {
		return Result{}, err
	}
	if err := atomicdir.Publish(request.DestinationRoot, func(root string) error {
		manifestBytes, err := marshalStable(manifest)
		if err != nil {
			return err
		}
		audit := baselineAudit(manifest)
		artifacts := map[string][]byte{CipherFile: ciphertext, ManifestFile: manifestBytes, AuditFile: audit}
		for path, data := range artifacts {
			if err := os.WriteFile(filepath.Join(root, path), data, 0o644); err != nil {
				return err
			}
		}
		return writeChecksums(root, artifacts)
	}); err != nil {
		return Result{}, err
	}
	keyCommitted = true
	return Result{Manifest: manifest}, nil
}

func validateRequest(request Request) error {
	if request.RepositoryRoot == "" {
		return fmt.Errorf("held-out baseline repository root is required")
	}
	if request.DestinationRoot == "" {
		return fmt.Errorf("held-out baseline destination root is required")
	}
	if request.KeyPath == "" {
		return fmt.Errorf("held-out baseline key path is required")
	}
	if request.CaseCount <= 0 || request.CaseCount > maximumCases {
		return fmt.Errorf("held-out baseline case count must be between 1 and %d", maximumCases)
	}
	if len(request.Payload) == 0 || len(request.Payload) > maximumPayloadSize {
		return fmt.Errorf("held-out baseline payload must be between 1 and %d bytes", maximumPayloadSize)
	}
	return validateBinding(request.Binding)
}

func validateDestination(repositoryRoot, destinationRoot string) error {
	repositoryRoot, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	repositoryRoot, err = filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve real repository root: %w", err)
	}
	destinationRoot, err = filepath.Abs(destinationRoot)
	if err != nil {
		return fmt.Errorf("resolve held-out baseline destination: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(destinationRoot))
	if err != nil {
		return fmt.Errorf("resolve held-out baseline destination parent: %w", err)
	}
	destinationRoot = filepath.Join(parent, filepath.Base(destinationRoot))
	relative, err := filepath.Rel(repositoryRoot, destinationRoot)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("held-out baseline destination must be inside the repository")
	}
	return nil
}

func validateBinding(binding Binding) error {
	for _, field := range binding.fields() {
		valid := false
		switch field.kind {
		case bindingCommit:
			valid = commitPattern.MatchString(field.value)
		case bindingHash:
			valid = hashPattern.MatchString(field.value)
		case bindingIdentifier:
			valid = policyIdentifierPattern.MatchString(field.value)
		}
		if !valid {
			return fmt.Errorf("held-out baseline binding %s is invalid", field.name)
		}
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Schema != ManifestSchema || manifest.Version != ManifestVersion || manifest.Algorithm != Algorithm || manifest.NonceBytes != 12 || manifest.CaseCount <= 0 || manifest.CaseCount > maximumCases || !hashPattern.MatchString(manifest.PayloadSHA256) || !hashPattern.MatchString(manifest.CiphertextSHA256) || !hashPattern.MatchString(manifest.AADSHA256) || !hashPattern.MatchString(manifest.Hash) {
		return fmt.Errorf("held-out baseline manifest is invalid")
	}
	if err := validateBinding(manifest.Binding); err != nil {
		return err
	}
	expected, err := manifestHash(manifest)
	if err != nil || expected != manifest.Hash {
		return fmt.Errorf("held-out baseline manifest hash mismatch")
	}
	return nil
}

func manifestHash(manifest Manifest) (string, error) {
	data, err := manifestCommitment(manifest)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func marshalStable(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeChecksums(root string, artifacts map[string][]byte) error {
	return os.WriteFile(filepath.Join(root, ChecksumFile), checksumManifest(artifacts), 0o644)
}

func checksumManifest(artifacts map[string][]byte) []byte {
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var checksum strings.Builder
	for _, path := range paths {
		checksum.WriteString(hashBytes(artifacts[path]))
		checksum.WriteString("  ")
		checksum.WriteString(path)
		checksum.WriteByte('\n')
	}
	return []byte(checksum.String())
}

func baselineAudit(manifest Manifest) []byte {
	return []byte(fmt.Sprintf("# V5 Held-Out Baseline Seal Audit\n\nThe isolated baseline custodian evaluated and sealed %d held-out cases after the public rank-one selection was committed. This artifact discloses no requirement, outcome, gap, diagnostic, package membership, or promotion detail.\n\n- manifest hash: `%s`\n- selection hash: `%s`\n- algorithm: `%s`\n", manifest.CaseCount, manifest.Hash, manifest.Binding.SelectionSHA256, manifest.Algorithm))
}

func readBounded(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open held-out baseline artifact: %w", err)
	}
	defer func() { _ = file.Close() }()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() <= 0 || openedInfo.Size() > maximum {
		return nil, fmt.Errorf("held-out baseline artifact is not a bounded regular file")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(openedInfo, pathInfo) {
		return nil, fmt.Errorf("held-out baseline artifact path changed or is not regular")
	}
	data := make([]byte, int(openedInfo.Size()))
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, fmt.Errorf("read held-out baseline artifact: %w", err)
	}
	var extra [1]byte
	if count, err := file.Read(extra[:]); count != 0 || err != io.EOF {
		return nil, fmt.Errorf("held-out baseline artifact changed while reading")
	}
	return data, nil
}
