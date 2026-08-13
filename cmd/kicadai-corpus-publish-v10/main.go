package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev10"
	"kicadai/internal/corpuspublication"
)

const (
	v10StartingCommit       = "370f001daf0d912c8092af307e615b06b02060d0"
	v10ContractFreezeCommit = "2bc33a1857feb88011d53a0f5f405569468ae2d1"
	v10AuthorPacketCommit   = "5ade6d06ed493b5eeeeed12bfa574a41009bd892"
	v10ValidatorCommit      = "5cf9717250c71ff437b79ed576efd45eacd3cfe9"
)

type bundleFlags []string

func (values *bundleFlags) String() string         { return strings.Join(*values, ",") }
func (values *bundleFlags) Set(value string) error { *values = append(*values, value); return nil }

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("kicadai-corpus-publish-v10", flag.ContinueOnError)
	var flagOutput bytes.Buffer
	flags.SetOutput(&flagOutput)
	packetRoot := flags.String("packet-root", "", "frozen V10 author packet root")
	historyPath := flags.String("history", "", "digest-only V1-V9 historical commitments")
	contractManifestPath := flags.String("contract-manifest", "", "frozen V10 contract manifest")
	validatorManifestPath := flags.String("validator-manifest", "", "frozen V10 validator manifest")
	publisherManifestPath := flags.String("publisher-manifest", "", "frozen V10 publisher manifest")
	repositoryRoot := flags.String("repository-root", "", "repository root")
	destinationRoot := flags.String("destination", "", "new V10 corpus root")
	keyPath := flags.String("key-output", "", "new external V10 held-out source key")
	startingCommit := flags.String("starting-commit", "", "immutable V10 starting commit")
	contractCommit := flags.String("contract-freeze-commit", "", "V10 contract freeze commit")
	packetCommit := flags.String("authoring-packet-commit", "", "V10 author packet commit")
	validatorCommit := flags.String("validator-commit", "", "V10 validator commit")
	freezeParentCommit := flags.String("freeze-parent-commit", "", "parent of V10 corpus publication commit")
	var bundleArguments bundleFlags
	flags.Var(&bundleArguments, "bundle", "AUTHOR_SLOT=PATH (repeat for all six authors)")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, copyErr := io.Copy(stdout, &flagOutput)
			return copyErr
		}
		if detail := strings.TrimSpace(flagOutput.String()); detail != "" {
			return errors.New(detail)
		}
		return fmt.Errorf("%w: %v", usageError(flags), err)
	}
	required := []struct {
		name  string
		value *string
	}{{"packet-root", packetRoot}, {"history", historyPath}, {"contract-manifest", contractManifestPath},
		{"validator-manifest", validatorManifestPath}, {"publisher-manifest", publisherManifestPath}, {"repository-root", repositoryRoot},
		{"destination", destinationRoot}, {"key-output", keyPath}, {"starting-commit", startingCommit}, {"contract-freeze-commit", contractCommit},
		{"authoring-packet-commit", packetCommit}, {"validator-commit", validatorCommit}, {"freeze-parent-commit", freezeParentCommit}}
	for _, item := range required {
		if strings.TrimSpace(*item.value) == "" {
			return fmt.Errorf("-%s is required: %w", item.name, usageError(flags))
		}
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %w", usageError(flags))
	}
	if err := verifyFrozenCommits(*startingCommit, *contractCommit, *packetCommit, *validatorCommit); err != nil {
		return err
	}
	historical, err := corpusfreezev10.LoadHistoricalCommitments(*historyPath)
	if err != nil {
		return err
	}
	if err := corpusfreezev10.ValidateHistoricalBoundary(historical); err != nil {
		return err
	}
	policy := corpusfreezev10.PolicyForHistory(historical.Base.SourceSHA256)
	bundlePaths, err := parseBundlePaths(bundleArguments, policy.AuthorSlots, usageError(flags))
	if err != nil {
		return err
	}
	packet, err := corpusfreezev10.LoadPacket(*packetRoot, policy)
	if err != nil {
		return err
	}
	bundles := make(map[string]corpusfreeze.Bundle, len(policy.AuthorSlots))
	for _, author := range policy.AuthorSlots {
		bundle, err := corpusfreezev10.LoadBundle(bundlePaths[author], packet.Assignments[author])
		if err != nil {
			return fmt.Errorf("load %s bundle: %w", author, err)
		}
		bundles[author] = bundle
	}
	report, err := corpusfreezev10.Validate(packet.Assignments, bundles, packet.Binding, historical, policy)
	if err != nil {
		return err
	}
	contractManifest, err := corpuspublication.VerifyV6ContractManifest(*repositoryRoot, *contractManifestPath)
	if err != nil {
		return fmt.Errorf("verify V10 contract manifest: %w", err)
	}
	validatorManifest, err := corpuspublication.VerifyChecksumManifest(*repositoryRoot, *validatorManifestPath)
	if err != nil {
		return fmt.Errorf("verify V10 validator manifest: %w", err)
	}
	publisherManifest, err := corpuspublication.VerifyChecksumManifest(*repositoryRoot, *publisherManifestPath)
	if err != nil {
		return fmt.Errorf("verify V10 publisher manifest: %w", err)
	}
	result, err := corpuspublication.PublishV10(corpuspublication.RequestV10{
		RepositoryRoot: *repositoryRoot, DestinationRoot: *destinationRoot, KeyPath: *keyPath,
		ContractManifestSHA256: digest(contractManifest), ValidatorManifest: validatorManifest, PublisherManifest: publisherManifest,
		Commits: corpuspublication.Commits{StartingCommit: *startingCommit, ContractFreezeCommit: *contractCommit, AuthoringPacketCommit: *packetCommit, ValidatorCommit: *validatorCommit, FreezeParentCommit: *freezeParentCommit},
		Report:  report, Bundles: bundles,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "published %d V10 behavior cases (%d discovery, %d held-out; %d/%d obligations)\n",
		result.DiscoveryCases+result.HeldOutCases, result.DiscoveryCases, result.HeldOutCases, result.DiscoveryObligations, result.HeldOutObligations)
	return err
}

func verifyFrozenCommits(starting, contract, packet, validator string) error {
	values := []struct{ name, got, want string }{
		{"starting", starting, v10StartingCommit},
		{"contract freeze", contract, v10ContractFreezeCommit},
		{"authoring packet", packet, v10AuthorPacketCommit},
		{"validator", validator, v10ValidatorCommit},
	}
	for _, value := range values {
		if value.got != value.want {
			return fmt.Errorf("%s commit %q does not match frozen V10 boundary %q", value.name, value.got, value.want)
		}
	}
	return nil
}

func parseBundlePaths(arguments []string, authors []string, usage error) (map[string]string, error) {
	want := map[string]bool{}
	for _, author := range authors {
		want[author] = true
	}
	result := map[string]string{}
	for _, argument := range arguments {
		author, path, ok := strings.Cut(argument, "=")
		if !ok {
			return nil, fmt.Errorf("bundle must be AUTHOR_SLOT=PATH: %w", usage)
		}
		if !want[author] {
			return nil, fmt.Errorf("bundle author %q is not frozen: %w", author, usage)
		}
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("bundle path for %s is empty: %w", author, usage)
		}
		if result[author] != "" {
			return nil, fmt.Errorf("bundle for %s is duplicated: %w", author, usage)
		}
		result[author] = path
	}
	for _, author := range authors {
		if result[author] == "" {
			return nil, fmt.Errorf("bundle for %s is required: %w", author, usage)
		}
	}
	return result, nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func usageError(flags *flag.FlagSet) error {
	var defaults bytes.Buffer
	output := flags.Output()
	flags.SetOutput(&defaults)
	flags.PrintDefaults()
	flags.SetOutput(output)
	return errors.New("usage: " + flags.Name() + " [options]\n" + strings.TrimSpace(defaults.String()))
}
