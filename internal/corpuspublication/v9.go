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
	"kicadai/internal/corpusfreezev9"
	ots "kicadai/internal/opentopologysynthesis"
)

const heldOutMagicV9 = "KICADAI-V9-HELDOUT-RECORDS\x00"

type preparedCorpusV9 struct {
	manifest        ManifestV9
	reportBytes     []byte
	authorshipBytes []byte
	discovery       map[string][]byte
	heldOut         []heldOutCaseV9
	destination     string
	repositoryRoot  string
	keyPath         string
	random          io.Reader
}

func PublishV9(request RequestV9) (ResultV9, error) {
	prepared, err := prepareV9(request)
	if err != nil {
		return ResultV9{}, err
	}
	return prepared.publish()
}

func prepareV9(request RequestV9) (preparedCorpusV9, error) {
	repositoryRoot, destination, keyPath, err := validatePaths(request.RepositoryRoot, request.DestinationRoot, request.KeyPath)
	if err != nil {
		return preparedCorpusV9{}, err
	}
	if request.Random == nil {
		request.Random = rand.Reader
	}
	if !validSHA256(request.ContractManifestSHA256) || len(request.ValidatorManifest) == 0 || len(request.PublisherManifest) == 0 {
		return preparedCorpusV9{}, fmt.Errorf("V9 contract, validator, or publisher commitment is invalid")
	}
	for name, commit := range map[string]string{
		"starting": request.Commits.StartingCommit, "contract freeze": request.Commits.ContractFreezeCommit,
		"authoring packet": request.Commits.AuthoringPacketCommit, "validator": request.Commits.ValidatorCommit,
		"freeze parent": request.Commits.FreezeParentCommit,
	} {
		if !commitPattern.MatchString(commit) {
			return preparedCorpusV9{}, fmt.Errorf("V9 %s commit is invalid", name)
		}
	}
	_, err = request.Report.MarshalJSONStable()
	if err != nil {
		return preparedCorpusV9{}, err
	}
	if request.Report.Schema != "kicadai.behavior-corpus-validation-report.v9" || request.Report.Version != 9 ||
		len(request.Report.Entries) != expectedCasesV9 || len(request.Bundles) != expectedAuthorsV9 ||
		len(request.Report.AuthorPacketSHA256) != expectedAuthorsV9 || len(request.Report.AssignmentSHA256) != expectedAuthorsV9 ||
		len(request.Report.AuthorshipSHA256) != expectedAuthorsV9 {
		return preparedCorpusV9{}, fmt.Errorf("V9 validated corpus size is invalid")
	}

	entries := toV9Entries(request.Report.Entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	manifestEntries := make([]EntryV9, 0, len(entries))
	discoveryStablePaths := make(map[string]string, expectedDiscoveryV9)
	discovery := map[string][]byte{}
	heldOut := make([]heldOutCaseV9, 0, expectedHeldOutV9)
	for index := 1; index <= expectedAuthorsV9; index++ {
		author := fmt.Sprintf("author_%d", index)
		bundle, exists := request.Bundles[author]
		if !exists || !validSHA256(request.Report.AuthorPacketSHA256[author]) || !validSHA256(request.Report.AssignmentSHA256[author]) {
			return preparedCorpusV9{}, fmt.Errorf("V9 %s bundle binding is invalid", author)
		}
		wantHash := request.Report.AuthorshipSHA256[author]
		if !validSHA256(wantHash) || hashBytes(bundle.AuthorshipJSON) != wantHash {
			return preparedCorpusV9{}, fmt.Errorf("V9 %s authorship mismatch", author)
		}
	}
	if len(request.Bundles) != len(request.Report.AuthorshipSHA256) {
		return preparedCorpusV9{}, fmt.Errorf("V9 authorship set is incomplete")
	}
	authorshipBytes, err := publicAuthorshipAttestationsV9(request.Report, request.Bundles)
	if err != nil {
		return preparedCorpusV9{}, err
	}
	seenIDs := make(map[string]bool, len(entries))
	seenPaths := make(map[string]bool, len(entries))
	roleCounts := make(map[string]int, 2)
	authorRoleCounts := map[string]map[string]int{}
	for index, evidence := range entries {
		wantRole := "discovery"
		partitionIndex := index
		if index >= expectedDiscoveryV9 {
			wantRole = "held_out"
			partitionIndex -= expectedDiscoveryV9
		}
		wantID := fmt.Sprintf("v9_case_%03d", index+1)
		wantSourceID := fmt.Sprintf("v9_source_%03d", index+1)
		wantAuthor := fmt.Sprintf("author_%d", (partitionIndex/4)+1)
		if evidence.ID != wantID || evidence.SourceID != wantSourceID || evidence.Role != wantRole || evidence.AuthorSlot != wantAuthor ||
			evidence.Domain == "" || evidence.CircuitRole == "" || evidence.SafetyImpact == "" || evidence.PrimaryClass == "" ||
			evidence.RequiredPrimaryAnalysis == "" || evidence.OutputMultiplicity == "" {
			return preparedCorpusV9{}, fmt.Errorf("V9 assignment projection is invalid")
		}
		if seenIDs[evidence.ID] {
			return preparedCorpusV9{}, fmt.Errorf("V9 duplicate case identity")
		}
		seenIDs[evidence.ID] = true
		bundle, exists := request.Bundles[evidence.AuthorSlot]
		if !exists {
			return preparedCorpusV9{}, fmt.Errorf("V9 unknown author")
		}
		source, exists := bundle.Requirements[evidence.RequirementFile]
		if !exists || hashBytes(source) != evidence.RequirementSHA256 {
			return preparedCorpusV9{}, fmt.Errorf("V9 %s source mismatch", evidence.ID)
		}
		if evidence.Role != "discovery" && evidence.Role != "held_out" {
			return preparedCorpusV9{}, fmt.Errorf("V9 invalid role")
		}
		stablePath := filepath.ToSlash(filepath.Join(evidence.Role, evidence.ID+".json"))
		if seenPaths[stablePath] {
			return preparedCorpusV9{}, fmt.Errorf("V9 duplicate stable path")
		}
		seenPaths[stablePath] = true
		entry := EntryV9{ID: evidence.ID, AuthorSlot: evidence.AuthorSlot, Role: evidence.Role, Domain: evidence.Domain,
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
			heldOut = append(heldOut, heldOutCaseV9{Entry: entry, Source: append([]byte(nil), source...)})
		}
	}
	if roleCounts["discovery"] != expectedDiscoveryV9 || roleCounts["held_out"] != expectedHeldOutV9 {
		return preparedCorpusV9{}, fmt.Errorf("V9 role counts are invalid")
	}
	for index := 1; index <= expectedAuthorsV9; index++ {
		counts := authorRoleCounts[fmt.Sprintf("author_%d", index)]
		if counts["discovery"] != 4 || counts["held_out"] != 4 {
			return preparedCorpusV9{}, fmt.Errorf("V9 author partition counts are invalid")
		}
	}
	publicReport, err := publicValidationReportV9(request.Report, discoveryStablePaths)
	if err != nil {
		return preparedCorpusV9{}, err
	}
	reportBytes, err := marshalStable(publicReport)
	if err != nil {
		return preparedCorpusV9{}, err
	}
	manifest := ManifestV9{Schema: ManifestSchemaV9, Version: ManifestVersionV9, Commits: request.Commits,
		ContractManifestSHA256: request.ContractManifestSHA256, ValidatorManifestSHA256: hashBytes(request.ValidatorManifest),
		PublisherManifestSHA256: hashBytes(request.PublisherManifest), ValidationReportSHA256: hashBytes(reportBytes),
		PolicySHA256: request.Report.PolicySHA256, PacketSetSHA256: request.Report.PacketSetSHA256,
		ContractBindingSHA256: request.Report.ContractBindingSHA256, HistoricalCommitmentsSHA256: request.Report.HistoricalCommitmentsSHA256,
		AuthorPacketSHA256: cloneMap(request.Report.AuthorPacketSHA256), AssignmentSHA256: cloneMap(request.Report.AssignmentSHA256),
		AuthorshipSHA256: cloneMap(request.Report.AuthorshipSHA256), AuthorshipAttestationsSHA256: hashBytes(authorshipBytes), Counts: cloneCounts(request.Report.Counts),
		DiscoveryCaseCount: expectedDiscoveryV9, HeldOutCaseCount: expectedHeldOutV9, Entries: manifestEntries}
	return preparedCorpusV9{manifest: manifest, reportBytes: reportBytes, authorshipBytes: authorshipBytes, discovery: discovery,
		heldOut: heldOut, destination: destination, repositoryRoot: repositoryRoot, keyPath: keyPath, random: request.Random}, nil
}

// corpusfreezev9.EntryEvidence is projected into a private alias so the
// publisher remains insulated from later report-type additions.
type corpusfreezev9Entry struct {
	ID, AuthorSlot, Role, Domain, CircuitRole, SafetyImpact, SourceID, RequirementFile string
	PrimaryClass, RequiredPrimaryAnalysis, OutputMultiplicity                          string
	RequireOffNominal                                                                  bool
	RequirementSHA256, NeutralSemanticSHA256, NormalizedSemanticSHA256                 string
}

func toV9Entries(entries []corpusfreezev9.EntryEvidence) []corpusfreezev9Entry {
	result := make([]corpusfreezev9Entry, len(entries))
	for index, entry := range entries {
		result[index] = corpusfreezev9Entry{ID: entry.ID, AuthorSlot: entry.AuthorSlot, Role: entry.Role, Domain: entry.Domain,
			CircuitRole: entry.CircuitRole, SafetyImpact: entry.SafetyImpact, SourceID: entry.SourceID, RequirementFile: entry.RequirementFile,
			PrimaryClass: entry.PrimaryClass, RequiredPrimaryAnalysis: entry.RequiredPrimaryAnalysis, OutputMultiplicity: entry.OutputMultiplicity,
			RequireOffNominal: entry.RequireOffNominal, RequirementSHA256: entry.RequirementSHA256,
			NeutralSemanticSHA256: entry.NeutralSemanticSHA256, NormalizedSemanticSHA256: entry.NormalizedSemanticSHA256}
	}
	return result
}

func publicAuthorshipAttestationsV9(report corpusfreezev9.Report, bundles map[string]corpusfreeze.Bundle) ([]byte, error) {
	authors := sortedKeys(bundles)
	if len(authors) != expectedAuthorsV9 {
		return nil, fmt.Errorf("V9 public authorship count is invalid")
	}
	records := make([]PublicAuthorshipV9, 0, len(authors))
	for _, author := range authors {
		bundle := bundles[author]
		var authorship corpusfreezev9.Authorship
		decoder := json.NewDecoder(bytes.NewReader(bundle.AuthorshipJSON))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&authorship); err != nil {
			return nil, fmt.Errorf("decode V9 %s authorship: %w", author, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("decode V9 %s authorship: trailing content", author)
		}
		if authorship.Schema != "kicadai.closed-loop-open-set-authorship.v9" || authorship.Version != 9 || authorship.AuthorSlot != author ||
			authorship.PerAuthorPacketSHA256 != report.AuthorPacketSHA256[author] || authorship.AssignmentSHA256 != report.AssignmentSHA256[author] ||
			authorship.ContractBindingSHA256 != report.ContractBindingSHA256 || !allAuthorshipAttestationsV9(authorship.Attestations) {
			return nil, fmt.Errorf("V9 %s public authorship binding is invalid", author)
		}
		records = append(records, PublicAuthorshipV9{
			AuthorSlot: author, AuthorshipSHA256: report.AuthorshipSHA256[author],
			PerAuthorPacketManifest: authorship.PerAuthorPacketManifest, PerAuthorPacketSHA256: authorship.PerAuthorPacketSHA256,
			ContractBindingSHA256: authorship.ContractBindingSHA256, AssignmentSHA256: authorship.AssignmentSHA256,
			AuthoringStartedUTC: authorship.AuthoringStartedUTC, AuthoringEndedUTC: authorship.AuthoringEndedUTC,
			UncertaintyCount: len(authorship.Uncertainties), Attestations: authorship.Attestations,
		})
	}
	return marshalStable(AuthorshipAttestationsV9{Schema: "kicadai.closed-loop-open-set-authorship-attestations.v9", Version: 9, Records: records})
}

func allAuthorshipAttestationsV9(value corpusfreezev9.AuthorshipAttestations) bool {
	return value.PacketOnlyInput && value.ContractBoundBeforeAuthoring && value.NoRepositoryOrPriorCorpusAccess &&
		value.NoCrossAuthorAssignmentOrContentAccess && value.IndependentlyConceivedBehaviorOnlyRequirements &&
		value.NoSynthesisSimulationClassificationRankingOrFeasibility && value.FixedDiscoveryHeldOutMembership &&
		value.NoImplementationOrExpectedOutcomePrescription && value.NoObligationAnchorGapExposureOrCausalPathAuthorship &&
		value.NoPostEvaluationInspectionOrModification && value.AllUncertaintiesDisclosed
}

func (prepared preparedCorpusV9) publish() (result ResultV9, err error) {
	if _, statErr := os.Lstat(prepared.destination); statErr == nil {
		return ResultV9{}, fmt.Errorf("V9 destination already exists")
	} else if !os.IsNotExist(statErr) {
		return ResultV9{}, fmt.Errorf("inspect V9 destination: %w", statErr)
	}
	parent := filepath.Dir(prepared.destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ResultV9{}, err
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(prepared.destination)+".stage-")
	if err != nil {
		return ResultV9{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	key, err := createExclusiveKey(prepared.repositoryRoot, prepared.keyPath, prepared.random)
	if err != nil {
		return ResultV9{}, err
	}
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			_ = os.Remove(prepared.keyPath)
		}
	}()

	ciphertext, sealMetadata, err := sealHeldOutRecordsV9(key, prepared.manifest, prepared.heldOut, prepared.random)
	if err != nil {
		return ResultV9{}, err
	}
	prepared.manifest.HeldOutSource = sealMetadata
	opened, err := OpenHeldOutV9(key, prepared.manifest, ciphertext)
	equalOpened := err == nil && equalHeldOutV9(opened, prepared.heldOut)
	clearHeldOutCasesV9(opened)
	if !equalOpened {
		return ResultV9{}, fmt.Errorf("verify V9 record seals")
	}
	manifestBytes, err := marshalStable(prepared.manifest)
	if err != nil {
		return ResultV9{}, err
	}
	manifestHash := hashBytes(manifestBytes)
	discoveryObligations, heldOutCommitment, err := deriveObligationsV9(manifestHash, prepared.manifest, prepared.discovery, prepared.heldOut)
	if err != nil {
		return ResultV9{}, err
	}
	if err := writePublicationV9(stage, prepared, ciphertext, manifestBytes, discoveryObligations, heldOutCommitment); err != nil {
		return ResultV9{}, err
	}
	if err := writeChecksums(stage); err != nil {
		return ResultV9{}, err
	}
	if err := syncTree(stage); err != nil {
		return ResultV9{}, err
	}
	if err := renameNoReplace(stage, prepared.destination); err != nil {
		return ResultV9{}, fmt.Errorf("publish V9 corpus atomically: %w", err)
	}
	keyCommitted = true
	return ResultV9{Manifest: prepared.manifest, ManifestSHA256: manifestHash, DiscoveryCases: expectedDiscoveryV9,
		HeldOutCases: expectedHeldOutV9, DiscoveryObligations: len(discoveryObligations.Obligations), HeldOutObligations: heldOutCommitment.ObligationCount}, nil
}

func writePublicationV9(stage string, prepared preparedCorpusV9, ciphertext, manifestBytes []byte, discovery DiscoveryObligationsV9, heldOut HeldOutObligationCommitmentV9) error {
	artifacts := []struct {
		path string
		data []byte
	}{{ValidationFileV9, prepared.reportBytes}, {AuthorshipAttestationsFileV9, prepared.authorshipBytes}, {HeldOutCipherFileV9, ciphertext}, {ManifestFileV9, manifestBytes}}
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
	}{DiscoveryObligationsFileV9, discoveryBytes}, struct {
		path string
		data []byte
	}{HeldOutCommitmentFileV9, heldOutBytes})
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
	audit := auditBytesV9(prepared.manifest, hashBytes(manifestBytes), discovery, heldOut)
	return writeExclusive(filepath.Join(stage, AuditFileV9), audit, 0o644)
}

func encodeHeldOutRecordV9(item heldOutCaseV9) ([]byte, error) {
	metadata, err := json.Marshal(item.Entry)
	if err != nil {
		return nil, err
	}
	if len(metadata) > maximumMetadata || len(item.Source) == 0 || len(item.Source) > ots.MaxRequirementBytes {
		return nil, fmt.Errorf("V9 held-out record exceeds bounds")
	}
	var output bytes.Buffer
	output.WriteString(heldOutMagicV9)
	if err := binary.Write(&output, binary.BigEndian, uint32(HeldOutVersionV9)); err != nil {
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

func decodeHeldOutRecordV9(data []byte) (heldOutCaseV9, error) {
	reader := bytes.NewReader(data)
	magic := make([]byte, len(heldOutMagicV9))
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != heldOutMagicV9 {
		return heldOutCaseV9{}, fmt.Errorf("V9 held-out magic invalid")
	}
	var version uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != HeldOutVersionV9 {
		return heldOutCaseV9{}, fmt.Errorf("V9 held-out version invalid")
	}
	metadata, err := readLengthDelimited(reader, maximumMetadata)
	if err != nil {
		return heldOutCaseV9{}, err
	}
	var entry EntryV9
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		return heldOutCaseV9{}, err
	}
	source, err := readLengthDelimited(reader, ots.MaxRequirementBytes)
	if err != nil {
		return heldOutCaseV9{}, err
	}
	if reader.Len() != 0 || !entry.Sealed || hashBytes(source) != entry.RequirementSHA256 {
		return heldOutCaseV9{}, fmt.Errorf("V9 held-out record commitment invalid")
	}
	return heldOutCaseV9{Entry: entry, Source: source}, nil
}

func publicValidationReportV9(report corpusfreezev9.Report, discoveryStablePaths map[string]string) (PublicValidationReportV9, error) {
	discovery := []corpusfreezev9.EntryEvidence{}
	heldOutDigests := []string{}
	for _, entry := range report.Entries {
		if entry.Role == "discovery" {
			stablePath, exists := discoveryStablePaths[entry.ID]
			if !exists {
				return PublicValidationReportV9{}, fmt.Errorf("V9 public validation discovery path missing")
			}
			entry.RequirementFile = stablePath
			discovery = append(discovery, entry)
			continue
		}
		if entry.Role != "held_out" {
			return PublicValidationReportV9{}, fmt.Errorf("V9 public validation role invalid")
		}
		entry.RequirementFile = filepath.ToSlash(filepath.Join("held_out", entry.ID+".json"))
		data, err := json.Marshal(entry)
		if err != nil {
			return PublicValidationReportV9{}, err
		}
		heldOutDigests = append(heldOutDigests, hashBytes(data))
	}
	sort.Slice(discovery, func(i, j int) bool { return discovery[i].ID < discovery[j].ID })
	sort.Strings(heldOutDigests)
	if len(discovery) != expectedDiscoveryV9 || len(discoveryStablePaths) != expectedDiscoveryV9 || len(heldOutDigests) != expectedHeldOutV9 {
		return PublicValidationReportV9{}, fmt.Errorf("V9 public validation counts invalid")
	}
	return PublicValidationReportV9{Schema: "kicadai.behavior-corpus-public-validation-report.v9", Version: 9,
		PolicySHA256: report.PolicySHA256, PacketSetSHA256: report.PacketSetSHA256, ContractBindingSHA256: report.ContractBindingSHA256,
		HistoricalCommitmentsSHA256: report.HistoricalCommitmentsSHA256, AuthorPacketSHA256: cloneMap(report.AuthorPacketSHA256),
		AssignmentSHA256: cloneMap(report.AssignmentSHA256), AuthorshipSHA256: cloneMap(report.AuthorshipSHA256), Counts: cloneCounts(report.Counts),
		DiscoveryEntries: discovery, HeldOutEntryCount: len(heldOutDigests), HeldOutEntryAggregateSHA256: aggregateDigestsV9(heldOutDigests)}, nil
}
