package corpuspublication

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"kicadai/internal/corpusfreeze"
)

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

type preparedCorpus struct {
	manifest       Manifest
	reportBytes    []byte
	authorship     map[string][]byte
	discovery      map[string][]byte
	heldOut        []HeldOutCase
	destination    string
	repositoryRoot string
	keyPath        string
	random         io.Reader
}

func Publish(request Request) (Result, error) {
	prepared, err := prepare(request)
	if err != nil {
		return Result{}, err
	}
	return prepared.publish()
}

func prepare(request Request) (preparedCorpus, error) {
	repositoryRoot, destination, keyPath, err := validatePaths(request.RepositoryRoot, request.DestinationRoot, request.KeyPath)
	if err != nil {
		return preparedCorpus{}, err
	}
	if request.Random == nil {
		request.Random = rand.Reader
	}
	if !validSHA256(request.ContractManifestSHA256) || len(request.ValidatorManifest) == 0 || len(request.PublisherManifest) == 0 {
		return preparedCorpus{}, fmt.Errorf("contract, validator, or publisher manifest commitment is invalid")
	}
	for name, commit := range map[string]string{
		"starting": request.Commits.StartingCommit, "contract freeze": request.Commits.ContractFreezeCommit,
		"authoring packet": request.Commits.AuthoringPacketCommit, "validator": request.Commits.ValidatorCommit,
		"freeze parent": request.Commits.FreezeParentCommit,
	} {
		if !commitPattern.MatchString(commit) {
			return preparedCorpus{}, fmt.Errorf("%s commit is invalid", name)
		}
	}
	reportBytes, err := request.Report.MarshalJSONStable()
	if err != nil {
		return preparedCorpus{}, err
	}
	if len(request.Report.Entries) != expectedCases || len(request.Bundles) != expectedAuthors {
		return preparedCorpus{}, fmt.Errorf("validated corpus size is %d cases across %d authors, want %d across %d", len(request.Report.Entries), len(request.Bundles), expectedCases, expectedAuthors)
	}

	entries := append([]corpusfreeze.EntryEvidence(nil), request.Report.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	manifestEntries := make([]Entry, 0, len(entries))
	discovery := map[string][]byte{}
	heldOut := make([]HeldOutCase, 0, expectedHeldOut)
	authorship := map[string][]byte{}
	seenIDs := map[string]bool{}
	seenPaths := map[string]bool{}
	roleCounts := map[string]int{}

	for author, bundle := range request.Bundles {
		wantHash := request.Report.AuthorshipSHA256[author]
		if !validSHA256(wantHash) || hashBytes(bundle.AuthorshipJSON) != wantHash {
			return preparedCorpus{}, fmt.Errorf("%s authorship does not match the validated report", author)
		}
		authorship[author] = append([]byte(nil), bundle.AuthorshipJSON...)
	}
	if len(authorship) != len(request.Report.AuthorshipSHA256) {
		return preparedCorpus{}, fmt.Errorf("authorship bundle set is incomplete")
	}

	for _, evidence := range entries {
		if seenIDs[evidence.ID] {
			return preparedCorpus{}, fmt.Errorf("validated report duplicates case identity")
		}
		seenIDs[evidence.ID] = true
		bundle, exists := request.Bundles[evidence.AuthorSlot]
		if !exists {
			return preparedCorpus{}, fmt.Errorf("validated report refers to an unknown author")
		}
		source, exists := bundle.Requirements[evidence.RequirementFile]
		if !exists || hashBytes(source) != evidence.RequirementSHA256 {
			return preparedCorpus{}, fmt.Errorf("%s source does not match the validated report", evidence.ID)
		}
		if evidence.Role != corpusfreeze.RoleDiscovery && evidence.Role != corpusfreeze.RoleHeldOut {
			return preparedCorpus{}, fmt.Errorf("%s has an invalid role", evidence.ID)
		}
		stablePath := filepath.ToSlash(filepath.Join(evidence.Role, evidence.ID+".json"))
		if seenPaths[stablePath] {
			return preparedCorpus{}, fmt.Errorf("stable corpus path is duplicated")
		}
		seenPaths[stablePath] = true
		entry := Entry{
			ID: evidence.ID, AuthorSlot: evidence.AuthorSlot, Role: evidence.Role, Domain: evidence.Domain,
			SafetyImpact: evidence.SafetyImpact, SourceID: evidence.SourceID, StablePath: stablePath,
			RequirementSHA256: evidence.RequirementSHA256, NeutralSemanticSHA256: evidence.NeutralSemanticSHA256,
			NormalizedSemanticSHA256: evidence.NormalizedSemanticSHA256, Sealed: evidence.Role == corpusfreeze.RoleHeldOut,
		}
		manifestEntries = append(manifestEntries, entry)
		roleCounts[evidence.Role]++
		if evidence.Role == corpusfreeze.RoleDiscovery {
			discovery[stablePath] = append([]byte(nil), source...)
		} else {
			heldOut = append(heldOut, HeldOutCase{Entry: entry, Source: append([]byte(nil), source...)})
		}
	}
	if roleCounts[corpusfreeze.RoleDiscovery] != expectedDiscovery || roleCounts[corpusfreeze.RoleHeldOut] != expectedHeldOut {
		return preparedCorpus{}, fmt.Errorf("validated role counts are discovery=%d held-out=%d, want %d/%d", roleCounts[corpusfreeze.RoleDiscovery], roleCounts[corpusfreeze.RoleHeldOut], expectedDiscovery, expectedHeldOut)
	}

	manifest := Manifest{
		Schema: ManifestSchema, Version: ManifestVersion, Commits: request.Commits,
		ContractManifestSHA256:  request.ContractManifestSHA256,
		ValidatorManifestSHA256: hashBytes(request.ValidatorManifest), ValidationReportSHA256: hashBytes(reportBytes),
		PublisherManifestSHA256: hashBytes(request.PublisherManifest),
		PolicySHA256:            request.Report.PolicySHA256, PacketSetSHA256: request.Report.PacketSetSHA256,
		ContractBindingSHA256:       request.Report.ContractBindingSHA256,
		HistoricalCommitmentsSHA256: request.Report.HistoricalCommitmentsSHA256,
		AuthorPacketSHA256:          cloneMap(request.Report.AuthorPacketSHA256), AssignmentSHA256: cloneMap(request.Report.AssignmentSHA256),
		AuthorshipSHA256: cloneMap(request.Report.AuthorshipSHA256), Counts: cloneCounts(request.Report.Counts),
		DiscoveryCaseCount: expectedDiscovery, HeldOutCaseCount: expectedHeldOut, Entries: manifestEntries,
	}
	return preparedCorpus{
		manifest: manifest, reportBytes: reportBytes, authorship: authorship, discovery: discovery, heldOut: heldOut,
		destination: destination, repositoryRoot: repositoryRoot, keyPath: keyPath, random: request.Random,
	}, nil
}

func (prepared preparedCorpus) publish() (result Result, err error) {
	if _, statErr := os.Lstat(prepared.destination); statErr == nil {
		return Result{}, fmt.Errorf("destination already exists")
	} else if !os.IsNotExist(statErr) {
		return Result{}, fmt.Errorf("inspect destination: %w", statErr)
	}
	parent := filepath.Dir(prepared.destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Result{}, fmt.Errorf("create destination parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(prepared.destination)+".stage-")
	if err != nil {
		return Result{}, fmt.Errorf("create publication stage: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	key, err := createExclusiveKey(prepared.repositoryRoot, prepared.keyPath, prepared.random)
	if err != nil {
		return Result{}, err
	}
	keyCommitted := false
	defer func() {
		if !keyCommitted {
			_ = os.Remove(prepared.keyPath)
		}
	}()

	payload, err := encodeHeldOutPayload(prepared.heldOut)
	if err != nil {
		return Result{}, err
	}
	payloadHash := hashBytes(payload)
	aad, err := publicationAAD(aadBindingFromManifest(prepared.manifest), payloadHash, len(prepared.heldOut))
	if err != nil {
		return Result{}, err
	}
	ciphertext, nonceBytes, err := seal(key, payload, aad, prepared.random)
	if err != nil {
		return Result{}, err
	}
	opened, err := open(key, ciphertext, aad, nonceBytes)
	if err != nil || !bytes.Equal(opened, payload) {
		return Result{}, fmt.Errorf("verify held-out seal")
	}
	prepared.manifest.HeldOutSource = HeldOutSeal{
		Algorithm: SealAlgorithm, File: HeldOutCipherFile, PayloadSHA256: payloadHash,
		CiphertextSHA256: hashBytes(ciphertext), AADSHA256: hashBytes(aad), NonceBytes: nonceBytes,
		CaseCount: len(prepared.heldOut),
	}

	if err := writePublication(stage, prepared, ciphertext); err != nil {
		return Result{}, err
	}
	manifestBytes, err := marshalStable(prepared.manifest)
	if err != nil {
		return Result{}, err
	}
	if err := writeExclusive(filepath.Join(stage, ManifestFile), manifestBytes, 0o644); err != nil {
		return Result{}, err
	}
	audit := auditBytes(prepared.manifest, hashBytes(manifestBytes))
	if err := writeExclusive(filepath.Join(stage, AuditFile), audit, 0o644); err != nil {
		return Result{}, err
	}
	if err := writeChecksums(stage); err != nil {
		return Result{}, err
	}
	if err := syncTree(stage); err != nil {
		return Result{}, err
	}
	// The stage is created in the destination parent, so the platform-native
	// no-replace rename is same-filesystem and atomic.
	if err := renameNoReplace(stage, prepared.destination); err != nil {
		return Result{}, fmt.Errorf("publish corpus atomically: %w", err)
	}
	keyCommitted = true
	return Result{Manifest: prepared.manifest, ManifestSHA256: hashBytes(manifestBytes), DiscoveryCases: expectedDiscovery, HeldOutCases: expectedHeldOut}, nil
}

func writePublication(stage string, prepared preparedCorpus, ciphertext []byte) error {
	if err := writeExclusive(filepath.Join(stage, ValidationFile), prepared.reportBytes, 0o644); err != nil {
		return err
	}
	if err := writeExclusive(filepath.Join(stage, HeldOutCipherFile), ciphertext, 0o644); err != nil {
		return err
	}
	authors := sortedKeys(prepared.authorship)
	for _, author := range authors {
		if err := writeExclusive(filepath.Join(stage, "authorship", author+".json"), prepared.authorship[author], 0o644); err != nil {
			return err
		}
	}
	paths := sortedKeys(prepared.discovery)
	for _, path := range paths {
		if err := writeExclusive(filepath.Join(stage, filepath.FromSlash(path)), prepared.discovery[path], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func marshalStable(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal publication artifact: %w", err)
	}
	return append(data, '\n'), nil
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func cloneMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneCounts(source map[string]map[string]int) map[string]map[string]int {
	result := make(map[string]map[string]int, len(source))
	for role, domains := range source {
		result[role] = make(map[string]int, len(domains))
		for domain, count := range domains {
			result[role][domain] = count
		}
	}
	return result
}

func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
