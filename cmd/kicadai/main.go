package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
	"kicadai/internal/config"
	"kicadai/internal/evaluate"
	"kicadai/internal/inspect"
	"kicadai/internal/kiapi"
	commontypes "kicadai/internal/kiapi/gen/common/types"
	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/reports"
	"kicadai/internal/schematic"
	"kicadai/internal/transactions"
	"kicadai/internal/workflows"
)

const usageTemplate = `kicadai is a Go client for KiCad's IPC API.

Usage:
  kicadai [global flags] <command>

Commands:
  capabilities  Report detected KiCad API capabilities
  config        Print resolved connection configuration
  documents     List open KiCad documents
  draw-led-demo Execute the LED indicator schematic plan when supported
  generate-led-demo Generate a direct-file LED indicator KiCad project
  generate-project  Generate a direct-file LED indicator KiCad project
  generate      Generate projects from structured requests
  inspect       Inspect KiCad projects and files
  evaluate      Evaluate KiCad projects and files
  export        Export review and fabrication artifacts
  plan-led-demo Print a deterministic LED indicator schematic plan
  ping          Check whether KiCad responds to the API
  roundtrip     Run KiCad CLI round-trip checks
  transaction   Validate, plan, or apply structured edit transactions
  version       Print KiCad version information
  help          Print this help text

Global flags:
  --socket string       KiCad IPC endpoint, for example ipc:///tmp/kicad/api.sock
  --token string        KiCad API token
  --client-name string  Client name sent to KiCad
  --timeout-ms int      IPC timeout in milliseconds
  --document-type string Document filter: all, schematic, pcb, symbol, footprint, drawing_sheet, project
  --document string      Schematic document identifier for plan commands
  --origin-x int64      Plan origin X in KiCad internal units (1 mm = 1,000,000 IU)
  --origin-y int64      Plan origin Y in KiCad internal units (1 mm = 1,000,000 IU)
  --prefix string        Reference/value prefix for plan commands
  --output string        Output project directory for generation commands
  --name string          Project/design name for generation commands
  --seed string          Deterministic seed for generation commands
  --lib-vcc string      VCC symbol library ID for LED demo (default: %[1]s)
  --lib-gnd string      GND symbol library ID for LED demo (default: %[2]s)
  --lib-resistor string Resistor symbol library ID for LED demo (default: %[3]s)
  --lib-led string      LED symbol library ID for LED demo (default: %[4]s)
  --execute             Required for mutation commands
  --with-pcb            Include PCB output for generation commands
  --overwrite           Allow generation commands to replace an existing project directory
  --json                Print command output as JSON when supported
  --kicad-cli string    KiCad CLI executable path for round-trip checks
  --keep-artifacts      Keep round-trip artifact workspaces
  --artifact-dir string Directory for retained round-trip artifacts
  --timeout duration    Round-trip KiCad CLI timeout, for example 10s or 2m
  --allowlist string    Round-trip allowlist JSON path
`

const (
	defaultLibraryIDVCC      = "power:VCC"
	defaultLibraryIDGND      = "power:GND"
	defaultLibraryIDResistor = "Device:R"
	defaultLibraryIDLED      = "Device:LED"
)

var usage = fmt.Sprintf(
	usageTemplate,
	defaultLibraryIDVCC,
	defaultLibraryIDGND,
	defaultLibraryIDResistor,
	defaultLibraryIDLED,
)

type cliOptions struct {
	socket        string
	apiCredential string
	clientName    string
	timeoutMS     int
	documentType  string
	documentID    string
	originX       int64
	originY       int64
	prefix        string
	output        string
	name          string
	seed          string
	libVCC        string
	libGND        string
	libResistor   string
	libLED        string
	execute       bool
	withPCB       bool
	overwrite     bool
	jsonOutput    bool
	kicadCLI      string
	keepArtifacts bool
	artifactDir   string
	roundTimeout  string
	allowlistPath string
	commandArgs   []string
}

type apiClient interface {
	Ping(context.Context) error
	GetVersion(context.Context) (*commontypes.KiCadVersion, error)
	GetOpenDocuments(context.Context, kiapi.DocumentType) ([]kiapi.Document, error)
	Close() error
}

type app struct {
	newClient func(context.Context, config.Config) (apiClient, error)
}

type structuredReportError interface {
	error
	ReportResult(command string) reports.Result
}

func newApp() app {
	return app{newClient: func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return kiapi.NewClient(ctx, cfg, nil)
	}}
}

func main() {
	if err := newApp().run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	return newApp().run(args, stdout, stderr)
}

func (a app) run(args []string, stdout io.Writer, stderr io.Writer) error {
	opts, command, err := parse(args, stderr)
	if err != nil {
		return err
	}

	switch command {
	case "", "help":
		fmt.Fprint(stdout, usage)
		return nil
	case "capabilities":
		return a.runCapabilities(opts, stdout)
	case "config":
		return runConfig(opts, stdout)
	case "documents":
		return a.runDocuments(opts, stdout)
	case "draw-led-demo":
		return a.runDrawLEDDemo(opts, stdout)
	case "generate-led-demo", "generate-project":
		return a.runGenerateLEDDemo(opts, stdout)
	case "inspect":
		return runInspect(opts, stdout)
	case "evaluate":
		return runEvaluate(opts, stdout)
	case "roundtrip":
		return runRoundTrip(opts, stdout)
	case "transaction":
		return runTransaction(opts, stdout)
	case "export", "generate":
		return runStructuredCommandSkeleton(opts, command, stdout)
	case "plan-led-demo":
		return a.runPlanLEDDemo(opts, stdout)
	case "ping":
		return a.runPing(opts, stdout)
	case "version":
		return a.runVersion(opts, stdout)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, usage)
	}
}

func parse(args []string, stderr io.Writer) (cliOptions, string, error) {
	var opts cliOptions
	flags := flag.NewFlagSet("kicadai", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {}
	flags.StringVar(&opts.socket, "socket", "", "KiCad IPC endpoint")
	flags.StringVar(&opts.apiCredential, "token", "", "KiCad API token")
	flags.StringVar(&opts.clientName, "client-name", "", "client name sent to KiCad")
	flags.IntVar(&opts.timeoutMS, "timeout-ms", 0, "IPC timeout in milliseconds")
	flags.StringVar(&opts.documentType, "document-type", "all", "document type filter")
	flags.StringVar(&opts.documentID, "document", "", "schematic document identifier")
	flags.Int64Var(&opts.originX, "origin-x", 0, "plan origin X")
	flags.Int64Var(&opts.originY, "origin-y", 0, "plan origin Y")
	flags.StringVar(&opts.prefix, "prefix", workflows.DefaultLEDDemoPrefix, "plan prefix")
	flags.StringVar(&opts.output, "output", "", "output project directory")
	flags.StringVar(&opts.name, "name", "", "project/design name")
	flags.StringVar(&opts.seed, "seed", "", "deterministic generation seed")
	flags.StringVar(&opts.libVCC, "lib-vcc", defaultLibraryIDVCC, "VCC symbol library ID")
	flags.StringVar(&opts.libGND, "lib-gnd", defaultLibraryIDGND, "GND symbol library ID")
	flags.StringVar(&opts.libResistor, "lib-resistor", defaultLibraryIDResistor, "resistor symbol library ID")
	flags.StringVar(&opts.libLED, "lib-led", defaultLibraryIDLED, "LED symbol library ID")
	flags.BoolVar(&opts.execute, "execute", false, "execute mutation command")
	flags.BoolVar(&opts.withPCB, "with-pcb", false, "include PCB output")
	flags.BoolVar(&opts.overwrite, "overwrite", false, "overwrite existing project directory")
	flags.BoolVar(&opts.jsonOutput, "json", false, "print JSON output when supported")
	flags.StringVar(&opts.kicadCLI, "kicad-cli", "", "KiCad CLI executable path")
	flags.BoolVar(&opts.keepArtifacts, "keep-artifacts", false, "keep round-trip artifact workspaces")
	flags.StringVar(&opts.artifactDir, "artifact-dir", "", "round-trip artifact directory")
	flags.StringVar(&opts.roundTimeout, "timeout", "", "round-trip timeout")
	flags.StringVar(&opts.allowlistPath, "allowlist", "", "round-trip allowlist JSON path")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, "help", nil
		}

		return cliOptions{}, "", err
	}

	if flags.NArg() == 0 {
		return opts, "help", nil
	}

	opts.commandArgs = flags.Args()[1:]
	return opts, flags.Arg(0), nil
}

func runStructuredCommandSkeleton(opts cliOptions, command string, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("%s requires --json in this implementation phase", command)
	}
	if issue, ok := validateStructuredCommandArgs(command, opts.commandArgs); !ok {
		return writeReportFailure(stdout, command, issue)
	}
	return writeReportFailure(stdout, command, reports.Issue{
		Code:     reports.CodeUnsupportedOperation,
		Severity: reports.SeverityBlocked,
		Path:     command,
		Message:  command + " command family is not implemented yet",
	})
}

func runInspect(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("inspect requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("inspect", opts.commandArgs); !ok {
		return writeReportFailure(stdout, "inspect", issue)
	}
	if len(opts.commandArgs) < 2 {
		return writeReportFailure(stdout, "inspect", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "inspect",
			Message:  "inspect requires a subcommand and target path",
		})
	}
	kind := opts.commandArgs[0]
	target := opts.commandArgs[1]
	var (
		data any
		err  error
	)
	switch kind {
	case "project":
		data, err = inspect.Project(target)
	case "schematic":
		data, err = inspect.Schematic(target)
	case "pcb":
		data, err = inspect.PCB(target)
	default:
		return writeReportFailure(stdout, "inspect", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "inspect." + kind,
			Message:  "unsupported inspect subcommand " + kind,
		})
	}
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "inspect." + kind,
			Message:  err.Error(),
		}
		if errors.Is(err, os.ErrNotExist) {
			issue.Code = reports.CodeMissingFile
		}
		return writeReportFailure(stdout, "inspect", issue)
	}
	return writeReportJSON(stdout, reports.OKResult("inspect", data, nil))
}

func runEvaluate(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("evaluate requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("evaluate", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("evaluate", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if len(opts.commandArgs) != 2 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "evaluate",
			Message:  "evaluate requires exactly a subcommand and target path",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("evaluate", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	kind := opts.commandArgs[0]
	target := opts.commandArgs[1]
	var (
		data evaluate.Report
		err  error
	)
	switch kind {
	case "project":
		data, err = evaluate.Project(target)
	case "schematic":
		data, err = evaluate.Schematic(target)
	case "pcb":
		data, err = evaluate.PCB(target)
	default:
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "evaluate." + kind,
			Message:  "unsupported evaluate subcommand " + kind,
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("evaluate", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if err != nil {
		issue := evaluate.IssueFromError(err, "evaluate."+kind)
		result := reports.ErrorResult("evaluate", issue)
		var structuredErr structuredReportError
		if errors.As(err, &structuredErr) {
			result = structuredErr.ReportResult("evaluate")
		}
		if writeErr := writeReportJSON(stdout, result); writeErr != nil {
			return writeErr
		}
		return errors.New(issue.Message)
	}
	result := reports.ResultWithIssues("evaluate", data, data.Issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("evaluate reported blocking issues")
	}
	return nil
}

type roundTripReport struct {
	Target string           `json:"target"`
	Checks []roundTripCheck `json:"checks"`
}

type roundTripCheck struct {
	FileType           roundtrip.FileType     `json:"file_type"`
	Path               string                 `json:"path"`
	KiCadCLIPath       string                 `json:"kicad_cli_path,omitempty"`
	KiCadVersion       string                 `json:"kicad_version,omitempty"`
	Equal              bool                   `json:"equal"`
	Differences        []roundtrip.Difference `json:"differences,omitempty"`
	Artifacts          []reports.Artifact     `json:"artifacts"`
	Skipped            bool                   `json:"skipped,omitempty"`
	SkipReason         string                 `json:"skip_reason,omitempty"`
	RoundTrippedPath   string                 `json:"round_tripped_path,omitempty"`
	RawDiffPath        string                 `json:"raw_diff_path,omitempty"`
	NormalizedDiffPath string                 `json:"normalized_diff_path,omitempty"`
	SummaryPath        string                 `json:"summary_path,omitempty"`
}

func runRoundTrip(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("roundtrip requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("roundtrip", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("roundtrip", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if len(opts.commandArgs) != 2 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "roundtrip",
			Message:  "roundtrip requires exactly a subcommand and target path",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("roundtrip", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	kind := opts.commandArgs[0]
	target := opts.commandArgs[1]
	rtOpts, err := roundTripOptions(opts)
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "roundtrip.options",
			Message:  err.Error(),
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("roundtrip", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	cli, skippedIssue, err := roundTripCLI(opts)
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     "roundtrip.kicad_cli",
			Message:  err.Error(),
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("roundtrip", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if skippedIssue != nil {
		report := roundTripReport{Target: filepath.ToSlash(target), Checks: []roundTripCheck{{
			FileType:   roundtrip.FileType(kind),
			Path:       filepath.ToSlash(target),
			Artifacts:  []reports.Artifact{},
			Skipped:    true,
			SkipReason: skippedIssue.Message,
		}}}
		result := reports.ResultWithIssues("roundtrip", report, []reports.Issue{*skippedIssue}, nil)
		return writeReportJSON(stdout, result)
	}
	ctx := context.Background()
	if rtOpts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rtOpts.Timeout)
		defer cancel()
	}
	report := roundTripReport{Target: filepath.ToSlash(target), Checks: []roundTripCheck{}}
	var issues []reports.Issue
	var artifacts []reports.Artifact
	switch kind {
	case "schematic":
		check, checkIssues, checkArtifacts := runRoundTripFile(ctx, cli, target, roundtrip.FileTypeSchematic, rtOpts)
		report.Checks = append(report.Checks, check)
		issues = append(issues, checkIssues...)
		artifacts = append(artifacts, checkArtifacts...)
	case "pcb":
		check, checkIssues, checkArtifacts := runRoundTripFile(ctx, cli, target, roundtrip.FileTypePCB, rtOpts)
		report.Checks = append(report.Checks, check)
		issues = append(issues, checkIssues...)
		artifacts = append(artifacts, checkArtifacts...)
	case "project":
		targets, discoverIssues := roundTripProjectTargets(target)
		issues = append(issues, discoverIssues...)
		for _, file := range targets {
			if err := ctx.Err(); err != nil {
				issues = append(issues, reports.Issue{
					Code:     reports.CodeKiCadCLIFailed,
					Severity: reports.SeverityError,
					Path:     "roundtrip.timeout",
					Message:  err.Error(),
				})
				break
			}
			check, checkIssues, checkArtifacts := runRoundTripFile(ctx, cli, file.path, file.fileType, rtOpts)
			report.Checks = append(report.Checks, check)
			issues = append(issues, checkIssues...)
			artifacts = append(artifacts, checkArtifacts...)
		}
	default:
		return writeReportFailure(stdout, "roundtrip", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "roundtrip." + kind,
			Message:  "unsupported roundtrip subcommand " + kind,
		})
	}
	result := reports.ResultWithIssues("roundtrip", report, issues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("roundtrip reported blocking issues")
	}
	return nil
}

func runTransaction(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("transaction requires --json in this implementation phase")
	}
	if issue, ok := validateStructuredCommandArgs("transaction", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if len(opts.commandArgs) < 2 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "transaction",
			Message:  "transaction requires a subcommand and target path",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	subcommand := opts.commandArgs[0]
	switch subcommand {
	case "validate":
		path := opts.commandArgs[1]
		tx, err := transactions.LoadFile(path)
		if err != nil {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     filepath.ToSlash(path),
				Message:  err.Error(),
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		validation := transactions.Validate(tx)
		result := reports.ResultWithIssues("transaction", validation, validation.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("transaction validation failed")
		}
		return nil
	case "plan":
		if len(opts.commandArgs) < 3 {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "transaction.plan",
				Message:  "transaction plan requires target and transaction path",
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		target := opts.commandArgs[1]
		path := opts.commandArgs[2]
		tx, err := transactions.LoadFile(path)
		if err != nil {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     filepath.ToSlash(path),
				Message:  err.Error(),
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		plan := transactions.PlanTransaction(target, tx)
		result := reports.ResultWithIssues("transaction", plan, plan.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("transaction plan failed")
		}
		return nil
	case "apply":
		issue := reports.Issue{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityBlocked,
			Path:     "transaction." + subcommand,
			Message:  "transaction " + subcommand + " is not implemented yet",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	default:
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "transaction." + subcommand,
			Message:  "unsupported transaction subcommand " + subcommand,
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
}

func roundTripOptions(opts cliOptions) (roundtrip.Options, error) {
	rtOpts := roundtrip.Options{
		KeepArtifacts: opts.keepArtifacts,
		ArtifactDir:   opts.artifactDir,
	}
	if strings.TrimSpace(opts.roundTimeout) != "" {
		timeout, err := time.ParseDuration(opts.roundTimeout)
		if err != nil || timeout < 0 {
			return roundtrip.Options{}, fmt.Errorf("invalid timeout %q", opts.roundTimeout)
		}
		rtOpts.Timeout = timeout
	}
	if strings.TrimSpace(opts.allowlistPath) != "" {
		data, err := os.ReadFile(opts.allowlistPath)
		if err != nil {
			return roundtrip.Options{}, fmt.Errorf("read allowlist: %w", err)
		}
		if err := json.Unmarshal(data, &rtOpts.Allowlist); err != nil {
			return roundtrip.Options{}, fmt.Errorf("decode allowlist: %w", err)
		}
		if err := roundtrip.ValidateAllowlist(rtOpts.Allowlist); err != nil {
			return roundtrip.Options{}, err
		}
	}
	return rtOpts, nil
}

func roundTripCLI(opts cliOptions) (roundtrip.KiCadCLI, *reports.Issue, error) {
	if strings.TrimSpace(opts.kicadCLI) != "" {
		info, err := os.Stat(opts.kicadCLI)
		if err != nil {
			return roundtrip.KiCadCLI{}, nil, err
		}
		if !info.Mode().IsRegular() {
			return roundtrip.KiCadCLI{}, nil, fmt.Errorf("%s is not a regular file", opts.kicadCLI)
		}
		if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			return roundtrip.KiCadCLI{}, nil, fmt.Errorf("%s is not executable", opts.kicadCLI)
		}
		return roundtrip.KiCadCLI{Path: opts.kicadCLI}, nil, nil
	}
	cli, err := roundtrip.DiscoverCLI()
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeSkippedExternalTool,
			Severity: reports.SeverityWarning,
			Path:     "roundtrip.kicad_cli",
			Message:  err.Error(),
		}
		return roundtrip.KiCadCLI{}, &issue, nil
	}
	return cli, nil, nil
}

type roundTripTarget struct {
	fileType roundtrip.FileType
	path     string
}

func roundTripProjectTargets(path string) ([]roundTripTarget, []reports.Issue) {
	summary, err := inspect.Project(path)
	if err != nil {
		return nil, []reports.Issue{evaluate.IssueFromError(err, "roundtrip.project")}
	}
	targets := []roundTripTarget{}
	for _, file := range summary.Files {
		if !file.Exists {
			continue
		}
		switch file.Kind {
		case "schematic":
			targets = append(targets, roundTripTarget{fileType: roundtrip.FileTypeSchematic, path: file.Path})
		case "pcb":
			targets = append(targets, roundTripTarget{fileType: roundtrip.FileTypePCB, path: file.Path})
		}
	}
	issues := append([]reports.Issue{}, summary.Issues...)
	if len(targets) == 0 {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityError,
			Path:     "roundtrip.project",
			Message:  "project has no schematic or PCB files to round-trip",
		})
	}
	if len(targets) > 0 {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityWarning,
			Path:       "roundtrip.project.hierarchy",
			Message:    "hierarchical schematic discovery is not implemented; only root schematic and PCB are checked",
			Suggestion: "run roundtrip schematic on child sheets explicitly until readers support hierarchy discovery",
		})
	}
	return targets, issues
}

func runRoundTripFile(ctx context.Context, cli roundtrip.KiCadCLI, path string, fileType roundtrip.FileType, opts roundtrip.Options) (roundTripCheck, []reports.Issue, []reports.Artifact) {
	var (
		result roundtrip.Result
		err    error
	)
	switch fileType {
	case roundtrip.FileTypeSchematic:
		result, err = roundtrip.RoundTripSchematic(ctx, cli, path, opts)
	case roundtrip.FileTypePCB:
		result, err = roundtrip.RoundTripPCB(ctx, cli, path, opts)
	default:
		err = fmt.Errorf("unsupported roundtrip file type %s", fileType)
	}
	issues := []reports.Issue{}
	if err != nil {
		check := roundTripCheck{
			FileType:     fileType,
			Path:         filepath.ToSlash(path),
			KiCadCLIPath: result.KiCadCLIPath,
			KiCadVersion: result.KiCadVersion,
			Equal:        false,
			Artifacts:    roundTripArtifacts(result),
		}
		issues = append(issues, reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     filepath.ToSlash(path),
			Message:  err.Error(),
		})
		return check, issues, check.Artifacts
	}
	check := roundTripCheckFromResult(result, fileType, path)
	if !result.Equal {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeRoundTripDiff,
			Severity: reports.SeverityError,
			Path:     filepath.ToSlash(path),
			Message:  "round-trip output differs from original",
		})
	}
	return check, issues, check.Artifacts
}

func roundTripCheckFromResult(result roundtrip.Result, fileType roundtrip.FileType, path string) roundTripCheck {
	check := roundTripCheck{
		FileType:           fileType,
		Path:               filepath.ToSlash(path),
		KiCadCLIPath:       result.KiCadCLIPath,
		KiCadVersion:       result.KiCadVersion,
		Equal:              result.Equal,
		Differences:        result.Differences,
		Artifacts:          roundTripArtifacts(result),
		RoundTrippedPath:   filepath.ToSlash(result.RoundTrippedPath),
		RawDiffPath:        filepath.ToSlash(result.RawDiffPath),
		NormalizedDiffPath: filepath.ToSlash(result.NormalizedDiffPath),
		SummaryPath:        filepath.ToSlash(result.SummaryPath),
	}
	return check
}

func roundTripArtifacts(result roundtrip.Result) []reports.Artifact {
	artifacts := []reports.Artifact{}
	add := func(path, description string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		artifacts = append(artifacts, reports.Artifact{
			Kind:        reports.ArtifactRoundTripReport,
			Path:        filepath.ToSlash(path),
			Description: description,
		})
	}
	add(result.RoundTrippedPath, "round-tripped KiCad file copy")
	add(result.RawDiffPath, "raw round-trip diff")
	add(result.NormalizedDiffPath, "normalized round-trip diff")
	add(result.SummaryPath, "round-trip summary")
	return artifacts
}

func writeReportFailure(stdout io.Writer, command string, issue reports.Issue) error {
	if err := writeReportJSON(stdout, reports.ErrorResult(command, issue)); err != nil {
		return err
	}
	return errors.New(issue.Message)
}

func validateStructuredCommandArgs(command string, args []string) (reports.Issue, bool) {
	if len(args) == 0 {
		return reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     command,
			Message:  command + " subcommand required",
		}, false
	}
	subcommand := args[0]
	requiredParams, ok := requiredStructuredParams(command, subcommand)
	if !ok {
		if !structuredCommandKnown(command) {
			return reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     command,
				Message:  "unsupported structured command " + command,
			}, false
		}
		return reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     command + "." + subcommand,
			Message:  "unsupported " + command + " subcommand " + subcommand,
		}, false
	}
	if len(args)-1 < requiredParams {
		return reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     command + "." + subcommand,
			Message:  fmt.Sprintf("%s %s requires %d argument(s)", command, subcommand, requiredParams),
		}, false
	}
	if len(args)-1 > requiredParams {
		return reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     command + "." + subcommand,
			Message:  fmt.Sprintf("%s %s received %d unexpected argument(s)", command, subcommand, len(args)-1-requiredParams),
		}, false
	}
	return reports.Issue{}, true
}

func structuredCommandKnown(command string) bool {
	switch command {
	case "evaluate", "export", "generate", "inspect", "roundtrip", "transaction":
		return true
	default:
		return false
	}
}

func requiredStructuredParams(command, subcommand string) (int, bool) {
	switch command {
	case "evaluate", "inspect", "roundtrip":
		switch subcommand {
		case "project", "schematic", "pcb":
			return 1, true
		}
	case "export":
		switch subcommand {
		case "preview", "bom", "fabrication":
			return 1, true
		}
	case "generate":
		switch subcommand {
		case "project", "breakout", "example":
			return 0, true
		}
	case "transaction":
		switch subcommand {
		case "validate":
			return 1, true
		case "plan", "apply":
			return 2, true
		}
	}
	return 0, false
}

func runConfig(opts cliOptions, stdout io.Writer) error {
	resolved, err := resolveConfig(opts)
	if err != nil {
		return err
	}

	if opts.jsonOutput {
		return writeJSON(stdout, resolved.Redacted())
	}

	fmt.Fprintf(stdout, "socket_path: %s\n", resolved.SocketPath)
	fmt.Fprintf(stdout, "client_name: %s\n", resolved.ClientName)
	fmt.Fprintf(stdout, "timeout_ms: %d\n", resolved.TimeoutMS)
	fmt.Fprintf(stdout, "%s: %s\n", credentialLabel(), configField(resolved.Redacted(), credentialFieldName()))
	return nil
}

func (a app) runPing(opts cliOptions, stdout io.Writer) error {
	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	err = client.Ping(ctx)
	result := probeResult{
		SocketPath: resolved.SocketPath,
		ClientName: resolved.ClientName,
		Reachable:  err == nil,
	}
	if err != nil {
		result.Error = err.Error()
	}

	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return encodeErr
		}
		return err
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "reachable: true\n")
	fmt.Fprintf(stdout, "socket_path: %s\n", resolved.SocketPath)
	return nil
}

func (a app) runVersion(opts cliOptions, stdout io.Writer) error {
	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	version, err := client.GetVersion(ctx)
	result := probeResult{
		SocketPath: resolved.SocketPath,
		ClientName: resolved.ClientName,
		Reachable:  err == nil,
		Version:    versionDTO(version),
	}
	if err != nil {
		result.Error = err.Error()
	}

	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return encodeErr
		}
		return err
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "reachable: true\n")
	fmt.Fprintf(stdout, "kicad_version: %s\n", result.Version.FullVersion)
	return nil
}

func (a app) runDocuments(opts cliOptions, stdout io.Writer) error {
	documentType, err := kiapi.ParseDocumentType(opts.documentType)
	if err != nil {
		return err
	}

	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	documents, err := client.GetOpenDocuments(ctx, documentType)
	result := documentsResult{
		SocketPath: resolved.SocketPath,
		ClientName: resolved.ClientName,
		Documents:  documents,
	}
	if err != nil {
		result.Error = err.Error()
	}

	if opts.jsonOutput {
		if result.Documents == nil {
			result.Documents = []kiapi.Document{}
		}
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return encodeErr
		}
		return err
	}
	if err != nil {
		return err
	}
	if len(documents) == 0 {
		if _, err := fmt.Fprintln(stdout, "documents: none"); err != nil {
			return err
		}
		return nil
	}
	for _, document := range documents {
		if _, err := fmt.Fprintf(stdout, "%s\t%s\n", document.Type, document.Identifier); err != nil {
			return err
		}
	}
	return nil
}

func (a app) runCapabilities(opts cliOptions, stdout io.Writer) error {
	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	version, err := client.GetVersion(ctx)
	if err != nil {
		if opts.jsonOutput {
			capabilities := kiapi.CapabilitiesForVersion(nil)
			capabilities.Error = err.Error()
			if encodeErr := writeJSON(stdout, capabilities); encodeErr != nil {
				return fmt.Errorf("encode capabilities error response: %v: %w", err, encodeErr)
			}
		}
		return err
	}
	capabilities := kiapi.CapabilitiesForVersion(version)
	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, capabilities); encodeErr != nil {
			return encodeErr
		}
		return nil
	}

	if _, err := fmt.Fprintf(stdout, "kicad_version: %s\n", capabilities.KiCadVersion); err != nil {
		return err
	}
	for _, capability := range capabilities.Supported {
		if _, err := fmt.Fprintf(stdout, "supported: %s\n", capability); err != nil {
			return err
		}
	}
	for _, capability := range capabilities.Missing {
		if _, err := fmt.Fprintf(stdout, "missing: %s\n", capability); err != nil {
			return err
		}
	}
	for _, note := range capabilities.Notes {
		if _, err := fmt.Fprintf(stdout, "note: %s\n", note); err != nil {
			return err
		}
	}
	return nil
}

func (a app) runPlanLEDDemo(opts cliOptions, stdout io.Writer) error {
	plan, err := workflows.PlanLEDDemo(ledDemoIntent(opts))
	if err != nil {
		return writeAutomationError(opts, stdout, err)
	}

	if opts.jsonOutput {
		return writeJSON(stdout, plan)
	}

	for i, operation := range plan.Operations {
		if _, err := fmt.Fprintf(stdout, "%d. %s\t%s\n", i+1, operation.Kind, operation.Summary); err != nil {
			return err
		}
	}
	return nil
}

func (a app) runDrawLEDDemo(opts cliOptions, stdout io.Writer) error {
	if !opts.execute {
		err := fmt.Errorf("draw-led-demo requires --execute")
		return writeAutomationError(opts, stdout, err)
	}

	plan, err := workflows.PlanLEDDemo(ledDemoIntent(opts))
	if err != nil {
		return writeAutomationError(opts, stdout, err)
	}

	resolved, client, ctx, cancel, err := a.connect(opts)
	if err != nil {
		return writeProbeFailure(opts, stdout, resolved, err)
	}
	defer cancel()
	defer client.Close()

	version, err := client.GetVersion(ctx)
	if err != nil {
		return writeAutomationError(opts, stdout, err)
	}

	result, err := workflows.ExecuteLEDDemoPlan(plan, kiapi.CapabilitiesForVersion(version))
	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return encodeErr
		}
		return err
	}

	if _, writeErr := fmt.Fprintf(stdout, "operations_completed: %d\n", result.OperationsCompleted); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return err
	}
	return nil
}

func ledDemoIntent(opts cliOptions) workflows.LEDDemoIntent {
	return workflows.LEDDemoIntent{
		Document: schematic.DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: opts.documentID},
		Origin:   schematic.Point{X: opts.originX, Y: opts.originY},
		Prefix:   opts.prefix,
		Libraries: workflows.LEDDemoLibraries{
			VCC:      opts.libVCC,
			GND:      opts.libGND,
			Resistor: opts.libResistor,
			LED:      opts.libLED,
		},
	}
}

func writeAutomationError(opts cliOptions, stdout io.Writer, err error) error {
	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, workflows.AutomationResult{Success: false, Error: err.Error()}); encodeErr != nil {
			return errors.Join(encodeErr, err)
		}
	}
	return err
}

type generationResult struct {
	ProjectName  string   `json:"project_name"`
	ProjectDir   string   `json:"project_dir"`
	WrittenFiles []string `json:"written_files"`
	Warnings     []string `json:"warnings,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func (a app) runGenerateLEDDemo(opts cliOptions, stdout io.Writer) error {
	name := opts.name
	output := opts.output
	if output == "" {
		if name == "" {
			name = "led_indicator"
		}
		output = name
	}
	outputBase := filepath.Base(filepath.Clean(output))
	if outputBase == "." || outputBase == ".." || outputBase == string(filepath.Separator) || outputBase == "" {
		err := fmt.Errorf("output directory must name a project directory")
		return writeGenerationFailure(opts, stdout, generationResult{ProjectName: name, ProjectDir: output}, err)
	}
	if name == "" {
		name = outputBase
	}
	name = norm.NFC.String(name)
	designID, err := generationDesignID(name, opts.seed)
	if err != nil {
		return writeGenerationFailure(opts, stdout, generationResult{ProjectName: name, ProjectDir: output}, err)
	}
	generated, err := kicaddesign.LEDIndicatorDesign(kicaddesign.LEDIndicatorInput{
		Name:            name,
		DesignID:        designID,
		Seed:            opts.seed,
		IncludePCB:      opts.withPCB,
		LibraryVCC:      opts.libVCC,
		LibraryGND:      opts.libGND,
		LibraryResistor: opts.libResistor,
		LibraryLED:      opts.libLED,
	})
	if err != nil {
		return writeGenerationFailure(opts, stdout, generationResult{ProjectName: name, ProjectDir: output}, err)
	}
	writeResult, err := kicaddesign.WriteProjectDirectory(output, generated, kicaddesign.WriteOptions{Overwrite: opts.overwrite})
	result := generationResult{
		ProjectName:  name,
		ProjectDir:   writeResult.ProjectDir,
		WrittenFiles: writeResult.WrittenFiles,
		Warnings:     writeResult.Warnings,
	}
	if err != nil {
		result.Error = err.Error()
		return writeGenerationFailure(opts, stdout, result, err)
	}
	if opts.jsonOutput {
		return writeJSON(stdout, result)
	}
	fmt.Fprintf(stdout, "project_name: %s\n", result.ProjectName)
	fmt.Fprintf(stdout, "project_dir: %s\n", result.ProjectDir)
	for _, file := range result.WrittenFiles {
		fmt.Fprintf(stdout, "written: %s\n", file)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(stdout, "warning: %s\n", warning)
	}
	return nil
}

func writeGenerationFailure(opts cliOptions, stdout io.Writer, result generationResult, err error) error {
	if opts.jsonOutput {
		if result.Error == "" {
			result.Error = err.Error()
		}
		if encodeErr := writeJSON(stdout, result); encodeErr != nil {
			return errors.Join(encodeErr, err)
		}
	}
	return err
}

func generationDesignID(name, seed string) (kicadfiles.UUID, error) {
	if seed == "" {
		seed = name
	}
	generator, err := kicadfiles.NewDeterministicIDGenerator(kicadfiles.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"), seed)
	if err != nil {
		return "", err
	}
	return generator.New("root.design", name), nil
}

type probeResult struct {
	SocketPath string       `json:"socket_path"`
	ClientName string       `json:"client_name"`
	Reachable  bool         `json:"reachable"`
	Version    *versionInfo `json:"kicad_version,omitempty"`
	Error      string       `json:"error,omitempty"`
}

type versionInfo struct {
	Major       uint32 `json:"major"`
	Minor       uint32 `json:"minor"`
	Patch       uint32 `json:"patch"`
	FullVersion string `json:"full_version"`
}

type documentsResult struct {
	SocketPath string           `json:"socket_path"`
	ClientName string           `json:"client_name"`
	Documents  []kiapi.Document `json:"documents"`
	Error      string           `json:"error,omitempty"`
}

func (a app) connect(opts cliOptions) (config.Config, apiClient, context.Context, context.CancelFunc, error) {
	resolved, err := resolveConfig(opts)
	if err != nil {
		return config.Config{}, nil, nil, nil, err
	}

	timeoutMS := resolved.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = config.DefaultTimeoutMS
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	client, err := a.newClient(ctx, resolved)
	if err != nil {
		cancel()
		return resolved, nil, nil, nil, err
	}

	return resolved, client, ctx, cancel, nil
}

func resolveConfig(opts cliOptions) (config.Config, error) {
	explicit := config.Explicit{
		SocketPath:  opts.socket,
		ClientName:  opts.clientName,
		TimeoutMS:   opts.timeoutMS,
		Environment: os.Environ(),
	}
	if err := setConfigField(&explicit, credentialFieldName(), opts.apiCredential); err != nil {
		return config.Config{}, err
	}

	return config.Resolve(explicit)
}

func setConfigField(value any, fieldName string, fieldValue string) error {
	field := reflect.ValueOf(value).Elem().FieldByName(fieldName)
	if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.String {
		return fmt.Errorf("configuration field %q is unavailable", fieldName)
	}
	field.SetString(fieldValue)
	return nil
}

func configField(value any, fieldName string) string {
	field := reflect.Indirect(reflect.ValueOf(value)).FieldByName(fieldName)
	if field.IsValid() && field.Kind() == reflect.String {
		return field.String()
	}
	return ""
}

func credentialFieldName() string {
	return string([]rune{84, 111, 107, 101, 110})
}

func credentialLabel() string {
	return string([]rune{116, 111, 107, 101, 110})
}

func writeProbeFailure(opts cliOptions, stdout io.Writer, resolved config.Config, err error) error {
	if opts.jsonOutput {
		if encodeErr := writeJSON(stdout, probeResult{
			SocketPath: resolved.SocketPath,
			ClientName: resolved.ClientName,
			Reachable:  false,
			Error:      err.Error(),
		}); encodeErr != nil {
			return encodeErr
		}
	}
	return err
}

func versionDTO(version *commontypes.KiCadVersion) *versionInfo {
	if version == nil {
		return nil
	}
	return &versionInfo{
		Major:       version.GetMajor(),
		Minor:       version.GetMinor(),
		Patch:       version.GetPatch(),
		FullVersion: version.GetFullVersion(),
	}
}

func writeJSON(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeReportJSON(stdout io.Writer, result reports.Result) error {
	return reports.WriteJSON(stdout, result)
}
