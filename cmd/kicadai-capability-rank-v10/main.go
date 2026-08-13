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
	"reflect"
	"strings"

	"kicadai/internal/capabilitybaselinepublicationv10"
	"kicadai/internal/capabilityselectionv10"
)

type options struct {
	repositoryRoot string
	baselineRoot   string
	planSetPath    string
	outputPath     string
}

type summary struct {
	Schema                  string `json:"schema"`
	Version                 int    `json:"version"`
	RankingPath             string `json:"ranking_path"`
	RankingSHA256           string `json:"ranking_sha256"`
	EligibleCandidateCount  int    `json:"eligible_candidate_count"`
	CoRankOneCount          int    `json:"co_rank_one_count"`
	SelectedCapabilityAtoms int    `json:"selected_capability_atoms"`
	ClaimedUnlocks          int    `json:"claimed_unlocks"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("kicadai-capability-rank-v10", flag.ContinueOnError)
	var diagnostics bytes.Buffer
	flags.SetOutput(&diagnostics)
	var opts options
	flags.StringVar(&opts.repositoryRoot, "repository-root", ".", "clean committed repository root")
	flags.StringVar(&opts.baselineRoot, "baseline-root", "", "authenticated V10 public baseline publication")
	flags.StringVar(&opts.planSetPath, "plans", "", "canonical mechanically evidenced V10 effect-plan set")
	flags.StringVar(&opts.outputPath, "output", "", "fresh path for the deterministic ranking")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse V10 capability-rank arguments: %s", strings.TrimSpace(diagnostics.String()))
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("parse V10 capability-rank arguments: unexpected positional arguments")
	}
	if err := normalize(&opts); err != nil {
		return err
	}
	if err := requireCleanRepository(ctx, opts.repositoryRoot); err != nil {
		return err
	}
	baseline, err := capabilitybaselinepublicationv10.Verify(opts.baselineRoot)
	if err != nil {
		return fmt.Errorf("authenticate V10 public baseline: %w", err)
	}
	planBytes, err := capabilityselectionv10.ReadRegularFile(opts.planSetPath)
	if err != nil {
		return fmt.Errorf("read V10 effect plans: %w", err)
	}
	plans, planHash, err := capabilityselectionv10.DecodePlanSet(planBytes)
	if err != nil {
		return err
	}
	ranking, err := capabilityselectionv10.Build(baseline, plans, planHash, opts.repositoryRoot)
	if err != nil {
		return err
	}
	baselineAfter, err := capabilitybaselinepublicationv10.Verify(opts.baselineRoot)
	if err != nil || !reflect.DeepEqual(baseline, baselineAfter) {
		return fmt.Errorf("V10 public baseline changed during selection")
	}
	if err := capabilityselectionv10.Validate(ranking, baselineAfter, plans, planHash, opts.repositoryRoot); err != nil {
		return fmt.Errorf("revalidate V10 ranking inputs: %w", err)
	}
	if err := requireCleanRepository(ctx, opts.repositoryRoot); err != nil {
		return err
	}
	data, err := capabilityselectionv10.MarshalRanking(ranking)
	if err != nil {
		return err
	}
	if err := capabilityselectionv10.WriteAtomicNoReplace(opts.outputPath, data); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(summary{
		Schema: "kicadai.closed-loop-open-set-ranking-run.v10", Version: 10,
		RankingPath: opts.outputPath, RankingSHA256: ranking.Hash,
		EligibleCandidateCount:  len(ranking.Selection.EligibleCandidates),
		CoRankOneCount:          len(ranking.Selection.CoRankOne),
		SelectedCapabilityAtoms: len(ranking.Selection.Selected.Atoms),
		ClaimedUnlocks:          len(ranking.Selection.Selected.FullyCoveredCaseIDs),
	})
}

func normalize(opts *options) error {
	root, err := filepath.Abs(opts.repositoryRoot)
	if err != nil {
		return err
	}
	opts.repositoryRoot = root
	if strings.TrimSpace(opts.baselineRoot) == "" || strings.TrimSpace(opts.planSetPath) == "" || strings.TrimSpace(opts.outputPath) == "" {
		return fmt.Errorf("--baseline-root, --plans, and --output are required")
	}
	for _, target := range []*string{&opts.baselineRoot, &opts.planSetPath, &opts.outputPath} {
		absolute, err := filepath.Abs(*target)
		if err != nil {
			return err
		}
		*target = absolute
	}
	return nil
}

func requireCleanRepository(ctx context.Context, root string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("V10 selection requires the git executable in PATH: %w", err)
	}
	top, err := exec.CommandContext(ctx, gitPath, "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("V10 selection requires the repository root")
	}
	rootInfo, rootErr := os.Stat(root)
	topInfo, topErr := os.Stat(strings.TrimSpace(string(top)))
	if rootErr != nil || topErr != nil || !rootInfo.IsDir() || !topInfo.IsDir() || !os.SameFile(rootInfo, topInfo) {
		return fmt.Errorf("V10 selection requires the repository root")
	}
	status, err := exec.CommandContext(ctx, gitPath, "-C", root, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil || len(bytes.TrimSpace(status)) != 0 {
		return fmt.Errorf("V10 selection requires a clean committed repository")
	}
	return nil
}
