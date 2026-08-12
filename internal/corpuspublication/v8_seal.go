package corpuspublication

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"

	ots "kicadai/internal/opentopologysynthesis"
)

const heldOutSetMagicV8 = "KICADAI-V8-SEALED-RECORD-SET\x00"

func sealHeldOutRecordsV8(key []byte, manifest ManifestV8, cases []heldOutCaseV8, random io.Reader) ([]byte, HeldOutSealV8, error) {
	if len(cases) != expectedHeldOutV8 {
		return nil, HeldOutSealV8{}, fmt.Errorf("V8 held-out record count invalid")
	}
	var output bytes.Buffer
	output.WriteString(heldOutSetMagicV8)
	_ = binary.Write(&output, binary.BigEndian, uint32(HeldOutVersionV8))
	_ = binary.Write(&output, binary.BigEndian, uint32(len(cases)))
	plaintextHashes, aadHashes, metadataHashes, ciphertextHashes := []string{}, []string{}, []string{}, []string{}
	nonces := map[string]bool{}
	nonceBytes := 0
	for index, item := range cases {
		plaintext, err := encodeHeldOutRecordV8(item)
		if err != nil {
			return nil, HeldOutSealV8{}, err
		}
		metadata, err := json.Marshal(item.Entry)
		if err != nil {
			return nil, HeldOutSealV8{}, err
		}
		metadataHash := hashBytes(metadata)
		aad, err := heldOutRecordAADV8(manifest, index)
		if err != nil {
			return nil, HeldOutSealV8{}, err
		}
		ciphertext, currentNonceBytes, err := seal(key, plaintext, aad, random)
		if err != nil {
			return nil, HeldOutSealV8{}, err
		}
		if nonceBytes == 0 {
			nonceBytes = currentNonceBytes
		}
		if currentNonceBytes != nonceBytes || len(ciphertext) <= nonceBytes {
			return nil, HeldOutSealV8{}, fmt.Errorf("V8 record nonce format changed")
		}
		nonce := hex.EncodeToString(ciphertext[:nonceBytes])
		if nonces[nonce] {
			return nil, HeldOutSealV8{}, fmt.Errorf("V8 record nonce reused")
		}
		nonces[nonce] = true
		opened, err := open(key, ciphertext, aad, nonceBytes)
		if err != nil || !bytes.Equal(opened, plaintext) {
			return nil, HeldOutSealV8{}, fmt.Errorf("verify V8 record seal")
		}
		plaintextHashes = append(plaintextHashes, hashBytes(plaintext))
		aadHashes = append(aadHashes, hashBytes(aad))
		metadataHashes = append(metadataHashes, metadataHash)
		ciphertextHashes = append(ciphertextHashes, hashBytes(ciphertext))
		if err := writeLengthDelimited(&output, ciphertext); err != nil {
			return nil, HeldOutSealV8{}, err
		}
	}
	result := output.Bytes()
	metadata := HeldOutSealV8{Algorithm: SealAlgorithmV8, File: HeldOutCipherFileV8, CiphertextSHA256: hashBytes(result),
		PlaintextAggregateSHA256: aggregateDigestsV8(plaintextHashes), AADAggregateSHA256: aggregateDigestsV8(aadHashes), MetadataAggregateSHA256: aggregateDigestsV8(metadataHashes),
		RecordCiphertextSHA256: ciphertextHashes, NonceBytes: nonceBytes, RecordCount: len(cases)}
	return result, metadata, nil
}

func OpenHeldOutV8(key []byte, manifest ManifestV8, ciphertext []byte) ([]heldOutCaseV8, error) {
	sealMetadata := manifest.HeldOutSource
	if manifest.Schema != ManifestSchemaV8 || manifest.Version != ManifestVersionV8 || sealMetadata.Algorithm != SealAlgorithmV8 || sealMetadata.File != HeldOutCipherFileV8 ||
		sealMetadata.RecordCount != expectedHeldOutV8 || sealMetadata.NonceBytes <= 0 || len(sealMetadata.RecordCiphertextSHA256) != expectedHeldOutV8 ||
		!validSHA256(sealMetadata.MetadataAggregateSHA256) || hashBytes(ciphertext) != sealMetadata.CiphertextSHA256 {
		return nil, fmt.Errorf("V8 held-out seal metadata invalid")
	}
	reader := bytes.NewReader(ciphertext)
	magic := make([]byte, len(heldOutSetMagicV8))
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != heldOutSetMagicV8 {
		return nil, fmt.Errorf("V8 record-set magic invalid")
	}
	var version, count uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != HeldOutVersionV8 {
		return nil, fmt.Errorf("V8 record-set version invalid")
	}
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil || int(count) != expectedHeldOutV8 {
		return nil, fmt.Errorf("V8 record-set count invalid")
	}
	result := make([]heldOutCaseV8, 0, count)
	plaintextHashes, aadHashes, metadataHashes := []string{}, []string{}, []string{}
	nonces, caseIDs := map[string]bool{}, map[string]bool{}
	for index := 0; index < int(count); index++ {
		recordCiphertext, err := readLengthDelimited(reader, ots.MaxRequirementBytes+maximumMetadata+1024)
		if err != nil {
			return nil, err
		}
		if hashBytes(recordCiphertext) != sealMetadata.RecordCiphertextSHA256[index] || len(recordCiphertext) <= sealMetadata.NonceBytes {
			return nil, fmt.Errorf("V8 record ciphertext commitment invalid")
		}
		nonce := hex.EncodeToString(recordCiphertext[:sealMetadata.NonceBytes])
		if nonces[nonce] {
			return nil, fmt.Errorf("V8 record nonce repeated")
		}
		nonces[nonce] = true
		aad, err := heldOutRecordAADV8(manifest, index)
		if err != nil {
			return nil, err
		}
		plaintext, err := open(key, recordCiphertext, aad, sealMetadata.NonceBytes)
		if err != nil {
			return nil, err
		}
		item, err := decodeHeldOutRecordV8(plaintext)
		if err != nil {
			return nil, err
		}
		metadata, err := json.Marshal(item.Entry)
		if err != nil {
			return nil, fmt.Errorf("V8 record metadata invalid")
		}
		metadataHash := hashBytes(metadata)
		if item.Entry.Role != "held_out" || !item.Entry.Sealed || item.Entry.StablePath != filepath.ToSlash(filepath.Join("held_out", item.Entry.ID+".json")) || caseIDs[item.Entry.ID] {
			return nil, fmt.Errorf("V8 record identity invalid")
		}
		caseIDs[item.Entry.ID] = true
		plaintextHashes = append(plaintextHashes, hashBytes(plaintext))
		aadHashes = append(aadHashes, hashBytes(aad))
		metadataHashes = append(metadataHashes, metadataHash)
		result = append(result, item)
	}
	if reader.Len() != 0 || aggregateDigestsV8(plaintextHashes) != sealMetadata.PlaintextAggregateSHA256 || aggregateDigestsV8(aadHashes) != sealMetadata.AADAggregateSHA256 || aggregateDigestsV8(metadataHashes) != sealMetadata.MetadataAggregateSHA256 {
		return nil, fmt.Errorf("V8 record-set aggregate commitment invalid")
	}
	return result, nil
}

func heldOutRecordAADV8(manifest ManifestV8, index int) ([]byte, error) {
	fields := []string{HeldOutSchemaV8, SealAlgorithmV8, fmt.Sprintf("%d", index),
		manifest.ValidationReportSHA256, manifest.PolicySHA256, manifest.PacketSetSHA256, manifest.ContractManifestSHA256,
		manifest.ValidatorManifestSHA256, manifest.PublisherManifestSHA256, manifest.HistoricalCommitmentsSHA256, manifest.Commits.FreezeParentCommit}
	var output bytes.Buffer
	for _, field := range fields {
		if err := writeLengthDelimited(&output, []byte(field)); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func aggregateDigestsV8(digests []string) string {
	var output bytes.Buffer
	for _, digest := range digests {
		_ = writeLengthDelimited(&output, []byte(digest))
	}
	return hashBytes(output.Bytes())
}
func equalHeldOutV8(left, right []heldOutCaseV8) bool { return reflect.DeepEqual(left, right) }
