package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kicadai/internal/capabilityadvancementv10"
	"kicadai/internal/capabilitybaselinepublicationv10"
	"kicadai/internal/capabilityselectionv10"
)

type options struct {
	repositoryRoot string
	baselineRoot   string
	nextReport     string
	rankingPath    string
	planSetPath    string
	sealRequest    string
	outputRoot     string
}

type summary struct {
	Schema                 string `json:"schema"`
	Version                int    `json:"version"`
	OutputRoot             string `json:"output_root"`
	Status                 string `json:"status"`
	PassBefore             int    `json:"pass_before"`
	PassAfter              int    `json:"pass_after"`
	NewActiveCohortPasses  int    `json:"new_active_cohort_passes"`
	AdvancedCaseCount      int    `json:"advanced_case_count"`
	ImplementationSealHash string `json:"implementation_seal_sha256"`
	RoundHash              string `json:"round_sha256"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("kicadai-capability-advance-v10", flag.ContinueOnError)
	var diagnostics bytes.Buffer
	flags.SetOutput(&diagnostics)
	var opts options
	flags.StringVar(&opts.repositoryRoot, "repository-root", ".", "clean committed implementation repository root")
	flags.StringVar(&opts.baselineRoot, "baseline-root", "", "authenticated generation-zero public baseline")
	flags.StringVar(&opts.nextReport, "next-report", "", "canonical committed-seal successor evaluator report")
	flags.StringVar(&opts.rankingPath, "ranking", "", "frozen generation-zero ranking")
	flags.StringVar(&opts.planSetPath, "plans", "", "frozen generation-zero effect-plan set")
	flags.StringVar(&opts.sealRequest, "seal-request", "", "reviewed implementation-seal request")
	flags.StringVar(&opts.outputRoot, "output-root", "", "fresh atomic public-round destination")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse V10 advancement arguments: %s", strings.TrimSpace(diagnostics.String()))
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("parse V10 advancement arguments: unexpected positional arguments")
	}
	if err := normalize(&opts); err != nil {
		return err
	}
	if err := requireCleanRepository(ctx, opts.repositoryRoot); err != nil {
		return err
	}
	baseline, err := capabilitybaselinepublicationv10.Verify(opts.baselineRoot)
	if err != nil {
		return fmt.Errorf("authenticate V10 baseline: %w", err)
	}
	nextBytes, err := read(opts.nextReport)
	if err != nil {
		return err
	}
	next, err := capabilityadvancementv10.DecodeReport(nextBytes)
	if err != nil {
		return err
	}
	rankingBytes, err := read(opts.rankingPath)
	if err != nil {
		return err
	}
	ranking, err := capabilityadvancementv10.DecodeRanking(rankingBytes)
	if err != nil {
		return err
	}
	planBytes, err := read(opts.planSetPath)
	if err != nil {
		return err
	}
	plans, planHash, err := capabilityselectionv10.DecodePlanSet(planBytes)
	if err != nil {
		return err
	}
	sealRequestBytes, err := read(opts.sealRequest)
	if err != nil {
		return err
	}
	request, err := capabilityadvancementv10.DecodeSealRequest(sealRequestBytes)
	if err != nil {
		return err
	}
	seal, err := capabilityadvancementv10.BuildSeal(opts.repositoryRoot, ranking, plans, planHash, request)
	if err != nil {
		return err
	}
	round, err := capabilityadvancementv10.BuildRound(baseline, next, ranking, seal)
	if err != nil {
		return err
	}
	if err := requireCleanRepository(ctx, opts.repositoryRoot); err != nil {
		return err
	}
	if err := capabilityadvancementv10.Publish(opts.outputRoot, seal, round); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(summary{Schema: "kicadai.closed-loop-open-set-advancement-run.v10", Version: 10, OutputRoot: opts.outputRoot,
		Status: string(round.Evaluation.Status), PassBefore: round.Evaluation.DiscoveryPassBefore, PassAfter: round.Evaluation.DiscoveryPassAfter,
		NewActiveCohortPasses: round.Evaluation.NewActiveCohortPasses, AdvancedCaseCount: len(round.Evaluation.AdvancedCaseIDs),
		ImplementationSealHash: seal.Hash, RoundHash: round.Hash})
}

func normalize(opts *options) error {
	root, err := filepath.Abs(opts.repositoryRoot)
	if err != nil {
		return err
	}
	opts.repositoryRoot = root
	values := []*string{&opts.baselineRoot, &opts.nextReport, &opts.rankingPath, &opts.planSetPath, &opts.sealRequest, &opts.outputRoot}
	for _, value := range values {
		if strings.TrimSpace(*value) == "" {
			return fmt.Errorf("--baseline-root, --next-report, --ranking, --plans, --seal-request, and --output-root are required")
		}
		absolute, err := filepath.Abs(*value)
		if err != nil {
			return err
		}
		*value = absolute
	}
	return nil
}

func read(filename string) ([]byte, error) { return capabilityselectionv10.ReadRegularFile(filename) }

func requireCleanRepository(ctx context.Context, root string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("V10 advancement requires git in PATH: %w", err)
	}
	top, err := exec.CommandContext(ctx, gitPath, "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("V10 advancement requires repository root")
	}
	rootInfo, rootErr := os.Stat(root)
	topInfo, topErr := os.Stat(strings.TrimSpace(string(top)))
	if rootErr != nil || topErr != nil || !os.SameFile(rootInfo, topInfo) {
		return fmt.Errorf("V10 advancement requires repository root")
	}
	status, err := exec.CommandContext(ctx, gitPath, "-C", root, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil || len(bytes.TrimSpace(status)) != 0 {
		return fmt.Errorf("V10 advancement requires a clean committed repository")
	}
	return nil
}
