package blindbaseline

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"strconv"
)

var sealMagicV6 = []byte("KICADAIV6BASE\x00")

func sealV6(key, payload, aad []byte, random io.Reader) ([]byte, int, error) {
	if len(key) != 32 || len(payload) == 0 || len(payload) > maximumPayloadSize {
		return nil, 0, fmt.Errorf("V6 held-out baseline key or payload is invalid")
	}
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
		return nil, 0, fmt.Errorf("read V6 held-out baseline nonce: %w", err)
	}
	result := append([]byte(nil), sealMagicV6...)
	result = append(result, nonce...)
	return gcm.Seal(result, nonce, payload, aad), len(nonce), nil
}

func OpenV6(key []byte, manifest V6Manifest, ciphertext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("V6 held-out baseline key must be 32 bytes for AES-256")
	}
	if err := validateManifestV6(manifest); err != nil {
		return nil, err
	}
	if hashBytes(ciphertext) != manifest.CiphertextSHA256 || len(ciphertext) <= len(sealMagicV6)+manifest.NonceBytes || !bytes.Equal(ciphertext[:len(sealMagicV6)], sealMagicV6) {
		return nil, fmt.Errorf("V6 held-out baseline ciphertext commitment is invalid")
	}
	aad := additionalDataV6(manifest.Binding, manifest.PayloadSHA256, manifest.CaseCount)
	if hashBytes(aad) != manifest.AADSHA256 {
		return nil, fmt.Errorf("V6 held-out baseline authenticated metadata is invalid")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if gcm.NonceSize() != manifest.NonceBytes {
		return nil, fmt.Errorf("V6 held-out baseline nonce size does not match the cipher")
	}
	nonceStart := len(sealMagicV6)
	nonceEnd := nonceStart + manifest.NonceBytes
	if nonceEnd > len(ciphertext) {
		return nil, fmt.Errorf("V6 held-out baseline ciphertext is truncated")
	}
	payload, err := gcm.Open(nil, ciphertext[nonceStart:nonceEnd], ciphertext[nonceEnd:], aad)
	if err != nil {
		return nil, fmt.Errorf("open V6 held-out baseline: %w", err)
	}
	if hashBytes(payload) != manifest.PayloadSHA256 {
		return nil, fmt.Errorf("V6 held-out baseline payload commitment is invalid")
	}
	return payload, nil
}

func additionalDataV6(binding V6Binding, payloadHash string, caseCount int) []byte {
	values := []string{ManifestSchemaV6, strconv.Itoa(ManifestVersionV6), AlgorithmV6}
	values = append(values, bindingCommitmentFieldsV6(binding)...)
	values = append(values, payloadHash, strconv.Itoa(caseCount))
	return lengthDelimited(values)
}

func manifestCommitmentV6(manifest V6Manifest) []byte {
	values := []string{manifest.Schema, strconv.Itoa(manifest.Version), manifest.Algorithm}
	values = append(values, bindingCommitmentFieldsV6(manifest.Binding)...)
	values = append(values, manifest.PayloadSHA256, manifest.CiphertextSHA256, manifest.AADSHA256, strconv.Itoa(manifest.NonceBytes), strconv.Itoa(manifest.CaseCount))
	return lengthDelimited(values)
}

func bindingCommitmentFieldsV6(binding V6Binding) []string {
	fields := binding.fields()
	values := make([]string, len(fields))
	for index := range fields {
		values[index] = fields[index].value
	}
	return values
}
