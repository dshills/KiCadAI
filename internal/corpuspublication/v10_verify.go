package corpuspublication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"kicadai/internal/corpusfreezev10"
)

type VerificationV10 struct {
	ManifestSHA256       string
	DiscoveryCases       int
	HeldOutCases         int
	DiscoveryObligations int
	HeldOutObligations   int
}

type publicPublicationV10 struct {
	manifest      ManifestV10
	manifestHash  string
	validation    PublicValidationReportV10
	authorship    AuthorshipAttestationsV10
	discovery     map[string][]byte
	discoveryObl  DiscoveryObligationsV10
	heldOutCommit HeldOutObligationCommitmentV10
	ciphertext    []byte
}

// VerifyPublicationV10 independently authenticates the public V10 publication.
// It opens no key and exposes no held-out identity or requirement content.
func VerifyPublicationV10(root string) (VerificationV10, error) {
	publication, err := verifyPublicPublicationV10(root)
	if err != nil {
		return VerificationV10{}, err
	}
	return VerificationV10{ManifestSHA256: publication.manifestHash, DiscoveryCases: expectedDiscoveryV10,
		HeldOutCases: expectedHeldOutV10, DiscoveryObligations: len(publication.discoveryObl.Obligations),
		HeldOutObligations: publication.heldOutCommit.ObligationCount}, nil
}

// VerifyPublicationV10WithKey additionally authenticates every encrypted V10
// held-out record. It returns aggregate counts only and retains no plaintext.
func VerifyPublicationV10WithKey(root string, key []byte) (VerificationV10, error) {
	publication, err := verifyPublicPublicationV10(root)
	if err != nil {
		return VerificationV10{}, err
	}
	heldOut, err := OpenHeldOutV10(key, publication.manifest, publication.ciphertext)
	if err != nil {
		return VerificationV10{}, fmt.Errorf("authenticate V10 held-out records: %w", err)
	}
	seen := make(map[string]bool, expectedCasesV10)
	for _, entry := range publication.manifest.Entries {
		seen[entry.ID] = true
	}
	for index := range heldOut {
		item := &heldOut[index]
		wantID := fmt.Sprintf("v10_case_%03d", expectedDiscoveryV10+index+1)
		wantAuthor := fmt.Sprintf("author_%d", (index/4)+1)
		if item.Entry.ID != wantID || item.Entry.AuthorSlot != wantAuthor || seen[item.Entry.ID] || !validEntryV10(item.Entry, true) {
			clearHeldOutCasesV10(heldOut)
			return VerificationV10{}, fmt.Errorf("V10 held-out record order or metadata is invalid")
		}
		seen[item.Entry.ID] = true
	}
	heldOutDigests := make([]string, 0, len(heldOut))
	for _, item := range heldOut {
		data, err := json.Marshal(entryEvidenceV10(item.Entry))
		if err != nil {
			clearHeldOutCasesV10(heldOut)
			return VerificationV10{}, err
		}
		heldOutDigests = append(heldOutDigests, hashBytes(data))
	}
	sort.Strings(heldOutDigests)
	if aggregateDigestsV10(heldOutDigests) != publication.validation.HeldOutEntryAggregateSHA256 {
		clearHeldOutCasesV10(heldOut)
		return VerificationV10{}, fmt.Errorf("V10 held-out validation aggregate does not reproduce")
	}
	discoveryObligations, heldOutCommitment, err := deriveObligationsV10(publication.manifestHash, publication.manifest, publication.discovery, heldOut)
	clearHeldOutCasesV10(heldOut)
	if err != nil {
		return VerificationV10{}, fmt.Errorf("rederive V10 obligations: %w", err)
	}
	if !reflect.DeepEqual(discoveryObligations, publication.discoveryObl) || !reflect.DeepEqual(heldOutCommitment, publication.heldOutCommit) {
		return VerificationV10{}, fmt.Errorf("V10 obligation commitments do not reproduce")
	}
	return VerificationV10{ManifestSHA256: publication.manifestHash, DiscoveryCases: expectedDiscoveryV10,
		HeldOutCases: expectedHeldOutV10, DiscoveryObligations: len(discoveryObligations.Obligations),
		HeldOutObligations: heldOutCommitment.ObligationCount}, nil
}

func verifyPublicPublicationV10(root string) (publicPublicationV10, error) {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return publicPublicationV10{}, fmt.Errorf("resolve V10 publication: %w", err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return publicPublicationV10{}, fmt.Errorf("V10 publication root is not a real directory")
	}
	manifestBytes, err := readPublicationFileV10(filepath.Join(root, ManifestFileV10), 1<<20)
	if err != nil {
		return publicPublicationV10{}, fmt.Errorf("read V10 manifest: %w", err)
	}
	var manifest ManifestV10
	if err := decodeStrictPublicationV10(manifestBytes, &manifest); err != nil || !validManifestV10(manifest) {
		return publicPublicationV10{}, fmt.Errorf("V10 manifest is invalid")
	}
	manifestHash := hashBytes(manifestBytes)
	expectedFiles := map[string]bool{
		AuditFileV10: true, AuthorshipAttestationsFileV10: true, ChecksumFileV10: true,
		DiscoveryObligationsFileV10: true, HeldOutCommitmentFileV10: true, HeldOutCipherFileV10: true,
		ManifestFileV10: true, ValidationFileV10: true,
	}
	for _, entry := range manifest.Entries {
		expectedFiles[entry.StablePath] = true
	}
	if err := verifyExactPublicationFilesV10(root, expectedFiles); err != nil {
		return publicPublicationV10{}, err
	}
	if err := verifyCanonicalChecksumsV10(root, expectedFiles); err != nil {
		return publicPublicationV10{}, err
	}

	validationBytes, err := readPublicationFileV10(filepath.Join(root, ValidationFileV10), 4<<20)
	if err != nil || hashBytes(validationBytes) != manifest.ValidationReportSHA256 {
		return publicPublicationV10{}, fmt.Errorf("V10 public validation report commitment is invalid")
	}
	var validation PublicValidationReportV10
	if err := decodeStrictPublicationV10(validationBytes, &validation); err != nil || !validPublicValidationV10(validation, manifest) {
		return publicPublicationV10{}, fmt.Errorf("V10 public validation report is invalid")
	}
	authorshipBytes, err := readPublicationFileV10(filepath.Join(root, AuthorshipAttestationsFileV10), 1<<20)
	if err != nil || hashBytes(authorshipBytes) != manifest.AuthorshipAttestationsSHA256 {
		return publicPublicationV10{}, fmt.Errorf("V10 authorship attestation commitment is invalid")
	}
	var authorship AuthorshipAttestationsV10
	if err := decodeStrictPublicationV10(authorshipBytes, &authorship); err != nil || !validPublicAuthorshipV10(authorship, manifest) {
		return publicPublicationV10{}, fmt.Errorf("V10 authorship attestations are invalid")
	}

	discovery := make(map[string][]byte, expectedDiscoveryV10)
	for index, entry := range manifest.Entries {
		wantID := fmt.Sprintf("v10_case_%03d", index+1)
		wantAuthor := fmt.Sprintf("author_%d", (index/4)+1)
		if entry.ID != wantID || entry.AuthorSlot != wantAuthor || !validEntryV10(entry, false) {
			return publicPublicationV10{}, fmt.Errorf("V10 discovery manifest entry is invalid")
		}
		source, err := readPublicationFileV10(filepath.Join(root, filepath.FromSlash(entry.StablePath)), 1<<20)
		if err != nil || hashBytes(source) != entry.RequirementSHA256 {
			return publicPublicationV10{}, fmt.Errorf("V10 discovery source commitment is invalid")
		}
		discovery[entry.StablePath] = source
		if !equalPublicEvidenceV10(validation.DiscoveryEntries[index], entry) {
			return publicPublicationV10{}, fmt.Errorf("V10 discovery validation evidence differs from manifest")
		}
	}

	var discoveryObligations DiscoveryObligationsV10
	discoveryObligationBytes, err := readPublicationFileV10(filepath.Join(root, DiscoveryObligationsFileV10), 8<<20)
	if err != nil || decodeStrictPublicationV10(discoveryObligationBytes, &discoveryObligations) != nil ||
		discoveryObligations.Schema != "kicadai.closed-loop-open-set-discovery-obligations.v10" || discoveryObligations.Version != 10 ||
		discoveryObligations.CorpusManifestSHA256 != manifestHash || len(discoveryObligations.Obligations) == 0 {
		return publicPublicationV10{}, fmt.Errorf("V10 discovery obligations are invalid")
	}
	seenAnchors := map[string]bool{}
	rederived := []ObligationV10{}
	for _, entry := range manifest.Entries {
		current, err := obligationsForEntryV10(manifestHash, entry, discovery[entry.StablePath], seenAnchors)
		if err != nil {
			return publicPublicationV10{}, err
		}
		rederived = append(rederived, current...)
	}
	sort.Slice(rederived, func(i, j int) bool { return rederived[i].Anchor < rederived[j].Anchor })
	if !reflect.DeepEqual(rederived, discoveryObligations.Obligations) {
		return publicPublicationV10{}, fmt.Errorf("V10 discovery obligations do not reproduce")
	}

	var heldOutCommitment HeldOutObligationCommitmentV10
	heldOutCommitmentBytes, err := readPublicationFileV10(filepath.Join(root, HeldOutCommitmentFileV10), 1<<20)
	if err != nil || decodeStrictPublicationV10(heldOutCommitmentBytes, &heldOutCommitment) != nil ||
		heldOutCommitment.Schema != "kicadai.closed-loop-open-set-held-out-obligation-commitment.v10" || heldOutCommitment.Version != 10 ||
		heldOutCommitment.CorpusManifestSHA256 != manifestHash || heldOutCommitment.ObligationCount <= 0 || !validSHA256(heldOutCommitment.AggregateSHA256) {
		return publicPublicationV10{}, fmt.Errorf("V10 held-out obligation commitment is invalid")
	}
	ciphertext, err := readPublicationFileV10(filepath.Join(root, HeldOutCipherFileV10), 32<<20)
	if err != nil || hashBytes(ciphertext) != manifest.HeldOutSource.CiphertextSHA256 {
		return publicPublicationV10{}, fmt.Errorf("V10 held-out ciphertext commitment is invalid")
	}
	return publicPublicationV10{manifest: manifest, manifestHash: manifestHash, validation: validation, authorship: authorship,
		discovery: discovery, discoveryObl: discoveryObligations, heldOutCommit: heldOutCommitment, ciphertext: ciphertext}, nil
}

func entryEvidenceV10(entry EntryV10) corpusfreezev10.EntryEvidence {
	return corpusfreezev10.EntryEvidence{ID: entry.ID, AuthorSlot: entry.AuthorSlot, Role: entry.Role, Domain: entry.Domain,
		CircuitRole: entry.CircuitRole, SafetyImpact: entry.SafetyImpact, PrimaryClass: entry.PrimaryClass,
		RequiredPrimaryAnalysis: entry.RequiredPrimaryAnalysis, OutputMultiplicity: entry.OutputMultiplicity,
		RequireOffNominal: entry.RequireOffNominal, SourceID: entry.SourceID, RequirementFile: entry.StablePath,
		RequirementSHA256: entry.RequirementSHA256, NeutralSemanticSHA256: entry.NeutralSemanticSHA256,
		NormalizedSemanticSHA256: entry.NormalizedSemanticSHA256}
}

func validManifestV10(manifest ManifestV10) bool {
	if manifest.Schema != ManifestSchemaV10 || manifest.Version != ManifestVersionV10 || manifest.DiscoveryCaseCount != expectedDiscoveryV10 ||
		manifest.HeldOutCaseCount != expectedHeldOutV10 || len(manifest.Entries) != expectedDiscoveryV10 ||
		!validSHA256(manifest.ContractManifestSHA256) || !validSHA256(manifest.ValidatorManifestSHA256) ||
		!validSHA256(manifest.PublisherManifestSHA256) || !validSHA256(manifest.ValidationReportSHA256) ||
		!validSHA256(manifest.AuthorshipAttestationsSHA256) || !validSHA256(manifest.PolicySHA256) ||
		!validSHA256(manifest.PacketSetSHA256) || !validSHA256(manifest.ContractBindingSHA256) ||
		!validSHA256(manifest.HistoricalCommitmentsSHA256) || len(manifest.AuthorPacketSHA256) != expectedAuthorsV10 ||
		len(manifest.AssignmentSHA256) != expectedAuthorsV10 || len(manifest.AuthorshipSHA256) != expectedAuthorsV10 {
		return false
	}
	if len(manifest.Counts) != 2 || countValuesV10(manifest.Counts["discovery"]) != expectedDiscoveryV10 ||
		countValuesV10(manifest.Counts["held_out"]) != expectedHeldOutV10 {
		return false
	}
	for _, commit := range []string{manifest.Commits.StartingCommit, manifest.Commits.ContractFreezeCommit, manifest.Commits.AuthoringPacketCommit, manifest.Commits.ValidatorCommit, manifest.Commits.FreezeParentCommit} {
		if !commitPattern.MatchString(commit) {
			return false
		}
	}
	for index := 1; index <= expectedAuthorsV10; index++ {
		author := fmt.Sprintf("author_%d", index)
		if !validSHA256(manifest.AuthorPacketSHA256[author]) || !validSHA256(manifest.AssignmentSHA256[author]) || !validSHA256(manifest.AuthorshipSHA256[author]) {
			return false
		}
	}
	seal := manifest.HeldOutSource
	if seal.Algorithm != SealAlgorithmV10 || seal.File != HeldOutCipherFileV10 || seal.RecordCount != expectedHeldOutV10 ||
		seal.NonceBytes <= 0 || len(seal.RecordCiphertextSHA256) != expectedHeldOutV10 || !validSHA256(seal.CiphertextSHA256) ||
		!validSHA256(seal.PlaintextAggregateSHA256) || !validSHA256(seal.AADAggregateSHA256) || !validSHA256(seal.MetadataAggregateSHA256) {
		return false
	}
	for _, digest := range seal.RecordCiphertextSHA256 {
		if !validSHA256(digest) {
			return false
		}
	}
	return true
}

func countValuesV10(values map[string]int) int {
	total := 0
	for key, value := range values {
		if key == "" || value <= 0 {
			return -1
		}
		total += value
	}
	return total
}

func validEntryV10(entry EntryV10, sealed bool) bool {
	return entry.ID != "" && entry.AuthorSlot != "" && entry.Role == map[bool]string{false: "discovery", true: "held_out"}[sealed] &&
		entry.Domain != "" && entry.CircuitRole != "" && entry.SafetyImpact != "" && entry.PrimaryClass != "" &&
		entry.RequiredPrimaryAnalysis != "" && entry.OutputMultiplicity != "" && entry.SourceID != "" &&
		entry.StablePath == filepath.ToSlash(filepath.Join(entry.Role, entry.ID+".json")) && validSHA256(entry.RequirementSHA256) &&
		validSHA256(entry.NeutralSemanticSHA256) && validSHA256(entry.NormalizedSemanticSHA256) && entry.Sealed == sealed
}

func validPublicValidationV10(report PublicValidationReportV10, manifest ManifestV10) bool {
	return report.Schema == "kicadai.behavior-corpus-public-validation-report.v10" && report.Version == 10 &&
		report.PolicySHA256 == manifest.PolicySHA256 && report.PacketSetSHA256 == manifest.PacketSetSHA256 &&
		report.ContractBindingSHA256 == manifest.ContractBindingSHA256 && report.HistoricalCommitmentsSHA256 == manifest.HistoricalCommitmentsSHA256 &&
		reflect.DeepEqual(report.AuthorPacketSHA256, manifest.AuthorPacketSHA256) && reflect.DeepEqual(report.AssignmentSHA256, manifest.AssignmentSHA256) &&
		reflect.DeepEqual(report.AuthorshipSHA256, manifest.AuthorshipSHA256) && reflect.DeepEqual(report.Counts, manifest.Counts) &&
		len(report.DiscoveryEntries) == expectedDiscoveryV10 && report.HeldOutEntryCount == expectedHeldOutV10 && validSHA256(report.HeldOutEntryAggregateSHA256)
}

func validPublicAuthorshipV10(authorship AuthorshipAttestationsV10, manifest ManifestV10) bool {
	if authorship.Schema != "kicadai.closed-loop-open-set-authorship-attestations.v10" || authorship.Version != 10 || len(authorship.Records) != expectedAuthorsV10 {
		return false
	}
	for index, record := range authorship.Records {
		author := fmt.Sprintf("author_%d", index+1)
		if record.AuthorSlot != author || record.AuthorshipSHA256 != manifest.AuthorshipSHA256[author] ||
			record.PerAuthorPacketSHA256 != manifest.AuthorPacketSHA256[author] || record.AssignmentSHA256 != manifest.AssignmentSHA256[author] ||
			record.ContractBindingSHA256 != manifest.ContractBindingSHA256 || record.PerAuthorPacketManifest == "" ||
			record.AuthoringStartedUTC == "" || record.AuthoringEndedUTC == "" || record.UncertaintyCount < 0 || !allAuthorshipAttestationsV10(record.Attestations) {
			return false
		}
	}
	return true
}

func equalPublicEvidenceV10(evidence corpusfreezev10.EntryEvidence, entry EntryV10) bool {
	return evidence.ID == entry.ID && evidence.AuthorSlot == entry.AuthorSlot && evidence.Role == entry.Role && evidence.Domain == entry.Domain &&
		evidence.CircuitRole == entry.CircuitRole && evidence.SafetyImpact == entry.SafetyImpact && evidence.PrimaryClass == entry.PrimaryClass &&
		evidence.RequiredPrimaryAnalysis == entry.RequiredPrimaryAnalysis && evidence.OutputMultiplicity == entry.OutputMultiplicity &&
		evidence.RequireOffNominal == entry.RequireOffNominal && evidence.SourceID == entry.SourceID && evidence.RequirementFile == entry.StablePath &&
		evidence.RequirementSHA256 == entry.RequirementSHA256 && evidence.NeutralSemanticSHA256 == entry.NeutralSemanticSHA256 &&
		evidence.NormalizedSemanticSHA256 == entry.NormalizedSemanticSHA256
}

func verifyExactPublicationFilesV10(root string, expected map[string]bool) error {
	actual := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(root, path)
			if err != nil || filepath.ToSlash(relative) != "discovery" {
				return fmt.Errorf("V10 publication contains an unexpected directory")
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("V10 publication contains a symbolic or special file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		actual[filepath.ToSlash(relative)] = true
		return nil
	})
	if err != nil || !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("V10 publication file set is invalid")
	}
	return nil
}

func verifyCanonicalChecksumsV10(root string, expected map[string]bool) error {
	checksumBytes, err := readPublicationFileV10(filepath.Join(root, ChecksumFileV10), 1<<20)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(expected)-1)
	for path := range expected {
		if path != ChecksumFileV10 {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	var canonical strings.Builder
	for _, path := range paths {
		digest, err := hashRegularFile(filepath.Join(root, filepath.FromSlash(path)), 32<<20)
		if err != nil {
			return err
		}
		fmt.Fprintf(&canonical, "%s  %s\n", digest, path)
	}
	if canonical.String() != string(checksumBytes) {
		return fmt.Errorf("V10 checksum manifest is not canonical or complete")
	}
	return nil
}

func readPublicationFileV10(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("V10 artifact is not a bounded regular file")
	}
	return os.ReadFile(path)
}

func decodeStrictPublicationV10(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON content")
	}
	return nil
}

func clearHeldOutCasesV10(cases []heldOutCaseV10) {
	for index := range cases {
		clear(cases[index].Source)
	}
}
