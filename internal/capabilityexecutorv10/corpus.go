package capabilityexecutorv10

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"kicadai/internal/corpuspublication"
)

const maximumPublicCorpusFileBytes int64 = 4 * 1024 * 1024

// LoadPublicDiscovery authenticates and loads only the public V10 discovery
// partition. It has no key parameter and never opens held-out plaintext.
func LoadPublicDiscovery(root string) (PublicCorpus, error) {
	before, err := corpuspublication.VerifyPublicationV10(root)
	if err != nil {
		return PublicCorpus{}, fmt.Errorf("authenticate V10 publication: %w", err)
	}
	loaded, err := loadPublicDiscovery(root, before.ManifestSHA256)
	if err != nil {
		return PublicCorpus{}, err
	}
	after, err := corpuspublication.VerifyPublicationV10(root)
	if err != nil || !reflect.DeepEqual(before, after) {
		return PublicCorpus{}, fmt.Errorf("V10 publication changed while loading")
	}
	return loaded, nil
}

func loadPublicDiscovery(root string, manifestSHA256 string) (PublicCorpus, error) {
	manifestSource, err := readRegularFile(filepath.Join(root, corpuspublication.ManifestFileV10))
	if err != nil {
		return PublicCorpus{}, err
	}
	if hashBytes(manifestSource) != manifestSHA256 {
		return PublicCorpus{}, fmt.Errorf("V10 manifest differs from authenticated commitment")
	}
	obligationSource, err := readRegularFile(filepath.Join(root, corpuspublication.DiscoveryObligationsFileV10))
	if err != nil {
		return PublicCorpus{}, err
	}
	var manifest corpuspublication.ManifestV10
	var obligations corpuspublication.DiscoveryObligationsV10
	if err := decodeStrictJSON(manifestSource, &manifest); err != nil {
		return PublicCorpus{}, fmt.Errorf("decode V10 manifest: %w", err)
	}
	if err := decodeStrictJSON(obligationSource, &obligations); err != nil {
		return PublicCorpus{}, fmt.Errorf("decode V10 discovery obligations: %w", err)
	}
	if manifest.Schema != corpuspublication.ManifestSchemaV10 || manifest.Version != corpuspublication.ManifestVersionV10 ||
		manifest.DiscoveryCaseCount != 24 || manifest.HeldOutCaseCount != 24 || len(manifest.Entries) != 24 ||
		obligations.Schema != "kicadai.closed-loop-open-set-discovery-obligations.v10" ||
		obligations.Version != corpuspublication.ManifestVersionV10 || obligations.CorpusManifestSHA256 != manifestSHA256 {
		return PublicCorpus{}, fmt.Errorf("V10 public corpus shape or binding is invalid")
	}
	knownCases := make(map[string]bool, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		knownCases[entry.ID] = true
	}
	byCase := make(map[string][]corpuspublication.ObligationV10, 24)
	for _, obligation := range obligations.Obligations {
		if !knownCases[obligation.CaseID] {
			return PublicCorpus{}, fmt.Errorf("V10 discovery obligation references an unknown case")
		}
		byCase[obligation.CaseID] = append(byCase[obligation.CaseID], obligation)
	}
	result := PublicCorpus{ManifestSHA256: manifestSHA256, Cases: make([]CaseInput, 0, 24)}
	for index, entry := range manifest.Entries {
		wantID := fmt.Sprintf("v10_case_%03d", index+1)
		wantPath := filepath.ToSlash(filepath.Join("discovery", wantID+".json"))
		if entry.ID != wantID || entry.Role != "discovery" || entry.Sealed || entry.StablePath != wantPath || len(byCase[entry.ID]) == 0 {
			return PublicCorpus{}, fmt.Errorf("V10 public manifest order or path is invalid")
		}
		source, err := readContainedRegularFile(root, entry.StablePath)
		if err != nil {
			return PublicCorpus{}, err
		}
		if hashBytes(source) != entry.RequirementSHA256 {
			return PublicCorpus{}, fmt.Errorf("V10 discovery requirement %q differs from its commitment", entry.ID)
		}
		caseObligations := append([]corpuspublication.ObligationV10(nil), byCase[entry.ID]...)
		sort.Slice(caseObligations, func(i, j int) bool { return caseObligations[i].Anchor < caseObligations[j].Anchor })
		result.Cases = append(result.Cases, CaseInput{Entry: entry, RequirementSource: source, Obligations: caseObligations})
	}
	return result, nil
}

func readContainedRegularFile(root, stablePath string) ([]byte, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(cleanRoot, filepath.FromSlash(stablePath))
	relative, err := filepath.Rel(cleanRoot, path)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("V10 discovery path escapes corpus root")
	}
	return readRegularFile(path)
}

func readRegularFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() <= 0 || opened.Size() > maximumPublicCorpusFileBytes {
		return nil, fmt.Errorf("V10 public corpus file is not a bounded nonempty regular file")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(opened, pathInfo) {
		return nil, fmt.Errorf("V10 public corpus path changed or is not regular")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumPublicCorpusFileBytes+1))
	if err != nil || int64(len(data)) != opened.Size() {
		return nil, fmt.Errorf("read stable V10 public corpus file")
	}
	return data, nil
}

func decodeStrictJSON(source []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
