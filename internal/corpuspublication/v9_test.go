package corpuspublication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev9"
)

func TestPublishV9RecordSealsAndObligations(t *testing.T) {
	request, sources := publicationFixtureV9(t)
	result, err := PublishV9(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Schema != ManifestSchemaV9 || result.Manifest.Version != 9 || result.DiscoveryCases != 24 || result.HeldOutCases != 24 ||
		result.DiscoveryObligations == 0 || result.HeldOutObligations == 0 || len(result.Manifest.Entries) != 24 || len(result.Manifest.HeldOutSource.RecordCiphertextSHA256) != 24 {
		t.Fatalf("unexpected V9 result: %+v", result)
	}
	if err := verifyChecksums(request.DestinationRoot); err != nil {
		t.Fatal(err)
	}
	publicVerification, err := VerifyPublicationV9(request.DestinationRoot)
	if err != nil || publicVerification.DiscoveryCases != 24 || publicVerification.HeldOutCases != 24 {
		t.Fatalf("verify public V9 publication: %+v, %v", publicVerification, err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(request.DestinationRoot, ManifestFileV9))
	if err != nil {
		t.Fatal(err)
	}
	if hashBytes(manifestBytes) != result.ManifestSHA256 {
		t.Fatal("V9 manifest hash mismatch")
	}
	validationBytes, err := os.ReadFile(filepath.Join(request.DestinationRoot, ValidationFileV9))
	if err != nil {
		t.Fatal(err)
	}
	var validation PublicValidationReportV9
	if err := json.Unmarshal(validationBytes, &validation); err != nil {
		t.Fatal(err)
	}
	stablePaths := make(map[string]string, len(result.Manifest.Entries))
	for _, entry := range result.Manifest.Entries {
		if entry.Role != "discovery" || entry.Sealed {
			t.Fatalf("public manifest disclosed a held-out entry: %+v", entry)
		}
		publishedPath := filepath.Join(request.DestinationRoot, filepath.FromSlash(entry.StablePath))
		data, err := os.ReadFile(publishedPath)
		if err != nil || !bytes.Equal(data, sources[entry.ID]) {
			t.Fatalf("discovery mismatch: %s", entry.ID)
		}
		stablePaths[entry.ID] = entry.StablePath
	}
	for _, entry := range validation.DiscoveryEntries {
		if entry.RequirementFile != stablePaths[entry.ID] {
			t.Fatalf("validation path %q does not match published path %q", entry.RequirementFile, stablePaths[entry.ID])
		}
	}
	for _, name := range []string{ManifestFileV9, ValidationFileV9, DiscoveryObligationsFileV9, AuthorshipAttestationsFileV9, AuditFileV9} {
		data, err := os.ReadFile(filepath.Join(request.DestinationRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("v9_case_025")) || bytes.Contains(data, []byte("held_out/request_025")) {
			t.Fatalf("public artifact %s disclosed held-out case mapping", name)
		}
	}
	if _, err := os.Lstat(filepath.Join(request.DestinationRoot, "authorship")); !os.IsNotExist(err) {
		t.Fatal("raw authorship metadata was published")
	}
	authorshipBytes, err := os.ReadFile(filepath.Join(request.DestinationRoot, AuthorshipAttestationsFileV9))
	if err != nil || hashBytes(authorshipBytes) != result.Manifest.AuthorshipAttestationsSHA256 {
		t.Fatalf("sanitized authorship attestations invalid: %v", err)
	}
	var authorship AuthorshipAttestationsV9
	if err := json.Unmarshal(authorshipBytes, &authorship); err != nil || authorship.Version != 9 || len(authorship.Records) != expectedAuthorsV9 {
		t.Fatalf("sanitized authorship attestations malformed: %v", err)
	}
	for _, forbidden := range []string{"author_context_identity", "authoring_tool_model_version", "returned_bundle_root", "requirement_source_sha256", "held_out/request_025", "v9_case_025"} {
		if bytes.Contains(authorshipBytes, []byte(forbidden)) {
			t.Fatalf("sanitized authorship disclosed %q", forbidden)
		}
	}
	key, err := os.ReadFile(request.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(request.KeyPath)
	if err != nil || info.Mode().Perm() != 0o600 || len(key) != 32 {
		t.Fatalf("V9 key metadata invalid")
	}
	ciphertext, err := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFileV9))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenHeldOutV9(key, result.Manifest, ciphertext)
	if err != nil || len(opened) != 24 {
		t.Fatalf("open V9 records: %d, %v", len(opened), err)
	}
	for _, item := range opened {
		if !bytes.Equal(item.Source, sources[item.Entry.ID]) {
			t.Fatalf("held-out mismatch: %s", item.Entry.ID)
		}
	}
	keyedVerification, err := VerifyPublicationV9WithKey(request.DestinationRoot, key)
	if err != nil || keyedVerification.ManifestSHA256 != result.ManifestSHA256 || keyedVerification.HeldOutCases != 24 {
		t.Fatalf("verify keyed V9 publication: %+v, %v", keyedVerification, err)
	}
	if _, err := VerifyPublicationV9WithKey(request.DestinationRoot, bytes.Repeat([]byte{0xff}, 32)); err == nil {
		t.Fatal("wrong V9 source key was accepted")
	}

	var discovery DiscoveryObligationsV9
	data, err := os.ReadFile(filepath.Join(request.DestinationRoot, DiscoveryObligationsFileV9))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &discovery); err != nil || discovery.CorpusManifestSHA256 != result.ManifestSHA256 || len(discovery.Obligations) != result.DiscoveryObligations {
		t.Fatalf("discovery obligations invalid: %v", err)
	}
	var heldOut HeldOutObligationCommitmentV9
	data, err = os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCommitmentFileV9))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &heldOut); err != nil || heldOut.CorpusManifestSHA256 != result.ManifestSHA256 || heldOut.ObligationCount != result.HeldOutObligations || !validSHA256(heldOut.AggregateSHA256) {
		t.Fatalf("held-out obligation commitment invalid: %v", err)
	}
}

func TestVerifyPublicationV9RejectsUnexpectedFileAndCrossVersionRecord(t *testing.T) {
	request, _ := publicationFixtureV9(t)
	_, err := PublishV9(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(request.DestinationRoot, "unexpected"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPublicationV9(request.DestinationRoot); err == nil {
		t.Fatal("V9 verifier accepted an unexpected artifact")
	}
	item := heldOutCaseV9{Entry: EntryV9{ID: "v9_case_025", Role: "held_out", StablePath: "held_out/v9_case_025.json", Sealed: true,
		RequirementSHA256: hashBytes([]byte("source"))}, Source: []byte("source")}
	record, err := encodeHeldOutRecordV9(item)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeHeldOutRecordV8(record); err == nil {
		t.Fatal("V8 decoder accepted a V9 held-out record")
	}
}

func TestOpenHeldOutV9RejectsRecordAndAADTampering(t *testing.T) {
	request, _ := publicationFixtureV9(t)
	result, err := PublishV9(request)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := os.ReadFile(request.KeyPath)
	ciphertext, _ := os.ReadFile(filepath.Join(request.DestinationRoot, HeldOutCipherFileV9))
	tampered := append([]byte(nil), ciphertext...)
	tampered[len(tampered)-1] ^= 1
	if _, err := OpenHeldOutV9(key, result.Manifest, tampered); err == nil {
		t.Fatal("tampered V9 record set accepted")
	}
	metadata := result.Manifest
	metadata.PacketSetSHA256 = strings.Repeat("a", 64)
	if _, err := OpenHeldOutV9(key, metadata, ciphertext); err == nil {
		t.Fatal("tampered V9 AAD metadata accepted")
	}
	metadata = result.Manifest
	metadata.AuthorshipAttestationsSHA256 = strings.Repeat("b", 64)
	if _, err := OpenHeldOutV9(key, metadata, ciphertext); err == nil {
		t.Fatal("tampered V9 authorship AAD metadata accepted")
	}
}

func TestPublishV9FailsClosedOnOccupiedDestinationAndEntropy(t *testing.T) {
	request, _ := publicationFixtureV9(t)
	if err := os.MkdirAll(request.DestinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishV9(request); err == nil {
		t.Fatal("occupied V9 destination accepted")
	}
	if _, err := os.Lstat(request.KeyPath); !os.IsNotExist(err) {
		t.Fatal("refused V9 publication created a key")
	}

	request, _ = publicationFixtureV9(t)
	request.Random = bytes.NewReader(make([]byte, 40))
	if _, err := PublishV9(request); err == nil {
		t.Fatal("short V9 entropy accepted")
	}
	if _, err := os.Lstat(request.DestinationRoot); !os.IsNotExist(err) {
		t.Fatal("failed V9 publication left destination")
	}
	if _, err := os.Lstat(request.KeyPath); !os.IsNotExist(err) {
		t.Fatal("failed V9 publication left key")
	}
}

func publicationFixtureV9(t *testing.T) (RequestV9, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join("..", "opentopologysynthesis", "testdata", "architecture_generalization_corpus", "regulated_low_voltage_output.json"))
	if err != nil {
		t.Fatal(err)
	}
	report := corpusfreezev9.Report{Schema: "kicadai.behavior-corpus-validation-report.v9", Version: 9,
		PolicySHA256: hashBytes([]byte("policy-v9")), PacketSetSHA256: hashBytes([]byte("packet-v9")),
		ContractBindingSHA256: hashBytes([]byte("binding-v9")), HistoricalCommitmentsSHA256: hashBytes([]byte("history-v9")),
		AuthorPacketSHA256: map[string]string{}, AssignmentSHA256: map[string]string{}, AuthorshipSHA256: map[string]string{},
		Counts: map[string]map[string]int{"discovery": {"analog_signal_path": 24}, "held_out": {"analog_signal_path": 24}}}
	bundles := map[string]corpusfreeze.Bundle{}
	sources := map[string][]byte{}
	for authorIndex := 1; authorIndex <= expectedAuthorsV9; authorIndex++ {
		author := fmt.Sprintf("author_%d", authorIndex)
		report.AuthorPacketSHA256[author] = hashBytes([]byte("packet-" + author))
		report.AssignmentSHA256[author] = hashBytes([]byte("assignment-" + author))
		bundles[author] = corpusfreeze.Bundle{Requirements: map[string][]byte{}}
	}
	for index := 1; index <= expectedCasesV9; index++ {
		id := fmt.Sprintf("v9_case_%03d", index)
		authorIndex := ((index - 1) / 4) + 1
		role := "discovery"
		if index > 24 {
			role = "held_out"
			authorIndex = ((index - 25) / 4) + 1
		}
		author := fmt.Sprintf("author_%d", authorIndex)
		requestPath := fmt.Sprintf("%s/request_%03d.json", role, index)
		bundle := bundles[author]
		bundle.Requirements[requestPath] = source
		bundles[author] = bundle
		sources[id] = source
		report.Entries = append(report.Entries, corpusfreezev9.EntryEvidence{ID: id, AuthorSlot: author, Role: role, Domain: "analog_signal_path", CircuitRole: "conversion_regulation", SafetyImpact: "non_safety",
			PrimaryClass: "static", RequiredPrimaryAnalysis: "dc_operating_point", OutputMultiplicity: "single", RequireOffNominal: false,
			SourceID: fmt.Sprintf("v9_source_%03d", index), RequirementFile: requestPath, RequirementSHA256: hashBytes(source),
			NeutralSemanticSHA256: hashBytes([]byte("neutral-" + id)), NormalizedSemanticSHA256: hashBytes([]byte("normalized-" + id))})
	}
	for authorIndex := 1; authorIndex <= expectedAuthorsV9; authorIndex++ {
		author := fmt.Sprintf("author_%d", authorIndex)
		bundle := bundles[author]
		paths := sortedKeys(bundle.Requirements)
		sourceHashes := make([]corpusfreezev9.SourceHash, 0, len(paths))
		for _, path := range paths {
			sourceHashes = append(sourceHashes, corpusfreezev9.SourceHash{Path: path, SHA256: hashBytes(bundle.Requirements[path])})
		}
		authorship := corpusfreezev9.Authorship{Schema: "kicadai.closed-loop-open-set-authorship.v9", Version: 9,
			AuthorContextIdentity: "fixture-" + author, AuthorSlot: author, AuthoringToolModelVersion: "fixture-model",
			AuthoringStartedUTC: "2026-08-13T12:00:00Z", AuthoringEndedUTC: "2026-08-13T12:01:00Z",
			PerAuthorPacketManifest: fmt.Sprintf("AUTHOR_%d_PACKET.sha256", authorIndex), PerAuthorPacketSHA256: report.AuthorPacketSHA256[author],
			ContractBindingSHA256: report.ContractBindingSHA256, AssignmentSHA256: report.AssignmentSHA256[author], ReturnedBundleRoot: author,
			RequirementSourceSHA256: sourceHashes, Uncertainties: []string{}, Attestations: allTrueAuthorshipAttestationsV9Fixture()}
		authorshipBytes, err := json.Marshal(authorship)
		if err != nil {
			t.Fatal(err)
		}
		authorshipBytes = append(authorshipBytes, '\n')
		bundle.AuthorshipJSON = authorshipBytes
		bundles[author] = bundle
		report.AuthorshipSHA256[author] = hashBytes(authorshipBytes)
	}
	entropy := make([]byte, 2048)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	return RequestV9{RepositoryRoot: repository, DestinationRoot: filepath.Join(repository, "corpus"), KeyPath: filepath.Join(root, "keys", "v9-source.key"),
		ContractManifestSHA256: hashBytes([]byte("contract-v9")), ValidatorManifest: []byte("validator-v9\n"), PublisherManifest: []byte("publisher-v9\n"),
		Commits: Commits{StartingCommit: strings.Repeat("1", 40), ContractFreezeCommit: strings.Repeat("2", 40), AuthoringPacketCommit: strings.Repeat("3", 40), ValidatorCommit: strings.Repeat("4", 40), FreezeParentCommit: strings.Repeat("5", 40)},
		Report:  report, Bundles: bundles, Random: bytes.NewReader(entropy)}, sources
}

func allTrueAuthorshipAttestationsV9Fixture() corpusfreezev9.AuthorshipAttestations {
	return corpusfreezev9.AuthorshipAttestations{
		PacketOnlyInput: true, ContractBoundBeforeAuthoring: true, NoRepositoryOrPriorCorpusAccess: true,
		NoCrossAuthorAssignmentOrContentAccess: true, IndependentlyConceivedBehaviorOnlyRequirements: true,
		NoSynthesisSimulationClassificationRankingOrFeasibility: true, FixedDiscoveryHeldOutMembership: true,
		NoImplementationOrExpectedOutcomePrescription: true, NoObligationAnchorGapExposureOrCausalPathAuthorship: true,
		NoPostEvaluationInspectionOrModification: true, AllUncertaintiesDisclosed: true,
	}
}
