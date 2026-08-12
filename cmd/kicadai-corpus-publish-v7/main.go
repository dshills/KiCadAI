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
	"kicadai/internal/corpusfreezev7"
	"kicadai/internal/corpuspublication"
)

const (
	v7StartingCommit       = "156f7eb439ca5313471c504ddb91db1b8a8724f0"
	v7ContractFreezeCommit = "e780c8cfca51623d81b9eae209fedf2b98816681"
	v7AuthorPacketCommit   = "5f2b0c72b7ca7418b14a5a943306d5a596bd3716"
	v7ValidatorCommit      = "d7677432aab118303954ca6b55420ae98a5074ad"
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
	flags := flag.NewFlagSet("kicadai-corpus-publish-v7", flag.ContinueOnError)
	var flagOutput bytes.Buffer
	flags.SetOutput(&flagOutput)
	packetRoot := flags.String("packet-root", "", "frozen V7 authoring packet root")
	historyPath := flags.String("history", "", "sanitized V1-V6 commitment JSON")
	contractManifestPath := flags.String("contract-manifest", "", "frozen V7 contract checksum manifest")
	validatorManifestPath := flags.String("validator-manifest", "", "re-frozen V7 validator contract checksum manifest")
	publisherManifestPath := flags.String("publisher-manifest", "", "frozen V7 publisher checksum manifest")
	repositoryRoot := flags.String("repository-root", "", "repository root")
	destinationRoot := flags.String("destination", "", "new in-repository V7 corpus root")
	keyPath := flags.String("key-output", "", "new external 32-byte V7 held-out key path")
	startingCommit := flags.String("starting-commit", "", "immutable V7 starting commit")
	contractCommit := flags.String("contract-freeze-commit", "", "V7 contract freeze commit")
	packetCommit := flags.String("authoring-packet-commit", "", "V7 authoring packet commit")
	validatorCommit := flags.String("validator-commit", "", "V7 validator re-freeze commit")
	freezeParentCommit := flags.String("freeze-parent-commit", "", "parent of the V7 corpus publication commit")
	var bundleArguments bundleFlags
	flags.Var(&bundleArguments, "bundle", "isolated author bundle as AUTHOR_SLOT=PATH (repeat once per author)")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, copyErr := io.Copy(stdout, &flagOutput)
			return copyErr
		}
		if detail := strings.TrimSpace(flagOutput.String()); detail != "" {
			return errors.New(detail)
		}
		return fmt.Errorf("%w: %v", usageError(), err)
	}
	required := []struct {
		name  string
		value *string
	}{
		{"packet-root", packetRoot}, {"history", historyPath}, {"contract-manifest", contractManifestPath},
		{"validator-manifest", validatorManifestPath}, {"publisher-manifest", publisherManifestPath},
		{"repository-root", repositoryRoot}, {"destination", destinationRoot}, {"key-output", keyPath},
		{"starting-commit", startingCommit}, {"contract-freeze-commit", contractCommit},
		{"authoring-packet-commit", packetCommit}, {"validator-commit", validatorCommit},
		{"freeze-parent-commit", freezeParentCommit},
	}
	for _, item := range required {
		if strings.TrimSpace(*item.value) == "" {
			return fmt.Errorf("-%s is required: %w", item.name, usageError())
		}
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %w", usageError())
	}
	if err := verifyFrozenCommits(*startingCommit, *contractCommit, *packetCommit, *validatorCommit); err != nil {
		return err
	}

	policy := corpusfreezev7.Policy()
	bundlePaths, err := parseBundlePaths(bundleArguments, policy.AuthorSlots)
	if err != nil {
		return err
	}
	packet, err := corpusfreeze.LoadPacket(*packetRoot, policy)
	if err != nil {
		return err
	}
	historical, err := corpusfreezev7.LoadHistoricalCommitments(*historyPath)
	if err != nil {
		return err
	}
	if historical.Base.SourceSHA256 != policy.HistoricalCommitmentsSHA256 {
		return fmt.Errorf("historical commitment source does not match frozen V7 policy")
	}
	bundles := make(map[string]corpusfreeze.Bundle, len(policy.AuthorSlots))
	for _, author := range policy.AuthorSlots {
		bundle, err := corpusfreeze.LoadBundle(bundlePaths[author], packet.Assignments[author])
		if err != nil {
			return fmt.Errorf("load %s bundle: %w", author, err)
		}
		bundles[author] = bundle
	}
	report, err := corpusfreezev7.Validate(packet.Assignments, bundles, packet.Binding, historical, policy)
	if err != nil {
		return err
	}
	contractManifest, err := corpuspublication.VerifyV7RepositoryManifest(*repositoryRoot, *contractManifestPath)
	if err != nil {
		return fmt.Errorf("verify V7 contract manifest: %w", err)
	}
	validatorManifest, err := corpuspublication.VerifyV7RepositoryManifest(*repositoryRoot, *validatorManifestPath)
	if err != nil {
		return fmt.Errorf("verify V7 validator manifest: %w", err)
	}
	publisherManifest, err := corpuspublication.VerifyChecksumManifest(*repositoryRoot, *publisherManifestPath)
	if err != nil {
		return fmt.Errorf("verify V7 publisher manifest: %w", err)
	}
	result, err := corpuspublication.PublishV7(corpuspublication.Request{
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
	_, err = fmt.Fprintf(stdout, "published %d V7 behavior cases (%d discovery, %d held-out)\n", len(result.Manifest.Entries), result.DiscoveryCases, result.HeldOutCases)
	return err
}

func verifyFrozenCommits(starting, contract, packet, validator string) error {
	values := []struct {
		name string
		got  string
		want string
	}{
		{"starting", starting, v7StartingCommit},
		{"contract freeze", contract, v7ContractFreezeCommit},
		{"authoring packet", packet, v7AuthorPacketCommit},
		{"validator", validator, v7ValidatorCommit},
	}
	for _, value := range values {
		if value.got != value.want {
			return fmt.Errorf("%s commit %q does not match the frozen V7 boundary %q", value.name, value.got, value.want)
		}
	}
	return nil
}

func parseBundlePaths(arguments []string, authorSlots []string) (map[string]string, error) {
	want := make(map[string]bool, len(authorSlots))
	for _, author := range authorSlots {
		want[author] = true
	}
	result := make(map[string]string, len(authorSlots))
	for _, argument := range arguments {
		author, path, ok := strings.Cut(argument, "=")
		if !ok {
			return nil, fmt.Errorf("bundle %q must be AUTHOR_SLOT=PATH: %w", argument, usageError())
		}
		if !want[author] {
			return nil, fmt.Errorf("bundle author %q is not frozen: %w", author, usageError())
		}
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("bundle path for %s is empty: %w", author, usageError())
		}
		if result[author] != "" {
			return nil, fmt.Errorf("bundle for %s is duplicated: %w", author, usageError())
		}
		result[author] = path
	}
	for _, author := range authorSlots {
		if result[author] == "" {
			return nil, fmt.Errorf("bundle for %s is required: %w", author, usageError())
		}
	}
	return result, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func usageError() error {
	return errors.New("usage: kicadai-corpus-publish-v7 -packet-root PATH -history PATH -contract-manifest PATH -validator-manifest PATH -publisher-manifest PATH -repository-root PATH -destination PATH -key-output EXTERNAL_PATH -bundle author_1=PATH -bundle author_2=PATH -bundle author_3=PATH -starting-commit HASH -contract-freeze-commit HASH -authoring-packet-commit HASH -validator-commit HASH -freeze-parent-commit HASH")
}
