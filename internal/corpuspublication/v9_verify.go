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

	"kicadai/internal/corpusfreezev9"
)

type VerificationV9 struct {
	ManifestSHA256       string
	DiscoveryCases       int
	HeldOutCases         int
	DiscoveryObligations int
	HeldOutObligations   int
}

type publicPublicationV9 struct {
	manifest      ManifestV9
	manifestHash  string
	validation    PublicValidationReportV9
	authorship    AuthorshipAttestationsV9
	discovery     map[string][]byte
	discoveryObl  DiscoveryObligationsV9
	heldOutCommit HeldOutObligationCommitmentV9
	ciphertext    []byte
}

// VerifyPublicationV9 independently authenticates the public V9 publication.
// It opens no key and exposes no held-out identity or requirement content.
func VerifyPublicationV9(root string) (VerificationV9, error) {
	publication, err := verifyPublicPublicationV9(root)
	if err != nil {
		return VerificationV9{}, err
	}
	return VerificationV9{ManifestSHA256: publication.manifestHash, DiscoveryCases: expectedDiscoveryV9,
		HeldOutCases: expectedHeldOutV9, DiscoveryObligations: len(publication.discoveryObl.Obligations),
		HeldOutObligations: publication.heldOutCommit.ObligationCount}, nil
}

// VerifyPublicationV9WithKey additionally authenticates every encrypted V9
// held-out record. It returns aggregate counts only and retains no plaintext.
func VerifyPublicationV9WithKey(root string, key []byte) (VerificationV9, error) {
	publication, err := verifyPublicPublicationV9(root)
	if err != nil {
		return VerificationV9{}, err
	}
	heldOut, err := OpenHeldOutV9(key, publication.manifest, publication.ciphertext)
	if err != nil {
		return VerificationV9{}, fmt.Errorf("authenticate V9 held-out records: %w", err)
	}
	seen := make(map[string]bool, expectedCasesV9)
	for _, entry := range publication.manifest.Entries {
		seen[entry.ID] = true
	}
	for index := range heldOut {
		item := &heldOut[index]
		wantID := fmt.Sprintf("v9_case_%03d", expectedDiscoveryV9+index+1)
		wantAuthor := fmt.Sprintf("author_%d", (index/4)+1)
		if item.Entry.ID != wantID || item.Entry.AuthorSlot != wantAuthor || seen[item.Entry.ID] || !validEntryV9(item.Entry, true) {
			clearHeldOutCasesV9(heldOut)
			return VerificationV9{}, fmt.Errorf("V9 held-out record order or metadata is invalid")
		}
		seen[item.Entry.ID] = true
	}
	heldOutDigests := make([]string, 0, len(heldOut))
	for _, item := range heldOut {
		data, err := json.Marshal(entryEvidenceV9(item.Entry))
		if err != nil {
			clearHeldOutCasesV9(heldOut)
			return VerificationV9{}, err
		}
		heldOutDigests = append(heldOutDigests, hashBytes(data))
	}
	sort.Strings(heldOutDigests)
	if aggregateDigestsV9(heldOutDigests) != publication.validation.HeldOutEntryAggregateSHA256 {
		clearHeldOutCasesV9(heldOut)
		return VerificationV9{}, fmt.Errorf("V9 held-out validation aggregate does not reproduce")
	}
	discoveryObligations, heldOutCommitment, err := deriveObligationsV9(publication.manifestHash, publication.manifest, publication.discovery, heldOut)
	clearHeldOutCasesV9(heldOut)
	if err != nil {
		return VerificationV9{}, fmt.Errorf("rederive V9 obligations: %w", err)
	}
	if !reflect.DeepEqual(discoveryObligations, publication.discoveryObl) || !reflect.DeepEqual(heldOutCommitment, publication.heldOutCommit) {
		return VerificationV9{}, fmt.Errorf("V9 obligation commitments do not reproduce")
	}
	return VerificationV9{ManifestSHA256: publication.manifestHash, DiscoveryCases: expectedDiscoveryV9,
		HeldOutCases: expectedHeldOutV9, DiscoveryObligations: len(discoveryObligations.Obligations),
		HeldOutObligations: heldOutCommitment.ObligationCount}, nil
}

func verifyPublicPublicationV9(root string) (publicPublicationV9, error) {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return publicPublicationV9{}, fmt.Errorf("resolve V9 publication: %w", err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return publicPublicationV9{}, fmt.Errorf("V9 publication root is not a real directory")
	}
	manifestBytes, err := readPublicationFileV9(filepath.Join(root, ManifestFileV9), 1<<20)
	if err != nil {
		return publicPublicationV9{}, fmt.Errorf("read V9 manifest: %w", err)
	}
	var manifest ManifestV9
	if err := decodeStrictPublicationV9(manifestBytes, &manifest); err != nil || !validManifestV9(manifest) {
		return publicPublicationV9{}, fmt.Errorf("V9 manifest is invalid")
	}
	manifestHash := hashBytes(manifestBytes)
	expectedFiles := map[string]bool{
		AuditFileV9: true, AuthorshipAttestationsFileV9: true, ChecksumFileV9: true,
		DiscoveryObligationsFileV9: true, HeldOutCommitmentFileV9: true, HeldOutCipherFileV9: true,
		ManifestFileV9: true, ValidationFileV9: true,
	}
	for _, entry := range manifest.Entries {
		expectedFiles[entry.StablePath] = true
	}
	if err := verifyExactPublicationFilesV9(root, expectedFiles); err != nil {
		return publicPublicationV9{}, err
	}
	if err := verifyCanonicalChecksumsV9(root, expectedFiles); err != nil {
		return publicPublicationV9{}, err
	}

	validationBytes, err := readPublicationFileV9(filepath.Join(root, ValidationFileV9), 4<<20)
	if err != nil || hashBytes(validationBytes) != manifest.ValidationReportSHA256 {
		return publicPublicationV9{}, fmt.Errorf("V9 public validation report commitment is invalid")
	}
	var validation PublicValidationReportV9
	if err := decodeStrictPublicationV9(validationBytes, &validation); err != nil || !validPublicValidationV9(validation, manifest) {
		return publicPublicationV9{}, fmt.Errorf("V9 public validation report is invalid")
	}
	authorshipBytes, err := readPublicationFileV9(filepath.Join(root, AuthorshipAttestationsFileV9), 1<<20)
	if err != nil || hashBytes(authorshipBytes) != manifest.AuthorshipAttestationsSHA256 {
		return publicPublicationV9{}, fmt.Errorf("V9 authorship attestation commitment is invalid")
	}
	var authorship AuthorshipAttestationsV9
	if err := decodeStrictPublicationV9(authorshipBytes, &authorship); err != nil || !validPublicAuthorshipV9(authorship, manifest) {
		return publicPublicationV9{}, fmt.Errorf("V9 authorship attestations are invalid")
	}

	discovery := make(map[string][]byte, expectedDiscoveryV9)
	for index, entry := range manifest.Entries {
		wantID := fmt.Sprintf("v9_case_%03d", index+1)
		wantAuthor := fmt.Sprintf("author_%d", (index/4)+1)
		if entry.ID != wantID || entry.AuthorSlot != wantAuthor || !validEntryV9(entry, false) {
			return publicPublicationV9{}, fmt.Errorf("V9 discovery manifest entry is invalid")
		}
		source, err := readPublicationFileV9(filepath.Join(root, filepath.FromSlash(entry.StablePath)), 1<<20)
		if err != nil || hashBytes(source) != entry.RequirementSHA256 {
			return publicPublicationV9{}, fmt.Errorf("V9 discovery source commitment is invalid")
		}
		discovery[entry.StablePath] = source
		if !equalPublicEvidenceV9(validation.DiscoveryEntries[index], entry) {
			return publicPublicationV9{}, fmt.Errorf("V9 discovery validation evidence differs from manifest")
		}
	}

	var discoveryObligations DiscoveryObligationsV9
	discoveryObligationBytes, err := readPublicationFileV9(filepath.Join(root, DiscoveryObligationsFileV9), 8<<20)
	if err != nil || decodeStrictPublicationV9(discoveryObligationBytes, &discoveryObligations) != nil ||
		discoveryObligations.Schema != "kicadai.closed-loop-open-set-discovery-obligations.v9" || discoveryObligations.Version != 9 ||
		discoveryObligations.CorpusManifestSHA256 != manifestHash || len(discoveryObligations.Obligations) == 0 {
		return publicPublicationV9{}, fmt.Errorf("V9 discovery obligations are invalid")
	}
	seenAnchors := map[string]bool{}
	rederived := []ObligationV9{}
	for _, entry := range manifest.Entries {
		current, err := obligationsForEntryV9(manifestHash, entry, discovery[entry.StablePath], seenAnchors)
		if err != nil {
			return publicPublicationV9{}, err
		}
		rederived = append(rederived, current...)
	}
	sort.Slice(rederived, func(i, j int) bool { return rederived[i].Anchor < rederived[j].Anchor })
	if !reflect.DeepEqual(rederived, discoveryObligations.Obligations) {
		return publicPublicationV9{}, fmt.Errorf("V9 discovery obligations do not reproduce")
	}

	var heldOutCommitment HeldOutObligationCommitmentV9
	heldOutCommitmentBytes, err := readPublicationFileV9(filepath.Join(root, HeldOutCommitmentFileV9), 1<<20)
	if err != nil || decodeStrictPublicationV9(heldOutCommitmentBytes, &heldOutCommitment) != nil ||
		heldOutCommitment.Schema != "kicadai.closed-loop-open-set-held-out-obligation-commitment.v9" || heldOutCommitment.Version != 9 ||
		heldOutCommitment.CorpusManifestSHA256 != manifestHash || heldOutCommitment.ObligationCount <= 0 || !validSHA256(heldOutCommitment.AggregateSHA256) {
		return publicPublicationV9{}, fmt.Errorf("V9 held-out obligation commitment is invalid")
	}
	ciphertext, err := readPublicationFileV9(filepath.Join(root, HeldOutCipherFileV9), 32<<20)
	if err != nil || hashBytes(ciphertext) != manifest.HeldOutSource.CiphertextSHA256 {
		return publicPublicationV9{}, fmt.Errorf("V9 held-out ciphertext commitment is invalid")
	}
	return publicPublicationV9{manifest: manifest, manifestHash: manifestHash, validation: validation, authorship: authorship,
		discovery: discovery, discoveryObl: discoveryObligations, heldOutCommit: heldOutCommitment, ciphertext: ciphertext}, nil
}

func entryEvidenceV9(entry EntryV9) corpusfreezev9.EntryEvidence {
	return corpusfreezev9.EntryEvidence{ID: entry.ID, AuthorSlot: entry.AuthorSlot, Role: entry.Role, Domain: entry.Domain,
		CircuitRole: entry.CircuitRole, SafetyImpact: entry.SafetyImpact, PrimaryClass: entry.PrimaryClass,
		RequiredPrimaryAnalysis: entry.RequiredPrimaryAnalysis, OutputMultiplicity: entry.OutputMultiplicity,
		RequireOffNominal: entry.RequireOffNominal, SourceID: entry.SourceID, RequirementFile: entry.StablePath,
		RequirementSHA256: entry.RequirementSHA256, NeutralSemanticSHA256: entry.NeutralSemanticSHA256,
		NormalizedSemanticSHA256: entry.NormalizedSemanticSHA256}
}

func validManifestV9(manifest ManifestV9) bool {
	if manifest.Schema != ManifestSchemaV9 || manifest.Version != ManifestVersionV9 || manifest.DiscoveryCaseCount != expectedDiscoveryV9 ||
		manifest.HeldOutCaseCount != expectedHeldOutV9 || len(manifest.Entries) != expectedDiscoveryV9 ||
		!validSHA256(manifest.ContractManifestSHA256) || !validSHA256(manifest.ValidatorManifestSHA256) ||
		!validSHA256(manifest.PublisherManifestSHA256) || !validSHA256(manifest.ValidationReportSHA256) ||
		!validSHA256(manifest.AuthorshipAttestationsSHA256) || !validSHA256(manifest.PolicySHA256) ||
		!validSHA256(manifest.PacketSetSHA256) || !validSHA256(manifest.ContractBindingSHA256) ||
		!validSHA256(manifest.HistoricalCommitmentsSHA256) || len(manifest.AuthorPacketSHA256) != expectedAuthorsV9 ||
		len(manifest.AssignmentSHA256) != expectedAuthorsV9 || len(manifest.AuthorshipSHA256) != expectedAuthorsV9 {
		return false
	}
	if len(manifest.Counts) != 2 || countValuesV9(manifest.Counts["discovery"]) != expectedDiscoveryV9 ||
		countValuesV9(manifest.Counts["held_out"]) != expectedHeldOutV9 {
		return false
	}
	for _, commit := range []string{manifest.Commits.StartingCommit, manifest.Commits.ContractFreezeCommit, manifest.Commits.AuthoringPacketCommit, manifest.Commits.ValidatorCommit, manifest.Commits.FreezeParentCommit} {
		if !commitPattern.MatchString(commit) {
			return false
		}
	}
	for index := 1; index <= expectedAuthorsV9; index++ {
		author := fmt.Sprintf("author_%d", index)
		if !validSHA256(manifest.AuthorPacketSHA256[author]) || !validSHA256(manifest.AssignmentSHA256[author]) || !validSHA256(manifest.AuthorshipSHA256[author]) {
			return false
		}
	}
	seal := manifest.HeldOutSource
	if seal.Algorithm != SealAlgorithmV9 || seal.File != HeldOutCipherFileV9 || seal.RecordCount != expectedHeldOutV9 ||
		seal.NonceBytes <= 0 || len(seal.RecordCiphertextSHA256) != expectedHeldOutV9 || !validSHA256(seal.CiphertextSHA256) ||
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

func countValuesV9(values map[string]int) int {
	total := 0
	for key, value := range values {
		if key == "" || value <= 0 {
			return -1
		}
		total += value
	}
	return total
}

func validEntryV9(entry EntryV9, sealed bool) bool {
	return entry.ID != "" && entry.AuthorSlot != "" && entry.Role == map[bool]string{false: "discovery", true: "held_out"}[sealed] &&
		entry.Domain != "" && entry.CircuitRole != "" && entry.SafetyImpact != "" && entry.PrimaryClass != "" &&
		entry.RequiredPrimaryAnalysis != "" && entry.OutputMultiplicity != "" && entry.SourceID != "" &&
		entry.StablePath == filepath.ToSlash(filepath.Join(entry.Role, entry.ID+".json")) && validSHA256(entry.RequirementSHA256) &&
		validSHA256(entry.NeutralSemanticSHA256) && validSHA256(entry.NormalizedSemanticSHA256) && entry.Sealed == sealed
}

func validPublicValidationV9(report PublicValidationReportV9, manifest ManifestV9) bool {
	return report.Schema == "kicadai.behavior-corpus-public-validation-report.v9" && report.Version == 9 &&
		report.PolicySHA256 == manifest.PolicySHA256 && report.PacketSetSHA256 == manifest.PacketSetSHA256 &&
		report.ContractBindingSHA256 == manifest.ContractBindingSHA256 && report.HistoricalCommitmentsSHA256 == manifest.HistoricalCommitmentsSHA256 &&
		reflect.DeepEqual(report.AuthorPacketSHA256, manifest.AuthorPacketSHA256) && reflect.DeepEqual(report.AssignmentSHA256, manifest.AssignmentSHA256) &&
		reflect.DeepEqual(report.AuthorshipSHA256, manifest.AuthorshipSHA256) && reflect.DeepEqual(report.Counts, manifest.Counts) &&
		len(report.DiscoveryEntries) == expectedDiscoveryV9 && report.HeldOutEntryCount == expectedHeldOutV9 && validSHA256(report.HeldOutEntryAggregateSHA256)
}

func validPublicAuthorshipV9(authorship AuthorshipAttestationsV9, manifest ManifestV9) bool {
	if authorship.Schema != "kicadai.closed-loop-open-set-authorship-attestations.v9" || authorship.Version != 9 || len(authorship.Records) != expectedAuthorsV9 {
		return false
	}
	for index, record := range authorship.Records {
		author := fmt.Sprintf("author_%d", index+1)
		if record.AuthorSlot != author || record.AuthorshipSHA256 != manifest.AuthorshipSHA256[author] ||
			record.PerAuthorPacketSHA256 != manifest.AuthorPacketSHA256[author] || record.AssignmentSHA256 != manifest.AssignmentSHA256[author] ||
			record.ContractBindingSHA256 != manifest.ContractBindingSHA256 || record.PerAuthorPacketManifest == "" ||
			record.AuthoringStartedUTC == "" || record.AuthoringEndedUTC == "" || record.UncertaintyCount < 0 || !allAuthorshipAttestationsV9(record.Attestations) {
			return false
		}
	}
	return true
}

func equalPublicEvidenceV9(evidence corpusfreezev9.EntryEvidence, entry EntryV9) bool {
	return evidence.ID == entry.ID && evidence.AuthorSlot == entry.AuthorSlot && evidence.Role == entry.Role && evidence.Domain == entry.Domain &&
		evidence.CircuitRole == entry.CircuitRole && evidence.SafetyImpact == entry.SafetyImpact && evidence.PrimaryClass == entry.PrimaryClass &&
		evidence.RequiredPrimaryAnalysis == entry.RequiredPrimaryAnalysis && evidence.OutputMultiplicity == entry.OutputMultiplicity &&
		evidence.RequireOffNominal == entry.RequireOffNominal && evidence.SourceID == entry.SourceID && evidence.RequirementFile == entry.StablePath &&
		evidence.RequirementSHA256 == entry.RequirementSHA256 && evidence.NeutralSemanticSHA256 == entry.NeutralSemanticSHA256 &&
		evidence.NormalizedSemanticSHA256 == entry.NormalizedSemanticSHA256
}

func verifyExactPublicationFilesV9(root string, expected map[string]bool) error {
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
				return fmt.Errorf("V9 publication contains an unexpected directory")
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("V9 publication contains a symbolic or special file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		actual[filepath.ToSlash(relative)] = true
		return nil
	})
	if err != nil || !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("V9 publication file set is invalid")
	}
	return nil
}

func verifyCanonicalChecksumsV9(root string, expected map[string]bool) error {
	checksumBytes, err := readPublicationFileV9(filepath.Join(root, ChecksumFileV9), 1<<20)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(expected)-1)
	for path := range expected {
		if path != ChecksumFileV9 {
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
		return fmt.Errorf("V9 checksum manifest is not canonical or complete")
	}
	return nil
}

func readPublicationFileV9(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("V9 artifact is not a bounded regular file")
	}
	return os.ReadFile(path)
}

func decodeStrictPublicationV9(data []byte, destination any) error {
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

func clearHeldOutCasesV9(cases []heldOutCaseV9) {
	for index := range cases {
		clear(cases[index].Source)
	}
}
