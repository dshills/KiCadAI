package blindbaseline

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
)

var sealMagic = []byte("KICADAIV5BASE\x00")

const maximumPayloadSize = 16 << 20

func seal(key, payload, aad []byte, random io.Reader) ([]byte, int, error) {
	if len(key) != 32 || len(payload) == 0 || len(payload) > maximumPayloadSize {
		return nil, 0, fmt.Errorf("held-out baseline key or payload is invalid")
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
		return nil, 0, fmt.Errorf("read held-out baseline nonce: %w", err)
	}
	result := append([]byte(nil), sealMagic...)
	result = append(result, nonce...)
	return gcm.Seal(result, nonce, payload, aad), len(nonce), nil
}

func Open(key []byte, manifest Manifest, ciphertext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("held-out baseline key must be 32 bytes for AES-256")
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	if hashBytes(ciphertext) != manifest.CiphertextSHA256 || len(ciphertext) <= len(sealMagic)+manifest.NonceBytes || !bytes.Equal(ciphertext[:len(sealMagic)], sealMagic) {
		return nil, fmt.Errorf("held-out baseline ciphertext commitment is invalid")
	}
	aad, err := additionalData(manifest.Binding, manifest.PayloadSHA256, manifest.CaseCount)
	if err != nil || hashBytes(aad) != manifest.AADSHA256 {
		return nil, fmt.Errorf("held-out baseline authenticated metadata is invalid")
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
		return nil, fmt.Errorf("held-out baseline manifest nonce size does not match the cipher")
	}
	nonceStart := len(sealMagic)
	nonceEnd := nonceStart + manifest.NonceBytes
	if nonceEnd > len(ciphertext) {
		return nil, fmt.Errorf("held-out baseline ciphertext is truncated")
	}
	payload, err := gcm.Open(nil, ciphertext[nonceStart:nonceEnd], ciphertext[nonceEnd:], aad)
	if err != nil {
		return nil, fmt.Errorf("open held-out baseline: %w", err)
	}
	if hashBytes(payload) != manifest.PayloadSHA256 {
		return nil, fmt.Errorf("held-out baseline payload commitment is invalid")
	}
	return payload, nil
}

func additionalData(binding Binding, payloadHash string, caseCount int) ([]byte, error) {
	values := []string{ManifestSchema, strconv.Itoa(ManifestVersion), Algorithm}
	values = append(values, bindingCommitmentFields(binding)...)
	values = append(values, payloadHash, strconv.Itoa(caseCount))
	return lengthDelimited(values), nil
}

func manifestCommitment(manifest Manifest) ([]byte, error) {
	values := []string{manifest.Schema, strconv.Itoa(manifest.Version), manifest.Algorithm}
	values = append(values, bindingCommitmentFields(manifest.Binding)...)
	values = append(values,
		manifest.PayloadSHA256,
		manifest.CiphertextSHA256,
		manifest.AADSHA256,
		strconv.Itoa(manifest.NonceBytes),
		strconv.Itoa(manifest.CaseCount),
	)
	return lengthDelimited(values), nil
}

func bindingCommitmentFields(binding Binding) []string {
	return []string{
		binding.StartingCommit,
		binding.ContractFreezeCommit,
		binding.CorpusFreezeCommit,
		binding.SelectionFreezeCommit,
		binding.PublisherParentCommit,
		binding.CorpusManifestSHA256,
		binding.SourceCiphertextSHA256,
		binding.SelectionSHA256,
		binding.EvaluatorPolicy,
		binding.ImpactRegistrySHA256,
		binding.SynthesisPolicySHA256,
		binding.GapPolicySHA256,
		binding.SelectionPolicySHA256,
		binding.InventorySHA256,
		binding.CatalogSHA256,
		binding.ModelRegistrySHA256,
	}
}

func lengthDelimited(values []string) []byte {
	var result bytes.Buffer
	for _, value := range values {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		result.Write(length[:])
		result.WriteString(value)
	}
	return result.Bytes()
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
