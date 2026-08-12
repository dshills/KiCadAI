package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev7"
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
	flags := flag.NewFlagSet("kicadai-corpus-validate-v7", flag.ContinueOnError)
	flags.SetOutput(stdout)
	packetRoot := flags.String("packet-root", "", "frozen V7 public authoring packet root")
	historyPath := flags.String("history", "", "sanitized V1-V6 commitment JSON")
	outputPath := flags.String("output", "", "aggregate V7 validation report JSON")
	var bundleArguments bundleFlags
	flags.Var(&bundleArguments, "bundle", "isolated author bundle as AUTHOR_SLOT=PATH (repeat once per author)")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("%w: %v", usageError(), err)
	}
	if flags.NArg() != 0 || *packetRoot == "" || *historyPath == "" || *outputPath == "" {
		return usageError()
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
	if err := corpusfreeze.WriteReport(*outputPath, report); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "validated %d V7 behavior cases across %d isolated authors\n", len(report.Entries), len(policy.AuthorSlots))
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
		if !ok {
			return nil, fmt.Errorf("%w: bundle must use author=path", usageError())
		}
		if !want[author] {
			return nil, fmt.Errorf("%w: unknown bundle author %q", usageError(), author)
		}
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("%w: empty bundle path for %s", usageError(), author)
		}
		if result[author] != "" {
			return nil, fmt.Errorf("%w: duplicate bundle for %s", usageError(), author)
		}
		result[author] = path
	}
	if len(result) != len(want) {
		return nil, fmt.Errorf("%w: require exactly one bundle for every author", usageError())
	}
	return result, nil
}

func usageError() error {
	return fmt.Errorf("usage: kicadai-corpus-validate-v7 -packet-root PATH -history PATH -bundle author_1=PATH -bundle author_2=PATH -bundle author_3=PATH -output PATH")
}
