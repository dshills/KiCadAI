package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpuspublication"
)

type bundleFlags []string

func (values *bundleFlags) String() string { return strings.Join(*values, ",") }

func (values *bundleFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("kicadai-corpus-publish", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	packetRoot := flags.String("packet-root", "", "frozen public authoring packet root")
	historyPath := flags.String("history", "", "sanitized historical commitment JSON")
	contractManifestPath := flags.String("contract-manifest", "", "frozen V5 contract checksum manifest")
	validatorManifestPath := flags.String("validator-manifest", "", "frozen V5 validator checksum manifest")
	publisherManifestPath := flags.String("publisher-manifest", "", "frozen V5 publisher checksum manifest")
	repositoryRoot := flags.String("repository-root", "", "repository root")
	destinationRoot := flags.String("destination", "", "new in-repository corpus root")
	keyPath := flags.String("key-output", "", "new external 32-byte held-out key path")
	startingCommit := flags.String("starting-commit", "", "immutable V5 starting commit")
	contractCommit := flags.String("contract-freeze-commit", "", "V5 contract freeze commit")
	packetCommit := flags.String("authoring-packet-commit", "", "V5 authoring packet commit")
	validatorCommit := flags.String("validator-commit", "", "V5 validator commit")
	freezeParentCommit := flags.String("freeze-parent-commit", "", "parent of the corpus publication commit")
	var bundleArguments bundleFlags
	flags.Var(&bundleArguments, "bundle", "isolated author bundle as AUTHOR_SLOT=PATH (repeat once per author)")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("%w: %v", usageError(), err)
	}
	required := []*string{packetRoot, historyPath, contractManifestPath, validatorManifestPath, publisherManifestPath, repositoryRoot, destinationRoot, keyPath, startingCommit, contractCommit, packetCommit, validatorCommit, freezeParentCommit}
	for _, value := range required {
		if strings.TrimSpace(*value) == "" {
			return usageError()
		}
	}
	if flags.NArg() != 0 {
		return usageError()
	}

	policy := corpusfreeze.V5Policy()
	bundlePaths, err := parseBundlePaths(bundleArguments, policy.AuthorSlots)
	if err != nil {
		return err
	}
	packet, err := corpusfreeze.LoadPacket(*packetRoot, policy)
	if err != nil {
		return err
	}
	historical, err := corpusfreeze.LoadHistoricalCommitments(*historyPath)
	if err != nil {
		return err
	}
	if historical.SourceSHA256 != policy.HistoricalCommitmentsSHA256 {
		return fmt.Errorf("historical commitment source does not match frozen policy")
	}
	bundles := make(map[string]corpusfreeze.Bundle, len(policy.AuthorSlots))
	for _, author := range policy.AuthorSlots {
		bundle, err := corpusfreeze.LoadBundle(bundlePaths[author], packet.Assignments[author])
		if err != nil {
			return fmt.Errorf("load %s bundle: %w", author, err)
		}
		bundles[author] = bundle
	}
	report, err := corpusfreeze.Validate(packet.Assignments, bundles, packet.Binding, historical, policy)
	if err != nil {
		return err
	}
	contractManifest, err := corpuspublication.VerifyChecksumManifest(filepath.Dir(*contractManifestPath), *contractManifestPath)
	if err != nil {
		return fmt.Errorf("verify contract manifest: %w", err)
	}
	validatorManifest, err := corpuspublication.VerifyChecksumManifest(*repositoryRoot, *validatorManifestPath)
	if err != nil {
		return fmt.Errorf("verify validator manifest: %w", err)
	}
	publisherManifest, err := corpuspublication.VerifyChecksumManifest(*repositoryRoot, *publisherManifestPath)
	if err != nil {
		return fmt.Errorf("verify publisher manifest: %w", err)
	}
	result, err := corpuspublication.Publish(corpuspublication.Request{
		RepositoryRoot: *repositoryRoot, DestinationRoot: *destinationRoot, KeyPath: *keyPath,
		ContractManifestSHA256: digest(contractManifest), ValidatorManifest: validatorManifest, PublisherManifest: publisherManifest,
		Commits: corpuspublication.Commits{
			StartingCommit: *startingCommit, ContractFreezeCommit: *contractCommit,
			AuthoringPacketCommit: *packetCommit, ValidatorCommit: *validatorCommit,
			FreezeParentCommit: *freezeParentCommit,
		},
		Report: report, Bundles: bundles,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "published %d behavior cases (%d discovery, %d held-out)\n", len(result.Manifest.Entries), result.DiscoveryCases, result.HeldOutCases)
	return err
}

func parseBundlePaths(arguments []string, authorSlots []string) (map[string]string, error) {
	want := make(map[string]bool, len(authorSlots))
	for _, author := range authorSlots {
		want[author] = true
	}
	result := make(map[string]string, len(authorSlots))
	for _, argument := range arguments {
		author, path, ok := strings.Cut(argument, "=")
		if !ok || !want[author] || strings.TrimSpace(path) == "" || result[author] != "" {
			return nil, usageError()
		}
		result[author] = path
	}
	if len(result) != len(want) {
		return nil, usageError()
	}
	return result, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func usageError() error {
	return fmt.Errorf("usage: kicadai-corpus-publish -packet-root PATH -history PATH -contract-manifest PATH -validator-manifest PATH -publisher-manifest PATH -repository-root PATH -destination PATH -key-output EXTERNAL_PATH -bundle author_1=PATH -bundle author_2=PATH -bundle author_3=PATH -starting-commit HASH -contract-freeze-commit HASH -authoring-packet-commit HASH -validator-commit HASH -freeze-parent-commit HASH")
}
