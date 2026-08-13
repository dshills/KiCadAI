package corpuspublication

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev10"
	ots "kicadai/internal/opentopologysynthesis"
)

const heldOutMagicV10 = "KICADAI-V10-HELDOUT-RECORDS\x00"

type preparedCorpusV10 struct {
	manifest        ManifestV10
	reportBytes     []byte
	authorshipBytes []byte
	discovery       map[string][]byte
	heldOut         []heldOutCaseV10
	destination     string
	repositoryRoot  string
	keyPath         string
	random          io.Reader
}

func PublishV10(request RequestV10) (ResultV10, error) {
	prepared, err := prepareV10(request)
	if err != nil {
		return ResultV10{}, err
	}
	return prepared.publish()
}

func prepareV10(request RequestV10) (preparedCorpusV10, error) {
	repositoryRoot, destination, keyPath, err := validatePaths(request.RepositoryRoot, request.DestinationRoot, request.KeyPath)
	if err != nil {
		return preparedCorpusV10{}, err
	}
	if request.Random == nil {
		request.Random = rand.Reader
	}
	if !validSHA256(request.ContractManifestSHA256) || len(request.ValidatorManifest) == 0 || len(request.PublisherManifest) == 0 {
		return preparedCorpusV10{}, fmt.Errorf("V10 contract, validator, or publisher commitment is invalid")
	}
	for name, commit := range map[string]string{
		"starting": request.Commits.StartingCommit, "contract freeze": request.Commits.ContractFreezeCommit,
		"authoring packet": request.Commits.AuthoringPacketCommit, "validator": request.Commits.ValidatorCommit,
		"freeze parent": request.Commits.FreezeParentCommit,
	} {
		if !commitPattern.MatchString(commit) {
			return preparedCorpusV10{}, fmt.Errorf("V10 %s commit is invalid", name)
		}
	}
	_, err = request.Report.MarshalJSONStable()
	if err != nil {
		return preparedCorpusV10{}, err
	}
	if request.Report.Schema != "kicadai.behavior-corpus-validation-report.v10" || request.Report.Version != 10 ||
		len(request.Report.Entries) != expectedCasesV10 || len(request.Bundles) != expectedAuthorsV10 ||
		len(request.Report.AuthorPacketSHA256) != expectedAuthorsV10 || len(request.Report.AssignmentSHA256) != expectedAuthorsV10 ||
		len(request.Report.AuthorshipSHA256) != expectedAuthorsV10 {
		return preparedCorpusV10{}, fmt.Errorf("V10 validated corpus size is invalid")
	}

	entries := toV10Entries(request.Report.Entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	manifestEntries := make([]EntryV10, 0, len(entries))
	discoveryStablePaths := make(map[string]string, expectedDiscoveryV10)
	discovery := map[string][]byte{}
	heldOut := make([]heldOutCaseV10, 0, expectedHeldOutV10)
	for index := 1; index <= expectedAuthorsV10; index++ {
		author := fmt.Sprintf("author_%d", index)
		bundle, exists := request.Bundles[author]
		if !exists || !validSHA256(request.Report.AuthorPacketSHA256[author]) || !validSHA256(request.Report.AssignmentSHA256[author]) {
			return preparedCorpusV10{}, fmt.Errorf("V10 %s bundle binding is invalid", author)
		}
		wantHash := request.Report.AuthorshipSHA256[author]
		if !validSHA256(wantHash) || hashBytes(bundle.AuthorshipJSON) != wantHash {
			return preparedCorpusV10{}, fmt.Errorf("V10 %s authorship mismatch", author)
		}
	}
	if len(request.Bundles) != len(request.Report.AuthorshipSHA256) {
		return preparedCorpusV10{}, fmt.Errorf("V10 authorship set is incomplete")
	}
	authorshipBytes, err := publicAuthorshipAttestationsV10(request.Report, request.Bundles)
	if err != nil {
		return preparedCorpusV10{}, err
	}
	seenIDs := make(map[string]bool, len(entries))
	seenPaths := make(map[string]bool, len(entries))
	roleCounts := make(map[string]int, 2)
	authorRoleCounts := map[string]map[string]int{}
	for index, evidence := range entries {
		wantRole := "discovery"
		partitionIndex := index
		if index >= expectedDiscoveryV10 {
			wantRole = "held_out"
			partitionIndex -= expectedDiscoveryV10
		}
		wantID := fmt.Sprintf("v10_case_%03d", index+1)
		wantSourceID := fmt.Sprintf("v10_source_%03d", index+1)
		wantAuthor := fmt.Sprintf("author_%d", (partitionIndex/4)+1)
		if evidence.ID != wantID || evidence.SourceID != wantSourceID || evidence.Role != wantRole || evidence.AuthorSlot != wantAuthor ||
			evidence.Domain == "" || evidence.CircuitRole == "" || evidence.SafetyImpact == "" || evidence.PrimaryClass == "" ||
			evidence.RequiredPrimaryAnalysis == "" || evidence.OutputMultiplicity == "" {
			return preparedCorpusV10{}, fmt.Errorf("V10 assignment projection is invalid")
		}
		if seenIDs[evidence.ID] {
			return preparedCorpusV10{}, fmt.Errorf("V10 duplicate case identity")
		}
		seenIDs[evidence.ID] = true
		bundle, exists := request.Bundles[evidence.AuthorSlot]
		if !exists {
			return preparedCorpusV10{}, fmt.Errorf("V10 unknown author")
		}
		source, exists := bundle.Requirements[evidence.RequirementFile]
		if !exists || hashBytes(source) != evidence.RequirementSHA256 {
			return preparedCorpusV10{}, fmt.Errorf("V10 %s source mismatch", evidence.ID)
		}
		if evidence.Role != "discovery" && evidence.Role != "held_out" {
			return preparedCorpusV10{}, fmt.Errorf("V10 invalid role")
		}
		stablePath := filepath.ToSlash(filepath.Join(evidence.Role, evidence.ID+".json"))
		if seenPaths[stablePath] {
			return preparedCorpusV10{}, fmt.Errorf("V10 duplicate stable path")
		}
		seenPaths[stablePath] = true
		entry := EntryV10{ID: evidence.ID, AuthorSlot: evidence.AuthorSlot, Role: evidence.Role, Domain: evidence.Domain,
			CircuitRole: evidence.CircuitRole, SafetyImpact: evidence.SafetyImpact, PrimaryClass: evidence.PrimaryClass,
			RequiredPrimaryAnalysis: evidence.RequiredPrimaryAnalysis, OutputMultiplicity: evidence.OutputMultiplicity,
			RequireOffNominal: evidence.RequireOffNominal, SourceID: evidence.SourceID, StablePath: stablePath,
			RequirementSHA256: evidence.RequirementSHA256, NeutralSemanticSHA256: evidence.NeutralSemanticSHA256,
			NormalizedSemanticSHA256: evidence.NormalizedSemanticSHA256, Sealed: evidence.Role == "held_out"}
		roleCounts[evidence.Role]++
		if authorRoleCounts[evidence.AuthorSlot] == nil {
			authorRoleCounts[evidence.AuthorSlot] = map[string]int{}
		}
		authorRoleCounts[evidence.AuthorSlot][evidence.Role]++
		if evidence.Role == "discovery" {
			manifestEntries = append(manifestEntries, entry)
			discoveryStablePaths[evidence.ID] = stablePath
			discovery[stablePath] = append([]byte(nil), source...)
		} else {
			heldOut = append(heldOut, heldOutCaseV10{Entry: entry, Source: append([]byte(nil), source...)})
		}
	}
	if roleCounts["discovery"] != expectedDiscoveryV10 || roleCounts["held_out"] != expectedHeldOutV10 {
		return preparedCorpusV10{}, fmt.Errorf("V10 role counts are invalid")
	}
	for index := 1; index <= expectedAuthorsV10; index++ {
		counts := authorRoleCounts[fmt.Sprintf("author_%d", index)]
		if counts["discovery"] != 4 || counts["held_out"] != 4 {
			return preparedCorpusV10{}, fmt.Errorf("V10 author partition counts are invalid")
		}
	}
	publicReport, err := publicValidationReportV10(request.Report, discoveryStablePaths)
	if err != nil {
		return preparedCorpusV10{}, err
	}
	reportBytes, err := marshalStable(publicReport)
	if err != nil {
		return preparedCorpusV10{}, err
	}
	manifest := ManifestV10{Schema: ManifestSchemaV10, Version: ManifestVersionV10, Commits: request.Commits,
		ContractManifestSHA256: request.ContractManifestSHA256, ValidatorManifestSHA256: hashBytes(request.ValidatorManifest),
		PublisherManifestSHA256: hashBytes(request.PublisherManifest), ValidationReportSHA256: hashBytes(reportBytes),
		PolicySHA256: request.Report.PolicySHA256, PacketSetSHA256: request.Report.PacketSetSHA256,
		ContractBindingSHA256: request.Report.ContractBindingSHA256, HistoricalCommitmentsSHA256: request.Report.HistoricalCommitmentsSHA256,
		AuthorPacketSHA256: cloneMap(request.Report.AuthorPacketSHA256), AssignmentSHA256: cloneMap(request.Report.AssignmentSHA256),
		AuthorshipSHA256: cloneMap(request.Report.AuthorshipSHA256), AuthorshipAttestationsSHA256: hashBytes(authorshipBytes), Counts: cloneCounts(request.Report.Counts),
		DiscoveryCaseCount: expectedDiscoveryV10, HeldOutCaseCount: expectedHeldOutV10, Entries: manifestEntries}
	return preparedCorpusV10{manifest: manifest, reportBytes: reportBytes, authorshipBytes: authorshipBytes, discovery: discovery,
		heldOut: heldOut, destination: destination, repositoryRoot: repositoryRoot, keyPath: keyPath, random: request.Random}, nil
}

// corpusfreezev10.EntryEvidence is projected into a private alias so the
// publisher remains insulated from later report-type additions.
type corpusfreezev10Entry struct {
	ID, AuthorSlot, Role, Domain, CircuitRole, SafetyImpact, SourceID, RequirementFile string
	PrimaryClass, RequiredPrimaryAnalysis, OutputMultiplicity                          string
	RequireOffNominal                                                                  bool
	RequirementSHA256, NeutralSemanticSHA256, NormalizedSemanticSHA256                 string
}

func toV10Entries(entries []corpusfreezev10.EntryEvidence) []corpusfreezev10Entry {
	result := make([]corpusfreezev10Entry, len(entries))
	for index, entry := range entries {
		result[index] = corpusfreezev10Entry{ID: entry.ID, AuthorSlot: entry.AuthorSlot, Role: entry.Role, Domain: entry.Domain,
			CircuitRole: entry.CircuitRole, SafetyImpact: entry.SafetyImpact, SourceID: entry.SourceID, RequirementFile: entry.RequirementFile,
			PrimaryClass: entry.PrimaryClass, RequiredPrimaryAnalysis: entry.RequiredPrimaryAnalysis, OutputMultiplicity: entry.OutputMultiplicity,
			RequireOffNominal: entry.RequireOffNominal, RequirementSHA256: entry.RequirementSHA256,
			NeutralSemanticSHA256: entry.NeutralSemanticSHA256, NormalizedSemanticSHA256: entry.NormalizedSemanticSHA256}
	}
	return result
}

func publicAuthorshipAttestationsV10(report corpusfreezev10.Report, bundles map[string]corpusfreeze.Bundle) ([]byte, error) {
	authors := sortedKeys(bundles)
	if len(authors) != expectedAuthorsV10 {
		return nil, fmt.Errorf("V10 public authorship count is invalid")
	}
	records := make([]PublicAuthorshipV10, 0, len(authors))
	for _, author := range authors {
		bundle := bundles[author]
		var authorship corpusfreezev10.Authorship
		decoder := json.NewDecoder(bytes.NewReader(bundle.AuthorshipJSON))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&authorship); err != nil {
			return nil, fmt.Errorf("decode V10 %s authorship: %w", author, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("decode V10 %s authorship: trailing content", author)
		}
		if authorship.Schema != "kicadai.closed-loop-open-set-authorship.v10" || authorship.Version != 10 || authorship.AuthorSlot != author ||
			authorship.PerAuthorPacketSHA256 != report.AuthorPacketSHA256[author] || authorship.AssignmentSHA256 != report.AssignmentSHA256[author] ||
			authorship.ContractBindingSHA256 != report.ContractBindingSHA256 || !allAuthorshipAttestationsV10(authorship.Attestations) {
			return nil, fmt.Errorf("V10 %s public authorship binding is invalid", author)
		}
		records = append(records, PublicAuthorshipV10{
			AuthorSlot: author, AuthorshipSHA256: report.AuthorshipSHA256[author],
			PerAuthorPacketManifest: authorship.PerAuthorPacketManifest, PerAuthorPacketSHA256: authorship.PerAuthorPacketSHA256,
			ContractBindingSHA256: authorship.ContractBindingSHA256, AssignmentSHA256: authorship.AssignmentSHA256,
			AuthoringStartedUTC: authorship.AuthoringStartedUTC, AuthoringEndedUTC: authorship.AuthoringEndedUTC,
			UncertaintyCount: len(authorship.Uncertainties), Attestations: authorship.Attestations,
		})
	}
	return marshalStable(AuthorshipAttestationsV10{Schema: "kicadai.closed-loop-open-set-authorship-attestations.v10", Version: 10, Records: records})
}

func allAuthorshipAttestationsV10(value corpusfreezev10.AuthorshipAttestations) bool {
	return value.PacketOnlyInput && value.ContractBoundBeforeAuthoring && value.NoRepositoryOrPriorCorpusAccess &&
		value.NoCrossAuthorAssignmentOrContentAccess && value.IndependentlyConceivedBehaviorOnlyRequirements &&
		value.NoSynthesisSimulationClassificationRankingOrFeasibility && value.FixedDiscoveryHeldOutMembership &&
		value.NoImplementationOrExpectedOutcomePrescription && value.NoObligationAnchorGapExposureOrCausalPathAuthorship &&
		value.NoPostEvaluationInspectionOrModification && value.AllUncertaintiesDisclosed
}

func (prepared preparedCorpusV10) publish() (result ResultV10, err error) {
	if _, statErr := os.Lstat(prepared.destination); statErr == nil {
		return ResultV10{}, fmt.Errorf("V10 destination already exists")
	} else if !os.IsNotExist(statErr) {
		return ResultV10{}, fmt.Errorf("inspect V10 destination: %w", statErr)
	}
	parent := filepath.Dir(prepared.destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ResultV10{}, err
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(prepared.destination)+".stage-")
	if err != nil {
		return ResultV10{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	key, err := createExclusiveKey(prepared.repositoryRoot, prepared.keyPath, prepared.random)
	if err != nil {
		return ResultV10{}, err
	}
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			_ = os.Remove(prepared.keyPath)
		}
	}()

	ciphertext, sealMetadata, err := sealHeldOutRecordsV10(key, prepared.manifest, prepared.heldOut, prepared.random)
	if err != nil {
		return ResultV10{}, err
	}
	prepared.manifest.HeldOutSource = sealMetadata
	opened, err := OpenHeldOutV10(key, prepared.manifest, ciphertext)
	equalOpened := err == nil && equalHeldOutV10(opened, prepared.heldOut)
	clearHeldOutCasesV10(opened)
	if !equalOpened {
		return ResultV10{}, fmt.Errorf("verify V10 record seals")
	}
	manifestBytes, err := marshalStable(prepared.manifest)
	if err != nil {
		return ResultV10{}, err
	}
	manifestHash := hashBytes(manifestBytes)
	discoveryObligations, heldOutCommitment, err := deriveObligationsV10(manifestHash, prepared.manifest, prepared.discovery, prepared.heldOut)
	if err != nil {
		return ResultV10{}, err
	}
	if err := writePublicationV10(stage, prepared, ciphertext, manifestBytes, discoveryObligations, heldOutCommitment); err != nil {
		return ResultV10{}, err
	}
	if err := writeChecksums(stage); err != nil {
		return ResultV10{}, err
	}
	if err := syncTree(stage); err != nil {
		return ResultV10{}, err
	}
	if err := renameNoReplace(stage, prepared.destination); err != nil {
		return ResultV10{}, fmt.Errorf("publish V10 corpus atomically: %w", err)
	}
	keyCommitted = true
	return ResultV10{Manifest: prepared.manifest, ManifestSHA256: manifestHash, DiscoveryCases: expectedDiscoveryV10,
		HeldOutCases: expectedHeldOutV10, DiscoveryObligations: len(discoveryObligations.Obligations), HeldOutObligations: heldOutCommitment.ObligationCount}, nil
}

func writePublicationV10(stage string, prepared preparedCorpusV10, ciphertext, manifestBytes []byte, discovery DiscoveryObligationsV10, heldOut HeldOutObligationCommitmentV10) error {
	artifacts := []struct {
		path string
		data []byte
	}{{ValidationFileV10, prepared.reportBytes}, {AuthorshipAttestationsFileV10, prepared.authorshipBytes}, {HeldOutCipherFileV10, ciphertext}, {ManifestFileV10, manifestBytes}}
	discoveryBytes, err := marshalStable(discovery)
	if err != nil {
		return err
	}
	heldOutBytes, err := marshalStable(heldOut)
	if err != nil {
		return err
	}
	artifacts = append(artifacts, struct {
		path string
		data []byte
	}{DiscoveryObligationsFileV10, discoveryBytes}, struct {
		path string
		data []byte
	}{HeldOutCommitmentFileV10, heldOutBytes})
	for _, artifact := range artifacts {
		if err := writeExclusive(filepath.Join(stage, artifact.path), artifact.data, 0o644); err != nil {
			return err
		}
	}
	for _, stablePath := range sortedKeys(prepared.discovery) {
		if err := writeExclusive(filepath.Join(stage, filepath.FromSlash(stablePath)), prepared.discovery[stablePath], 0o644); err != nil {
			return err
		}
	}
	audit := auditBytesV10(prepared.manifest, hashBytes(manifestBytes), discovery, heldOut)
	return writeExclusive(filepath.Join(stage, AuditFileV10), audit, 0o644)
}

func encodeHeldOutRecordV10(item heldOutCaseV10) ([]byte, error) {
	metadata, err := json.Marshal(item.Entry)
	if err != nil {
		return nil, err
	}
	if len(metadata) > maximumMetadata || len(item.Source) == 0 || len(item.Source) > ots.MaxRequirementBytes {
		return nil, fmt.Errorf("V10 held-out record exceeds bounds")
	}
	var output bytes.Buffer
	output.WriteString(heldOutMagicV10)
	if err := binary.Write(&output, binary.BigEndian, uint32(HeldOutVersionV10)); err != nil {
		return nil, err
	}
	if err := writeLengthDelimited(&output, metadata); err != nil {
		return nil, err
	}
	if err := writeLengthDelimited(&output, item.Source); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func decodeHeldOutRecordV10(data []byte) (heldOutCaseV10, error) {
	reader := bytes.NewReader(data)
	magic := make([]byte, len(heldOutMagicV10))
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != heldOutMagicV10 {
		return heldOutCaseV10{}, fmt.Errorf("V10 held-out magic invalid")
	}
	var version uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != HeldOutVersionV10 {
		return heldOutCaseV10{}, fmt.Errorf("V10 held-out version invalid")
	}
	metadata, err := readLengthDelimited(reader, maximumMetadata)
	if err != nil {
		return heldOutCaseV10{}, err
	}
	var entry EntryV10
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		return heldOutCaseV10{}, err
	}
	source, err := readLengthDelimited(reader, ots.MaxRequirementBytes)
	if err != nil {
		return heldOutCaseV10{}, err
	}
	if reader.Len() != 0 || !entry.Sealed || hashBytes(source) != entry.RequirementSHA256 {
		return heldOutCaseV10{}, fmt.Errorf("V10 held-out record commitment invalid")
	}
	return heldOutCaseV10{Entry: entry, Source: source}, nil
}

func publicValidationReportV10(report corpusfreezev10.Report, discoveryStablePaths map[string]string) (PublicValidationReportV10, error) {
	discovery := []corpusfreezev10.EntryEvidence{}
	heldOutDigests := []string{}
	for _, entry := range report.Entries {
		if entry.Role == "discovery" {
			stablePath, exists := discoveryStablePaths[entry.ID]
			if !exists {
				return PublicValidationReportV10{}, fmt.Errorf("V10 public validation discovery path missing")
			}
			entry.RequirementFile = stablePath
			discovery = append(discovery, entry)
			continue
		}
		if entry.Role != "held_out" {
			return PublicValidationReportV10{}, fmt.Errorf("V10 public validation role invalid")
		}
		entry.RequirementFile = filepath.ToSlash(filepath.Join("held_out", entry.ID+".json"))
		data, err := json.Marshal(entry)
		if err != nil {
			return PublicValidationReportV10{}, err
		}
		heldOutDigests = append(heldOutDigests, hashBytes(data))
	}
	sort.Slice(discovery, func(i, j int) bool { return discovery[i].ID < discovery[j].ID })
	sort.Strings(heldOutDigests)
	if len(discovery) != expectedDiscoveryV10 || len(discoveryStablePaths) != expectedDiscoveryV10 || len(heldOutDigests) != expectedHeldOutV10 {
		return PublicValidationReportV10{}, fmt.Errorf("V10 public validation counts invalid")
	}
	return PublicValidationReportV10{Schema: "kicadai.behavior-corpus-public-validation-report.v10", Version: 10,
		PolicySHA256: report.PolicySHA256, PacketSetSHA256: report.PacketSetSHA256, ContractBindingSHA256: report.ContractBindingSHA256,
		HistoricalCommitmentsSHA256: report.HistoricalCommitmentsSHA256, AuthorPacketSHA256: cloneMap(report.AuthorPacketSHA256),
		AssignmentSHA256: cloneMap(report.AssignmentSHA256), AuthorshipSHA256: cloneMap(report.AuthorshipSHA256), Counts: cloneCounts(report.Counts),
		DiscoveryEntries: discovery, HeldOutEntryCount: len(heldOutDigests), HeldOutEntryAggregateSHA256: aggregateDigestsV10(heldOutDigests)}, nil
}
