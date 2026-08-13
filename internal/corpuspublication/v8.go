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

	"kicadai/internal/corpusfreezev8"
	ots "kicadai/internal/opentopologysynthesis"
)

const heldOutMagicV8 = "KICADAI-V8-HELDOUT-RECORDS\x00"

type preparedCorpusV8 struct {
	manifest       ManifestV8
	reportBytes    []byte
	discovery      map[string][]byte
	heldOut        []heldOutCaseV8
	destination    string
	repositoryRoot string
	keyPath        string
	random         io.Reader
}

func PublishV8(request RequestV8) (ResultV8, error) {
	prepared, err := prepareV8(request)
	if err != nil {
		return ResultV8{}, err
	}
	return prepared.publish()
}

func prepareV8(request RequestV8) (preparedCorpusV8, error) {
	repositoryRoot, destination, keyPath, err := validatePaths(request.RepositoryRoot, request.DestinationRoot, request.KeyPath)
	if err != nil {
		return preparedCorpusV8{}, err
	}
	if request.Random == nil {
		request.Random = rand.Reader
	}
	if !validSHA256(request.ContractManifestSHA256) || len(request.ValidatorManifest) == 0 || len(request.PublisherManifest) == 0 {
		return preparedCorpusV8{}, fmt.Errorf("V8 contract, validator, or publisher commitment is invalid")
	}
	for name, commit := range map[string]string{
		"starting": request.Commits.StartingCommit, "contract freeze": request.Commits.ContractFreezeCommit,
		"authoring packet": request.Commits.AuthoringPacketCommit, "validator": request.Commits.ValidatorCommit,
		"freeze parent": request.Commits.FreezeParentCommit,
	} {
		if !commitPattern.MatchString(commit) {
			return preparedCorpusV8{}, fmt.Errorf("V8 %s commit is invalid", name)
		}
	}
	_, err = request.Report.MarshalJSONStable()
	if err != nil {
		return preparedCorpusV8{}, err
	}
	if len(request.Report.Entries) != expectedCasesV8 || len(request.Bundles) != expectedAuthorsV8 {
		return preparedCorpusV8{}, fmt.Errorf("V8 validated corpus size is invalid")
	}

	entries := toV8Entries(request.Report.Entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	manifestEntries := make([]EntryV8, 0, len(entries))
	discoveryStablePaths := make(map[string]string, expectedDiscoveryV8)
	discovery := map[string][]byte{}
	heldOut := make([]heldOutCaseV8, 0, expectedHeldOutV8)
	for author, bundle := range request.Bundles {
		wantHash := request.Report.AuthorshipSHA256[author]
		if !validSHA256(wantHash) || hashBytes(bundle.AuthorshipJSON) != wantHash {
			return preparedCorpusV8{}, fmt.Errorf("V8 %s authorship mismatch", author)
		}
	}
	if len(request.Bundles) != len(request.Report.AuthorshipSHA256) {
		return preparedCorpusV8{}, fmt.Errorf("V8 authorship set is incomplete")
	}
	seenIDs := make(map[string]bool, len(entries))
	seenPaths := make(map[string]bool, len(entries))
	roleCounts := make(map[string]int, 2)
	for _, evidence := range entries {
		if seenIDs[evidence.ID] {
			return preparedCorpusV8{}, fmt.Errorf("V8 duplicate case identity")
		}
		seenIDs[evidence.ID] = true
		bundle, exists := request.Bundles[evidence.AuthorSlot]
		if !exists {
			return preparedCorpusV8{}, fmt.Errorf("V8 unknown author")
		}
		source, exists := bundle.Requirements[evidence.RequirementFile]
		if !exists || hashBytes(source) != evidence.RequirementSHA256 {
			return preparedCorpusV8{}, fmt.Errorf("V8 %s source mismatch", evidence.ID)
		}
		if evidence.Role != "discovery" && evidence.Role != "held_out" {
			return preparedCorpusV8{}, fmt.Errorf("V8 invalid role")
		}
		stablePath := filepath.ToSlash(filepath.Join(evidence.Role, evidence.ID+".json"))
		if seenPaths[stablePath] {
			return preparedCorpusV8{}, fmt.Errorf("V8 duplicate stable path")
		}
		seenPaths[stablePath] = true
		entry := EntryV8{ID: evidence.ID, AuthorSlot: evidence.AuthorSlot, Role: evidence.Role, Domain: evidence.Domain,
			CircuitRole: evidence.CircuitRole, SafetyImpact: evidence.SafetyImpact, SourceID: evidence.SourceID, StablePath: stablePath,
			RequirementSHA256: evidence.RequirementSHA256, NeutralSemanticSHA256: evidence.NeutralSemanticSHA256,
			NormalizedSemanticSHA256: evidence.NormalizedSemanticSHA256, Sealed: evidence.Role == "held_out"}
		roleCounts[evidence.Role]++
		if evidence.Role == "discovery" {
			manifestEntries = append(manifestEntries, entry)
			discoveryStablePaths[evidence.ID] = stablePath
			discovery[stablePath] = append([]byte(nil), source...)
		} else {
			heldOut = append(heldOut, heldOutCaseV8{Entry: entry, Source: append([]byte(nil), source...)})
		}
	}
	if roleCounts["discovery"] != expectedDiscoveryV8 || roleCounts["held_out"] != expectedHeldOutV8 {
		return preparedCorpusV8{}, fmt.Errorf("V8 role counts are invalid")
	}
	publicReport, err := publicValidationReportV8(request.Report, discoveryStablePaths)
	if err != nil {
		return preparedCorpusV8{}, err
	}
	reportBytes, err := marshalStable(publicReport)
	if err != nil {
		return preparedCorpusV8{}, err
	}
	manifest := ManifestV8{Schema: ManifestSchemaV8, Version: ManifestVersionV8, Commits: request.Commits,
		ContractManifestSHA256: request.ContractManifestSHA256, ValidatorManifestSHA256: hashBytes(request.ValidatorManifest),
		PublisherManifestSHA256: hashBytes(request.PublisherManifest), ValidationReportSHA256: hashBytes(reportBytes),
		PolicySHA256: request.Report.PolicySHA256, PacketSetSHA256: request.Report.PacketSetSHA256,
		ContractBindingSHA256: request.Report.ContractBindingSHA256, HistoricalCommitmentsSHA256: request.Report.HistoricalCommitmentsSHA256,
		AuthorPacketSHA256: cloneMap(request.Report.AuthorPacketSHA256), AssignmentSHA256: cloneMap(request.Report.AssignmentSHA256),
		AuthorshipSHA256: cloneMap(request.Report.AuthorshipSHA256), Counts: cloneCounts(request.Report.Counts),
		DiscoveryCaseCount: expectedDiscoveryV8, HeldOutCaseCount: expectedHeldOutV8, Entries: manifestEntries}
	return preparedCorpusV8{manifest: manifest, reportBytes: reportBytes, discovery: discovery,
		heldOut: heldOut, destination: destination, repositoryRoot: repositoryRoot, keyPath: keyPath, random: request.Random}, nil
}

// corpusfreezev8.EntryEvidence is projected into a private alias so the
// publisher remains insulated from later report-type additions.
type corpusfreezev8Entry struct {
	ID, AuthorSlot, Role, Domain, CircuitRole, SafetyImpact, SourceID, RequirementFile string
	RequirementSHA256, NeutralSemanticSHA256, NormalizedSemanticSHA256                 string
}

func toV8Entries(entries []corpusfreezev8.EntryEvidence) []corpusfreezev8Entry {
	result := make([]corpusfreezev8Entry, len(entries))
	for index, entry := range entries {
		result[index] = corpusfreezev8Entry{entry.ID, entry.AuthorSlot, entry.Role, entry.Domain, entry.CircuitRole,
			entry.SafetyImpact, entry.SourceID, entry.RequirementFile, entry.RequirementSHA256, entry.NeutralSemanticSHA256, entry.NormalizedSemanticSHA256}
	}
	return result
}

func (prepared preparedCorpusV8) publish() (result ResultV8, err error) {
	if _, statErr := os.Lstat(prepared.destination); statErr == nil {
		return ResultV8{}, fmt.Errorf("V8 destination already exists")
	} else if !os.IsNotExist(statErr) {
		return ResultV8{}, fmt.Errorf("inspect V8 destination: %w", statErr)
	}
	parent := filepath.Dir(prepared.destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ResultV8{}, err
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(prepared.destination)+".stage-")
	if err != nil {
		return ResultV8{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	key, err := createExclusiveKey(prepared.repositoryRoot, prepared.keyPath, prepared.random)
	if err != nil {
		return ResultV8{}, err
	}
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			_ = os.Remove(prepared.keyPath)
		}
	}()

	ciphertext, sealMetadata, err := sealHeldOutRecordsV8(key, prepared.manifest, prepared.heldOut, prepared.random)
	if err != nil {
		return ResultV8{}, err
	}
	prepared.manifest.HeldOutSource = sealMetadata
	opened, err := OpenHeldOutV8(key, prepared.manifest, ciphertext)
	if err != nil || !equalHeldOutV8(opened, prepared.heldOut) {
		return ResultV8{}, fmt.Errorf("verify V8 record seals")
	}
	manifestBytes, err := marshalStable(prepared.manifest)
	if err != nil {
		return ResultV8{}, err
	}
	manifestHash := hashBytes(manifestBytes)
	discoveryObligations, heldOutCommitment, err := deriveObligationsV8(manifestHash, prepared.manifest, prepared.discovery, prepared.heldOut)
	if err != nil {
		return ResultV8{}, err
	}
	if err := writePublicationV8(stage, prepared, ciphertext, manifestBytes, discoveryObligations, heldOutCommitment); err != nil {
		return ResultV8{}, err
	}
	if err := writeChecksums(stage); err != nil {
		return ResultV8{}, err
	}
	if err := syncTree(stage); err != nil {
		return ResultV8{}, err
	}
	if err := renameNoReplace(stage, prepared.destination); err != nil {
		return ResultV8{}, fmt.Errorf("publish V8 corpus atomically: %w", err)
	}
	keyCommitted = true
	return ResultV8{Manifest: prepared.manifest, ManifestSHA256: manifestHash, DiscoveryCases: expectedDiscoveryV8,
		HeldOutCases: expectedHeldOutV8, DiscoveryObligations: len(discoveryObligations.Obligations), HeldOutObligations: heldOutCommitment.ObligationCount}, nil
}

func writePublicationV8(stage string, prepared preparedCorpusV8, ciphertext, manifestBytes []byte, discovery DiscoveryObligationsV8, heldOut HeldOutObligationCommitmentV8) error {
	artifacts := []struct {
		path string
		data []byte
	}{{ValidationFileV8, prepared.reportBytes}, {HeldOutCipherFileV8, ciphertext}, {ManifestFileV8, manifestBytes}}
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
	}{DiscoveryObligationsFileV8, discoveryBytes}, struct {
		path string
		data []byte
	}{HeldOutCommitmentFileV8, heldOutBytes})
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
	audit := auditBytesV8(prepared.manifest, hashBytes(manifestBytes), discovery, heldOut)
	return writeExclusive(filepath.Join(stage, AuditFileV8), audit, 0o644)
}

func encodeHeldOutRecordV8(item heldOutCaseV8) ([]byte, error) {
	metadata, err := json.Marshal(item.Entry)
	if err != nil {
		return nil, err
	}
	if len(metadata) > maximumMetadata || len(item.Source) == 0 || len(item.Source) > ots.MaxRequirementBytes {
		return nil, fmt.Errorf("V8 held-out record exceeds bounds")
	}
	var output bytes.Buffer
	output.WriteString(heldOutMagicV8)
	if err := binary.Write(&output, binary.BigEndian, uint32(HeldOutVersionV8)); err != nil {
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

func decodeHeldOutRecordV8(data []byte) (heldOutCaseV8, error) {
	reader := bytes.NewReader(data)
	magic := make([]byte, len(heldOutMagicV8))
	if _, err := io.ReadFull(reader, magic); err != nil || string(magic) != heldOutMagicV8 {
		return heldOutCaseV8{}, fmt.Errorf("V8 held-out magic invalid")
	}
	var version uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil || version != HeldOutVersionV8 {
		return heldOutCaseV8{}, fmt.Errorf("V8 held-out version invalid")
	}
	metadata, err := readLengthDelimited(reader, maximumMetadata)
	if err != nil {
		return heldOutCaseV8{}, err
	}
	var entry EntryV8
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		return heldOutCaseV8{}, err
	}
	source, err := readLengthDelimited(reader, ots.MaxRequirementBytes)
	if err != nil {
		return heldOutCaseV8{}, err
	}
	if reader.Len() != 0 || !entry.Sealed || hashBytes(source) != entry.RequirementSHA256 {
		return heldOutCaseV8{}, fmt.Errorf("V8 held-out record commitment invalid")
	}
	return heldOutCaseV8{Entry: entry, Source: source}, nil
}

func publicValidationReportV8(report corpusfreezev8.Report, discoveryStablePaths map[string]string) (PublicValidationReportV8, error) {
	discovery := []corpusfreezev8.EntryEvidence{}
	heldOutDigests := []string{}
	for _, entry := range report.Entries {
		if entry.Role == "discovery" {
			stablePath, exists := discoveryStablePaths[entry.ID]
			if !exists {
				return PublicValidationReportV8{}, fmt.Errorf("V8 public validation discovery path missing")
			}
			entry.RequirementFile = stablePath
			discovery = append(discovery, entry)
			continue
		}
		if entry.Role != "held_out" {
			return PublicValidationReportV8{}, fmt.Errorf("V8 public validation role invalid")
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return PublicValidationReportV8{}, err
		}
		heldOutDigests = append(heldOutDigests, hashBytes(data))
	}
	sort.Slice(discovery, func(i, j int) bool { return discovery[i].ID < discovery[j].ID })
	sort.Strings(heldOutDigests)
	if len(discovery) != expectedDiscoveryV8 || len(discoveryStablePaths) != expectedDiscoveryV8 || len(heldOutDigests) != expectedHeldOutV8 {
		return PublicValidationReportV8{}, fmt.Errorf("V8 public validation counts invalid")
	}
	return PublicValidationReportV8{Schema: "kicadai.behavior-corpus-public-validation-report.v8", Version: 8,
		PolicySHA256: report.PolicySHA256, PacketSetSHA256: report.PacketSetSHA256, ContractBindingSHA256: report.ContractBindingSHA256,
		HistoricalCommitmentsSHA256: report.HistoricalCommitmentsSHA256, AuthorPacketSHA256: cloneMap(report.AuthorPacketSHA256),
		AssignmentSHA256: cloneMap(report.AssignmentSHA256), AuthorshipSHA256: cloneMap(report.AuthorshipSHA256), Counts: cloneCounts(report.Counts),
		DiscoveryEntries: discovery, HeldOutEntryCount: len(heldOutDigests), HeldOutEntryAggregateSHA256: aggregateDigestsV8(heldOutDigests)}, nil
}
