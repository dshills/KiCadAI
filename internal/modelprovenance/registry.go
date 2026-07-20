// Package modelprovenance owns reviewed catalog-to-simulation-model trust
// records independently from provider intent and legacy simulation plans.
package modelprovenance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	assets "kicadai"
	"kicadai/internal/simmodel"
)

const (
	Schema           = "kicadai.model-provenance-registry.v1"
	Version          = 1
	MaxRegistryBytes = 2 * 1024 * 1024
	MaxRecords       = 4096
	DefaultPath      = "data/model-provenance/registry.json"
)

type Registry struct {
	Schema  string   `json:"schema"`
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

type Record struct {
	CatalogID  string                   `json:"catalog_id"`
	Family     string                   `json:"family"`
	ModelID    string                   `json:"model_id"`
	Provenance simmodel.ModelProvenance `json:"provenance"`
}

type Diagnostic struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func LoadDefault() (Registry, []Diagnostic) {
	data, err := assets.DefaultModelProvenance.ReadFile(DefaultPath)
	if err != nil {
		return Registry{}, []Diagnostic{{Path: "document", Message: "read embedded model provenance registry: " + err.Error()}}
	}
	return DecodeStrict(bytes.NewReader(data))
}

func DecodeStrict(reader io.Reader) (Registry, []Diagnostic) {
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, io.LimitReader(reader, MaxRegistryBytes+1)); err != nil {
		return Registry{}, []Diagnostic{{Path: "document", Message: err.Error()}}
	}
	if buffer.Len() > MaxRegistryBytes {
		return Registry{}, []Diagnostic{{Path: "document", Message: "model provenance registry exceeds maximum encoded size"}}
	}
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var registry Registry
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, []Diagnostic{{Path: "document", Message: "decode model provenance registry: " + err.Error()}}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Registry{}, []Diagnostic{{Path: "document", Message: "model provenance registry contains trailing data"}}
	}
	registry = Normalize(registry)
	return registry, Validate(registry)
}

func Normalize(registry Registry) Registry {
	clone := Registry{Schema: strings.TrimSpace(registry.Schema), Version: registry.Version, Records: append([]Record(nil), registry.Records...)}
	for index := range clone.Records {
		record := &clone.Records[index]
		record.CatalogID = strings.TrimSpace(record.CatalogID)
		record.Family = strings.TrimSpace(record.Family)
		record.ModelID = strings.TrimSpace(record.ModelID)
		record.Provenance.Source = strings.TrimSpace(record.Provenance.Source)
		record.Provenance.Revision = strings.TrimSpace(record.Provenance.Revision)
		record.Provenance.SHA256 = strings.TrimSpace(record.Provenance.SHA256)
		record.Provenance.ReviewStatus = strings.TrimSpace(record.Provenance.ReviewStatus)
		record.Provenance.AllowedAnalyses = append([]string(nil), record.Provenance.AllowedAnalyses...)
		for analysisIndex := range record.Provenance.AllowedAnalyses {
			record.Provenance.AllowedAnalyses[analysisIndex] = strings.TrimSpace(record.Provenance.AllowedAnalyses[analysisIndex])
		}
		slices.Sort(record.Provenance.AllowedAnalyses)
	}
	slices.SortStableFunc(clone.Records, compareRecords)
	return clone
}

func Validate(registry Registry) []Diagnostic {
	var diagnostics []Diagnostic
	if registry.Schema != Schema || registry.Version != Version {
		diagnostics = append(diagnostics, Diagnostic{Path: "schema", Message: fmt.Sprintf("registry identity must be %s/%d", Schema, Version)})
	}
	if len(registry.Records) == 0 || len(registry.Records) > MaxRecords {
		diagnostics = append(diagnostics, Diagnostic{Path: "records", Message: "registry must contain a bounded nonempty record set"})
	}
	seen := map[string]bool{}
	previous := Record{}
	for index, record := range registry.Records {
		path := fmt.Sprintf("records[%d]", index)
		if record.CatalogID == "" || record.Family == "" || record.ModelID == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "record requires catalog, family, and model identity"})
		}
		key := record.CatalogID + "\x00" + record.ModelID
		if seen[key] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "catalog/model provenance record is duplicated"})
		}
		seen[key] = true
		if index != 0 && compareRecords(previous, record) >= 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "records must be uniquely and canonically ordered"})
		}
		previous = record
		if modelHash, exists := simmodel.ModelContentHash(record.ModelID); !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".model_id", Message: "provenance record references an unknown trusted model"})
		} else if record.Provenance.SHA256 != modelHash {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".provenance.sha256", Message: "provenance hash does not match the canonical trusted model definition"})
		}
		for _, diagnostic := range simmodel.ValidateRequiredModelProvenance(&record.Provenance, record.Provenance.AllowedAnalyses) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + "." + diagnostic.Path, Message: diagnostic.Message})
		}
	}
	slices.SortStableFunc(diagnostics, func(left, right Diagnostic) int {
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		return strings.Compare(left.Message, right.Message)
	})
	return diagnostics
}

func Hash(registry Registry) (string, error) {
	normalized := Normalize(registry)
	if diagnostics := Validate(normalized); len(diagnostics) != 0 {
		return "", fmt.Errorf("invalid model provenance registry: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func Lookup(registry Registry, catalogID, modelID string) (Record, bool) {
	normalized := Normalize(registry)
	index, found := slices.BinarySearchFunc(normalized.Records, Record{CatalogID: strings.TrimSpace(catalogID), ModelID: strings.TrimSpace(modelID)}, compareRecords)
	if !found {
		return Record{}, false
	}
	return normalized.Records[index], true
}

func compareRecords(left, right Record) int {
	if order := strings.Compare(left.CatalogID, right.CatalogID); order != 0 {
		return order
	}
	return strings.Compare(left.ModelID, right.ModelID)
}
