package capabilitypackages

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func hashPlan(plan GenericPlan) (string, error) {
	plan.Hash = ""
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
