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

var sealSetMagicV8 = []byte("KICADAIV8BASERECORDS\x00")

func sealRecordsV8(key []byte, binding V8Binding, records [][]byte, random io.Reader) ([]byte, V8Manifest, error) {
	if len(key) != 32 || len(records) != expectedCasesV8 {
		return nil, V8Manifest{}, fmt.Errorf("V8 held-out baseline key or record set is invalid")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, V8Manifest{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, V8Manifest{}, err
	}
	if gcm.NonceSize() != nonceBytesV8 {
		return nil, V8Manifest{}, fmt.Errorf("V8 held-out baseline nonce format changed")
	}
	var output bytes.Buffer
	_, _ = output.Write(sealSetMagicV8)
	if err := binary.Write(&output, binary.BigEndian, uint32(ManifestVersionV8)); err != nil {
		return nil, V8Manifest{}, err
	}
	if err := binary.Write(&output, binary.BigEndian, uint32(len(records))); err != nil {
		return nil, V8Manifest{}, err
	}
	plaintextHashes := make([]string, 0, len(records))
	aadHashes := make([]string, 0, len(records))
	ciphertextHashes := make([]string, 0, len(records))
	nonces := make(map[string]bool, len(records))
	total := 0
	for index, record := range records {
		if len(record) == 0 || len(record) > maximumPayloadSize-total {
			return nil, V8Manifest{}, fmt.Errorf("V8 held-out baseline record set exceeds bounds")
		}
		total += len(record)
		aad := additionalDataV8(binding, index, len(records))
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(random, nonce); err != nil {
			return nil, V8Manifest{}, fmt.Errorf("read V8 held-out baseline nonce: %w", err)
		}
		nonceID := hex.EncodeToString(nonce)
		if nonces[nonceID] {
			return nil, V8Manifest{}, fmt.Errorf("V8 held-out baseline nonce reused")
		}
		nonces[nonceID] = true
		sealed := make([]byte, len(nonce), len(nonce)+len(record)+gcm.Overhead())
		copy(sealed, nonce)
		sealed = gcm.Seal(sealed, nonce, record, aad)
		if err := writeRecordV8(&output, sealed); err != nil {
			return nil, V8Manifest{}, err
		}
		plaintextHashes = append(plaintextHashes, hashBytes(record))
		aadHashes = append(aadHashes, hashBytes(aad))
		ciphertextHashes = append(ciphertextHashes, hashBytes(sealed))
	}
	ciphertext := output.Bytes()
	manifest := V8Manifest{Schema: ManifestSchemaV8, Version: ManifestVersionV8, Algorithm: AlgorithmV8, Binding: binding,
		CiphertextSHA256: hashBytes(ciphertext), PlaintextAggregateSHA256: aggregateHashesV8(plaintextHashes),
		AADAggregateSHA256: aggregateHashesV8(aadHashes), RecordCiphertextSHA256: ciphertextHashes,
		NonceBytes: nonceBytesV8, CaseCount: len(records)}
	manifest.Hash = hashBytes(manifestCommitmentV8(manifest))
	return ciphertext, manifest, nil
}

// OpenV8 authenticates and opens every V8 record without publishing its
// plaintext or deriving any outcome-sensitive public metadata.
func OpenV8(key []byte, manifest V8Manifest, ciphertext []byte) ([][]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("V8 held-out baseline key must be 32 bytes for AES-256")
	}
	if err := validateManifestV8(manifest); err != nil {
		return nil, err
	}
	if hashBytes(ciphertext) != manifest.CiphertextSHA256 {
		return nil, fmt.Errorf("V8 held-out baseline ciphertext commitment is invalid")
	}
	reader := bytes.NewReader(ciphertext)
	magic := make([]byte, len(sealSetMagicV8))
	if _, err := io.ReadFull(reader, magic); err != nil || !bytes.Equal(magic, sealSetMagicV8) {
		return nil, fmt.Errorf("V8 held-out baseline record-set magic is invalid")
	}
	var version, count uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != ManifestVersionV8 {
		return nil, fmt.Errorf("V8 held-out baseline record-set version is invalid")
	}
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil || int(count) != manifest.CaseCount {
		return nil, fmt.Errorf("V8 held-out baseline record-set count is invalid")
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
		sealed, err := readRecordV8(reader, maximumPayloadSize+nonceBytesV8+gcm.Overhead())
		if err != nil || len(sealed) <= nonceBytesV8+gcm.Overhead() || hashBytes(sealed) != manifest.RecordCiphertextSHA256[index] {
			return nil, fmt.Errorf("V8 held-out baseline record ciphertext is invalid")
		}
		nonce := sealed[:nonceBytesV8]
		nonceID := hex.EncodeToString(nonce)
		if nonces[nonceID] {
			return nil, fmt.Errorf("V8 held-out baseline nonce repeated")
		}
		nonces[nonceID] = true
		aad := additionalDataV8(manifest.Binding, index, int(count))
		plaintext, err := gcm.Open(nil, nonce, sealed[nonceBytesV8:], aad)
		if err != nil || len(plaintext) == 0 {
			return nil, fmt.Errorf("open V8 held-out baseline record")
		}
		if len(plaintext) > maximumPayloadSize-total {
			return nil, fmt.Errorf("V8 held-out baseline plaintext exceeds bounds")
		}
		total += len(plaintext)
		records = append(records, plaintext)
		plaintextHashes = append(plaintextHashes, hashBytes(plaintext))
		aadHashes = append(aadHashes, hashBytes(aad))
	}
	if reader.Len() != 0 || aggregateHashesV8(plaintextHashes) != manifest.PlaintextAggregateSHA256 || aggregateHashesV8(aadHashes) != manifest.AADAggregateSHA256 {
		return nil, fmt.Errorf("V8 held-out baseline record-set aggregate is invalid")
	}
	return records, nil
}

func additionalDataV8(binding V8Binding, index, count int) []byte {
	values := []string{ManifestSchemaV8, strconv.Itoa(ManifestVersionV8), AlgorithmV8}
	values = append(values, bindingCommitmentFieldsV8(binding)...)
	values = append(values, strconv.Itoa(index), strconv.Itoa(count))
	return lengthDelimited(values)
}

func manifestCommitmentV8(manifest V8Manifest) []byte {
	values := []string{manifest.Schema, strconv.Itoa(manifest.Version), manifest.Algorithm}
	values = append(values, bindingCommitmentFieldsV8(manifest.Binding)...)
	values = append(values, manifest.CiphertextSHA256, manifest.PlaintextAggregateSHA256, manifest.AADAggregateSHA256)
	values = append(values, manifest.RecordCiphertextSHA256...)
	values = append(values, strconv.Itoa(manifest.NonceBytes), strconv.Itoa(manifest.CaseCount))
	return lengthDelimited(values)
}

func bindingCommitmentFieldsV8(binding V8Binding) []string {
	fields := binding.fields()
	values := make([]string, len(fields))
	for index := range fields {
		values[index] = fields[index].value
	}
	return values
}

func aggregateHashesV8(hashes []string) string { return hashBytes(lengthDelimited(hashes)) }

func writeRecordV8(writer io.Writer, data []byte) error {
	if len(data) == 0 || uint64(len(data)) > uint64(^uint32(0)) {
		return fmt.Errorf("V8 held-out baseline record length is invalid")
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err := writer.Write(data)
	return err
}

func readRecordV8(reader *bytes.Reader, maximum int) ([]byte, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil || length == 0 || int64(length) > int64(maximum) || int64(length) > int64(reader.Len()) {
		return nil, fmt.Errorf("V8 held-out baseline record length is invalid")
	}
	data := make([]byte, int(length))
	_, err := io.ReadFull(reader, data)
	return data, err
}
