package corpuspublication

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	heldOutMagic       = "KICADAI-V5-HELDOUT\x00"
	maximumPayloadSize = expectedHeldOut * (ots.MaxRequirementBytes + 64*1024)
	maximumMetadata    = 64 * 1024
)

type aadBinding struct {
	validationReportSHA256      string
	packetSetSHA256             string
	contractManifestSHA256      string
	validatorManifestSHA256     string
	publisherManifestSHA256     string
	historicalCommitmentsSHA256 string
	freezeParentCommit          string
}

func encodeHeldOutPayload(cases []HeldOutCase) ([]byte, error) {
	if len(cases) != expectedHeldOut {
		return nil, fmt.Errorf("held-out payload case count is %d, want %d", len(cases), expectedHeldOut)
	}
	var output bytes.Buffer
	output.WriteString(heldOutMagic)
	if err := binary.Write(&output, binary.BigEndian, uint32(HeldOutVersion)); err != nil {
		return nil, err
	}
	if err := binary.Write(&output, binary.BigEndian, uint32(len(cases))); err != nil {
		return nil, err
	}
	for _, item := range cases {
		metadata, err := json.Marshal(item.Entry)
		if err != nil {
			return nil, fmt.Errorf("marshal held-out metadata: %w", err)
		}
		if len(metadata) > maximumMetadata || len(item.Source) == 0 || len(item.Source) > ots.MaxRequirementBytes {
			return nil, fmt.Errorf("held-out payload entry length is invalid")
		}
		if err := writeLengthDelimited(&output, metadata); err != nil {
			return nil, err
		}
		if err := writeLengthDelimited(&output, item.Source); err != nil {
			return nil, err
		}
	}
	if output.Len() > maximumPayloadSize {
		return nil, fmt.Errorf("held-out payload exceeds size limit")
	}
	return output.Bytes(), nil
}

func decodeHeldOutPayload(data []byte) ([]HeldOutCase, error) {
	if len(data) > maximumPayloadSize {
		return nil, fmt.Errorf("held-out payload exceeds size limit")
	}
	reader := bytes.NewReader(data)
	magic := make([]byte, len(heldOutMagic))
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != heldOutMagic {
		return nil, fmt.Errorf("held-out payload magic is invalid")
	}
	var version, count uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != HeldOutVersion {
		return nil, fmt.Errorf("held-out payload version is invalid")
	}
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil || count != expectedHeldOut {
		return nil, fmt.Errorf("held-out payload count is invalid")
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
			return nil, fmt.Errorf("decode held-out metadata: %w", err)
		}
		source, err := readLengthDelimited(reader, ots.MaxRequirementBytes)
		if err != nil {
			return nil, err
		}
		if hashBytes(source) != entry.RequirementSHA256 || !entry.Sealed {
			return nil, fmt.Errorf("held-out payload source commitment is invalid")
		}
		result = append(result, HeldOutCase{Entry: entry, Source: source})
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("held-out payload contains trailing bytes")
	}
	return result, nil
}

func aadBindingFromManifest(manifest Manifest) aadBinding {
	return aadBinding{
		validationReportSHA256: manifest.ValidationReportSHA256, packetSetSHA256: manifest.PacketSetSHA256,
		contractManifestSHA256: manifest.ContractManifestSHA256, validatorManifestSHA256: manifest.ValidatorManifestSHA256,
		publisherManifestSHA256:     manifest.PublisherManifestSHA256,
		historicalCommitmentsSHA256: manifest.HistoricalCommitmentsSHA256, freezeParentCommit: manifest.Commits.FreezeParentCommit,
	}
}

func publicationAAD(binding aadBinding, payloadHash string, count int) ([]byte, error) {
	fields := []string{
		HeldOutSchema, SealAlgorithm, payloadHash, binding.validationReportSHA256,
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

func seal(key, plaintext, aad []byte, random io.Reader) ([]byte, int, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, 0, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, 0, fmt.Errorf("generate held-out nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, aad), len(nonce), nil
}

func open(key, ciphertext, aad []byte, nonceBytes int) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if nonceBytes != gcm.NonceSize() || len(ciphertext) < nonceBytes+gcm.Overhead() {
		return nil, fmt.Errorf("held-out ciphertext is truncated or uses an invalid nonce")
	}
	return gcm.Open(nil, ciphertext[:nonceBytes], ciphertext[nonceBytes:], aad)
}

func OpenHeldOut(key []byte, manifest Manifest, ciphertext []byte) ([]HeldOutCase, error) {
	sealMetadata := manifest.HeldOutSource
	if manifest.Schema != ManifestSchema || manifest.Version != ManifestVersion || sealMetadata.Algorithm != SealAlgorithm ||
		sealMetadata.File != HeldOutCipherFile || sealMetadata.CaseCount != expectedHeldOut ||
		hashBytes(ciphertext) != sealMetadata.CiphertextSHA256 {
		return nil, fmt.Errorf("held-out seal metadata is invalid")
	}
	aad, err := publicationAAD(aadBindingFromManifest(manifest), sealMetadata.PayloadSHA256, sealMetadata.CaseCount)
	if err != nil {
		return nil, err
	}
	if hashBytes(aad) != sealMetadata.AADSHA256 {
		return nil, fmt.Errorf("held-out AAD commitment is invalid")
	}
	plaintext, err := open(key, ciphertext, aad, sealMetadata.NonceBytes)
	if err != nil {
		return nil, err
	}
	if hashBytes(plaintext) != sealMetadata.PayloadSHA256 {
		return nil, fmt.Errorf("held-out payload commitment is invalid")
	}
	return decodeHeldOutPayload(plaintext)
}

func writeLengthDelimited(writer io.Writer, data []byte) error {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(data)))
	if written, err := writer.Write(length[:]); err != nil {
		return err
	} else if written != len(length) {
		return io.ErrShortWrite
	}
	if written, err := writer.Write(data); err != nil {
		return err
	} else if written != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func readLengthDelimited(reader *bytes.Reader, maximum int) ([]byte, error) {
	var length uint64
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil || length > uint64(maximum) || length > uint64(reader.Len()) {
		return nil, fmt.Errorf("length-delimited held-out field is invalid")
	}
	result := make([]byte, int(length))
	if _, err := io.ReadFull(reader, result); err != nil {
		return nil, fmt.Errorf("read length-delimited held-out field: %w", err)
	}
	return result, nil
}
