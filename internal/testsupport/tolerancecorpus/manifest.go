package tolerancecorpus

import (
	_ "embed"
	"encoding/json"
)

//go:embed testdata/manifest.json
var manifestBytes []byte

type Manifest struct {
	Schema          string `json:"schema"`
	Version         int    `json:"version"`
	FrozenAt        string `json:"frozen_at"`
	RegistryVersion string `json:"registry_version"`
	RegistrySHA256  string `json:"registry_sha256"`
	Cases           []Case `json:"cases"`
}

type Case struct {
	ID                    string `json:"id"`
	Category              string `json:"category"`
	Expected              string `json:"expected"`
	PlanSHA256            string `json:"plan_sha256"`
	CatalogEvidenceSHA256 string `json:"catalog_evidence_sha256"`
	PromotionCorpus       string `json:"promotion_corpus,omitempty"`
	PromotionFixture      string `json:"promotion_fixture,omitempty"`
}

func Load() (Manifest, []byte, error) {
	contents := append([]byte(nil), manifestBytes...)
	var manifest Manifest
	err := json.Unmarshal(contents, &manifest)
	return manifest, contents, err
}
