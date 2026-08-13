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

const heldOutSetMagicV10 = "KICADAI-V10-SEALED-RECORD-SET\x00"

func sealHeldOutRecordsV10(key []byte, manifest ManifestV10, cases []heldOutCaseV10, random io.Reader) ([]byte, HeldOutSealV10, error) {
	if len(cases) != expectedHeldOutV10 {
		return nil, HeldOutSealV10{}, fmt.Errorf("V10 held-out record count invalid")
	}
	var output bytes.Buffer
	output.WriteString(heldOutSetMagicV10)
	_ = binary.Write(&output, binary.BigEndian, uint32(HeldOutVersionV10))
	_ = binary.Write(&output, binary.BigEndian, uint32(len(cases)))
	plaintextHashes, aadHashes, metadataHashes, ciphertextHashes := []string{}, []string{}, []string{}, []string{}
	nonces := map[string]bool{}
	nonceBytes := 0
	for index, item := range cases {
		plaintext, err := encodeHeldOutRecordV10(item)
		if err != nil {
			return nil, HeldOutSealV10{}, err
		}
		metadata, err := json.Marshal(item.Entry)
		if err != nil {
			return nil, HeldOutSealV10{}, err
		}
		metadataHash := hashBytes(metadata)
		aad, err := heldOutRecordAADV10(manifest, index)
		if err != nil {
			return nil, HeldOutSealV10{}, err
		}
		ciphertext, currentNonceBytes, err := seal(key, plaintext, aad, random)
		if err != nil {
			return nil, HeldOutSealV10{}, err
		}
		if nonceBytes == 0 {
			nonceBytes = currentNonceBytes
		}
		if currentNonceBytes != nonceBytes || len(ciphertext) <= nonceBytes {
			return nil, HeldOutSealV10{}, fmt.Errorf("V10 record nonce format changed")
		}
		nonce := hex.EncodeToString(ciphertext[:nonceBytes])
		if nonces[nonce] {
			return nil, HeldOutSealV10{}, fmt.Errorf("V10 record nonce reused")
		}
		nonces[nonce] = true
		opened, err := open(key, ciphertext, aad, nonceBytes)
		if err != nil || !bytes.Equal(opened, plaintext) {
			return nil, HeldOutSealV10{}, fmt.Errorf("verify V10 record seal")
		}
		plaintextHashes = append(plaintextHashes, hashBytes(plaintext))
		aadHashes = append(aadHashes, hashBytes(aad))
		metadataHashes = append(metadataHashes, metadataHash)
		ciphertextHashes = append(ciphertextHashes, hashBytes(ciphertext))
		if err := writeLengthDelimited(&output, ciphertext); err != nil {
			return nil, HeldOutSealV10{}, err
		}
	}
	result := output.Bytes()
	metadata := HeldOutSealV10{Algorithm: SealAlgorithmV10, File: HeldOutCipherFileV10, CiphertextSHA256: hashBytes(result),
		PlaintextAggregateSHA256: aggregateDigestsV10(plaintextHashes), AADAggregateSHA256: aggregateDigestsV10(aadHashes), MetadataAggregateSHA256: aggregateDigestsV10(metadataHashes),
		RecordCiphertextSHA256: ciphertextHashes, NonceBytes: nonceBytes, RecordCount: len(cases)}
	return result, metadata, nil
}

func OpenHeldOutV10(key []byte, manifest ManifestV10, ciphertext []byte) (result []heldOutCaseV10, err error) {
	defer func() {
		if err != nil {
			clearHeldOutCasesV10(result)
		}
	}()
	sealMetadata := manifest.HeldOutSource
	if manifest.Schema != ManifestSchemaV10 || manifest.Version != ManifestVersionV10 || sealMetadata.Algorithm != SealAlgorithmV10 || sealMetadata.File != HeldOutCipherFileV10 ||
		sealMetadata.RecordCount != expectedHeldOutV10 || sealMetadata.NonceBytes <= 0 || len(sealMetadata.RecordCiphertextSHA256) != expectedHeldOutV10 ||
		!validSHA256(sealMetadata.MetadataAggregateSHA256) || hashBytes(ciphertext) != sealMetadata.CiphertextSHA256 {
		return nil, fmt.Errorf("V10 held-out seal metadata invalid")
	}
	reader := bytes.NewReader(ciphertext)
	magic := make([]byte, len(heldOutSetMagicV10))
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != heldOutSetMagicV10 {
		return nil, fmt.Errorf("V10 record-set magic invalid")
	}
	var version, count uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != HeldOutVersionV10 {
		return nil, fmt.Errorf("V10 record-set version invalid")
	}
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil || int(count) != expectedHeldOutV10 {
		return nil, fmt.Errorf("V10 record-set count invalid")
	}
	result = make([]heldOutCaseV10, 0, count)
	plaintextHashes, aadHashes, metadataHashes := []string{}, []string{}, []string{}
	nonces, caseIDs := map[string]bool{}, map[string]bool{}
	for index := 0; index < int(count); index++ {
		recordCiphertext, err := readLengthDelimited(reader, ots.MaxRequirementBytes+maximumMetadata+1024)
		if err != nil {
			return nil, err
		}
		if hashBytes(recordCiphertext) != sealMetadata.RecordCiphertextSHA256[index] || len(recordCiphertext) <= sealMetadata.NonceBytes {
			return nil, fmt.Errorf("V10 record ciphertext commitment invalid")
		}
		nonce := hex.EncodeToString(recordCiphertext[:sealMetadata.NonceBytes])
		if nonces[nonce] {
			return nil, fmt.Errorf("V10 record nonce repeated")
		}
		nonces[nonce] = true
		aad, err := heldOutRecordAADV10(manifest, index)
		if err != nil {
			return nil, err
		}
		plaintext, err := open(key, recordCiphertext, aad, sealMetadata.NonceBytes)
		if err != nil {
			return nil, err
		}
		item, err := decodeHeldOutRecordV10(plaintext)
		if err != nil {
			return nil, err
		}
		metadata, err := json.Marshal(item.Entry)
		if err != nil {
			return nil, fmt.Errorf("V10 record metadata invalid")
		}
		metadataHash := hashBytes(metadata)
		if item.Entry.Role != "held_out" || !item.Entry.Sealed || item.Entry.StablePath != filepath.ToSlash(filepath.Join("held_out", item.Entry.ID+".json")) || caseIDs[item.Entry.ID] {
			return nil, fmt.Errorf("V10 record identity invalid")
		}
		caseIDs[item.Entry.ID] = true
		plaintextHashes = append(plaintextHashes, hashBytes(plaintext))
		aadHashes = append(aadHashes, hashBytes(aad))
		metadataHashes = append(metadataHashes, metadataHash)
		result = append(result, item)
	}
	if reader.Len() != 0 || aggregateDigestsV10(plaintextHashes) != sealMetadata.PlaintextAggregateSHA256 || aggregateDigestsV10(aadHashes) != sealMetadata.AADAggregateSHA256 || aggregateDigestsV10(metadataHashes) != sealMetadata.MetadataAggregateSHA256 {
		return nil, fmt.Errorf("V10 record-set aggregate commitment invalid")
	}
	return result, nil
}

func heldOutRecordAADV10(manifest ManifestV10, index int) ([]byte, error) {
	fields := []string{HeldOutSchemaV10, SealAlgorithmV10, fmt.Sprintf("%d", index),
		manifest.ValidationReportSHA256, manifest.AuthorshipAttestationsSHA256, manifest.PolicySHA256, manifest.PacketSetSHA256, manifest.ContractManifestSHA256,
		manifest.ValidatorManifestSHA256, manifest.PublisherManifestSHA256, manifest.HistoricalCommitmentsSHA256, manifest.Commits.FreezeParentCommit}
	var output bytes.Buffer
	for _, field := range fields {
		if err := writeLengthDelimited(&output, []byte(field)); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func aggregateDigestsV10(digests []string) string {
	var output bytes.Buffer
	for _, digest := range digests {
		_ = writeLengthDelimited(&output, []byte(digest))
	}
	return hashBytes(output.Bytes())
}
func equalHeldOutV10(left, right []heldOutCaseV10) bool { return reflect.DeepEqual(left, right) }
