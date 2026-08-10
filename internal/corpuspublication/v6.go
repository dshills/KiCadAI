package corpuspublication

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	ManifestSchemaV6  = "kicadai.closed-loop-open-set-corpus.v6"
	ManifestVersionV6 = 6
	HeldOutSchemaV6   = "kicadai.closed-loop-open-set-held-out-source.v6"
	HeldOutVersionV6  = 6
	heldOutMagicV6    = "KICADAI-V6-HELDOUT\x00"
)

// PublishV6 reuses the frozen V5 validation-to-publication preparation while
// giving V6 ciphertext an unambiguous schema, payload magic, version, and AAD.
func PublishV6(request Request) (Result, error) {
	prepared, err := prepare(request)
	if err != nil {
		return Result{}, err
	}
	prepared.manifest.Schema = ManifestSchemaV6
	prepared.manifest.Version = ManifestVersionV6
	return prepared.publishV6()
}

func (prepared preparedCorpus) publishV6() (result Result, err error) {
	if _, statErr := os.Lstat(prepared.destination); statErr == nil {
		return Result{}, fmt.Errorf("destination already exists")
	} else if !os.IsNotExist(statErr) {
		return Result{}, fmt.Errorf("inspect destination: %w", statErr)
	}
	parent := filepath.Dir(prepared.destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Result{}, fmt.Errorf("create destination parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(prepared.destination)+".stage-")
	if err != nil {
		return Result{}, fmt.Errorf("create publication stage: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	key, err := createExclusiveKey(prepared.repositoryRoot, prepared.keyPath, prepared.random)
	if err != nil {
		return Result{}, err
	}
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			_ = os.Remove(prepared.keyPath)
		}
	}()

	payload, err := encodeHeldOutPayloadV6(prepared.heldOut)
	if err != nil {
		return Result{}, err
	}
	payloadHash := hashBytes(payload)
	aad, err := publicationAADV6(aadBindingFromManifest(prepared.manifest), payloadHash, len(prepared.heldOut))
	if err != nil {
		return Result{}, err
	}
	ciphertext, nonceBytes, err := seal(key, payload, aad, prepared.random)
	if err != nil {
		return Result{}, err
	}
	opened, err := open(key, ciphertext, aad, nonceBytes)
	if err != nil || !bytes.Equal(opened, payload) {
		return Result{}, fmt.Errorf("verify V6 held-out seal")
	}
	prepared.manifest.HeldOutSource = HeldOutSeal{
		Algorithm: SealAlgorithm, File: HeldOutCipherFile, PayloadSHA256: payloadHash,
		CiphertextSHA256: hashBytes(ciphertext), AADSHA256: hashBytes(aad), NonceBytes: nonceBytes,
		CaseCount: len(prepared.heldOut),
	}

	if err := writePublication(stage, prepared, ciphertext); err != nil {
		return Result{}, err
	}
	manifestBytes, err := marshalStable(prepared.manifest)
	if err != nil {
		return Result{}, err
	}
	if err := writeExclusive(filepath.Join(stage, ManifestFile), manifestBytes, 0o644); err != nil {
		return Result{}, err
	}
	audit := auditBytesV6(prepared.manifest, hashBytes(manifestBytes))
	if err := writeExclusive(filepath.Join(stage, AuditFile), audit, 0o644); err != nil {
		return Result{}, err
	}
	if err := writeChecksums(stage); err != nil {
		return Result{}, err
	}
	if err := syncTree(stage); err != nil {
		return Result{}, err
	}
	if err := renameNoReplace(stage, prepared.destination); err != nil {
		return Result{}, err
	}
	keyCommitted = true
	return Result{
		Manifest: prepared.manifest, ManifestSHA256: hashBytes(manifestBytes),
		DiscoveryCases: len(prepared.discovery), HeldOutCases: len(prepared.heldOut),
	}, nil
}

func encodeHeldOutPayloadV6(cases []HeldOutCase) ([]byte, error) {
	if len(cases) != expectedHeldOut {
		return nil, fmt.Errorf("V6 held-out payload case count is %d, want %d", len(cases), expectedHeldOut)
	}
	var output bytes.Buffer
	output.WriteString(heldOutMagicV6)
	if err := binary.Write(&output, binary.BigEndian, uint32(HeldOutVersionV6)); err != nil {
		return nil, err
	}
	if err := binary.Write(&output, binary.BigEndian, uint32(len(cases))); err != nil {
		return nil, err
	}
	for _, item := range cases {
		metadata, err := json.Marshal(item.Entry)
		if err != nil {
			return nil, fmt.Errorf("marshal V6 held-out metadata: %w", err)
		}
		if len(metadata) > maximumMetadata {
			return nil, fmt.Errorf("V6 held-out metadata length %d exceeds limit %d", len(metadata), maximumMetadata)
		}
		if len(item.Source) == 0 {
			return nil, fmt.Errorf("V6 held-out source for %s is empty", item.Entry.ID)
		}
		if len(item.Source) > ots.MaxRequirementBytes {
			return nil, fmt.Errorf("V6 held-out source for %s exceeds limit %d", item.Entry.ID, ots.MaxRequirementBytes)
		}
		if err := writeLengthDelimited(&output, metadata); err != nil {
			return nil, err
		}
		if err := writeLengthDelimited(&output, item.Source); err != nil {
			return nil, err
		}
	}
	if output.Len() > maximumPayloadSize {
		return nil, fmt.Errorf("V6 held-out payload exceeds size limit")
	}
	return output.Bytes(), nil
}

func decodeHeldOutPayloadV6(data []byte) ([]HeldOutCase, error) {
	if len(data) > maximumPayloadSize {
		return nil, fmt.Errorf("V6 held-out payload exceeds size limit")
	}
	reader := bytes.NewReader(data)
	magic := make([]byte, len(heldOutMagicV6))
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != heldOutMagicV6 {
		return nil, fmt.Errorf("V6 held-out payload magic is invalid")
	}
	var version, count uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != HeldOutVersionV6 {
		return nil, fmt.Errorf("V6 held-out payload version is invalid")
	}
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil || count != expectedHeldOut {
		return nil, fmt.Errorf("V6 held-out payload count is invalid")
	}
	result := make([]HeldOutCase, 0, count)
	for range count {
		metadata, err := readLengthDelimited(reader, maximumMetadata)
		if err != nil {
			return nil, err
		}
		var entry Entry
		decoder := json.NewDecoder(bytes.NewReader(metadata))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&entry); err != nil {
			return nil, fmt.Errorf("decode V6 held-out metadata: %w", err)
		}
		source, err := readLengthDelimited(reader, ots.MaxRequirementBytes)
		if err != nil {
			return nil, err
		}
		if hashBytes(source) != entry.RequirementSHA256 || !entry.Sealed {
			return nil, fmt.Errorf("V6 held-out payload source commitment is invalid")
		}
		result = append(result, HeldOutCase{Entry: entry, Source: source})
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("V6 held-out payload contains trailing bytes")
	}
	return result, nil
}

func publicationAADV6(binding aadBinding, payloadHash string, count int) ([]byte, error) {
	fields := []string{
		HeldOutSchemaV6, SealAlgorithm, payloadHash, binding.validationReportSHA256,
		binding.packetSetSHA256, binding.contractManifestSHA256, binding.validatorManifestSHA256,
		binding.publisherManifestSHA256, binding.historicalCommitmentsSHA256, binding.freezeParentCommit,
		fmt.Sprintf("%d", count),
	}
	var output bytes.Buffer
	for _, field := range fields {
		if err := writeLengthDelimited(&output, []byte(field)); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func OpenHeldOutV6(key []byte, manifest Manifest, ciphertext []byte) ([]HeldOutCase, error) {
	sealMetadata := manifest.HeldOutSource
	if manifest.Schema != ManifestSchemaV6 || manifest.Version != ManifestVersionV6 || sealMetadata.Algorithm != SealAlgorithm ||
		sealMetadata.File != HeldOutCipherFile || sealMetadata.CaseCount != expectedHeldOut ||
		hashBytes(ciphertext) != sealMetadata.CiphertextSHA256 {
		return nil, fmt.Errorf("V6 held-out seal metadata is invalid")
	}
	aad, err := publicationAADV6(aadBindingFromManifest(manifest), sealMetadata.PayloadSHA256, sealMetadata.CaseCount)
	if err != nil {
		return nil, err
	}
	if hashBytes(aad) != sealMetadata.AADSHA256 {
		return nil, fmt.Errorf("V6 held-out AAD commitment is invalid")
	}
	plaintext, err := open(key, ciphertext, aad, sealMetadata.NonceBytes)
	if err != nil {
		return nil, err
	}
	if hashBytes(plaintext) != sealMetadata.PayloadSHA256 {
		return nil, fmt.Errorf("V6 held-out payload commitment is invalid")
	}
	return decodeHeldOutPayloadV6(plaintext)
}
