package blindbaseline

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
)

var sealSetMagicV10 = []byte("KICADAIV10BASERECORDS\x00")

func sealRecordsV10(key []byte, binding V10Binding, records [][]byte, random io.Reader) ([]byte, V10Manifest, error) {
	if len(key) != 32 || len(records) != expectedCasesV10 {
		return nil, V10Manifest{}, fmt.Errorf("V10 held-out baseline key or record set is invalid")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, V10Manifest{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, V10Manifest{}, err
	}
	if gcm.NonceSize() != nonceBytesV10 {
		return nil, V10Manifest{}, fmt.Errorf("V10 held-out baseline nonce format changed")
	}
	var output bytes.Buffer
	_, _ = output.Write(sealSetMagicV10)
	if err := binary.Write(&output, binary.BigEndian, uint32(ManifestVersionV10)); err != nil {
		return nil, V10Manifest{}, err
	}
	if err := binary.Write(&output, binary.BigEndian, uint32(len(records))); err != nil {
		return nil, V10Manifest{}, err
	}
	plaintextHashes := make([]string, 0, len(records))
	aadHashes := make([]string, 0, len(records))
	ciphertextHashes := make([]string, 0, len(records))
	nonces := make(map[string]bool, len(records))
	total := 0
	for index, record := range records {
		if len(record) == 0 || len(record) > maximumPayloadSize-total {
			return nil, V10Manifest{}, fmt.Errorf("V10 held-out baseline record set exceeds bounds")
		}
		total += len(record)
		aad := additionalDataV10(binding, index, len(records))
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(random, nonce); err != nil {
			return nil, V10Manifest{}, fmt.Errorf("read V10 held-out baseline nonce: %w", err)
		}
		nonceID := hex.EncodeToString(nonce)
		if nonces[nonceID] {
			return nil, V10Manifest{}, fmt.Errorf("V10 held-out baseline nonce reused")
		}
		nonces[nonceID] = true
		sealed := make([]byte, len(nonce), len(nonce)+len(record)+gcm.Overhead())
		copy(sealed, nonce)
		sealed = gcm.Seal(sealed, nonce, record, aad)
		if err := writeRecordV10(&output, sealed); err != nil {
			return nil, V10Manifest{}, err
		}
		plaintextHashes = append(plaintextHashes, hashBytes(record))
		aadHashes = append(aadHashes, hashBytes(aad))
		ciphertextHashes = append(ciphertextHashes, hashBytes(sealed))
	}
	ciphertext := output.Bytes()
	manifest := V10Manifest{Schema: ManifestSchemaV10, Version: ManifestVersionV10, Algorithm: AlgorithmV10, Binding: binding,
		CiphertextSHA256: hashBytes(ciphertext), PlaintextAggregateSHA256: aggregateHashesV10(plaintextHashes),
		AADAggregateSHA256: aggregateHashesV10(aadHashes), RecordCiphertextSHA256: ciphertextHashes,
		NonceBytes: nonceBytesV10, CaseCount: len(records)}
	manifest.Hash = hashBytes(manifestCommitmentV10(manifest))
	return ciphertext, manifest, nil
}

// OpenV10 authenticates and opens every V10 record without publishing its
// plaintext or deriving any outcome-sensitive public metadata.
func OpenV10(key []byte, manifest V10Manifest, ciphertext []byte) ([][]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("V10 held-out baseline key must be 32 bytes for AES-256")
	}
	if err := validateManifestV10(manifest); err != nil {
		return nil, err
	}
	if hashBytes(ciphertext) != manifest.CiphertextSHA256 {
		return nil, fmt.Errorf("V10 held-out baseline ciphertext commitment is invalid")
	}
	reader := bytes.NewReader(ciphertext)
	magic := make([]byte, len(sealSetMagicV10))
	if _, err := io.ReadFull(reader, magic); err != nil || !bytes.Equal(magic, sealSetMagicV10) {
		return nil, fmt.Errorf("V10 held-out baseline record-set magic is invalid")
	}
	var version, count uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != ManifestVersionV10 {
		return nil, fmt.Errorf("V10 held-out baseline record-set version is invalid")
	}
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil || int(count) != manifest.CaseCount {
		return nil, fmt.Errorf("V10 held-out baseline record-set count is invalid")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	records := make([][]byte, 0, count)
	plaintextHashes := make([]string, 0, count)
	aadHashes := make([]string, 0, count)
	nonces := make(map[string]bool, count)
	total := 0
	for index := 0; index < int(count); index++ {
		sealed, err := readRecordV10(reader, maximumPayloadSize+nonceBytesV10+gcm.Overhead())
		if err != nil || len(sealed) <= nonceBytesV10+gcm.Overhead() || hashBytes(sealed) != manifest.RecordCiphertextSHA256[index] {
			return nil, fmt.Errorf("V10 held-out baseline record ciphertext is invalid")
		}
		nonce := sealed[:nonceBytesV10]
		nonceID := hex.EncodeToString(nonce)
		if nonces[nonceID] {
			return nil, fmt.Errorf("V10 held-out baseline nonce repeated")
		}
		nonces[nonceID] = true
		aad := additionalDataV10(manifest.Binding, index, int(count))
		plaintext, err := gcm.Open(nil, nonce, sealed[nonceBytesV10:], aad)
		if err != nil || len(plaintext) == 0 {
			return nil, fmt.Errorf("open V10 held-out baseline record")
		}
		if len(plaintext) > maximumPayloadSize-total {
			return nil, fmt.Errorf("V10 held-out baseline plaintext exceeds bounds")
		}
		total += len(plaintext)
		records = append(records, plaintext)
		plaintextHashes = append(plaintextHashes, hashBytes(plaintext))
		aadHashes = append(aadHashes, hashBytes(aad))
	}
	if reader.Len() != 0 || aggregateHashesV10(plaintextHashes) != manifest.PlaintextAggregateSHA256 || aggregateHashesV10(aadHashes) != manifest.AADAggregateSHA256 {
		return nil, fmt.Errorf("V10 held-out baseline record-set aggregate is invalid")
	}
	return records, nil
}

func additionalDataV10(binding V10Binding, index, count int) []byte {
	values := []string{ManifestSchemaV10, strconv.Itoa(ManifestVersionV10), AlgorithmV10}
	values = append(values, bindingCommitmentFieldsV10(binding)...)
	values = append(values, strconv.Itoa(index), strconv.Itoa(count))
	return lengthDelimited(values)
}

func manifestCommitmentV10(manifest V10Manifest) []byte {
	values := []string{manifest.Schema, strconv.Itoa(manifest.Version), manifest.Algorithm}
	values = append(values, bindingCommitmentFieldsV10(manifest.Binding)...)
	values = append(values, manifest.CiphertextSHA256, manifest.PlaintextAggregateSHA256, manifest.AADAggregateSHA256)
	values = append(values, manifest.RecordCiphertextSHA256...)
	values = append(values, strconv.Itoa(manifest.NonceBytes), strconv.Itoa(manifest.CaseCount))
	return lengthDelimited(values)
}

func bindingCommitmentFieldsV10(binding V10Binding) []string {
	fields := binding.fields()
	values := make([]string, len(fields))
	for index := range fields {
		values[index] = fields[index].value
	}
	return values
}

func aggregateHashesV10(hashes []string) string { return hashBytes(lengthDelimited(hashes)) }

func writeRecordV10(writer io.Writer, data []byte) error {
	if len(data) == 0 || uint64(len(data)) > uint64(^uint32(0)) {
		return fmt.Errorf("V10 held-out baseline record length is invalid")
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err := writer.Write(data)
	return err
}

func readRecordV10(reader *bytes.Reader, maximum int) ([]byte, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil || length == 0 || int64(length) > int64(maximum) || int64(length) > int64(reader.Len()) {
		return nil, fmt.Errorf("V10 held-out baseline record length is invalid")
	}
	data := make([]byte, int(length))
	_, err := io.ReadFull(reader, data)
	return data, err
}
