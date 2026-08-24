package canonicaljsonstream

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256 returns the digest of the exact compact byte representation produced
// by encoding/json.Marshal without retaining that complete representation.
func SHA256(value any) (string, error) {
	hasher := sha256.New()
	if err := Encode(hasher, value); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
