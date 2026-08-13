package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev9"
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
	flags := flag.NewFlagSet("kicadai-corpus-validate-v9", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	packetRoot := flags.String("packet-root", "", "frozen V9 public authoring packet root")
	historyPath := flags.String("history", "", "sanitized V1-V8 commitment JSON")
	outputPath := flags.String("output", "", "aggregate V9 validation report JSON")
	var bundleArguments bundleFlags
	flags.Var(&bundleArguments, "bundle", "isolated author bundle as AUTHOR_SLOT=PATH (repeat once per author)")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("%w: %v", usageError(), err)
	}
	if flags.NArg() != 0 || *packetRoot == "" || *historyPath == "" || *outputPath == "" {
		return usageError()
	}

	historical, err := corpusfreezev9.LoadHistoricalCommitments(*historyPath)
	if err != nil {
		return err
	}
	if err := corpusfreezev9.ValidateHistoricalBoundary(historical); err != nil {
		return err
	}
	policy := corpusfreezev9.PolicyForHistory(historical.Base.SourceSHA256)
	bundlePaths, err := parseBundlePaths(bundleArguments, policy.AuthorSlots)
	if err != nil {
		return err
	}
	packet, err := corpusfreezev9.LoadPacket(*packetRoot, policy)
	if err != nil {
		return err
	}
	bundles := make(map[string]corpusfreeze.Bundle, len(policy.AuthorSlots))
	for _, author := range policy.AuthorSlots {
		bundle, err := corpusfreezev9.LoadBundle(bundlePaths[author], packet.Assignments[author])
		if err != nil {
			return fmt.Errorf("load %s bundle: %w", author, err)
		}
		bundles[author] = bundle
	}
	report, err := corpusfreezev9.Validate(packet.Assignments, bundles, packet.Binding, historical, policy)
	if err != nil {
		return err
	}
	if err := corpusfreezev9.WriteReport(*outputPath, report); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "validated %d V9 behavior cases across %d isolated authors\n", len(report.Entries), len(policy.AuthorSlots))
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
	return fmt.Errorf("usage: kicadai-corpus-validate-v9 -packet-root PATH -history PATH -bundle author_1=PATH ... -bundle author_6=PATH -output PATH")
}
