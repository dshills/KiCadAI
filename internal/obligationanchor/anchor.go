// Package obligationanchor derives immutable behavior-obligation identities.
// It contains no outcome, failure-classification, or implementation semantics.
package obligationanchor

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"regexp"
)

const CircuitOutput = "@circuit"

var semanticID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type Input struct {
	CorpusManifestSHA256 string
	Role                 string
	CaseID               string
	OperatingCaseID      string
	AssertionID          string
	ObservationKind      string
	ObservationID        string
	OutputID             string
}

// Derive uses the frozen ordered UTF-8 encoding in which every field is
// prefixed by its unsigned 32-bit big-endian byte length.
func Derive(input Input) (string, error) {
	if !validSHA256(input.CorpusManifestSHA256) {
		return "", fmt.Errorf("obligation anchor manifest hash is invalid")
	}
	if input.Role != "discovery" && input.Role != "held_out" {
		return "", fmt.Errorf("obligation anchor role is invalid")
	}
	for name, value := range map[string]string{
		"case": input.CaseID, "operating case": input.OperatingCaseID, "assertion": input.AssertionID,
		"observation": input.ObservationID,
	} {
		if !semanticID.MatchString(value) {
			return "", fmt.Errorf("obligation anchor %s ID is invalid", name)
		}
	}
	if input.ObservationKind != "port" && input.ObservationKind != "domain" && input.ObservationKind != "circuit" {
		return "", fmt.Errorf("obligation anchor observation kind is invalid")
	}
	if input.ObservationKind == "circuit" {
		if input.OutputID != CircuitOutput {
			return "", fmt.Errorf("whole-circuit obligation output is invalid")
		}
	} else if input.OutputID != input.ObservationID || !semanticID.MatchString(input.OutputID) {
		return "", fmt.Errorf("observed obligation output is invalid")
	}
	fields := []string{
		input.CorpusManifestSHA256, input.Role, input.CaseID, input.OperatingCaseID,
		input.AssertionID, input.ObservationKind, input.ObservationID, input.OutputID,
	}
	digest := sha256.New()
	for _, field := range fields {
		if err := writeField(digest, field); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeField(destination hash.Hash, value string) error {
	data := []byte(value)
	if uint64(len(data)) > uint64(^uint32(0)) {
		return fmt.Errorf("obligation anchor field exceeds uint32 length")
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	_, _ = destination.Write(length[:])
	_, _ = destination.Write(data)
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}
