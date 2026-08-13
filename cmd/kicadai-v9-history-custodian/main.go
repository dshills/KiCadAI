package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/corpushistoryv9"
	"kicadai/internal/corpuspublication"
	"kicadai/internal/externalkey"
)

const maximumCustodianFileBytes = 32 << 20

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("kicadai-v9-history-custodian", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repositoryRoot := flags.String("repository-root", "", "repository root")
	corpusRoot := flags.String("v8-corpus-root", "", "authenticated published V8 corpus root")
	sourceKey := flags.String("v8-source-key", "", "external 0600 V8 source key")
	predecessor := flags.String("predecessor-history", "", "byte-frozen V1-V7 commitment JSON")
	output := flags.String("output", "", "exclusive V1-V8 commitment JSON destination")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("%w: %v", usageError(), err)
	}
	if flags.NArg() != 0 || *repositoryRoot == "" || *corpusRoot == "" || *sourceKey == "" || *predecessor == "" || *output == "" {
		return usageError()
	}

	repository, err := realDirectory(*repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	corpus, err := realDirectory(*corpusRoot)
	if err != nil || !pathWithin(repository, corpus) {
		return fmt.Errorf("V8 corpus root must be a real repository directory")
	}
	predecessorPath, err := realRegularFile(*predecessor)
	if err != nil || !pathWithin(repository, predecessorPath) {
		return fmt.Errorf("predecessor history must be a real repository file")
	}
	outputPath, err := futureRegularPath(*output)
	if err != nil || !pathWithin(repository, outputPath) {
		return fmt.Errorf("output must be a new repository file")
	}
	checksumData, err := corpuspublication.VerifyChecksumManifest(corpus, filepath.Join(corpus, corpuspublication.ChecksumFileV8))
	if err != nil {
		return fmt.Errorf("verify V8 corpus checksums: %w", err)
	}

	manifestData, err := readBoundedRegular(filepath.Join(corpus, corpuspublication.ManifestFileV8), 1<<20)
	if err != nil {
		return err
	}
	var manifest corpuspublication.ManifestV8
	decoder := json.NewDecoder(bytes.NewReader(manifestData))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode V8 corpus manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("decode V8 corpus manifest: trailing JSON value")
	}
	if manifest.Schema != corpuspublication.ManifestSchemaV8 || manifest.Version != corpuspublication.ManifestVersionV8 ||
		manifest.DiscoveryCaseCount != 18 || manifest.HeldOutCaseCount != 18 || len(manifest.Entries) != 18 {
		return fmt.Errorf("V8 corpus manifest boundary is invalid")
	}
	if err := verifyV8CorpusSet(corpus, checksumData, manifest); err != nil {
		return err
	}
	ciphertext, err := readBoundedRegular(filepath.Join(corpus, corpuspublication.HeldOutCipherFileV8), maximumCustodianFileBytes)
	if err != nil {
		return err
	}
	previous, err := readBoundedRegular(predecessorPath, 4<<20)
	if err != nil {
		return err
	}

	key, err := externalkey.Read(repository, *sourceKey)
	if err != nil {
		return err
	}
	defer clear(key)
	heldOut, err := corpuspublication.OpenHeldOutCommitmentsV8(key, manifest, ciphertext)
	if err != nil {
		return fmt.Errorf("authenticate V8 held-out commitments: %w", err)
	}
	entries, err := combineCommitments(manifest.Entries, heldOut)
	if err != nil {
		return err
	}
	data, err := corpushistoryv9.ExtendHistoricalCommitments(previous, entries)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	digestText := hex.EncodeToString(digest[:])
	if err := publishExclusive(outputPath, data, 0o644, func(stage string) error {
		loaded, err := corpushistoryv9.LoadHistoricalCommitments(stage)
		if err != nil {
			return err
		}
		if err := corpushistoryv9.ValidateHistoricalBoundary(loaded); err != nil {
			return err
		}
		if loaded.Base.SourceSHA256 != digestText {
			return fmt.Errorf("staged V9 history digest mismatch")
		}
		return nil
	}); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "published V9 historical commitments sha256=%s raw=%d neutral=%d normalized=%d\n",
		digestText, corpushistoryv9.HistoricalRawCount, corpushistoryv9.HistoricalNeutralCount, corpushistoryv9.HistoricalNormalizedCount)
	return err
}

func verifyV8CorpusSet(root string, checksums []byte, manifest corpuspublication.ManifestV8) error {
	wantChecksums := map[string]bool{
		corpuspublication.AuditFileV8: true, corpuspublication.DiscoveryObligationsFileV8: true,
		corpuspublication.HeldOutCommitmentFileV8: true, corpuspublication.HeldOutCipherFileV8: true,
		corpuspublication.ManifestFileV8: true, corpuspublication.ValidationFileV8: true,
	}
	wantDiscovery := map[string]bool{}
	for _, entry := range manifest.Entries {
		if entry.Role != "discovery" || entry.Sealed || !strings.HasPrefix(entry.StablePath, "discovery/") {
			return fmt.Errorf("V8 public entry mapping is invalid")
		}
		wantChecksums[entry.StablePath] = true
		wantDiscovery[filepath.Base(entry.StablePath)] = true
	}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(checksums))
	for scanner.Scan() {
		_, name, ok := strings.Cut(scanner.Text(), "  ")
		if !ok || !wantChecksums[name] || seen[name] {
			return fmt.Errorf("V8 checksum set is noncanonical")
		}
		seen[name] = true
	}
	if err := scanner.Err(); err != nil || len(seen) != len(wantChecksums) {
		return fmt.Errorf("V8 checksum set is incomplete")
	}

	rootEntries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	wantRoot := map[string]bool{corpuspublication.ChecksumFileV8: true, "discovery": true}
	for name := range wantChecksums {
		if !strings.Contains(name, "/") {
			wantRoot[name] = true
		}
	}
	if len(rootEntries) != len(wantRoot) {
		return fmt.Errorf("V8 corpus root file set is noncanonical")
	}
	for _, entry := range rootEntries {
		if !wantRoot[entry.Name()] || (entry.Name() == "discovery") != entry.IsDir() {
			return fmt.Errorf("V8 corpus root file set is noncanonical")
		}
		if entry.Name() != "discovery" {
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("V8 corpus root contains a non-regular artifact")
			}
		}
	}
	discoveryEntries, err := os.ReadDir(filepath.Join(root, "discovery"))
	if err != nil || len(discoveryEntries) != len(wantDiscovery) {
		return fmt.Errorf("V8 discovery file set is noncanonical")
	}
	for _, entry := range discoveryEntries {
		info, err := entry.Info()
		if err != nil || !wantDiscovery[entry.Name()] || !info.Mode().IsRegular() {
			return fmt.Errorf("V8 discovery file set is noncanonical")
		}
	}
	return nil
}

func combineCommitments(discovery, heldOut []corpuspublication.EntryV8) ([]corpushistoryv9.CommitmentEntry, error) {
	if len(discovery) != 18 || len(heldOut) != 18 {
		return nil, fmt.Errorf("V8 commitment partitions must contain 18 entries each")
	}
	all := make([]corpuspublication.EntryV8, 0, 36)
	for _, entry := range discovery {
		if entry.Role != "discovery" || entry.Sealed {
			return nil, fmt.Errorf("V8 discovery commitment metadata is invalid")
		}
		all = append(all, entry)
	}
	for _, entry := range heldOut {
		if entry.Role != "held_out" || !entry.Sealed {
			return nil, fmt.Errorf("V8 held-out commitment metadata is invalid")
		}
		all = append(all, entry)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].SourceID < all[j].SourceID })
	result := make([]corpushistoryv9.CommitmentEntry, len(all))
	for index, entry := range all {
		result[index] = corpushistoryv9.CommitmentEntry{SourceID: entry.SourceID, RequirementSHA256: entry.RequirementSHA256,
			NeutralSemanticSHA256: entry.NeutralSemanticSHA256, NormalizedSemanticSHA256: entry.NormalizedSemanticSHA256}
	}
	return result, nil
}

func realDirectory(path string) (string, error) {
	resolved, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("path is not a real directory")
	}
	return resolved, nil
}

func realRegularFile(path string) (string, error) {
	resolved, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("path is not a real regular file")
	}
	return resolved, nil
}

func futureRegularPath(path string) (string, error) {
	resolved, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(resolved))
	if err != nil {
		return "", err
	}
	resolved = filepath.Join(parent, filepath.Base(resolved))
	if _, err := os.Lstat(resolved); err == nil || !os.IsNotExist(err) {
		return "", fmt.Errorf("destination already exists or is unavailable")
	}
	return resolved, nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func readBoundedRegular(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("path is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("file changed during open")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, fmt.Errorf("read bounded file: %w", err)
	}
	return data, nil
}

func publishExclusive(path string, data []byte, mode os.FileMode, validate func(string) error) error {
	stage, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".staging-")
	if err != nil {
		return fmt.Errorf("create history staging file: %w", err)
	}
	committed := false
	defer func() {
		_ = stage.Close()
		if !committed {
			_ = os.Remove(stage.Name())
		}
	}()
	if err := stage.Chmod(mode); err != nil {
		return err
	}
	if _, err := stage.Write(data); err != nil {
		return err
	}
	if err := stage.Sync(); err != nil {
		return err
	}
	if err := stage.Close(); err != nil {
		return err
	}
	if validate == nil {
		return fmt.Errorf("history staging validator is required")
	}
	if err := validate(stage.Name()); err != nil {
		return fmt.Errorf("validate staged history: %w", err)
	}
	if err := os.Link(stage.Name(), path); err != nil {
		return fmt.Errorf("commit history without replacement: %w", err)
	}
	committed = true
	if err := os.Remove(stage.Name()); err != nil {
		return fmt.Errorf("remove committed history staging link: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func usageError() error {
	return fmt.Errorf("usage: kicadai-v9-history-custodian -repository-root PATH -v8-corpus-root PATH -v8-source-key PATH -predecessor-history PATH -output PATH")
}
