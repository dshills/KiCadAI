package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
	"kicadai/internal/blocks"
	"kicadai/internal/blocks/verification"
	"kicadai/internal/boardvalidation"
	"kicadai/internal/components"
	"kicadai/internal/config"
	"kicadai/internal/designworkflow"
	"kicadai/internal/evaluate"
	"kicadai/internal/fabrication"
	fabricationprofiles "kicadai/internal/fabrication/profiles"
	breakoutgen "kicadai/internal/generate"
	"kicadai/internal/inspect"
	"kicadai/internal/intentdraft"
	"kicadai/internal/intentplanner"
	"kicadai/internal/kiapi"
	commontypes "kicadai/internal/kiapi/gen/common/types"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/checks"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/manifest"
	"kicadai/internal/pinmap"
	"kicadai/internal/placement"
	"kicadai/internal/rationale"
	"kicadai/internal/repair"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/schematic"
	"kicadai/internal/schematicpcb"
	"kicadai/internal/transactions"
	"kicadai/internal/workflows"
	"kicadai/internal/writercorrectness"
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
  block         List, inspect, and validate built-in circuit blocks
  fabrication   List, show, and validate fabrication profiles
  component     List, inspect, select, validate, and report component catalog records
  check         Run KiCad CLI ERC/DRC checks
  design        Create AI design workflow projects
  writer        Check generated writer correctness
  validate      Validate generated board electrical correctness
  inspect       Inspect KiCad projects and files
  intent        Plan or explain high-level AI design intent requests
  library       Index and query KiCad symbol and footprint libraries
  evaluate      Evaluate KiCad projects and files
  pinmap        List or validate symbol-footprint pinmaps
  place         Run PCB placement planning
  route         Run PCB routing
  repair        Plan, export-bundle, or apply validation repair attempts
  export        Export review and fabrication artifacts
  plan-led-demo Print a deterministic LED indicator schematic plan
  ping          Check whether KiCad responds to the API
  roundtrip     Run KiCad CLI round-trip checks
  transaction   Build, validate, plan, or apply structured edit transactions
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
  --target string        Project, schematic, or PCB target for repair commands
  --request string       Structured request JSON path for generator commands
  --text string          Natural-language intent text
  --file string          Natural-language intent text file
  --format string        Output format for supported commands: json or text (default json)
  --strict              Treat blocking draft clarifications as command errors
  --name string          Project/design name for generation commands
  --seed string          Deterministic seed for generation commands
  --lib-vcc string      VCC symbol library ID for LED demo (default: {{defaultLibraryIDVCC}})
  --lib-gnd string      GND symbol library ID for LED demo (default: {{defaultLibraryIDGND}})
  --lib-resistor string Resistor symbol library ID for LED demo (default: {{defaultLibraryIDResistor}})
  --lib-led string      LED symbol library ID for LED demo (default: {{defaultLibraryIDLED}})
  --execute             Required for mutation commands
  --with-pcb            Include PCB output for generation commands
  --overwrite           Allow generation commands to replace an existing project directory
  --allow-imported-apply Explicitly allow transaction apply to mutate an existing imported project
  --json                Compatibility alias for --format json
  --kicad-cli string    KiCad CLI executable path for KiCad-backed checks and fabrication plotting
  --manufacturer-profile string Local fabrication manufacturer profile ID, for example generic_assembly
  --manufacturer-profile-dir string Directory of local fabrication profile JSON snapshots
  --keep-artifacts      Keep KiCad-backed artifact workspaces
  --artifact-dir string Directory for retained KiCad-backed artifacts
  --timeout duration    KiCad CLI timeout, for example 10s or 2m
  --allowlist string    Round-trip or ERC/DRC allowlist JSON path
  --case string         Block verification manifest path
  --suite string        Block verification suite directory
  --builtins            Run built-in block verification manifests
  --kicad-corpus        Run only built-in block manifests included in the KiCad corpus
  --kicad-corpus-tier string Comma-separated KiCad corpus tier filter
  --require-erc         Require KiCad ERC evidence for block verification
  --require-drc         Require KiCad DRC evidence for board validation or block verification
  --allow-missing-drc   Do not fail board validation when KiCad DRC is unavailable
  --strict-zones        Treat zones without fill evidence as blocking
  --strict-unrouted     Treat unrouted multi-pad nets as blocking
  --zone-refill string  KiCad zone refill policy: never, before_validation, after_repair_before_validation
  --require-kicad-roundtrip Require KiCad round-trip evidence for writer checks
  --strict-diffs        Treat benign round-trip differences as writer failures
  --allow-unrouted      Allow unrouted nets in writer checks
  --klc-root string        KiCad Library Convention repository root
  --symbols-root string    KiCad symbol library root
  --footprints-root string KiCad footprint library root
  --templates-root string  KiCad template library root
  --library-cache string   Library resolver cache file path
  --refresh-library-cache  Rebuild library resolver cache
  --catalog-dir string     Component catalog directory (default: data/components)
  --source-dir string      Component lifecycle/availability source directory
  --family string          Component family filter
  --package string         Component package filter
  --value-kind string      Component value kind filter
  --value string           Component value filter
  --acceptance string      Component acceptance level
  --mode string                      Routing mode: single_layer, two_layer, validate_only
  --route-mode string                Alias for --mode
  --grid float                       Routing grid in millimeters
  --trace-width float                Routing trace width in millimeters
  --clearance float                  Routing clearance in millimeters
  --allow-partial                    Allow partial routing results
  --repair                           Include validation repair planning in design create
  --repair-apply                     Apply generated-project validation repairs during design create
  --max-repair-attempts int          Maximum validation repair attempts (default 3)
  --skip-routing                     Skip design workflow board routing
  --placement-board-width float      Placement feedback board width in millimeters (default {{defaultPlacementBoardWidthMM}})
  --placement-board-height float     Placement feedback board height in millimeters (default {{defaultPlacementBoardHeightMM}})
  --placement-board-margin float     Placement feedback board margin in millimeters (default {{defaultPlacementBoardMarginMM}})
  --placement-estimated-width float  Placement feedback estimated component width in millimeters (default {{defaultPlacementEstimatedWidthMM}})
  --placement-estimated-height float Placement feedback estimated component height in millimeters (default {{defaultPlacementEstimatedHeightMM}})
  --skip-placement-feedback          Skip placement feedback in block project generation output
  --feedback                         Include grouped operation feedback for transaction commands
  --pretty                           Pretty-print JSON output
`

const (
	defaultLibraryIDVCC      = "power:VCC"
	defaultLibraryIDGND      = "power:GND"
	defaultLibraryIDResistor = "Device:R"
	defaultLibraryIDLED      = "Device:LED"

	defaultPlacementBoardWidthMM      = 100.0
	defaultPlacementBoardHeightMM     = 60.0
	defaultPlacementBoardMarginMM     = 2.0
	defaultPlacementEstimatedWidthMM  = 2.0
	defaultPlacementEstimatedHeightMM = 1.25
)

var usage = strings.NewReplacer(
	"{{defaultLibraryIDVCC}}", defaultLibraryIDVCC,
	"{{defaultLibraryIDGND}}", defaultLibraryIDGND,
	"{{defaultLibraryIDResistor}}", defaultLibraryIDResistor,
	"{{defaultLibraryIDLED}}", defaultLibraryIDLED,
	"{{defaultPlacementBoardWidthMM}}", fmt.Sprintf("%g", defaultPlacementBoardWidthMM),
	"{{defaultPlacementBoardHeightMM}}", fmt.Sprintf("%g", defaultPlacementBoardHeightMM),
	"{{defaultPlacementBoardMarginMM}}", fmt.Sprintf("%g", defaultPlacementBoardMarginMM),
	"{{defaultPlacementEstimatedWidthMM}}", fmt.Sprintf("%g", defaultPlacementEstimatedWidthMM),
	"{{defaultPlacementEstimatedHeightMM}}", fmt.Sprintf("%g", defaultPlacementEstimatedHeightMM),
).Replace(usageTemplate)

type cliOptions struct {
	socket                      string
	apiCredential               string
	clientName                  string
	timeoutMS                   int
	documentType                string
	documentID                  string
	originX                     int64
	originY                     int64
	prefix                      string
	output                      string
	target                      string
	requestPath                 string
	intentText                  string
	intentFile                  string
	outputFormat                string
	strictDraft                 bool
	name                        string
	seed                        string
	libVCC                      string
	libGND                      string
	libResistor                 string
	libLED                      string
	execute                     bool
	withPCB                     bool
	overwrite                   bool
	allowImportedApply          bool
	jsonOutput                  bool
	kicadCLI                    string
	manufacturerProfile         string
	manufacturerProfileDir      string
	keepArtifacts               bool
	artifactDir                 string
	roundTimeout                string
	allowlistPath               string
	blockVerifyCase             string
	blockVerifySuite            string
	blockVerifyBuiltins         bool
	blockVerifyKiCadCorpus      bool
	blockVerifyKiCadCorpusTiers string
	requireERC                  bool
	requireDRC                  bool
	allowMissingDRC             bool
	strictZones                 bool
	strictUnrouted              bool
	zoneRefill                  string
	requireKiCadRoundTrip       bool
	strictDiffs                 bool
	allowUnrouted               bool
	klcRoot                     string
	symbolsRoot                 string
	footprintsRoot              string
	templatesRoot               string
	libraryCache                string
	refreshLibraryCache         bool
	catalogDir                  string
	sourceDir                   string
	componentFamily             string
	componentPackage            string
	componentValueKind          string
	componentValue              string
	componentAcceptance         string
	routeMode                   string
	routeGridMM                 float64
	routeTraceWidthMM           float64
	routeClearanceMM            float64
	routeAllowPartial           bool
	routeAllowPartialSet        bool
	repairEnabled               bool
	repairApply                 bool
	maxRepairAttempts           int
	skipRouting                 bool
	placementBoardWidth         float64
	placementBoardHeight        float64
	placementBoardMargin        float64
	placementEstWidth           float64
	placementEstHeight          float64
	skipPlacementFeedback       bool
	feedbackOutput              bool
	prettyOutput                bool
	commandArgs                 []string
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer func() {
		stop()
	}()

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
	case "intent":
		return runIntent(ctx, opts, stdout)
	case "evaluate":
		return runEvaluate(opts, stdout)
	case "library":
		return runLibrary(ctx, opts, stdout)
	case "component":
		return runComponent(ctx, opts, stdout)
	case "check":
		return runCheckCommand(ctx, opts, stdout)
	case "design":
		return runDesign(ctx, opts, stdout)
	case "validate":
		return runValidateCommand(ctx, opts, stdout)
	case "writer":
		return runWriterCommand(ctx, opts, stdout)
	case "roundtrip":
		return runRoundTrip(opts, stdout)
	case "pinmap":
		return runPinmap(opts, stdout)
	case "place":
		return runPlace(opts, stdout)
	case "route":
		return runRoute(opts, stdout)
	case "repair":
		return runRepairCommand(opts, stdout)
	case "transaction":
		return runTransaction(opts, stdout)
	case "export":
		return runExport(ctx, opts, stdout)
	case "fabrication":
		return runFabrication(opts, stdout)
	case "generate":
		return runGenerate(opts, stdout)
	case "block":
		return runBlock(ctx, opts, stdout)
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
	var jsonFlag bool
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
	flags.StringVar(&opts.target, "target", "", "repair target project or file")
	flags.StringVar(&opts.requestPath, "request", "", "structured request JSON path")
	flags.StringVar(&opts.intentText, "text", "", "natural-language intent text")
	flags.StringVar(&opts.intentFile, "file", "", "natural-language intent text file")
	flags.StringVar(&opts.outputFormat, "format", "json", "output format: json or text")
	flags.BoolVar(&opts.strictDraft, "strict", false, "treat blocking draft clarifications as command errors")
	flags.StringVar(&opts.name, "name", "", "project/design name")
	flags.StringVar(&opts.seed, "seed", "", "deterministic generation seed")
	flags.StringVar(&opts.libVCC, "lib-vcc", defaultLibraryIDVCC, "VCC symbol library ID")
	flags.StringVar(&opts.libGND, "lib-gnd", defaultLibraryIDGND, "GND symbol library ID")
	flags.StringVar(&opts.libResistor, "lib-resistor", defaultLibraryIDResistor, "resistor symbol library ID")
	flags.StringVar(&opts.libLED, "lib-led", defaultLibraryIDLED, "LED symbol library ID")
	flags.BoolVar(&opts.execute, "execute", false, "execute mutation command")
	flags.BoolVar(&opts.withPCB, "with-pcb", false, "include PCB output")
	flags.BoolVar(&opts.overwrite, "overwrite", false, "overwrite existing project directory")
	flags.BoolVar(&opts.allowImportedApply, "allow-imported-apply", false, "allow transaction apply to mutate an existing imported project")
	flags.BoolVar(&jsonFlag, "json", false, "compatibility alias for --format json")
	flags.StringVar(&opts.kicadCLI, "kicad-cli", "", "KiCad CLI executable path")
	flags.StringVar(&opts.manufacturerProfile, "manufacturer-profile", "", "local fabrication manufacturer profile ID")
	flags.StringVar(&opts.manufacturerProfileDir, "manufacturer-profile-dir", os.Getenv(fabricationprofiles.EnvProfileDir), "local fabrication profile directory")
	flags.BoolVar(&opts.keepArtifacts, "keep-artifacts", false, "keep round-trip artifact workspaces")
	flags.StringVar(&opts.artifactDir, "artifact-dir", "", "round-trip artifact directory")
	flags.StringVar(&opts.roundTimeout, "timeout", "", "round-trip timeout")
	flags.StringVar(&opts.allowlistPath, "allowlist", "", "round-trip allowlist JSON path")
	flags.StringVar(&opts.blockVerifyCase, "case", "", "block verification manifest path")
	flags.StringVar(&opts.blockVerifySuite, "suite", "", "block verification suite directory")
	flags.BoolVar(&opts.blockVerifyBuiltins, "builtins", false, "run built-in block verification manifests")
	flags.BoolVar(&opts.blockVerifyKiCadCorpus, "kicad-corpus", false, "run only block verification manifests included in the KiCad corpus")
	flags.StringVar(&opts.blockVerifyKiCadCorpusTiers, "kicad-corpus-tier", "", "comma-separated KiCad corpus tier filter")
	flags.BoolVar(&opts.requireERC, "require-erc", false, "require KiCad ERC evidence for block verification")
	flags.BoolVar(&opts.requireDRC, "require-drc", false, "require KiCad DRC evidence for board validation or block verification")
	flags.BoolVar(&opts.allowMissingDRC, "allow-missing-drc", false, "do not fail board validation when KiCad DRC is unavailable")
	flags.BoolVar(&opts.strictZones, "strict-zones", false, "treat zones without fill evidence as blocking")
	flags.BoolVar(&opts.strictUnrouted, "strict-unrouted", false, "treat unrouted multi-pad nets as blocking")
	flags.StringVar(&opts.zoneRefill, "zone-refill", string(repair.ZoneRefillNever), "KiCad zone refill policy: never, before_validation, after_repair_before_validation")
	flags.BoolVar(&opts.requireKiCadRoundTrip, "require-kicad-roundtrip", false, "require KiCad round-trip evidence for writer checks")
	flags.BoolVar(&opts.strictDiffs, "strict-diffs", false, "treat benign round-trip differences as writer failures")
	flags.BoolVar(&opts.allowUnrouted, "allow-unrouted", false, "allow unrouted nets in writer checks")
	flags.StringVar(&opts.klcRoot, "klc-root", "", "KiCad Library Convention repository root")
	flags.StringVar(&opts.symbolsRoot, "symbols-root", "", "KiCad symbol library root")
	flags.StringVar(&opts.footprintsRoot, "footprints-root", "", "KiCad footprint library root")
	flags.StringVar(&opts.templatesRoot, "templates-root", "", "KiCad template library root")
	flags.StringVar(&opts.libraryCache, "library-cache", os.Getenv(libraryresolver.EnvLibraryCache), "library resolver cache file path")
	flags.BoolVar(&opts.refreshLibraryCache, "refresh-library-cache", false, "rebuild library resolver cache")
	flags.StringVar(&opts.catalogDir, "catalog-dir", components.DefaultCatalogDir, "component catalog directory")
	flags.StringVar(&opts.sourceDir, "source-dir", "", "component lifecycle/availability source directory")
	flags.StringVar(&opts.componentFamily, "family", "", "component family filter")
	flags.StringVar(&opts.componentPackage, "package", "", "component package filter")
	flags.StringVar(&opts.componentValueKind, "value-kind", "", "component value kind filter")
	flags.StringVar(&opts.componentValue, "value", "", "component value filter")
	flags.StringVar(&opts.componentAcceptance, "acceptance", "", "component acceptance level")
	flags.StringVar(&opts.routeMode, "mode", "", "routing mode")
	flags.StringVar(&opts.routeMode, "route-mode", "", "routing mode")
	flags.Float64Var(&opts.routeGridMM, "grid", 0, "routing grid in millimeters")
	flags.Float64Var(&opts.routeTraceWidthMM, "trace-width", 0, "routing trace width in millimeters")
	flags.Float64Var(&opts.routeClearanceMM, "clearance", 0, "routing clearance in millimeters")
	flags.BoolVar(&opts.routeAllowPartial, "allow-partial", false, "allow partial routing results")
	flags.BoolVar(&opts.repairEnabled, "repair", false, "include validation repair planning in design create")
	flags.BoolVar(&opts.repairApply, "repair-apply", false, "apply generated-project validation repairs during design create")
	flags.IntVar(&opts.maxRepairAttempts, "max-repair-attempts", 3, "maximum validation repair attempts")
	flags.BoolVar(&opts.skipRouting, "skip-routing", false, "skip design workflow board routing")
	flags.Float64Var(&opts.placementBoardWidth, "placement-board-width", defaultPlacementBoardWidthMM, "placement feedback board width in millimeters")
	flags.Float64Var(&opts.placementBoardHeight, "placement-board-height", defaultPlacementBoardHeightMM, "placement feedback board height in millimeters")
	flags.Float64Var(&opts.placementBoardMargin, "placement-board-margin", defaultPlacementBoardMarginMM, "placement feedback board margin in millimeters")
	flags.Float64Var(&opts.placementEstWidth, "placement-estimated-width", defaultPlacementEstimatedWidthMM, "placement feedback estimated component width in millimeters")
	flags.Float64Var(&opts.placementEstHeight, "placement-estimated-height", defaultPlacementEstimatedHeightMM, "placement feedback estimated component height in millimeters")
	flags.BoolVar(&opts.skipPlacementFeedback, "skip-placement-feedback", false, "skip placement feedback in block project generation output")
	flags.BoolVar(&opts.feedbackOutput, "feedback", false, "include grouped operation feedback for transaction commands")
	flags.BoolVar(&opts.prettyOutput, "pretty", false, "pretty-print JSON output")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, "help", nil
		}

		return cliOptions{}, "", err
	}

	formatFlagSet := false
	jsonFlagSet := false
	jsonFlagVisited := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "allow-partial" {
			opts.routeAllowPartialSet = true
		}
		if f.Name == "format" {
			formatFlagSet = true
		}
		if f.Name == "json" {
			jsonFlagVisited = true
			if jsonFlag {
				jsonFlagSet = true
			}
		}
	})
	opts.outputFormat = strings.ToLower(strings.TrimSpace(opts.outputFormat))
	if jsonFlagSet {
		opts.outputFormat = "json"
	} else if jsonFlagVisited && !formatFlagSet {
		opts.outputFormat = "text"
	}
	switch opts.outputFormat {
	case "json":
		opts.jsonOutput = true
	case "text":
		opts.jsonOutput = false
	default:
		return opts, "", fmt.Errorf("invalid --format %q: must be json or text", opts.outputFormat)
	}
	if err := validatePlacementFeedbackOptions(opts); err != nil {
		return opts, "", err
	}
	if flags.NArg() == 0 {
		return opts, "help", nil
	}
	if opts.repairApply {
		opts.repairEnabled = true
	}
	opts.commandArgs = flags.Args()[1:]
	return opts, flags.Arg(0), nil
}

func validatePlacementFeedbackOptions(opts cliOptions) error {
	if !positiveFiniteFloat(opts.placementBoardWidth) {
		return fmt.Errorf("--placement-board-width must be positive")
	}
	if !positiveFiniteFloat(opts.placementBoardHeight) {
		return fmt.Errorf("--placement-board-height must be positive")
	}
	if !nonNegativeFiniteFloat(opts.placementBoardMargin) {
		return fmt.Errorf("--placement-board-margin must be zero or positive")
	}
	if opts.placementBoardMargin*2 >= opts.placementBoardWidth || opts.placementBoardMargin*2 >= opts.placementBoardHeight {
		return fmt.Errorf("--placement-board-margin must leave positive usable board area")
	}
	if opts.placementBoardWidth-2*opts.placementBoardMargin < opts.placementEstWidth ||
		opts.placementBoardHeight-2*opts.placementBoardMargin < opts.placementEstHeight {
		return fmt.Errorf("placement board dimensions and margin must leave enough usable area for estimated component bounds")
	}
	if !positiveFiniteFloat(opts.placementEstWidth) {
		return fmt.Errorf("--placement-estimated-width must be positive")
	}
	if !positiveFiniteFloat(opts.placementEstHeight) {
		return fmt.Errorf("--placement-estimated-height must be positive")
	}
	return nil
}

func positiveFiniteFloat(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func nonNegativeFiniteFloat(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func runStructuredCommandSkeleton(opts cliOptions, command string, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("%s requires --format json", command)
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

func runGenerate(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("generate requires --format json")
	}
	if issue, ok := validateStructuredCommandArgs("generate", opts.commandArgs); !ok {
		return writeReportFailure(stdout, "generate", issue)
	}
	switch opts.commandArgs[0] {
	case "breakout":
		return runGenerateBreakout(opts, stdout)
	default:
		return writeReportFailure(stdout, "generate", reports.Issue{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityBlocked,
			Path:     "generate." + opts.commandArgs[0],
			Message:  "generate " + opts.commandArgs[0] + " is not implemented yet",
		})
	}
}

func runGenerateBreakout(opts cliOptions, stdout io.Writer) error {
	if strings.TrimSpace(opts.requestPath) == "" {
		return writeReportFailure(stdout, "generate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "request",
			Message:  "generate breakout requires --request",
		})
	}
	if strings.TrimSpace(opts.output) == "" {
		return writeReportFailure(stdout, "generate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "output",
			Message:  "generate breakout requires --output",
		})
	}
	req, err := breakoutgen.LoadBreakoutRequest(opts.requestPath)
	if err != nil {
		return writeReportFailure(stdout, "generate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "request",
			Message:  err.Error(),
		})
	}
	data := breakoutgen.GenerateBreakout(req, breakoutgen.BreakoutOptions{
		OutputDir: opts.output,
		Overwrite: opts.overwrite,
		Seed:      opts.seed,
	})
	result := reports.ResultWithIssues("generate", data, data.Issues, data.Artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("generate breakout failed")
	}
	return nil
}

func runBlock(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("block requires --format json")
	}
	if len(opts.commandArgs) == 0 {
		return writeBlockFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "block",
			Message:  "block requires a subcommand",
		})
	}
	registry := blocks.NewBuiltinRegistry()
	subcommand := opts.commandArgs[0]
	var feedbackOptions blockPlacementFeedbackOptions
	feedbackOptionsReady := false
	blockFeedbackOptions := func() blockPlacementFeedbackOptions {
		if !feedbackOptionsReady {
			feedbackOptions = blockPlacementFeedbackOptionsFromCLI(ctx, opts)
			feedbackOptionsReady = true
		}
		return feedbackOptions
	}
	switch subcommand {
	case "list":
		if len(opts.commandArgs) != 1 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("list", 0))
		}
		return writeBlockResult(stdout, registry.ListBlocks(), nil)
	case "show":
		if len(opts.commandArgs) != 2 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("show", 1))
		}
		id := opts.commandArgs[1]
		definition, ok := registry.GetBlock(id)
		if !ok {
			return writeBlockResult(stdout, nil, []reports.Issue{missingBlockIssue(id)})
		}
		return writeBlockResult(stdout, definition, nil)
	case "validate":
		if len(opts.commandArgs) != 2 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("validate", 1))
		}
		id := opts.commandArgs[1]
		request, err := blockRequestFromOptions(opts, id)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "request",
				Message:  err.Error(),
			})
		}
		issues := registry.ValidateRequest(request)
		return writeBlockResult(stdout, request, issues)
	case "verify":
		if len(opts.commandArgs) != 1 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("verify", 0))
		}
		report, issues, artifacts, err := runBlockVerify(ctx, opts, registry)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "block.verify",
				Message:  err.Error(),
			})
		}
		return writeBlockApplyResult(stdout, report, issues, artifacts)
	case "instantiate":
		if len(opts.commandArgs) != 2 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("instantiate", 1))
		}
		id := opts.commandArgs[1]
		request, err := blockRequestFromOptions(opts, id)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "request",
				Message:  err.Error(),
			})
		}
		output, issues := registry.Instantiate(ctx, request)
		if opts.output == "" || reports.HasBlockingIssue(issues) {
			return writeBlockResult(stdout, output, issues)
		}
		projectName := blockProjectName(opts, request.InstanceID)
		tx, err := blocks.ProjectTransactionForBlockOutput(projectName, output, opts.overwrite)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "block.instantiate",
				Message:  err.Error(),
			})
		}
		feedbackOptions := blockFeedbackOptions()
		placementApplication := blockPlacementApplicationForBlockOutput(ctx, tx, output, feedbackOptions)
		tx = placementApplication.Transaction
		applyResult := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: opts.output, Overwrite: opts.overwrite, Seed: opts.seed})
		resultIssues := combinedBlockIssues(issues, placementApplication.Issues)
		return writeBlockApplyResult(stdout, blockProjectGenerationResult{
			Output:      output,
			Transaction: tx,
			ApplyResult: applyResult,
			Feedback:    placementApplication.Feedback,
		}, combinedBlockIssues(resultIssues, applyResult.Issues), applyResult.Artifacts)
	case "realize-pcb":
		if len(opts.commandArgs) != 2 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("realize-pcb", 1))
		}
		id := opts.commandArgs[1]
		definition, ok := registry.GetBlock(id)
		if !ok {
			return writeBlockResult(stdout, nil, []reports.Issue{missingBlockIssue(id)})
		}
		request, err := blockRequestFromOptions(opts, id)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "request",
				Message:  err.Error(),
			})
		}
		if request.InstanceID == "" {
			request.InstanceID = blockProjectName(opts, id)
		}
		output, issues := registry.Instantiate(ctx, request)
		if reports.HasBlockingIssue(issues) {
			return writeBlockResult(stdout, blockPCBRealizationCLIResult{Output: output}, issues)
		}
		realization := blocks.RealizeBlockPCB(definition, output, blocks.PCBRealizationOptions{})
		if reports.HasBlockingIssue(realization.Issues) {
			return writeBlockResult(stdout, blockPCBRealizationCLIResult{
				Output:      output,
				Realization: &realization,
			}, combinedBlockIssues(issues, realization.Issues))
		}
		feedbackOptions := blockFeedbackOptions()
		placementRequest, placementIssues := placement.RequestFromBlockPCBRealization(realization, feedbackOptions.Adapter)
		resultIssues := combinedBlockIssues(realization.Issues, placementIssues)
		return writeBlockResult(stdout, blockPCBRealizationCLIResult{
			Output:           output,
			Realization:      &realization,
			PlacementRequest: &placementRequest,
		}, combinedBlockIssues(issues, resultIssues))
	case "compose":
		if len(opts.commandArgs) != 1 {
			return writeBlockFailure(stdout, invalidBlockArgCountIssue("compose", 0))
		}
		request, err := compositionRequestFromOptions(opts)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "request",
				Message:  err.Error(),
			})
		}
		output := blocks.ComposeBlocks(ctx, registry, request)
		issues := output.Issues
		if opts.output == "" || reports.HasBlockingIssue(issues) {
			return writeBlockResult(stdout, output, issues)
		}
		projectName := blockProjectName(opts, output.ProjectName)
		tx, err := blocks.ProjectTransactionForCompositionOutput(projectName, output, opts.overwrite)
		if err != nil {
			return writeBlockFailure(stdout, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "block.compose",
				Message:  err.Error(),
			})
		}
		feedbackOptions := blockFeedbackOptions()
		placementApplication := compositionPlacementApplication(ctx, tx, output, feedbackOptions)
		tx = placementApplication.Transaction
		applyResult := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: opts.output, Overwrite: opts.overwrite, Seed: opts.seed})
		resultIssues := combinedBlockIssues(issues, placementApplication.Issues)
		return writeBlockApplyResult(stdout, compositionProjectGenerationResult{
			Output:      output,
			Transaction: tx,
			ApplyResult: applyResult,
			Feedback:    placementApplication.Feedback,
		}, combinedBlockIssues(resultIssues, applyResult.Issues), applyResult.Artifacts)
	default:
		return writeBlockFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "block." + subcommand,
			Message:  "unsupported block subcommand " + subcommand,
		})
	}
}

func blockRequestFromOptions(opts cliOptions, id string) (blocks.BlockRequest, error) {
	if strings.TrimSpace(opts.requestPath) == "" {
		request := blocks.BlockRequest{BlockID: id}
		if strings.TrimSpace(opts.output) != "" {
			request.InstanceID = blockProjectName(opts, id)
		}
		return request, nil
	}
	data, err := os.ReadFile(opts.requestPath)
	if err != nil {
		return blocks.BlockRequest{}, err
	}
	var request blocks.BlockRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return blocks.BlockRequest{}, err
	}
	if request.BlockID == "" {
		request.BlockID = id
	}
	if request.BlockID != id {
		return blocks.BlockRequest{}, fmt.Errorf("request block_id %q does not match command block ID %q", request.BlockID, id)
	}
	if request.InstanceID == "" && strings.TrimSpace(opts.output) != "" {
		request.InstanceID = blockProjectName(opts, id)
	}
	return request, nil
}

func compositionRequestFromOptions(opts cliOptions) (blocks.CompositionRequest, error) {
	if strings.TrimSpace(opts.requestPath) == "" {
		return blocks.CompositionRequest{}, fmt.Errorf("block compose requires --request")
	}
	data, err := os.ReadFile(opts.requestPath)
	if err != nil {
		return blocks.CompositionRequest{}, err
	}
	var request blocks.CompositionRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return blocks.CompositionRequest{}, err
	}
	if len(request.Instances) == 0 {
		return blocks.CompositionRequest{}, fmt.Errorf("composition request must contain at least one instance")
	}
	return request, nil
}

func runBlockVerify(ctx context.Context, opts cliOptions, registry blocks.Registry) (blockVerificationCLIReport, []reports.Issue, []reports.Artifact, error) {
	manifests, loadIssues, err := blockVerificationManifests(opts)
	if err != nil {
		return blockVerificationCLIReport{}, nil, nil, err
	}
	if reports.HasBlockingIssue(loadIssues) {
		return blockVerificationCLIReport{}, loadIssues, nil, nil
	}
	runOptions, err := blockVerificationRunOptions(opts, registry)
	if err != nil {
		return blockVerificationCLIReport{}, nil, nil, err
	}
	manifests, initialCorpusSummary := verification.SelectKiCadCorpusManifests(manifests, runOptions.KiCadCorpus)
	results := make([]verification.RunResult, 0, len(manifests))
	issues := append([]reports.Issue(nil), loadIssues...)
	var artifacts []reports.Artifact
	for _, manifest := range manifests {
		caseOptions := runOptions
		if strings.TrimSpace(runOptions.CheckOptions.ArtifactDir) != "" {
			artifactDir := blockVerificationCaseArtifactDir(runOptions.CheckOptions.ArtifactDir, manifest.ID)
			caseOptions.CheckOptions.ArtifactDir = artifactDir
			caseOptions.WriterOptions.ArtifactDir = artifactDir
		}
		result := verification.RunCase(ctx, manifest, caseOptions)
		results = append(results, result)
		issues = append(issues, result.Issues...)
		artifacts = append(artifacts, result.Artifacts...)
		if runOptions.KiCadCorpus.Enabled && result.KiCadCorpus != nil {
			corpusArtifacts, corpusIssues := writeBlockVerificationCorpusCaseResult(opts, manifest.ID, *result.KiCadCorpus)
			issues = append(issues, corpusIssues...)
			artifacts = append(artifacts, corpusArtifacts...)
		}
	}
	corpusSummary := initialCorpusSummary
	if runOptions.KiCadCorpus.Enabled {
		corpusSummary = verification.BuildKiCadCorpusSummary(results)
		corpusSummary.TotalCount = initialCorpusSummary.TotalCount
		corpusArtifacts, corpusIssues := writeBlockVerificationCorpusSummary(opts, corpusSummary)
		issues = append(issues, corpusIssues...)
		artifacts = append(artifacts, corpusArtifacts...)
	}
	report := blockVerificationCLIReport{Count: len(results), Results: results}
	if runOptions.KiCadCorpus.Enabled {
		report.KiCadCorpus = &corpusSummary
	}
	return report, issues, artifacts, nil
}

func blockVerificationManifests(opts cliOptions) ([]verification.Manifest, []reports.Issue, error) {
	selected := 0
	if strings.TrimSpace(opts.blockVerifyCase) != "" {
		selected++
	}
	if strings.TrimSpace(opts.blockVerifySuite) != "" {
		selected++
	}
	if opts.blockVerifyBuiltins {
		selected++
	}
	if selected != 1 {
		return nil, nil, fmt.Errorf("block verify requires exactly one of --case, --suite, or --builtins")
	}
	if strings.TrimSpace(opts.blockVerifyCase) != "" {
		manifest, issues := verification.LoadManifest(opts.blockVerifyCase)
		if len(issues) != 0 {
			return nil, issues, nil
		}
		return []verification.Manifest{manifest}, nil, nil
	}
	root := strings.TrimSpace(opts.blockVerifySuite)
	if opts.blockVerifyBuiltins {
		manifests, issues := verification.LoadSuiteFS(blocks.BuiltinVerificationFS(), ".")
		return manifests, issues, nil
	}
	manifests, issues := verification.LoadSuite(root)
	return manifests, issues, nil
}

func blockVerificationCaseArtifactDir(root string, caseID string) string {
	return filepath.Join(root, safeBlockVerificationPathSegment(caseID))
}

func safeBlockVerificationPathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return builder.String()
}

func blockVerificationRunOptions(opts cliOptions, registry blocks.Registry) (verification.RunOptions, error) {
	checkOpts, err := checkOptions(opts)
	if err != nil {
		return verification.RunOptions{}, err
	}
	corpusTiers, err := blockVerificationKiCadCorpusTiers(opts.blockVerifyKiCadCorpusTiers)
	if err != nil {
		return verification.RunOptions{}, err
	}
	return verification.RunOptions{
		Registry:      registry,
		OutputDir:     opts.output,
		Overwrite:     opts.overwrite,
		KeepArtifacts: opts.keepArtifacts,
		KiCadCorpus: verification.KiCadCorpusOptions{
			Enabled: opts.blockVerifyKiCadCorpus,
			Tiers:   corpusTiers,
		},
		KiCadCLI:     opts.kicadCLI,
		RequireERC:   opts.requireERC,
		RequireDRC:   opts.requireDRC,
		CheckOptions: checkOpts,
		WriterOptions: writercorrectness.Options{
			KiCadCLI:              opts.kicadCLI,
			AllowUnrouted:         opts.allowUnrouted,
			RequireKiCadRoundTrip: opts.requireKiCadRoundTrip,
			StrictDiffs:           opts.strictDiffs,
			KeepArtifacts:         opts.keepArtifacts,
			ArtifactDir:           opts.artifactDir,
		},
	}, nil
}

func blockVerificationKiCadCorpusTiers(value string) ([]verification.KiCadCorpusTier, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	tiers := make([]verification.KiCadCorpusTier, 0, len(parts))
	for _, part := range parts {
		tier := verification.KiCadCorpusTier(strings.TrimSpace(part))
		switch tier {
		case verification.KiCadCorpusTierSmoke, verification.KiCadCorpusTierBlock, verification.KiCadCorpusTierLayout, verification.KiCadCorpusTierRegression:
			tiers = append(tiers, tier)
		case "":
		default:
			return nil, fmt.Errorf("unsupported KiCad corpus tier %q", tier)
		}
	}
	return tiers, nil
}

func writeBlockVerificationCorpusSummary(opts cliOptions, summary verification.KiCadCorpusSummary) ([]reports.Artifact, []reports.Issue) {
	if strings.TrimSpace(opts.output) == "" {
		return nil, nil
	}
	if err := os.MkdirAll(opts.output, 0o755); err != nil {
		return nil, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "block.verify.kicad_corpus.output",
			Message:  err.Error(),
		}}
	}
	path := filepath.Join(opts.output, "corpus-summary.json")
	if err := writeBlockVerificationJSONReport(path, summary, opts.overwrite); err != nil {
		return nil, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "block.verify.kicad_corpus.summary",
			Message:  err.Error(),
		}}
	}
	return []reports.Artifact{{
		Kind:        reports.ArtifactValidationReport,
		Path:        filepath.ToSlash(path),
		Description: "KiCad corpus summary JSON report",
	}}, nil
}

func writeBlockVerificationCorpusCaseResult(opts cliOptions, caseID string, result verification.KiCadCorpusCaseResult) ([]reports.Artifact, []reports.Issue) {
	if strings.TrimSpace(opts.output) == "" {
		return nil, nil
	}
	safeCaseID := safeBlockVerificationPathSegment(caseID)
	dir := filepath.Join(filepath.Clean(opts.output), safeCaseID, "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "block.verify.kicad_corpus.case_result",
			Message:  err.Error(),
		}}
	}
	path := filepath.Join(dir, "corpus-result.json")
	if err := writeBlockVerificationJSONReport(path, result, opts.overwrite); err != nil {
		return nil, []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "block.verify.kicad_corpus.case_result",
			Message:  err.Error(),
		}}
	}
	return []reports.Artifact{{
		Kind:        reports.ArtifactValidationReport,
		Path:        filepath.ToSlash(path),
		Description: "KiCad corpus case result JSON report",
	}}, nil
}

const blockVerificationReportFileMode = 0o644

func writeBlockVerificationJSONReport(path string, data any, overwrite bool) (err error) {
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, blockVerificationReportFileMode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func blockProjectName(opts cliOptions, fallback string) string {
	if strings.TrimSpace(opts.name) != "" {
		return opts.name
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	if strings.TrimSpace(opts.output) != "" {
		base := filepath.Base(filepath.Clean(opts.output))
		if base != "." && base != ".." && strings.TrimSpace(base) != "" {
			return base
		}
	}
	return blocks.DefaultGeneratedProjectName
}

type blockProjectGenerationResult struct {
	Output      blocks.BlockOutput        `json:"output"`
	Transaction transactions.Transaction  `json:"transaction"`
	ApplyResult transactions.ApplyResult  `json:"apply_result"`
	Feedback    *workflows.DesignFeedback `json:"feedback,omitempty"`
}

type compositionProjectGenerationResult struct {
	Output      blocks.CompositionOutput  `json:"output"`
	Transaction transactions.Transaction  `json:"transaction"`
	ApplyResult transactions.ApplyResult  `json:"apply_result"`
	Feedback    *workflows.DesignFeedback `json:"feedback,omitempty"`
}

type blockPCBRealizationCLIResult struct {
	Output           blocks.BlockOutput                `json:"output"`
	Realization      *blocks.BlockPCBRealizationResult `json:"realization,omitempty"`
	PlacementRequest *placement.Request                `json:"placement_request,omitempty"`
}

type blockVerificationCLIReport struct {
	Count       int                              `json:"count"`
	KiCadCorpus *verification.KiCadCorpusSummary `json:"kicad_corpus,omitempty"`
	Results     []verification.RunResult         `json:"results"`
}

type blockPlacementFeedbackOptions struct {
	Adapter placement.AdapterOptions
	Issues  []reports.Issue
	Enabled bool
}

type blockPlacementApplication struct {
	Transaction transactions.Transaction
	Feedback    *workflows.DesignFeedback
	Issues      []reports.Issue
}

func blockPlacementApplicationForBlockOutput(ctx context.Context, tx transactions.Transaction, output blocks.BlockOutput, feedbackOptions blockPlacementFeedbackOptions) blockPlacementApplication {
	if !feedbackOptions.Enabled {
		return blockPlacementApplication{Transaction: tx}
	}
	request, issues := placement.RequestFromBlockOutput(output, feedbackOptions.Adapter)
	issues = combinedPlacementFeedbackIssues(feedbackOptions.Issues, issues)
	return blockPlacementApplicationForRequest(ctx, tx, request, issues)
}

func compositionPlacementApplication(ctx context.Context, tx transactions.Transaction, output blocks.CompositionOutput, feedbackOptions blockPlacementFeedbackOptions) blockPlacementApplication {
	if !feedbackOptions.Enabled {
		return blockPlacementApplication{Transaction: tx}
	}
	request, issues := placement.RequestFromCompositionOutput(output, feedbackOptions.Adapter)
	issues = combinedPlacementFeedbackIssues(feedbackOptions.Issues, issues)
	return blockPlacementApplicationForRequest(ctx, tx, request, issues)
}

func blockPlacementApplicationForRequest(ctx context.Context, tx transactions.Transaction, request placement.Request, adapterIssues []reports.Issue) blockPlacementApplication {
	if reports.HasBlockingIssue(adapterIssues) {
		feedback := placementFeedbackForBlockedRequest(request, adapterIssues)
		return blockPlacementApplication{Transaction: tx, Feedback: feedback, Issues: cloneIssues(adapterIssues)}
	}
	result := placement.PlaceContext(ctx, request)
	result.Issues = combinedPlacementFeedbackIssues(adapterIssues, result.Issues)
	if result.Status != placement.StatusPlaced || reports.HasBlockingIssue(result.Issues) {
		feedback := workflows.EvaluatePlacement(request, result)
		return blockPlacementApplication{Transaction: tx, Feedback: &feedback, Issues: result.Issues}
	}
	operations, operationIssues := placement.PlacementOperations(request, result.Placements)
	result.Issues = combinedPlacementFeedbackIssues(result.Issues, operationIssues)
	if len(operations) != 0 && !reports.HasBlockingIssue(operationIssues) {
		tx = replaceSupersededPlacementOperationsBeforeWriteProject(tx, operations)
	}
	feedback := workflows.EvaluatePlacement(request, result)
	return blockPlacementApplication{Transaction: tx, Feedback: &feedback, Issues: result.Issues}
}

func replaceSupersededPlacementOperationsBeforeWriteProject(tx transactions.Transaction, operations []transactions.Operation) transactions.Transaction {
	if len(operations) == 0 {
		return tx
	}
	replacementRefs := placementOperationRefs(operations)
	if len(replacementRefs) == 0 {
		return tx
	}
	filtered := make([]transactions.Operation, 0, len(tx.Operations))
	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpPlaceFootprint {
			if _, replace := replacementRefs[normalizedPlacementRef(operation.Ref)]; replace {
				continue
			}
		}
		filtered = append(filtered, operation)
	}
	insertAt := len(filtered)
	for index := len(filtered) - 1; index >= 0; index-- {
		if filtered[index].Op == transactions.OpWriteProject {
			insertAt = index
			break
		}
	}
	next := make([]transactions.Operation, 0, len(filtered)+len(operations))
	next = append(next, filtered[:insertAt]...)
	next = append(next, operations...)
	next = append(next, filtered[insertAt:]...)
	tx.Operations = next
	return tx
}

func placementOperationRefs(operations []transactions.Operation) map[string]struct{} {
	refs := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		if operation.Op != transactions.OpPlaceFootprint {
			continue
		}
		key := normalizedPlacementRef(operation.Ref)
		if key != "" {
			refs[key] = struct{}{}
		}
	}
	return refs
}

func normalizedPlacementRef(ref string) string {
	return strings.ToUpper(strings.TrimSpace(ref))
}

func combinedPlacementFeedbackIssues(first []reports.Issue, second []reports.Issue) []reports.Issue {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	if len(first) == 0 {
		return cloneIssues(second)
	}
	if len(second) == 0 {
		return cloneIssues(first)
	}
	combined := make([]reports.Issue, 0, len(first)+len(second))
	combined = append(combined, first...)
	combined = append(combined, second...)
	return combined
}

func cloneIssues(issues []reports.Issue) []reports.Issue {
	return slices.Clone(issues)
}

func placementFeedbackForBlockedRequest(request placement.Request, issues []reports.Issue) *workflows.DesignFeedback {
	result := placement.Result{
		Status: placement.StatusBlocked,
		Issues: issues,
		Metrics: placement.Metrics{
			ComponentCount: len(request.Components),
			UnplacedCount:  len(request.Components),
		},
	}
	feedback := workflows.EvaluatePlacement(request, result)
	return &feedback
}

func blockPlacementFeedbackOptionsFromCLI(ctx context.Context, opts cliOptions) blockPlacementFeedbackOptions {
	adapterOptions := blockPlacementAdapterOptions(opts)
	if opts.skipPlacementFeedback {
		return blockPlacementFeedbackOptions{Adapter: adapterOptions}
	}
	roots := libraryRootsFromOptions(opts)
	index, ok, issues := blockPlacementLibraryIndex(ctx, roots, opts)
	if ok {
		adapterOptions.LibraryIndex = &index
	}
	return blockPlacementFeedbackOptions{Adapter: adapterOptions, Issues: issues, Enabled: true}
}

func blockPlacementAdapterOptions(opts cliOptions) placement.AdapterOptions {
	adapterOptions := placement.AdapterOptions{
		Board: placement.BoardPlacementArea{
			WidthMM:  opts.placementBoardWidth,
			HeightMM: opts.placementBoardHeight,
			MarginMM: opts.placementBoardMargin,
		},
		Rules: placement.DefaultRules(),
		DefaultBounds: placement.Bounds{
			WidthMM:  opts.placementEstWidth,
			HeightMM: opts.placementEstHeight,
			Source:   placement.BoundsEstimated,
		},
	}
	return adapterOptions
}

func blockPlacementLibraryIndex(ctx context.Context, roots libraryresolver.LibraryRoots, opts cliOptions) (libraryresolver.LibraryIndex, bool, []reports.Issue) {
	if !blockPlacementShouldUseLibraryResolver(roots) {
		return libraryresolver.LibraryIndex{}, false, nil
	}
	loadedIndex, issues := libraryresolver.Load(ctx, roots, libraryresolver.LoadOptions{
		CachePath: opts.libraryCache,
		Refresh:   opts.refreshLibraryCache,
	})
	if reports.HasBlockingIssue(issues) {
		return libraryresolver.LibraryIndex{}, false, issues
	}
	if len(loadedIndex.Footprints) == 0 {
		return libraryresolver.LibraryIndex{}, false, issues
	}
	return loadedIndex, true, issues
}

func blockPlacementShouldUseLibraryResolver(roots libraryresolver.LibraryRoots) bool {
	return strings.TrimSpace(roots.FootprintsRoot) != ""
}

func writeBlockResult(stdout io.Writer, data any, issues []reports.Issue) error {
	result := reports.ResultWithIssues("block", data, issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("block command failed")
	}
	return nil
}

func writeBlockApplyResult(stdout io.Writer, data any, issues []reports.Issue, artifacts []reports.Artifact) error {
	result := reports.ResultWithIssues("block", data, issues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("block command failed")
	}
	return nil
}

func combinedBlockIssues(groups ...[]reports.Issue) []reports.Issue {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	combined := make([]reports.Issue, 0, total)
	for _, group := range groups {
		combined = append(combined, group...)
	}
	return combined
}

func writeBlockFailure(stdout io.Writer, issue reports.Issue) error {
	if err := writeReportJSON(stdout, reports.ErrorResult("block", issue)); err != nil {
		return err
	}
	return errors.New(issue.Message)
}

func missingBlockIssue(id string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeMissingFile,
		Severity: reports.SeverityError,
		Path:     "block_id",
		Message:  "block not found: " + id,
	}
}

func invalidBlockArgCountIssue(subcommand string, required int) reports.Issue {
	message := fmt.Sprintf("block %s requires %d argument(s)", subcommand, required)
	if required == 0 {
		message = "block " + subcommand + " does not accept arguments"
	} else if required == 1 {
		message = "block " + subcommand + " requires 1 argument"
	}
	return reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: reports.SeverityError,
		Path:     "block." + subcommand,
		Message:  message,
	}
}

type libraryIndexData struct {
	Summary   libraryresolver.LoadSummary      `json:"summary"`
	Inventory libraryresolver.LibraryInventory `json:"inventory"`
}

func runLibrary(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("library requires --format json")
	}
	if len(opts.commandArgs) == 0 {
		return writeLibraryFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library",
			Message:  "library requires a subcommand",
		})
	}
	subcommand := opts.commandArgs[0]
	if subcommand == "symbols" {
		libraryIndex, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
			CachePath: opts.libraryCache,
			Refresh:   opts.refreshLibraryCache,
		})
		return runLibrarySymbols(opts.commandArgs[1:], stdout, libraryIndex, issues)
	}
	requiredArgs, ok := requiredLibraryParams(subcommand)
	if !ok {
		return writeLibraryFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library." + subcommand,
			Message:  "unsupported library subcommand " + subcommand,
		})
	}
	if got := len(opts.commandArgs) - 1; got != requiredArgs {
		return writeLibraryFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "library." + subcommand,
			Message:  fmt.Sprintf("library %s requires %d argument(s)", subcommand, requiredArgs),
		})
	}
	if subcommand == "templates" || subcommand == "template" {
		if subcommand == "templates" {
			records, issues := libraryresolver.DiscoverTemplates(ctx, libraryRootsFromOptions(opts))
			return writeLibraryResult(stdout, records, issues)
		}
		name := opts.commandArgs[1]
		record, issues, ok := libraryresolver.DiscoverTemplate(ctx, libraryRootsFromOptions(opts), name)
		if !ok {
			if !hasBlockingIssue(issues) {
				issues = append(issues, missingLibraryRecordIssue("library.template", name))
			}
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryResult(stdout, record, issues)
	}
	libraryIndex, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
		CachePath: opts.libraryCache,
		Refresh:   opts.refreshLibraryCache,
	})
	switch subcommand {
	case "index":
		data := libraryIndexData{Summary: libraryresolver.Summary(libraryIndex), Inventory: libraryIndex.Inventory}
		return writeLibraryResult(stdout, data, issues)
	case "symbol":
		id := opts.commandArgs[1]
		record, ok := libraryresolver.ResolveSymbol(libraryIndex, id)
		if !ok {
			issues = append(issues, missingLibraryRecordIssue("library.symbol", id))
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryResult(stdout, record, issues)
	case "footprint":
		id := opts.commandArgs[1]
		record, ok := libraryresolver.ResolveFootprint(libraryIndex, id)
		if !ok {
			issues = append(issues, missingLibraryRecordIssue("library.footprint", id))
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryResult(stdout, record, issues)
	case "search-symbols":
		return writeLibraryResult(stdout, libraryresolver.FindSymbols(libraryIndex, libraryresolver.Query{Text: opts.commandArgs[1]}), issues)
	case "search-footprints":
		return writeLibraryResult(stdout, libraryresolver.FindFootprints(libraryIndex, libraryresolver.Query{Text: opts.commandArgs[1]}), issues)
	case "compatible-footprints":
		return writeLibraryResult(stdout, libraryresolver.CompatibleFootprints(libraryIndex, opts.commandArgs[1], libraryresolver.MatchOptions{}), issues)
	case "validate-assignment":
		return writeLibraryResult(stdout, libraryresolver.ValidateAssignment(libraryIndex, opts.commandArgs[1], opts.commandArgs[2]), issues)
	case "pinmap-candidate":
		return writeLibraryResult(stdout, libraryresolver.GeneratePinmapCandidate(libraryIndex, opts.commandArgs[1], opts.commandArgs[2]), issues)
	case "klc-symbol":
		return writeLibraryKLCResult(stdout, libraryresolver.ValidateSymbolKLC(libraryIndex, opts.commandArgs[1]), issues)
	case "klc-footprint":
		return writeLibraryKLCResult(stdout, libraryresolver.ValidateFootprintKLC(libraryIndex, opts.commandArgs[1]), issues)
	}
	return writeLibraryFailure(stdout, reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: reports.SeverityError,
		Path:     "library." + subcommand,
		Message:  "unsupported library subcommand " + subcommand,
	})
}

type librarySymbolSummary struct {
	LibraryID       string `json:"library_id"`
	LibraryNickname string `json:"library_nickname"`
	Name            string `json:"name"`
	Path            string `json:"path,omitempty"`
	Description     string `json:"description,omitempty"`
	PinCount        int    `json:"pin_count"`
	UnitCount       int    `json:"unit_count"`
	PowerSymbol     bool   `json:"power_symbol,omitempty"`
}

func runLibrarySymbols(args []string, stdout io.Writer, index libraryresolver.LibraryIndex, issues []reports.Issue) error {
	if len(args) == 0 {
		return writeLibraryFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "library.symbols", Message: "library symbols requires a subcommand"})
	}
	subcommand := args[0]
	switch subcommand {
	case "list":
		if len(args) != 1 {
			return writeLibraryFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "library.symbols.list", Message: "library symbols list requires 0 argument(s)"})
		}
		records := libraryresolver.FindSymbols(index, libraryresolver.Query{})
		summaries := make([]librarySymbolSummary, 0, len(records))
		for _, record := range records {
			summaries = append(summaries, librarySymbolSummary{
				LibraryID:       record.LibraryID,
				LibraryNickname: record.LibraryNickname,
				Name:            record.Name,
				Path:            record.Path,
				Description:     record.Description,
				PinCount:        len(record.Pins),
				UnitCount:       len(record.Units),
				PowerSymbol:     record.PowerSymbol,
			})
		}
		return writeLibraryResult(stdout, summaries, issues)
	case "show":
		if len(args) != 2 {
			return writeLibraryFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "library.symbols.show", Message: "library symbols show requires 1 argument(s)"})
		}
		record, ok := libraryresolver.ResolveSymbol(index, args[1])
		if !ok {
			issues = append(issues, missingLibraryRecordIssue("library.symbol", args[1]))
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryResult(stdout, record, issues)
	case "pins":
		if len(args) != 2 {
			return writeLibraryFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "library.symbols.pins", Message: "library symbols pins requires 1 argument(s)"})
		}
		record, ok := libraryresolver.ResolveSymbol(index, args[1])
		if !ok {
			issues = append(issues, missingLibraryRecordIssue("library.symbol", args[1]))
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryResult(stdout, record.Pins, issues)
	case "validate":
		if len(args) != 2 {
			return writeLibraryFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "library.symbols.validate", Message: "library symbols validate requires 1 argument(s)"})
		}
		if _, ok := libraryresolver.ResolveSymbol(index, args[1]); !ok {
			issues = append(issues, missingLibraryRecordIssue("library.symbol", args[1]))
			return writeLibraryResult(stdout, nil, issues)
		}
		return writeLibraryKLCResult(stdout, libraryresolver.ValidateSymbolKLC(index, args[1]), issues)
	default:
		return writeLibraryFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "library.symbols." + subcommand, Message: "unsupported library symbols subcommand " + subcommand})
	}
}

func writeLibraryFailure(stdout io.Writer, issue reports.Issue) error {
	return writeReportFailure(stdout, "library", issue)
}

func writeLibraryKLCResult(stdout io.Writer, report libraryresolver.KLCReport, issues []reports.Issue) error {
	reportIssues := report.Issues
	report.Issues = nil
	return writeLibraryResult(stdout, report, append(issues, reportIssues...))
}

func requiredLibraryParams(subcommand string) (int, bool) {
	switch subcommand {
	case "index", "templates":
		return 0, true
	case "symbol", "footprint", "search-symbols", "search-footprints", "compatible-footprints", "klc-symbol", "klc-footprint", "template":
		return 1, true
	case "validate-assignment", "pinmap-candidate":
		return 2, true
	default:
		return 0, false
	}
}

func writeLibraryResult(stdout io.Writer, data any, issues []reports.Issue) error {
	result := reports.ResultWithIssues("library", data, issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("library command failed")
	}
	return nil
}

func runComponent(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("component requires --format json")
	}
	if len(opts.commandArgs) == 0 {
		return writeReportFailure(stdout, "component", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "component",
			Message:  "component requires a subcommand",
		})
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: opts.catalogDir})
	if err != nil {
		return writeReportFailure(stdout, "component", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "catalog-dir",
			Message:  err.Error(),
		})
	}
	subcommand := opts.commandArgs[0]
	var sources *components.SourceCollection
	var sourceIssues []reports.Issue
	if componentSubcommandUsesSources(subcommand) {
		sources, sourceIssues, err = loadComponentSources(ctx, opts.sourceDir)
		if err != nil {
			return writeReportFailure(stdout, "component", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "source-dir", Message: err.Error()})
		}
	}
	switch subcommand {
	case "list":
		return writeComponentResult(stdout, map[string]any{
			"families": catalog.Families,
			"records":  catalog.Records,
		}, catalog.Diagnostics)
	case "show":
		if len(opts.commandArgs) != 2 {
			return writeComponentFailure(stdout, "component show requires component id")
		}
		record, ok := componentRecordByID(catalog, opts.commandArgs[1])
		if !ok {
			return writeReportFailure(stdout, "component", reports.Issue{Code: components.CodeComponentNotFound, Severity: reports.SeverityError, Path: "component.show", Message: "component not found: " + opts.commandArgs[1]})
		}
		return writeComponentResult(stdout, record, catalog.Diagnostics)
	case "find":
		query := componentQueryFromOptions(opts)
		query.MinimumConfidence = minimumConfidenceForAcceptance(components.AcceptanceLevel(opts.componentAcceptance))
		candidates, result := components.Find(ctx, catalog, query)
		result.Data = map[string]any{"candidates": candidates}
		return writeComponentReport(stdout, result)
	case "select":
		request, err := componentSelectionRequestFromOptions(opts)
		if err != nil {
			return writeReportFailure(stdout, "component", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: err.Error()})
		}
		request.Sources = sources
		selection, result := components.Select(ctx, catalog, request)
		if result.OK {
			result.Data = selection
		}
		result.Issues = append(sourceIssues, result.Issues...)
		return writeComponentReport(stdout, result)
	case "validate":
		result := components.ValidateCatalog(catalog)
		if sources != nil {
			result.Issues = append(sourceIssues, result.Issues...)
			data := map[string]any{
				"family_count": len(catalog.Families),
				"record_count": len(catalog.Records),
			}
			if existing, ok := result.Data.(map[string]any); ok {
				for key, value := range existing {
					data[key] = value
				}
			}
			data["source_record_count"] = len(sources.Records)
			result.Data = data
		}
		return writeComponentReport(stdout, result)
	case "coverage":
		_, result := components.ComponentCoverage(catalog, components.CoverageOptions{Sources: sources})
		result.Issues = append(sourceIssues, result.Issues...)
		return writeComponentReport(stdout, result)
	default:
		return writeReportFailure(stdout, "component", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "component." + subcommand,
			Message:  "unsupported component subcommand " + subcommand,
		})
	}
}

func componentSubcommandUsesSources(subcommand string) bool {
	switch subcommand {
	case "select", "validate", "coverage":
		return true
	default:
		return false
	}
}

func minimumConfidenceForAcceptance(level components.AcceptanceLevel) components.ConfidenceLevel {
	switch level {
	case components.AcceptanceStructural:
		return components.ConfidenceRuleInferred
	case components.AcceptanceConnectivity, components.AcceptanceERCDRC, components.AcceptanceFabricationCandidate:
		return components.ConfidenceVerified
	default:
		return ""
	}
}

func loadComponentSources(ctx context.Context, sourceDir string) (*components.SourceCollection, []reports.Issue, error) {
	if strings.TrimSpace(sourceDir) == "" {
		return nil, nil, nil
	}
	sources, err := components.LoadSources(ctx, components.SourceLoadOptions{SourceDir: sourceDir})
	if err != nil {
		return nil, nil, err
	}
	if sources == nil {
		return nil, nil, fmt.Errorf("component source loader returned nil collection")
	}
	return sources, append([]reports.Issue(nil), sources.Diagnostics...), nil
}

func componentQueryFromOptions(opts cliOptions) components.Query {
	return components.Query{
		Family:    opts.componentFamily,
		Package:   opts.componentPackage,
		ValueKind: opts.componentValueKind,
		Value:     opts.componentValue,
	}
}

func componentSelectionRequestFromOptions(opts cliOptions) (components.SelectionRequest, error) {
	if strings.TrimSpace(opts.requestPath) != "" {
		var request components.SelectionRequest
		body, err := os.ReadFile(opts.requestPath)
		if err != nil {
			return components.SelectionRequest{}, err
		}
		if err := json.Unmarshal(body, &request); err != nil {
			return components.SelectionRequest{}, err
		}
		return request, nil
	}
	return components.SelectionRequest{
		Query:      componentQueryFromOptions(opts),
		Acceptance: components.AcceptanceLevel(opts.componentAcceptance),
	}, nil
}

func componentRecordByID(catalog *components.Catalog, id string) (components.ComponentRecord, bool) {
	if catalog == nil {
		return components.ComponentRecord{}, false
	}
	for _, record := range catalog.Records {
		if record.ID == id {
			return record, true
		}
	}
	return components.ComponentRecord{}, false
}

func writeComponentFailure(stdout io.Writer, message string) error {
	return writeReportFailure(stdout, "component", reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: reports.SeverityError,
		Path:     "component",
		Message:  message,
	})
}

func writeComponentResult(stdout io.Writer, data any, issues []reports.Issue) error {
	return writeComponentReport(stdout, reports.ResultWithIssues("component", data, issues, nil))
}

func writeComponentReport(stdout io.Writer, result reports.Result) error {
	result.Command = "component"
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("component command failed")
	}
	return nil
}

func missingLibraryRecordIssue(path string, id string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeMissingFile,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  "library record not found: " + id,
	}
}

func hasBlockingIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
}

func libraryRootsFromOptions(opts cliOptions) libraryresolver.LibraryRoots {
	roots := libraryresolver.LibraryRoots{
		KLCRoot:        os.Getenv(libraryresolver.EnvKLCRoot),
		SymbolsRoot:    os.Getenv(libraryresolver.EnvSymbolsRoot),
		FootprintsRoot: os.Getenv(libraryresolver.EnvFootprintsRoot),
		TemplatesRoot:  os.Getenv(libraryresolver.EnvTemplatesRoot),
	}
	if strings.TrimSpace(opts.klcRoot) != "" {
		roots.KLCRoot = opts.klcRoot
	}
	if strings.TrimSpace(opts.symbolsRoot) != "" {
		roots.SymbolsRoot = opts.symbolsRoot
	}
	if strings.TrimSpace(opts.footprintsRoot) != "" {
		roots.FootprintsRoot = opts.footprintsRoot
	}
	if strings.TrimSpace(opts.templatesRoot) != "" {
		roots.TemplatesRoot = opts.templatesRoot
	}
	return roots
}

func runInspect(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("inspect requires --format json")
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
		return fmt.Errorf("evaluate requires --format json")
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

func runExport(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("export requires --format json")
	}
	if issue, ok := validateStructuredCommandArgs("export", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("export", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	if len(opts.commandArgs) != 2 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "export",
			Message:  "export requires exactly a subcommand and target path",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("export", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	options := fabrication.Options{
		Command:                "export " + opts.commandArgs[0],
		Execute:                opts.execute,
		Overwrite:              opts.overwrite,
		Output:                 opts.output,
		KiCadCLI:               opts.kicadCLI,
		CLIPolicy:              exportCLIPolicy(opts),
		ManufacturerProfile:    opts.manufacturerProfile,
		ManufacturerProfileDir: opts.manufacturerProfileDir,
		SourceDir:              opts.sourceDir,
	}
	if strings.TrimSpace(opts.sourceDir) != "" {
		sources, err := components.LoadSources(ctx, components.SourceLoadOptions{SourceDir: opts.sourceDir})
		if err != nil {
			issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "fabrication.source_dir", Message: err.Error()}
			if err := writeReportJSON(stdout, reports.ErrorResult("export", issue)); err != nil {
				return err
			}
			return fmt.Errorf("load component sources: %w", err)
		}
		if sources == nil {
			issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "fabrication.source_dir", Message: "component source loader returned nil collection"}
			if err := writeReportJSON(stdout, reports.ErrorResult("export", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		options.Sources = sources
	}
	target := opts.commandArgs[1]
	var data fabrication.Result
	switch opts.commandArgs[0] {
	case "preview":
		data = fabrication.ExportPreview(ctx, target, options)
	case "bom":
		data = fabrication.ExportBOM(ctx, target, options)
	case "fabrication":
		data = fabrication.ExportPackage(ctx, target, options)
	default:
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "export." + opts.commandArgs[0],
			Message:  "unsupported export subcommand " + opts.commandArgs[0],
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("export", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	result := reports.ResultWithIssues("export", data, data.Issues, fabrication.ReportArtifacts(data.Artifacts))
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("export reported blocking issues")
	}
	return nil
}

func exportCLIPolicy(opts cliOptions) fabrication.CLIPolicy {
	if opts.requireDRC {
		return fabrication.CLIPolicyRequired
	}
	if strings.TrimSpace(opts.kicadCLI) != "" {
		return fabrication.CLIPolicyOptional
	}
	return fabrication.CLIPolicyDisabled
}

type fabricationProfileListResult struct {
	Profiles []fabricationProfileSummary `json:"profiles"`
}

type fabricationProfileSummary struct {
	ID         string                       `json:"id"`
	Name       string                       `json:"name"`
	Version    string                       `json:"version"`
	Source     fabricationprofiles.Source   `json:"source,omitempty"`
	Hash       string                       `json:"hash"`
	Thresholds fabricationProfileThresholds `json:"thresholds"`
	Issues     []reports.Issue              `json:"issues,omitempty"`
}

type fabricationProfileShowResult struct {
	Summary fabricationProfileSummary   `json:"summary"`
	Profile fabricationprofiles.Profile `json:"profile"`
}

type fabricationProfileValidateResult struct {
	Path    string                      `json:"path"`
	Summary fabricationProfileSummary   `json:"summary"`
	Profile fabricationprofiles.Profile `json:"profile,omitempty"`
}

type fabricationProfileThresholds struct {
	LayerCounts             []int   `json:"layer_counts,omitempty"`
	MinTraceWidthMM         float64 `json:"min_trace_width_mm,omitempty"`
	MinSpacingMM            float64 `json:"min_spacing_mm,omitempty"`
	MinCopperToEdgeMM       float64 `json:"min_copper_to_edge_mm,omitempty"`
	MinPadAnnularRingMM     float64 `json:"min_pad_annular_ring_mm,omitempty"`
	MinViaAnnularRingMM     float64 `json:"min_via_annular_ring_mm,omitempty"`
	MinSolderMaskWebMM      float64 `json:"min_solder_mask_web_mm,omitempty"`
	RequireCourtyards       bool    `json:"require_courtyards"`
	RequireFabricationNotes bool    `json:"require_fabrication_notes"`
	AllowCastellations      bool    `json:"allow_castellations"`
	RequiresManualReview    bool    `json:"requires_manual_review"`
}

func runFabrication(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("fabrication requires --format json")
	}
	if len(opts.commandArgs) < 2 || opts.commandArgs[0] != "profile" {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "fabrication.profile",
			Message:  "fabrication requires a profile subcommand: list, show, or validate",
		}
		return writeReportFailure(stdout, "fabrication", issue)
	}
	switch opts.commandArgs[1] {
	case "list":
		if len(opts.commandArgs) != 2 {
			return writeReportFailure(stdout, "fabrication", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "fabrication.profile.list", Message: "fabrication profile list does not accept positional arguments"})
		}
		registry, issues := fabricationprofiles.LoadRegistry(fabricationprofiles.LoadOptions{ProfileDir: opts.manufacturerProfileDir})
		data := fabricationProfileListResult{Profiles: profileSummaries(registry)}
		result := reports.ResultWithIssues("fabrication", data, issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("fabrication profile list reported blocking issues")
		}
		return nil
	case "show":
		if len(opts.commandArgs) != 3 {
			return writeReportFailure(stdout, "fabrication", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "fabrication.profile.show", Message: "fabrication profile show requires exactly one profile ID"})
		}
		registry, loadIssues := fabricationprofiles.LoadRegistry(fabricationprofiles.LoadOptions{ProfileDir: opts.manufacturerProfileDir})
		profile, resolveIssues := registry.Resolve(opts.commandArgs[2])
		issues := append(loadIssues, resolveIssues...)
		data := fabricationProfileShowResult{Summary: profileSummary(fabricationprofiles.Summarize(profile), profile), Profile: profile}
		result := reports.ResultWithIssues("fabrication", data, issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("fabrication profile show reported blocking issues")
		}
		return nil
	case "validate":
		if len(opts.commandArgs) != 3 {
			return writeReportFailure(stdout, "fabrication", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "fabrication.profile.validate", Message: "fabrication profile validate requires exactly one profile JSON path"})
		}
		data, issues := validateFabricationProfilePath(opts.commandArgs[2])
		result := reports.ResultWithIssues("fabrication", data, issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("fabrication profile validate reported blocking issues")
		}
		return nil
	default:
		return writeReportFailure(stdout, "fabrication", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "fabrication.profile." + opts.commandArgs[1], Message: "unsupported fabrication profile subcommand " + opts.commandArgs[1]})
	}
}

func profileSummaries(registry fabricationprofiles.Registry) []fabricationProfileSummary {
	summaries := registry.List()
	out := make([]fabricationProfileSummary, 0, len(summaries))
	for _, summary := range summaries {
		profile, resolveIssues := registry.Resolve(summary.ID)
		item := profileSummary(summary, profile)
		item.Issues = append(item.Issues, resolveIssues...)
		out = append(out, item)
	}
	return out
}

func profileSummary(summary fabricationprofiles.Summary, profile fabricationprofiles.Profile) fabricationProfileSummary {
	return fabricationProfileSummary{
		ID:      summary.ID,
		Name:    summary.Name,
		Version: summary.Version,
		Source:  summary.Source,
		Hash:    summary.Hash,
		Thresholds: fabricationProfileThresholds{
			LayerCounts:             slices.Clone(profile.Stackup.AllowedLayerCounts),
			MinTraceWidthMM:         profile.Copper.MinTraceWidthMM,
			MinSpacingMM:            profile.Copper.MinSpacingMM,
			MinCopperToEdgeMM:       profile.Copper.MinCopperToEdgeMM,
			MinPadAnnularRingMM:     profile.Drill.MinPadAnnularRingMM,
			MinViaAnnularRingMM:     profile.Drill.MinViaAnnularRingMM,
			MinSolderMaskWebMM:      profile.SolderMask.MinSolderMaskWebMM,
			RequireCourtyards:       profile.Assembly.RequireCourtyards,
			RequireFabricationNotes: profile.Metadata.RequireFabricationNotes,
			AllowCastellations:      profile.EdgePlating.AllowCastellations,
			RequiresManualReview:    profile.EdgePlating.RequiresManualReview,
		},
		Issues: summary.Issues,
	}
}

func validateFabricationProfilePath(path string) (fabricationProfileValidateResult, []reports.Issue) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fabricationProfileValidateResult{Path: path}, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "fabrication.profile.validate", Message: err.Error()}}
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return fabricationProfileValidateResult{Path: filepath.ToSlash(absolute)}, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "fabrication.profile.validate", Message: err.Error()}}
	}
	var profile fabricationprofiles.Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return fabricationProfileValidateResult{Path: filepath.ToSlash(absolute)}, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "fabrication.profile.validate", Message: "invalid fabrication profile JSON: " + err.Error()}}
	}
	profile.Source.Kind = fabricationprofiles.SourceLocal
	profile.Source.Path = filepath.ToSlash(absolute)
	summary := fabricationprofiles.Summarize(profile)
	return fabricationProfileValidateResult{Path: filepath.ToSlash(absolute), Summary: profileSummary(summary, profile), Profile: profile}, summary.Issues
}

func runPinmap(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("pinmap requires --format json")
	}
	if issue, ok := validateStructuredCommandArgs("pinmap", opts.commandArgs); !ok {
		if err := writeReportJSON(stdout, reports.ErrorResult("pinmap", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	switch opts.commandArgs[0] {
	case "list":
		return writeReportJSON(stdout, reports.OKResult("pinmap", pinmap.Builtins(), nil))
	case "validate":
		target := opts.commandArgs[1]
		validateOptions := pinmap.ValidateOptions{}
		if pinmapShouldUseLibraryResolver(opts) {
			index, issues := libraryresolver.Load(context.Background(), libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
				CachePath: opts.libraryCache,
				Refresh:   opts.refreshLibraryCache,
			})
			validateOptions.LibraryIndex = &index
			validateOptions.LibraryIssues = issues
		}
		report, err := pinmap.ValidateProjectWithOptions(target, validateOptions)
		if err != nil {
			issue := reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Path:     "pinmap.validate",
				Message:  err.Error(),
			}
			if errors.Is(err, os.ErrNotExist) {
				issue.Code = reports.CodeMissingFile
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("pinmap", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		result := reports.ResultWithIssues("pinmap", report, report.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("pinmap validation failed")
		}
		return nil
	default:
		return writeReportFailure(stdout, "pinmap", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "pinmap." + opts.commandArgs[0],
			Message:  "unsupported pinmap subcommand " + opts.commandArgs[0],
		})
	}
}

func pinmapShouldUseLibraryResolver(opts cliOptions) bool {
	roots := libraryRootsFromOptions(opts)
	return strings.TrimSpace(roots.SymbolsRoot) != "" || strings.TrimSpace(roots.FootprintsRoot) != "" || strings.TrimSpace(opts.libraryCache) != ""
}

func runPlace(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("place requires --format json")
	}
	if len(opts.commandArgs) == 0 {
		return writeReportFailure(stdout, "place", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "place",
			Message:  "place requires a subcommand",
		})
	}
	switch opts.commandArgs[0] {
	case "request":
		if opts.requestPath == "" {
			return writeReportFailure(stdout, "place", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "place.request",
				Message:  "place request requires --request",
			})
		}
		file, err := os.Open(opts.requestPath)
		if err != nil {
			return writeReportFailure(stdout, "place", reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityError,
				Path:     opts.requestPath,
				Message:  err.Error(),
			})
		}
		defer file.Close()
		var request placement.Request
		if err := json.NewDecoder(file).Decode(&request); err != nil {
			return writeReportFailure(stdout, "place", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     opts.requestPath,
				Message:  err.Error(),
			})
		}
		result := placement.Place(request)
		quality := placement.BuildQualityReport(request, result)
		result.Quality = &quality
		return writeReportJSON(stdout, reports.ResultWithIssues("place", result, result.Issues, nil))
	default:
		return writeReportFailure(stdout, "place", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "place." + opts.commandArgs[0],
			Message:  "unsupported place subcommand " + opts.commandArgs[0],
		})
	}
}

func runRoute(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("route requires --format json")
	}
	if len(opts.commandArgs) == 0 {
		return writeReportFailure(stdout, "route", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "route",
			Message:  "route requires a subcommand",
		})
	}
	switch opts.commandArgs[0] {
	case "request":
		if opts.requestPath == "" {
			return writeReportFailure(stdout, "route", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "route.request",
				Message:  "route request requires --request",
			})
		}
		file, err := os.Open(opts.requestPath)
		if err != nil {
			return writeReportFailure(stdout, "route", reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityError,
				Path:     opts.requestPath,
				Message:  err.Error(),
			})
		}
		defer file.Close()
		var request routing.Request
		if err := json.NewDecoder(file).Decode(&request); err != nil {
			return writeReportFailure(stdout, "route", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     opts.requestPath,
				Message:  err.Error(),
			})
		}
		if err := applyRouteOverrides(&request, opts); err != nil {
			return writeReportFailure(stdout, "route", reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "route",
				Message:  err.Error(),
			})
		}
		result := routing.RouteRequest(request)
		report := reports.ResultWithIssues("route", result, result.Issues, nil)
		if opts.prettyOutput {
			return writePrettyReportJSON(stdout, report)
		}
		return writeReportJSON(stdout, report)
	default:
		return writeReportFailure(stdout, "route", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "route." + opts.commandArgs[0],
			Message:  "unsupported route subcommand " + opts.commandArgs[0],
		})
	}
}

func applyRouteOverrides(request *routing.Request, opts cliOptions) error {
	if opts.routeMode != "" {
		switch routing.RouteMode(opts.routeMode) {
		case routing.ModeSingleLayer, routing.ModeTwoLayer, routing.ModeValidateOnly:
		default:
			return fmt.Errorf("unsupported route mode %q", opts.routeMode)
		}
		request.Strategy.Mode = routing.RouteMode(opts.routeMode)
	}
	if opts.routeGridMM > 0 {
		request.Rules.GridMM = opts.routeGridMM
	}
	if opts.routeTraceWidthMM > 0 {
		request.Rules.TraceWidthMM = opts.routeTraceWidthMM
	}
	if opts.routeClearanceMM > 0 {
		request.Rules.ClearanceMM = opts.routeClearanceMM
	}
	if opts.routeAllowPartialSet {
		request.Strategy.AllowPartial = opts.routeAllowPartial
	}
	return nil
}

type checkCommandReport struct {
	Target string               `json:"target"`
	Checks []checks.CheckResult `json:"checks"`
}

func runCheckCommand(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("check requires --format json")
	}
	if issue, ok := validateStructuredCommandArgs("check", opts.commandArgs); !ok {
		return writeReportFailure(stdout, "check", issue)
	}
	if len(opts.commandArgs) < 2 {
		return writeReportFailure(stdout, "check", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "check",
			Message:  "check requires a subcommand and target path",
		})
	}
	kind := opts.commandArgs[0]
	target := opts.commandArgs[1]
	checkOpts, err := checkOptions(opts)
	if err != nil {
		return writeReportFailure(stdout, "check", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "check.options",
			Message:  err.Error(),
		})
	}
	cli, skippedIssue, err := checkCLI(opts)
	if err != nil {
		return writeReportFailure(stdout, "check", reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     "check.kicad_cli",
			Message:  err.Error(),
		})
	}
	if skippedIssue != nil {
		report := checkCommandReport{Target: filepath.ToSlash(target), Checks: []checks.CheckResult{{
			TargetPath: filepath.ToSlash(target),
			Status:     checks.CheckStatusSkipped,
		}}}
		result := reports.ResultWithIssues("check", report, []reports.Issue{*skippedIssue}, nil)
		return writeReportJSON(stdout, result)
	}

	report := checkCommandReport{Target: filepath.ToSlash(target), Checks: []checks.CheckResult{}}
	var issues []reports.Issue
	var artifacts []reports.Artifact
	switch kind {
	case "erc":
		check, checkIssues, resultArtifacts := runCheckERC(ctx, cli, target, checkOpts)
		report.Checks = append(report.Checks, check)
		issues = append(issues, checkIssues...)
		artifacts = append(artifacts, resultArtifacts...)
	case "drc":
		check, checkIssues, resultArtifacts := runCheckDRC(ctx, cli, target, checkOpts)
		report.Checks = append(report.Checks, check)
		issues = append(issues, checkIssues...)
		artifacts = append(artifacts, resultArtifacts...)
	case "project":
		erc, ercIssues, ercArtifacts := runCheckERC(ctx, cli, target, checkOpts)
		report.Checks = append(report.Checks, erc)
		issues = append(issues, ercIssues...)
		artifacts = append(artifacts, ercArtifacts...)
		if err := ctx.Err(); err != nil {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeKiCadCLIFailed,
				Severity: reports.SeverityError,
				Path:     "check.project",
				Message:  err.Error(),
			})
			break
		}
		drc, drcIssues, drcArtifacts := runCheckDRC(ctx, cli, target, checkOpts)
		report.Checks = append(report.Checks, drc)
		issues = append(issues, drcIssues...)
		artifacts = append(artifacts, drcArtifacts...)
	default:
		return writeReportFailure(stdout, "check", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "check." + kind,
			Message:  "unsupported check subcommand " + kind,
		})
	}
	result := reports.ResultWithIssues("check", report, issues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("check reported blocking issues")
	}
	return nil
}

func runValidateCommand(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) == 0 {
		return writeReportFailure(stdout, "validate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "validate",
			Message:  "validate requires a subcommand",
		})
	}
	if opts.commandArgs[0] != "board" {
		return writeReportFailure(stdout, "validate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "validate." + opts.commandArgs[0],
			Message:  "unsupported validate subcommand " + opts.commandArgs[0],
		})
	}
	if len(opts.commandArgs) != 2 {
		return writeReportFailure(stdout, "validate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "validate.board",
			Message:  "validate board requires 1 argument",
		})
	}
	validationOpts, err := boardValidationOptions(opts)
	if err != nil {
		return writeReportFailure(stdout, "validate", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "validate.options",
			Message:  err.Error(),
		})
	}
	report := boardvalidation.Validate(ctx, opts.commandArgs[1], validationOpts)
	result := reports.ResultWithIssues("validate", report, report.Issues, report.Artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("validate reported blocking issues")
	}
	return nil
}

func runWriterCommand(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) == 0 {
		if err := writeReportFailure(stdout, "writer", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "writer",
			Message:  "writer requires a subcommand",
		}); err != nil {
			return err
		}
		return errors.New("writer requires a subcommand")
	}
	if opts.commandArgs[0] != "check" {
		if err := writeReportFailure(stdout, "writer", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "writer." + opts.commandArgs[0],
			Message:  "unsupported writer subcommand " + opts.commandArgs[0],
		}); err != nil {
			return err
		}
		return errors.New("unsupported writer subcommand " + opts.commandArgs[0])
	}
	if len(opts.commandArgs) != 2 {
		if err := writeReportFailure(stdout, "writer", reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "writer.check",
			Message:  "writer check requires 1 argument",
		}); err != nil {
			return err
		}
		return errors.New("writer check requires 1 argument")
	}
	wcOpts := writerCorrectnessOptions(opts)
	var libraryIssues []reports.Issue
	if pinmapShouldUseLibraryResolver(opts) {
		wcOpts.LibraryResolutionUsed = true
		index, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
			CachePath: opts.libraryCache,
			Refresh:   opts.refreshLibraryCache,
		})
		libraryIssues = issues
		if !reports.HasBlockingIssue(issues) {
			wcOpts.LibraryIndex = index
			wcOpts.HasLibraryIndex = true
		}
	}
	wcOpts.LibraryIssues = libraryIssues
	result := writercorrectness.Validate(ctx, opts.commandArgs[1], wcOpts)
	report := result.ReportResult("writer")
	if err := writeReportJSON(stdout, report); err != nil {
		return err
	}
	if !report.OK {
		return errors.New("writer check reported blocking issues")
	}
	return nil
}

func writerCorrectnessOptions(opts cliOptions) writercorrectness.Options {
	return writercorrectness.Options{
		RequireKiCadRoundTrip: opts.requireKiCadRoundTrip,
		KiCadCLI:              opts.kicadCLI,
		KeepArtifacts:         opts.keepArtifacts,
		ArtifactDir:           opts.artifactDir,
		StrictDiffs:           opts.strictDiffs,
		AllowUnrouted:         opts.allowUnrouted,
	}
}

func runDesign(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) == 0 {
		return writeDesignFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "design",
			Message:  "design requires a subcommand",
		})
	}
	if opts.commandArgs[0] != "create" {
		return writeDesignFailure(stdout, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "design." + opts.commandArgs[0],
			Message:  "unsupported design subcommand " + opts.commandArgs[0],
		})
	}
	return runDesignCreate(ctx, opts, stdout)
}

type intentCreateResult struct {
	Plan      intentplanner.PlanResult      `json:"plan"`
	Workflow  designworkflow.WorkflowResult `json:"workflow,omitempty"`
	Draft     *intentDraftOutput            `json:"draft,omitempty"`
	Rationale *rationale.Report             `json:"rationale,omitempty"`
	AIStatus  *aiLaneStatus                 `json:"ai_status,omitempty"`
}

type aiLaneStatusValue string

const (
	aiLaneStatusReady              aiLaneStatusValue = "ready"
	aiLaneStatusCandidate          aiLaneStatusValue = "candidate"
	aiLaneStatusBlocked            aiLaneStatusValue = "blocked"
	aiLaneStatusNeedsClarification aiLaneStatusValue = "needs_clarification"
	aiLaneStatusUnsupported        aiLaneStatusValue = "unsupported"
	aiLaneStatusToolError          aiLaneStatusValue = "tool_error"
)

type aiLaneStatus struct {
	Status                    aiLaneStatusValue `json:"status"`
	Stage                     string            `json:"stage"`
	IssueCode                 reports.Code      `json:"issue_code,omitempty"`
	Message                   string            `json:"message"`
	Detail                    string            `json:"detail,omitempty"`
	ArtifactPaths             []string          `json:"artifact_paths,omitempty"`
	RepairCategory            repair.Category   `json:"repair_category,omitempty"`
	RepairBundlePath          string            `json:"repair_bundle_path,omitempty"`
	SuggestedNextAction       string            `json:"suggested_next_action,omitempty"`
	RetryAllowed              bool              `json:"retry_allowed"`
	RetryKey                  string            `json:"retry_key,omitempty"`
	MaxAutomaticRetryAttempts int               `json:"max_automatic_retry_attempts"`
	UserClarificationRequired bool              `json:"user_clarification_required"`
}

func runIntent(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) == 0 {
		return writeReportFailure(stdout, "intent", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "intent", Message: "intent requires subcommand: plan or explain"})
	}
	switch opts.commandArgs[0] {
	case "draft":
		return runIntentDraft(opts, stdout)
	case "plan", "explain":
		return runIntentPlan(ctx, opts, stdout, opts.commandArgs[0])
	case "rationale":
		return runIntentRationale(ctx, opts, stdout)
	case "create":
		return runIntentCreate(ctx, opts, stdout)
	default:
		return writeReportFailure(stdout, "intent", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "intent." + opts.commandArgs[0], Message: "unsupported intent subcommand " + opts.commandArgs[0]})
	}
}

type intentDraftOutput struct {
	Request        intentplanner.Request        `json:"request"`
	Extraction     intentdraft.ExtractionReport `json:"extraction"`
	Clarifications []intentdraft.Clarification  `json:"clarifications,omitempty"`
}

func runIntentDraft(opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("intent draft requires --format json")
	}
	text, sourceType, sourceID, inputIssues := loadIntentDraftText(opts)
	if len(inputIssues) > 0 {
		result := reports.ResultWithIssues("intent", nil, inputIssues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		return errors.New("intent draft reported input issues")
	}
	draft := intentdraft.Draft(text, intentdraft.Options{SourceType: sourceType, SourceID: sourceID, AcceptanceOverride: parseIntentAcceptance(opts.componentAcceptance), Strict: opts.strictDraft})
	artifactIssues := writeIntentDraftArtifacts(opts.output, text, draft, opts.overwrite)
	issues := append([]reports.Issue(nil), draft.Issues...)
	issues = append(issues, artifactIssues...)
	output := intentDraftOutput{Request: draft.Request, Extraction: draft.Extraction, Clarifications: draft.Clarifications}
	result := reports.ResultWithIssues("intent", output, issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("intent draft reported issues")
	}
	return nil
}

func loadIntentDraftText(opts cliOptions) (string, string, string, []reports.Issue) {
	hasText := strings.TrimSpace(opts.intentText) != ""
	hasFile := strings.TrimSpace(opts.intentFile) != ""
	if hasText == hasFile {
		return "", "", "", []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "text", Message: "provide exactly one of --text or --file"}}
	}
	if hasText {
		return opts.intentText, intentdraft.SourceTypeText, "text", nil
	}
	data, err := os.ReadFile(opts.intentFile)
	if err != nil {
		return "", "", "", []reports.Issue{{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: opts.intentFile, Message: err.Error()}}
	}
	return string(data), intentdraft.SourceTypeFile, opts.intentFile, nil
}

func writeIntentDraftArtifacts(outputDir string, source string, draft intentdraft.Result, overwrite bool) []reports.Issue {
	if strings.TrimSpace(outputDir) == "" {
		return nil
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: err.Error()}}
	}
	files := []struct {
		name string
		data any
	}{
		{name: "intent-draft.json", data: draft.Request},
		{name: "intent-extraction.json", data: draft.Extraction},
		{name: "intent-clarifications.json", data: draft.Clarifications},
	}
	var issues []reports.Issue
	if issue := writeLocalArtifact(filepath.Join(outputDir, "intent-source.txt"), []byte(source+"\n"), overwrite); issue != nil {
		issues = append(issues, *issue)
	}
	for _, file := range files {
		data, err := json.MarshalIndent(file.data, "", "  ")
		if err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: file.name, Message: err.Error()})
			continue
		}
		if issue := writeLocalArtifact(filepath.Join(outputDir, file.name), append(data, '\n'), overwrite); issue != nil {
			issues = append(issues, *issue)
		}
	}
	return issues
}

func writeLocalArtifact(path string, data []byte, overwrite bool) *reports.Issue {
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: path, Message: err.Error()}
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: path, Message: err.Error()}
	}
	return nil
}

func parseIntentAcceptance(value string) designworkflow.AcceptanceLevel {
	return designworkflow.AcceptanceLevel(strings.TrimSpace(value))
}

type intentExplainResult struct {
	Status         intentplanner.PlanStatus            `json:"status"`
	Score          int                                 `json:"score"`
	Intent         intentplanner.PlanIntentSummary     `json:"intent"`
	Requirements   []intentplanner.RequirementRecord   `json:"requirements"`
	SelectedBlocks []intentplanner.SelectedBlockRecord `json:"selected_blocks"`
	Connections    []intentplanner.ConnectionRecord    `json:"connections"`
	Assumptions    []intentplanner.PlanNote            `json:"assumptions"`
	Clarifications []intentplanner.PlanNote            `json:"clarifications"`
	KnownGaps      []intentplanner.PlanNote            `json:"known_gaps"`
	Draft          *intentDraftOutput                  `json:"draft,omitempty"`
}

func runIntentRationale(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !opts.jsonOutput {
		return fmt.Errorf("intent rationale requires --format json")
	}
	report, issues, artifacts, err := buildIntentRationale(ctx, opts)
	if err != nil {
		return err
	}
	result := reports.ResultWithIssues("intent", report, issues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("intent rationale reported issues")
	}
	return nil
}

func buildIntentRationale(ctx context.Context, opts cliOptions) (rationale.Report, []reports.Issue, []reports.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return rationale.Report{}, nil, nil, err
	}
	mode, issue := intentRationaleMode(opts)
	if issue != nil {
		report := rationale.Build(rationale.BuildOptions{Source: rationale.SourceSummary{Mode: "invalid"}})
		return report, []reports.Issue{*issue}, nil, nil
	}
	switch mode {
	case "target":
		loaded := rationale.LoadFromTarget(opts.target)
		artifact, writeIssue := rationale.WriteArtifact(rationale.ReportPathForTarget(opts.target), loaded.Report)
		issues := append([]reports.Issue(nil), loaded.Issues...)
		var artifacts []reports.Artifact
		if writeIssue != nil {
			issues = append(issues, *writeIssue)
		} else {
			artifact.Path = filepath.Join(rationale.MetadataDirName, rationale.ArtifactName)
			artifacts = append(artifacts, artifact)
		}
		return loaded.Report, issues, artifacts, nil
	case "request":
		plan, issues, err := loadIntentPlan(opts)
		if err != nil {
			return rationale.Report{}, nil, nil, err
		}
		source := rationale.SourceSummary{Mode: "request", Path: opts.requestPath}
		report := rationale.BuildFromPlan(plan, source)
		return writeOptionalRationaleOutput(opts, report, issues)
	case "text", "file":
		text, sourceType, sourceID, inputIssues := loadIntentDraftText(opts)
		if len(inputIssues) > 0 {
			report := rationale.Build(rationale.BuildOptions{Source: rationale.SourceSummary{Mode: mode, Path: sourceID}})
			return report, inputIssues, nil, nil
		}
		draft := intentdraft.Draft(text, intentdraft.Options{SourceType: sourceType, SourceID: sourceID, AcceptanceOverride: parseIntentAcceptance(opts.componentAcceptance), Strict: false})
		source := rationale.SourceSummary{Mode: sourceType, Path: sourceID, SourceHash: draft.Extraction.SourceHash, Summary: draft.Extraction.Summary}
		var plan intentplanner.PlanResult
		if !intentdraft.BlockingClarifications(draft.Clarifications) {
			plan = intentplanner.Plan(draft.Request)
		} else {
			plan = intentplanner.PlanResult{
				Schema: intentplanner.PlanSchema,
				Status: intentplanner.PlanStatusNeedsClarification,
				Intent: intentplanner.PlanIntentSummary{
					Name:       draft.Request.Name,
					Kind:       draft.Request.Kind,
					Acceptance: draft.Request.Acceptance,
					Summary:    draft.Request.Summary,
				},
			}
		}
		report := rationale.BuildFromDraftAndPlan(draft, plan, source)
		issues := append([]reports.Issue(nil), draft.Issues...)
		issues = append(issues, plan.Issues...)
		for _, clarification := range draft.Clarifications {
			if clarification.Severity == intentdraft.ClarificationBlocking {
				issues = append(issues, clarification.Issue())
			}
		}
		return writeOptionalRationaleOutput(opts, report, issues)
	default:
		return rationale.Report{}, nil, nil, fmt.Errorf("unsupported rationale mode %q", mode)
	}
}

func intentRationaleMode(opts cliOptions) (string, *reports.Issue) {
	var modes []string
	if strings.TrimSpace(opts.requestPath) != "" {
		modes = append(modes, "request")
	}
	if strings.TrimSpace(opts.intentText) != "" {
		modes = append(modes, "text")
	}
	if strings.TrimSpace(opts.intentFile) != "" {
		modes = append(modes, "file")
	}
	if strings.TrimSpace(opts.target) != "" {
		modes = append(modes, "target")
	}
	if len(modes) != 1 {
		issue := reports.Issue{
			Code:       reports.CodeInvalidArgument,
			Severity:   reports.SeverityError,
			Path:       "source",
			Message:    "provide exactly one of --request, --text, --file, or --target",
			Suggestion: "choose one rationale source mode",
		}
		return "", &issue
	}
	return modes[0], nil
}

func writeOptionalRationaleOutput(opts cliOptions, report rationale.Report, issues []reports.Issue) (rationale.Report, []reports.Issue, []reports.Artifact, error) {
	var artifacts []reports.Artifact
	if strings.TrimSpace(opts.output) == "" {
		return report, issues, artifacts, nil
	}
	path := filepath.Join(opts.output, rationale.ArtifactName)
	artifact, writeIssue := rationale.WriteArtifact(path, report)
	if writeIssue != nil {
		issues = append(issues, *writeIssue)
	} else {
		artifacts = append(artifacts, artifact)
	}
	return report, issues, artifacts, nil
}

func persistCreateRationale(output string, plan intentplanner.PlanResult, draft *intentDraftOutput, workflow *designworkflow.WorkflowResult) (rationale.Report, []reports.Issue, []reports.Artifact) {
	source := rationale.SourceSummary{Mode: "request"}
	var draftResult *intentdraft.Result
	var request *intentplanner.Request
	if draft != nil {
		source.Mode = draft.Extraction.SourceType
		source.Path = draft.Extraction.SourceID
		source.SourceHash = draft.Extraction.SourceHash
		source.Summary = draft.Extraction.Summary
		d := intentdraft.Result{Request: draft.Request, Extraction: draft.Extraction, Clarifications: draft.Clarifications}
		draftResult = &d
		request = &draft.Request
	}
	normalizedPlan := intentplanner.NormalizePlan(plan)
	report := rationale.Build(rationale.BuildOptions{
		Source:   source,
		Request:  request,
		Draft:    draftResult,
		Plan:     &normalizedPlan,
		Workflow: workflow,
	})
	if strings.TrimSpace(output) == "" {
		return report, nil, nil
	}
	artifact, issue := rationale.WriteArtifact(rationale.ReportPathForTarget(output), report)
	if issue != nil {
		return report, []reports.Issue{*issue}, nil
	}
	artifact.Path = filepath.Join(rationale.MetadataDirName, rationale.ArtifactName)
	return report, nil, []reports.Artifact{artifact}
}

func writeWorkflowResultArtifact(outputDir string, workflow designworkflow.WorkflowResult) []reports.Issue {
	if strings.TrimSpace(outputDir) == "" {
		return nil
	}
	data, err := json.MarshalIndent(workflow, "", "  ")
	if err != nil {
		return []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "workflow-result.json", Message: err.Error()}}
	}
	if issue := writeLocalArtifact(filepath.Join(outputDir, "workflow-result.json"), append(data, '\n'), true); issue != nil {
		return []reports.Issue{*issue}
	}
	return nil
}

func writeAILaneArtifacts(outputDir string, plan intentplanner.PlanResult, draft *intentDraftOutput, sourceText string, status aiLaneStatus, artifacts []reports.Artifact) ([]reports.Artifact, []reports.Issue) {
	if strings.TrimSpace(outputDir) == "" {
		return nil, nil
	}
	artifactDir := filepath.Join(outputDir, ".kicadai")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, []reports.Issue{{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: err.Error()}}
	}
	var written []reports.Artifact
	var issues []reports.Issue
	if plan.GeneratedRequest != nil {
		if artifact, issue := writeJSONArtifact(filepath.Join(artifactDir, "design-request.json"), plan.GeneratedRequest, reports.ArtifactPreview, ".kicadai/design-request.json", "AI lane design workflow request"); issue != nil {
			issues = append(issues, *issue)
		} else {
			written = append(written, artifact)
		}
	}
	if artifact, issue := writeJSONArtifact(filepath.Join(artifactDir, "validation-summary.json"), status, reports.ArtifactValidationReport, ".kicadai/validation-summary.json", "AI lane status summary"); issue != nil {
		issues = append(issues, *issue)
	} else {
		written = append(written, artifact)
	}
	retryState := map[string]any{
		"retry_allowed":                   status.RetryAllowed,
		"retry_key":                       status.RetryKey,
		"max_automatic_retry_attempts":    status.MaxAutomaticRetryAttempts,
		"user_clarification_required":     status.UserClarificationRequired,
		"current_automatic_retry_attempt": aiLaneRetryAttempt(artifactDir, status.RetryKey),
	}
	if status.RepairCategory != "" {
		retryState["repair_category"] = status.RepairCategory
	}
	if status.RepairBundlePath != "" {
		retryState["repair_bundle_path"] = status.RepairBundlePath
	}
	if artifact, issue := writeJSONArtifact(filepath.Join(artifactDir, "retry-state.json"), retryState, reports.ArtifactValidationReport, ".kicadai/retry-state.json", "AI lane retry state"); issue != nil {
		issues = append(issues, *issue)
	} else {
		written = append(written, artifact)
	}
	manifestArtifacts := append([]reports.Artifact(nil), artifacts...)
	manifestArtifacts = append(manifestArtifacts, written...)
	if artifact, issue := writeAILaneManifest(outputDir, plan, draft, sourceText, status, manifestArtifacts); issue != nil {
		issues = append(issues, *issue)
	} else {
		written = append(written, artifact)
	}
	return written, issues
}

func writeJSONArtifact(path string, value any, kind reports.ArtifactKind, relPath string, description string) (reports.Artifact, *reports.Issue) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: relPath, Message: err.Error()}
	}
	if issue := writeLocalArtifact(path, append(data, '\n'), true); issue != nil {
		issue.Path = relPath
		return reports.Artifact{}, issue
	}
	return reports.Artifact{Kind: kind, Path: relPath, Description: description}, nil
}

func aiLaneRetryAttempt(artifactDir string, retryKey string) int {
	if strings.TrimSpace(retryKey) == "" {
		return 0
	}
	data, err := os.ReadFile(filepath.Join(artifactDir, "retry-state.json"))
	if err != nil {
		return 0
	}
	var previous struct {
		RetryKey                string `json:"retry_key"`
		CurrentAutomaticAttempt int    `json:"current_automatic_retry_attempt"`
	}
	if err := json.Unmarshal(data, &previous); err != nil {
		return 0
	}
	if previous.RetryKey != retryKey {
		return 0
	}
	return previous.CurrentAutomaticAttempt + 1
}

func writeAILaneManifest(outputDir string, plan intentplanner.PlanResult, draft *intentDraftOutput, sourceText string, status aiLaneStatus, artifacts []reports.Artifact) (reports.Artifact, *reports.Issue) {
	current, _, err := manifest.Read(outputDir)
	if err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: manifest.RelativePath, Message: "read existing manifest: " + err.Error()}
	}
	current.ProjectName = designworkflow.NormalizeProjectName(plan.Intent.Name)
	if current.ProjectName == "" && plan.GeneratedRequest != nil {
		current.ProjectName = designworkflow.NormalizeProjectName(plan.GeneratedRequest.Name)
	}
	current.GeneratorVersion = reports.Version
	current.Artifacts = mergeArtifacts(current.Artifacts, normalizeManifestArtifacts(outputDir, artifacts))
	current.AILane = &manifest.AILaneSummary{
		Status:         string(status.Status),
		Stage:          status.Stage,
		SourceHash:     aiLaneSourceHash(draft, sourceText),
		RequestHash:    aiLaneRequestHash(plan),
		RetryStatePath: ".kicadai/retry-state.json",
	}
	if err := os.MkdirAll(filepath.Join(outputDir, ".kicadai"), 0o755); err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: manifest.RelativePath, Message: err.Error()}
	}
	artifact, err := manifest.Write(outputDir, current)
	if err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: manifest.RelativePath, Message: err.Error()}
	}
	artifact.Path = manifest.RelativePath
	return artifact, nil
}

func mergeArtifacts(existing []reports.Artifact, incoming []reports.Artifact) []reports.Artifact {
	merged := make([]reports.Artifact, 0, len(existing)+len(incoming))
	seen := map[string]bool{}
	for _, group := range [][]reports.Artifact{incoming, existing} {
		for _, artifact := range group {
			path := filepath.ToSlash(strings.TrimSpace(artifact.Path))
			if path == "" || seen[path] {
				continue
			}
			artifact.Path = path
			seen[path] = true
			merged = append(merged, artifact)
		}
	}
	return merged
}

func normalizeManifestArtifacts(outputDir string, artifacts []reports.Artifact) []reports.Artifact {
	normalized := make([]reports.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		path := strings.TrimSpace(artifact.Path)
		if path == "" || filepath.IsAbs(path) {
			normalized = append(normalized, artifact)
			continue
		}
		if _, err := os.Stat(filepath.Join(outputDir, path)); err == nil {
			artifact.Path = filepath.ToSlash(path)
			normalized = append(normalized, artifact)
			continue
		}
		metaPath := filepath.ToSlash(filepath.Join(".kicadai", path))
		if _, err := os.Stat(filepath.Join(outputDir, metaPath)); err == nil {
			artifact.Path = metaPath
		}
		normalized = append(normalized, artifact)
	}
	return normalized
}

func aiLaneSourceHash(draft *intentDraftOutput, sourceText string) string {
	if draft != nil && strings.TrimSpace(draft.Extraction.SourceHash) != "" {
		return draft.Extraction.SourceHash
	}
	if strings.TrimSpace(sourceText) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sourceText))
	return fmt.Sprintf("%x", sum[:])
}

func aiLaneRequestHash(plan intentplanner.PlanResult) string {
	if plan.GeneratedRequest == nil {
		return ""
	}
	data, err := json.Marshal(plan.GeneratedRequest)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func buildAILaneStatus(plan intentplanner.PlanResult, workflow *designworkflow.WorkflowResult, issues []reports.Issue, artifacts []reports.Artifact) aiLaneStatus {
	repairBundlePath := repairBundleArtifactPath(artifacts)
	status := aiLaneStatus{
		Status:              aiLaneStatusReady,
		Stage:               "validation",
		Message:             "generated project satisfies the requested lane checks",
		ArtifactPaths:       artifactPaths(artifacts),
		RetryAllowed:        false,
		SuggestedNextAction: "review generated KiCad project and evidence artifacts",
	}
	if plan.Status == intentplanner.PlanStatusNeedsClarification {
		status.Status = aiLaneStatusNeedsClarification
		status.Stage = "plan"
		status.Message = "user clarification is required before generation can continue"
		status.SuggestedNextAction = "ask the user for the missing design choice"
		status.UserClarificationRequired = true
		issue, ok := firstBlockingIssue(issues, plan.Issues, nil)
		fillAILaneIssue(&status, issue, ok)
		return status
	}
	statusIssues := [][]reports.Issue{issues, plan.Issues, workflowIssuesForStatus(workflow)}
	if issue, ok := firstBlockingIssueByCode(reports.CodeUnsupportedOperation, statusIssues...); ok {
		status.Status = aiLaneStatusUnsupported
		status.Stage = aiLaneStageForIssue(issue, workflow)
		status.Message = issue.Message
		status.Detail = issue.Suggestion
		status.SuggestedNextAction = "choose a supported first-lane prompt or structured intent"
		status.RetryAllowed = false
		status.MaxAutomaticRetryAttempts = 0
		fillAILaneIssue(&status, issue, ok)
		return status
	}
	if issue, ok := firstBlockingIssueByCode(reports.CodeKiCadCLIFailed, statusIssues...); ok {
		status.Status = aiLaneStatusToolError
		status.Stage = aiLaneStageForIssue(issue, workflow)
		status.Message = issue.Message
		status.Detail = issue.Suggestion
		status.SuggestedNextAction = "fix the local tool or file path and rerun"
		status.RetryAllowed = false
		status.MaxAutomaticRetryAttempts = 0
		fillAILaneIssue(&status, issue, ok)
		return status
	}
	if issue, ok := firstBlockingIssueByCode(reports.CodeMissingFile, statusIssues...); ok {
		status.Status = aiLaneStatusToolError
		status.Stage = aiLaneStageForIssue(issue, workflow)
		status.Message = issue.Message
		status.Detail = issue.Suggestion
		status.SuggestedNextAction = "fix the local tool or file path and rerun"
		status.RetryAllowed = false
		status.MaxAutomaticRetryAttempts = 0
		fillAILaneIssue(&status, issue, ok)
		return status
	}
	if issue, ok := firstBlockingIssue(statusIssues...); ok {
		status.Status = aiLaneStatusBlocked
		status.Stage = aiLaneStageForIssue(issue, workflow)
		status.Message = issue.Message
		status.Detail = issue.Suggestion
		status.SuggestedNextAction = aiLaneNextAction(issue)
		applyAILaneRepairGuidance(&status, issue, repairBundlePath)
		if status.RetryAllowed {
			status.MaxAutomaticRetryAttempts = 1
		}
		fillAILaneIssue(&status, issue, ok)
		return status
	}
	if workflow != nil && workflowHasWarning(*workflow, issues) {
		status.Status = aiLaneStatusCandidate
		status.Stage = "validation"
		status.Message = "generated project is ready for review with warning-level evidence"
		status.SuggestedNextAction = "inspect warnings before treating the project as ready"
		return status
	}
	if plan.Status == intentplanner.PlanStatusPartial {
		status.Status = aiLaneStatusCandidate
		status.Stage = "plan"
		status.Message = "intent plan is partial but not blocked"
		status.SuggestedNextAction = "review known gaps before generating fabrication evidence"
	}
	return status
}

func artifactPaths(artifacts []reports.Artifact) []string {
	paths := make([]string, 0, len(artifacts))
	seen := map[string]bool{}
	for _, artifact := range artifacts {
		path := strings.TrimSpace(artifact.Path)
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

func workflowIssuesForStatus(workflow *designworkflow.WorkflowResult) []reports.Issue {
	if workflow == nil {
		return nil
	}
	return designworkflow.WorkflowIssues(*workflow)
}

func firstBlockingIssue(groups ...[]reports.Issue) (reports.Issue, bool) {
	for _, group := range groups {
		for _, issue := range group {
			if issue.Blocking() {
				return issue, true
			}
		}
	}
	return reports.Issue{}, false
}

func firstBlockingIssueByCode(code reports.Code, groups ...[]reports.Issue) (reports.Issue, bool) {
	for _, group := range groups {
		for _, issue := range group {
			if issue.Code == code && issue.Blocking() {
				return issue, true
			}
		}
	}
	return reports.Issue{}, false
}

func fillAILaneIssue(status *aiLaneStatus, issue reports.Issue, ok bool) {
	if !ok {
		return
	}
	status.IssueCode = issue.Code
	if status.Message == "" {
		status.Message = issue.Message
	}
	if status.Detail == "" {
		status.Detail = issue.Suggestion
	}
	status.RetryKey = aiLaneRetryKey(status.Stage, issue)
}

func aiLaneStageForIssue(issue reports.Issue, workflow *designworkflow.WorkflowResult) string {
	if issue.Code == reports.CodeUnsupportedOperation {
		return "plan"
	}
	if workflow != nil {
		for _, stage := range workflow.Stages {
			for _, stageIssue := range stage.Issues {
				if stageIssue.Code == issue.Code && stageIssue.Path == issue.Path {
					return aiLaneNormalizeStage(string(stage.Name))
				}
			}
		}
	}
	path := strings.ToLower(issue.Path)
	switch {
	case strings.Contains(path, "draft"), strings.Contains(path, "text"):
		return "draft"
	case strings.Contains(path, "plan"), strings.Contains(path, "intent"), strings.Contains(path, "blocks"):
		return "plan"
	case strings.Contains(path, "schematic"):
		return "schematic"
	case strings.Contains(path, "pcb"):
		return "pcb"
	case strings.Contains(path, "placement"):
		return "placement"
	case strings.Contains(path, "routing"), strings.Contains(path, "route"):
		return "routing"
	case strings.Contains(path, "kicad"):
		return "kicad_checks"
	case strings.Contains(path, "artifact"):
		return "manifest_generation"
	default:
		return "orchestration"
	}
}

func aiLaneNormalizeStage(stage string) string {
	switch designworkflow.StageName(stage) {
	case designworkflow.StageSchematic, designworkflow.StageSchematicElectrical:
		return "schematic"
	case designworkflow.StagePCBRealization, designworkflow.StageSchematicToPCB:
		return "pcb"
	case designworkflow.StagePlacement:
		return "placement"
	case designworkflow.StageRouting:
		return "routing"
	case designworkflow.StageValidation, designworkflow.StageWriterCorrect, designworkflow.StageFabricationReady:
		return "validation"
	case designworkflow.StageKiCadChecks:
		return "kicad_checks"
	case designworkflow.StageValidationRepair:
		return "repair"
	case designworkflow.StageProjectWrite:
		return "manifest_generation"
	default:
		return "orchestration"
	}
}

func aiLaneNextAction(issue reports.Issue) string {
	if strings.TrimSpace(issue.Suggestion) != "" {
		return issue.Suggestion
	}
	switch issue.Code {
	case reports.CodeUnsupportedOperation:
		return "choose a supported first-lane prompt or block family"
	case reports.CodeMissingFootprint, reports.CodeUnknownFootprintLibrary, reports.CodePinmapUnverified:
		return "select a verified component and footprint mapping"
	case reports.CodePlacementCollision, reports.CodePlacementOutsideBoard:
		return "retry with placement repair or relax board constraints"
	case reports.CodeRouteGraphIncomplete, reports.CodeRouteCompletionPartial, reports.CodeRouteContactMiss, reports.CodeRouteContactMissingTarget:
		return "retry with routing repair or simplify the board"
	case reports.CodeKiCadCLIFailed, reports.CodeSkippedExternalTool:
		return "configure KiCad CLI or lower the requested acceptance"
	default:
		return "inspect the referenced stage evidence and revise the request"
	}
}

func applyAILaneRepairGuidance(status *aiLaneStatus, issue reports.Issue, repairBundlePath string) {
	classification := repair.Classify(issue)
	status.RepairCategory = classification.Category
	status.RetryAllowed = classification.Repairable
	status.Detail = joinNonEmptyString("; ", status.Detail, classification.Reason)
	if repairBundlePath != "" {
		status.RepairBundlePath = repairBundlePath
	}
	if !classification.Repairable {
		return
	}
	if strings.TrimSpace(issue.Suggestion) == "" {
		status.SuggestedNextAction = aiLaneRepairAction(classification.Category)
	}
	if repairBundlePath != "" {
		status.SuggestedNextAction = "apply the repair bundle, then rerun validation"
	}
}

func aiLaneRepairAction(category repair.Category) string {
	switch category {
	case repair.CategoryPlacementCollision, repair.CategoryPlacementOutside:
		return "retry with placement repair, then rerun validation"
	case repair.CategoryUnroutedNet, repair.CategoryRouteClearance:
		return "retry with routing repair, then rerun validation"
	case repair.CategoryInvalidNetAssignment, repair.CategoryDisconnectedPad:
		return "repair net-to-pad assignments, then rerun validation"
	case repair.CategoryMissingFootprint, repair.CategoryUnknownSymbol:
		return "select verified library mappings, then rerun validation"
	case repair.CategoryMissingBoardOutline:
		return "generate a board outline, then rerun validation"
	case repair.CategoryZoneUnfilled, repair.CategoryZoneWrongNet:
		return "repair zone connectivity, refill zones, then rerun validation"
	default:
		return fmt.Sprintf("inspect repair classification %q and rerun validation after repair", category)
	}
}

func repairBundleArtifactPath(artifacts []reports.Artifact) string {
	for _, artifact := range artifacts {
		path := filepath.ToSlash(strings.TrimSpace(artifact.Path))
		if filepath.Base(path) == "repair-bundle.json" {
			return path
		}
	}
	return ""
}

func joinNonEmptyString(separator string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, separator)
}

func aiLaneRetryKey(stage string, issue reports.Issue) string {
	nets := append([]string(nil), issue.Nets...)
	refs := append([]string(nil), issue.Refs...)
	uuids := append([]string(nil), issue.UUIDs...)
	slices.Sort(nets)
	slices.Sort(refs)
	slices.Sort(uuids)
	parts := []string{
		stage,
		string(issue.Code),
		aiLaneRetryPath(issue.Path),
		strings.Join(nets, ","),
		strings.Join(refs, ","),
		strings.Join(uuids, ","),
		strings.Join(strings.Fields(issue.Message), " "),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", sum[:])
}

func aiLaneRetryPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." {
		return ""
	}
	if filepath.IsAbs(path) {
		if cwd, err := os.Getwd(); err == nil {
			if rel, relErr := filepath.Rel(cwd, path); relErr == nil && !strings.HasPrefix(rel, "..") {
				path = rel
			} else {
				path = filepath.Base(path)
			}
		} else {
			path = filepath.Base(path)
		}
	}
	return filepath.ToSlash(path)
}

func workflowHasWarning(workflow designworkflow.WorkflowResult, issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Severity == reports.SeverityWarning {
			return true
		}
	}
	for _, stage := range workflow.Stages {
		if stage.Status == designworkflow.StageStatusWarning || stage.Status == designworkflow.StageStatusSkipped {
			return true
		}
		for _, issue := range stage.Issues {
			if issue.Severity == reports.SeverityWarning {
				return true
			}
		}
	}
	return false
}

func runIntentCreate(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("intent create requires --format json")
	}
	if strings.TrimSpace(opts.output) == "" {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "--output is required"}
		if err := writeReportFailure(stdout, "intent", issue); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	plan, issues, draft, sourceText, err := loadIntentPlanForCreate(opts)
	if err != nil {
		return err
	}
	if reports.HasBlockingIssue(issues) || reports.HasBlockingIssue(plan.Issues) || plan.GeneratedRequest == nil || plan.Status == intentplanner.PlanStatusBlocked || plan.Status == intentplanner.PlanStatusNeedsClarification {
		allIssues := append([]reports.Issue(nil), issues...)
		allIssues = append(allIssues, plan.Issues...)
		report, rationaleIssues, rationaleArtifacts := persistCreateRationale(opts.output, plan, draft, nil)
		allIssues = append(allIssues, rationaleIssues...)
		artifacts := append([]reports.Artifact(nil), plan.Artifacts...)
		artifacts = append(artifacts, rationaleArtifacts...)
		aiStatus := buildAILaneStatus(plan, nil, allIssues, artifacts)
		aiArtifacts, aiArtifactIssues := writeAILaneArtifacts(opts.output, plan, draft, sourceText, aiStatus, artifacts)
		allIssues = append(allIssues, aiArtifactIssues...)
		artifacts = append(artifacts, aiArtifacts...)
		aiStatus.ArtifactPaths = artifactPaths(artifacts)
		result := reports.ResultWithIssues("intent", intentCreateResult{Plan: plan, Draft: draft, Rationale: &report, AIStatus: &aiStatus}, allIssues, artifacts)
		if writeErr := writeReportJSON(stdout, result); writeErr != nil {
			return writeErr
		}
		return errors.New("intent create blocked by plan issues")
	}
	checkOpts, err := checkOptions(opts)
	if err != nil {
		return writeReportFailure(stdout, "intent", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "check_options", Message: err.Error()})
	}
	createOpts, err := designCreateOptions(opts, checkOpts)
	if err != nil {
		return writeReportFailure(stdout, "intent", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "mode", Message: err.Error()})
	}
	if plan.GeneratedRequest == nil {
		return writeReportFailure(stdout, "intent", reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "generated_request", Message: "intent plan did not produce a design request"})
	}
	request := *plan.GeneratedRequest
	workflow := designworkflow.Create(ctx, request, createOpts)
	promotion := designworkflow.BuildInternalPromotionReport(designPromotionFixture(opts, request, workflow), workflow)
	workflow.Promotion = promotionSummaryPointer(designworkflow.PromotionSummaryFromReport(promotion, designworkflow.PromotionReportArtifactPath))
	promotionArtifact, promotionIssue := designworkflow.WritePromotionReportArtifact(opts.output, promotion, opts.overwrite)
	var promotionArtifacts []reports.Artifact
	var promotionIssues []reports.Issue
	if promotionIssue != nil {
		promotionIssues = append(promotionIssues, *promotionIssue)
	}
	if strings.TrimSpace(promotionArtifact.Path) != "" {
		promotionArtifacts = append(promotionArtifacts, promotionArtifact)
	}
	artifactDir := filepath.Join(opts.output, ".kicadai")
	plan, artifactIssues := intentplanner.WriteArtifacts(plan, intentplanner.ArtifactOptions{OutputDir: artifactDir, Overwrite: true})
	if draft != nil {
		artifactIssues = append(artifactIssues, writeIntentDraftArtifacts(artifactDir, sourceText, intentdraft.Result{Request: draft.Request, Extraction: draft.Extraction, Clarifications: draft.Clarifications}, true)...)
	}
	artifactIssues = append(artifactIssues, writeWorkflowResultArtifact(artifactDir, workflow)...)
	rationaleReport, rationaleIssues, rationaleArtifacts := persistCreateRationale(opts.output, plan, draft, &workflow)
	allIssues := append([]reports.Issue(nil), issues...)
	allIssues = append(allIssues, plan.Issues...)
	allIssues = append(allIssues, artifactIssues...)
	allIssues = append(allIssues, promotionIssues...)
	allIssues = append(allIssues, rationaleIssues...)
	allIssues = append(allIssues, designworkflow.WorkflowIssues(workflow)...)
	artifacts := append([]reports.Artifact(nil), plan.Artifacts...)
	artifacts = append(artifacts, designworkflow.WorkflowArtifacts(workflow)...)
	artifacts = append(artifacts, promotionArtifacts...)
	artifacts = append(artifacts, rationaleArtifacts...)
	aiStatus := buildAILaneStatus(plan, &workflow, allIssues, artifacts)
	aiArtifacts, aiArtifactIssues := writeAILaneArtifacts(opts.output, plan, draft, sourceText, aiStatus, artifacts)
	allIssues = append(allIssues, aiArtifactIssues...)
	artifacts = append(artifacts, aiArtifacts...)
	aiStatus.ArtifactPaths = artifactPaths(artifacts)
	result := reports.ResultWithIssues("intent", intentCreateResult{Plan: plan, Workflow: workflow, Draft: draft, Rationale: &rationaleReport, AIStatus: &aiStatus}, allIssues, artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("intent create reported issues")
	}
	return nil
}

func loadIntentPlanForCreate(opts cliOptions) (intentplanner.PlanResult, []reports.Issue, *intentDraftOutput, string, error) {
	if strings.TrimSpace(opts.intentText) == "" && strings.TrimSpace(opts.intentFile) == "" {
		plan, issues, err := loadIntentPlan(opts)
		return plan, issues, nil, "", err
	}
	if strings.TrimSpace(opts.requestPath) != "" {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "use either --request or --text/--file for intent create, not both"}
		return intentplanner.PlanResult{}, []reports.Issue{issue}, nil, "", nil
	}
	text, sourceType, sourceID, inputIssues := loadIntentDraftText(opts)
	if len(inputIssues) > 0 {
		return intentplanner.PlanResult{}, inputIssues, nil, "", nil
	}
	draft := intentdraft.Draft(text, intentdraft.Options{SourceType: sourceType, SourceID: sourceID, AcceptanceOverride: parseIntentAcceptance(opts.componentAcceptance), Strict: false})
	draftOutput := &intentDraftOutput{Request: draft.Request, Extraction: draft.Extraction, Clarifications: draft.Clarifications}
	if intentdraft.BlockingClarifications(draft.Clarifications) {
		issues := append([]reports.Issue(nil), draft.Issues...)
		for _, clarification := range draft.Clarifications {
			if clarification.Severity == intentdraft.ClarificationBlocking {
				issues = append(issues, clarification.Issue())
			}
		}
		return intentplanner.PlanResult{Status: intentplanner.PlanStatusNeedsClarification}, issues, draftOutput, text, nil
	}
	plan := intentplanner.Plan(draft.Request)
	enablePromptGeneratedRoutingRetry(&plan)
	return plan, draft.Issues, draftOutput, text, nil
}

func enablePromptGeneratedRoutingRetry(plan *intentplanner.PlanResult) {
	if plan == nil || plan.GeneratedRequest == nil {
		return
	}
	designworkflow.EnableGeneratedRoutingRetry(plan.GeneratedRequest, 2)
}

func loadIntentPlan(opts cliOptions) (intentplanner.PlanResult, []reports.Issue, error) {
	if strings.TrimSpace(opts.requestPath) == "" {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "--request is required"}
		return intentplanner.PlanResult{}, []reports.Issue{issue}, nil
	}
	file, err := os.Open(opts.requestPath)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: opts.requestPath, Message: err.Error()}
		return intentplanner.PlanResult{}, []reports.Issue{issue}, nil
	}
	defer file.Close()
	request, issues := intentplanner.DecodeRequestStrict(file)
	if reports.HasBlockingIssue(issues) {
		return intentplanner.PlanResult{}, issues, nil
	}
	return intentplanner.Plan(request), issues, nil
}

func runIntentPlan(ctx context.Context, opts cliOptions, stdout io.Writer, subcommand string) error {
	if !opts.jsonOutput {
		return fmt.Errorf("intent plan requires --format json")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if subcommand == "explain" && (strings.TrimSpace(opts.intentText) != "" || strings.TrimSpace(opts.intentFile) != "") {
		if strings.TrimSpace(opts.requestPath) != "" {
			return writeReportFailure(stdout, "intent", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "use either --request or --text/--file for intent explain, not both"})
		}
		return runIntentExplainDraft(ctx, opts, stdout)
	}
	if strings.TrimSpace(opts.requestPath) == "" {
		return writeReportFailure(stdout, "intent", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "--request is required"})
	}
	file, err := os.Open(opts.requestPath)
	if err != nil {
		return writeReportFailure(stdout, "intent", reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: opts.requestPath, Message: err.Error()})
	}
	defer file.Close()
	request, issues := intentplanner.DecodeRequestStrict(file)
	if reports.HasBlockingIssue(issues) {
		result := reports.ResultWithIssues("intent", nil, issues, nil)
		if writeErr := writeReportJSON(stdout, result); writeErr != nil {
			return writeErr
		}
		return errors.New("intent request reported blocking issues")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	plan, err := intentplanner.PlanContext(ctx, request)
	if err != nil {
		return err
	}
	var artifactIssues []reports.Issue
	if strings.TrimSpace(opts.output) != "" {
		if err := ctx.Err(); err != nil {
			return err
		}
		plan, artifactIssues = intentplanner.WriteArtifacts(plan, intentplanner.ArtifactOptions{OutputDir: opts.output, Overwrite: opts.overwrite})
	}
	plan.Issues = append(plan.Issues, issues...)
	plan.Issues = append(plan.Issues, artifactIssues...)
	plan = intentplanner.NormalizePlan(plan)
	data := any(plan)
	if subcommand == "explain" {
		data = intentExplainResult{
			Status:         plan.Status,
			Score:          plan.Score,
			Intent:         plan.Intent,
			Requirements:   plan.Requirements,
			SelectedBlocks: plan.SelectedBlocks,
			Connections:    plan.Connections,
			Assumptions:    plan.Assumptions,
			Clarifications: plan.Clarifications,
			KnownGaps:      plan.KnownGaps,
		}
	}
	result := reports.ResultWithIssues("intent", data, plan.Issues, plan.Artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("intent plan reported issues")
	}
	return nil
}

func runIntentExplainDraft(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	text, sourceType, sourceID, inputIssues := loadIntentDraftText(opts)
	if len(inputIssues) > 0 {
		result := reports.ResultWithIssues("intent", nil, inputIssues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		return errors.New("intent explain reported input issues")
	}
	draft := intentdraft.Draft(text, intentdraft.Options{SourceType: sourceType, SourceID: sourceID, AcceptanceOverride: parseIntentAcceptance(opts.componentAcceptance), Strict: false})
	draftOutput := intentDraftOutput{Request: draft.Request, Extraction: draft.Extraction, Clarifications: draft.Clarifications}
	if intentdraft.BlockingClarifications(draft.Clarifications) {
		result := reports.ResultWithIssues("intent", intentExplainResult{Status: intentplanner.PlanStatusNeedsClarification, Draft: &draftOutput}, draft.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		return errors.New("intent explain needs clarification")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	plan := intentplanner.Plan(draft.Request)
	data := intentExplainResult{
		Status:         plan.Status,
		Score:          plan.Score,
		Intent:         plan.Intent,
		Requirements:   plan.Requirements,
		SelectedBlocks: plan.SelectedBlocks,
		Connections:    plan.Connections,
		Assumptions:    plan.Assumptions,
		Clarifications: plan.Clarifications,
		KnownGaps:      plan.KnownGaps,
		Draft:          &draftOutput,
	}
	issues := append([]reports.Issue(nil), draft.Issues...)
	issues = append(issues, plan.Issues...)
	result := reports.ResultWithIssues("intent", data, issues, plan.Artifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("intent explain reported issues")
	}
	return nil
}

func runDesignCreate(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if !opts.jsonOutput {
		return fmt.Errorf("design create requires --format json")
	}
	if strings.TrimSpace(opts.requestPath) == "" {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "--request is required"})
	}
	if strings.TrimSpace(opts.output) == "" {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "--output is required"})
	}
	file, err := os.Open(opts.requestPath)
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: opts.requestPath, Message: err.Error()})
	}
	defer file.Close()
	request, issues := designworkflow.DecodeRequestStrict(file)
	if reports.HasBlockingIssue(issues) {
		result := reports.ResultWithIssues("design", nil, issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		return errors.New("design request reported blocking issues")
	}
	checkOpts, err := checkOptions(opts)
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "check_options", Message: err.Error()})
	}
	createOpts, err := designCreateOptions(opts, checkOpts)
	if err != nil {
		return writeDesignFailure(stdout, reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "mode", Message: err.Error()})
	}
	workflow := designworkflow.Create(ctx, request, createOpts)
	promotion := designworkflow.BuildInternalPromotionReport(designPromotionFixture(opts, request, workflow), workflow)
	promotionArtifact, promotionIssue := designworkflow.WritePromotionReportArtifact(opts.output, promotion, opts.overwrite)
	var artifactIssues []reports.Issue
	var promotionArtifacts []reports.Artifact
	if promotionIssue != nil {
		artifactIssues = append(artifactIssues, *promotionIssue)
	} else {
		workflow.Promotion = promotionSummaryPointer(designworkflow.PromotionSummaryFromReport(promotion, designworkflow.PromotionReportArtifactPath))
		promotionArtifacts = append(promotionArtifacts, promotionArtifact)
	}
	result := designWorkflowReport(workflow, artifactIssues, promotionArtifacts)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("design reported blocking issues")
	}
	return nil
}

func writeDesignFailure(stdout io.Writer, issue reports.Issue) error {
	if err := writeReportFailure(stdout, "design", issue); err != nil {
		return err
	}
	return errors.New(issue.Message)
}

func designWorkflowReport(workflow designworkflow.WorkflowResult, extraIssues []reports.Issue, extraArtifacts []reports.Artifact) reports.Result {
	issues := designworkflow.WorkflowIssues(workflow)
	artifacts := designworkflow.WorkflowArtifacts(workflow)
	issues = append(issues, extraIssues...)
	artifacts = append(artifacts, extraArtifacts...)
	if issues == nil {
		issues = []reports.Issue{}
	}
	if artifacts == nil {
		artifacts = []reports.Artifact{}
	}
	return reports.Result{
		OK:        designworkflow.AcceptanceSatisfied(workflow.Acceptance.Requested, workflow.Acceptance.Achieved),
		Command:   "design",
		Version:   reports.Version,
		Data:      workflow,
		Issues:    issues,
		Artifacts: artifacts,
	}
}

// designPromotionFixture converts a completed CLI workflow into promotion gates.
func designPromotionFixture(opts cliOptions, request designworkflow.Request, workflow designworkflow.WorkflowResult) designworkflow.PromotionFixture {
	requestName := filepath.Base(opts.requestPath)
	readiness := designworkflow.PromotionReadinessCandidate
	if designworkflow.AcceptanceSatisfied(request.Validation.Acceptance, workflow.Acceptance.Achieved) && designworkflow.AcceptanceSatisfied(designworkflow.AcceptanceFabricationCandidate, workflow.Acceptance.Achieved) {
		readiness = designworkflow.PromotionReadinessPass
	}
	return designworkflow.PromotionFixture{
		ID:                designworkflow.NormalizeProjectName(request.Name),
		Request:           requestName,
		Tier:              "cli",
		DeclaredReadiness: readiness,
		Acceptance:        workflow.Acceptance.Requested,
		RequireERC:        request.Validation.RequireERC,
		RequireDRC:        request.Validation.RequireDRC,
		ExpectedArtifacts: []string{".kicadai/transaction.json", ".kicadai/manifest.json"},
		ExpectedStages:    designPromotionExpectedStages(workflow),
	}
}

func designPromotionExpectedStages(workflow designworkflow.WorkflowResult) []designworkflow.StageName {
	stages := make([]designworkflow.StageName, 0, len(workflow.Stages))
	for _, stage := range workflow.Stages {
		stages = append(stages, stage.Name)
	}
	return stages
}

// promotionSummaryPointer stores promotion summaries on optional result fields.
func promotionSummaryPointer(summary designworkflow.PromotionSummary) *designworkflow.PromotionSummary {
	return &summary
}

func designCreateOptions(opts cliOptions, checkOpts checks.Options) (designworkflow.CreateOptions, error) {
	routeMode, err := designworkflow.ParseRoutingMode(opts.routeMode)
	if err != nil {
		return designworkflow.CreateOptions{}, err
	}
	if !designworkflow.ComponentSourceDirAllowed(opts.sourceDir) {
		return designworkflow.CreateOptions{}, fmt.Errorf("component source directory must be a project-relative path without parent traversal")
	}
	createOpts := designworkflow.CreateOptions{
		OutputDir:   opts.output,
		Overwrite:   opts.overwrite,
		Seed:        opts.seed,
		SkipRouting: opts.skipRouting,
		Components: designworkflow.ComponentSelectionOptions{
			CatalogDir: opts.catalogDir,
			SourceDir:  opts.sourceDir,
		},
		Placement: designworkflow.PlacementOptions{
			DefaultBounds: placement.Bounds{WidthMM: opts.placementEstWidth, HeightMM: opts.placementEstHeight, Source: placement.BoundsEstimated},
			Rules:         placement.Rules{BoardEdgeClearanceMM: opts.placementBoardMargin},
		},
		Routing: designworkflow.RoutingOptions{
			Mode:         routeMode,
			GridMM:       opts.routeGridMM,
			TraceWidthMM: opts.routeTraceWidthMM,
			ClearanceMM:  opts.routeClearanceMM,
		},
		Validation: designworkflow.ValidationOptions{
			StrictZones:    opts.strictZones,
			StrictUnrouted: opts.strictUnrouted,
			RequireDRC:     opts.requireDRC,
			KiCadCLI:       checkOpts.KiCadCLI,
			KeepArtifacts:  checkOpts.KeepArtifacts,
			ArtifactDir:    checkOpts.ArtifactDir,
		},
		KiCadChecks: designworkflow.KiCadCheckOptions{
			KiCadCLI:      checkOpts.KiCadCLI,
			Timeout:       checkOpts.Timeout,
			RequireDRC:    opts.requireDRC,
			KeepArtifacts: checkOpts.KeepArtifacts,
			ArtifactDir:   checkOpts.ArtifactDir,
			Allowlist:     checkOpts.Allowlist,
		},
		Repair:     repairOptionsForDesignCreate(opts),
		PostRepair: repairPostValidationOptions(opts),
	}
	if opts.routeAllowPartialSet {
		createOpts.Routing.AllowPartial = &opts.routeAllowPartial
	}
	return createOpts, nil
}

func repairOptionsForDesignCreate(opts cliOptions) repair.Options {
	if !opts.repairEnabled && !opts.repairApply {
		return repair.Options{}
	}
	repairOptions := repairOptionsFromCLI(opts, opts.repairApply)
	repairOptions.Enabled = true
	repairOptions.Acceptance = opts.componentAcceptance
	return repairOptions
}

func boardValidationOptions(opts cliOptions) (boardvalidation.Options, error) {
	if opts.requireDRC && opts.allowMissingDRC {
		return boardvalidation.Options{}, fmt.Errorf("--require-drc and --allow-missing-drc cannot both be set")
	}
	validationOpts := boardvalidation.Options{
		StrictZones:     opts.strictZones,
		StrictUnrouted:  opts.strictUnrouted,
		RequireDRC:      opts.requireDRC,
		AllowMissingDRC: opts.allowMissingDRC,
		KiCadCLI:        opts.kicadCLI,
		KeepArtifacts:   opts.keepArtifacts,
		ArtifactDir:     opts.artifactDir,
		AllowlistPath:   opts.allowlistPath,
	}
	if strings.TrimSpace(opts.allowlistPath) != "" {
		data, err := os.ReadFile(opts.allowlistPath)
		if err != nil {
			return boardvalidation.Options{}, fmt.Errorf("read allowlist: %w", err)
		}
		if err := json.Unmarshal(data, &validationOpts.Allowlist); err != nil {
			return boardvalidation.Options{}, fmt.Errorf("decode allowlist: %w", err)
		}
	}
	return validationOpts, nil
}

func checkOptions(opts cliOptions) (checks.Options, error) {
	checkOpts := checks.DefaultOptions()
	checkOpts.KiCadCLI = opts.kicadCLI
	checkOpts.KeepArtifacts = opts.keepArtifacts
	checkOpts.ArtifactDir = opts.artifactDir
	if strings.TrimSpace(opts.roundTimeout) != "" {
		timeout, err := time.ParseDuration(opts.roundTimeout)
		if err != nil || timeout < 0 {
			return checks.Options{}, fmt.Errorf("invalid timeout %q", opts.roundTimeout)
		}
		checkOpts.Timeout = timeout
	}
	if strings.TrimSpace(opts.allowlistPath) != "" {
		data, err := os.ReadFile(opts.allowlistPath)
		if err != nil {
			return checks.Options{}, fmt.Errorf("read allowlist: %w", err)
		}
		if err := json.Unmarshal(data, &checkOpts.Allowlist); err != nil {
			return checks.Options{}, fmt.Errorf("decode allowlist: %w", err)
		}
	}
	return checkOpts, nil
}

func checkCLI(opts cliOptions) (checks.KiCadCLI, *reports.Issue, error) {
	cli, err := checks.DiscoverCLI(opts.kicadCLI)
	if err != nil {
		if strings.TrimSpace(opts.kicadCLI) != "" {
			return checks.KiCadCLI{}, nil, err
		}
		issue := reports.Issue{
			Code:     reports.CodeSkippedExternalTool,
			Severity: reports.SeverityWarning,
			Path:     "check.kicad_cli",
			Message:  err.Error(),
		}
		return checks.KiCadCLI{}, &issue, nil
	}
	return cli, nil, nil
}

func runCheckERC(ctx context.Context, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	result, err := checks.RunERC(ctx, cli, target, opts)
	return checkResultWithIssues(result, err)
}

func runCheckDRC(ctx context.Context, cli checks.KiCadCLI, target string, opts checks.Options) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	result, err := checks.RunDRC(ctx, cli, target, opts)
	return checkResultWithIssues(result, err)
}

func checkResultWithIssues(result checks.CheckResult, err error) (checks.CheckResult, []reports.Issue, []reports.Artifact) {
	issues := []reports.Issue{}
	for _, finding := range result.Findings {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   checkSeverity(finding.Severity),
			Path:       filepath.ToSlash(finding.File),
			Message:    finding.Message,
			Refs:       finding.References,
			Nets:       checkFindingNets(finding),
			Suggestion: "repair category: " + string(finding.RepairCategory),
		})
	}
	for _, parserIssue := range result.ParserIssues {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     result.ReportPath,
			Message:  parserIssue.Message,
		})
	}
	if err != nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityError,
			Path:     result.TargetPath,
			Message:  err.Error(),
		})
	}
	return result, issues, checkArtifacts(result)
}

func checkFindingNets(finding checks.CheckFinding) []string {
	seen := map[string]struct{}{}
	nets := make([]string, 0, len(finding.Nets)+1)
	add := func(net string) {
		net = strings.TrimSpace(net)
		if net == "" {
			return
		}
		key := net
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		nets = append(nets, net)
	}
	for _, net := range finding.Nets {
		add(net)
	}
	if strings.TrimSpace(finding.Net) != "" {
		add(finding.Net)
	}
	return nets
}

func checkSeverity(severity string) reports.Severity {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "warning", "warn", "exclusion", "excluded":
		return reports.SeverityWarning
	case "info", "notice":
		return reports.SeverityInfo
	default:
		return reports.SeverityError
	}
}

func checkArtifacts(result checks.CheckResult) []reports.Artifact {
	if strings.TrimSpace(result.ReportPath) == "" {
		return nil
	}
	kind := reports.ArtifactERCReport
	if result.Kind == checks.CheckKindDRC {
		kind = reports.ArtifactDRCReport
	}
	return []reports.Artifact{{
		Kind:        kind,
		Path:        filepath.ToSlash(result.ReportPath),
		Description: string(result.Kind) + " JSON report",
	}}
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
		return fmt.Errorf("roundtrip requires --format json")
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
		return fmt.Errorf("transaction requires --format json")
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
	case "from-schematic":
		return runTransactionFromSchematic(opts, stdout)
	case "validate":
		if len(opts.commandArgs) < 2 {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "transaction.validate",
				Message:  "transaction validate requires transaction path",
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
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
		data := any(validation)
		if opts.feedbackOutput {
			data = transactionValidationFeedbackPayload{
				Validation: validation,
				Feedback:   transactions.FeedbackFromValidation(validation),
			}
		}
		result := reports.ResultWithIssues("transaction", data, validation.Issues, nil)
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
		planOptions := transactions.PlanOptions{RequireLibraryValidation: true}
		if transactionShouldUseLibraryResolver(opts) {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			index, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
				CachePath: opts.libraryCache,
				Refresh:   opts.refreshLibraryCache,
			})
			stop()
			planOptions.LibraryIndex = &index
			planOptions.LibraryIssues = issues
		}
		plan := transactions.PlanTransactionWithOptions(target, tx, planOptions)
		data := any(plan)
		if opts.feedbackOutput {
			data = transactionPlanFeedbackPayload{
				Plan:     plan,
				Feedback: transactions.FeedbackFromPlan(plan),
			}
		}
		result := reports.ResultWithIssues("transaction", data, plan.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("transaction plan failed")
		}
		return nil
	case "apply":
		if len(opts.commandArgs) < 3 {
			issue := reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     "transaction.apply",
				Message:  "transaction apply requires output directory and transaction path",
			}
			if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
				return err
			}
			return errors.New(issue.Message)
		}
		outputDir := opts.commandArgs[1]
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
		applyOptions := transactions.ApplyOptions{OutputDir: outputDir, Overwrite: opts.overwrite, Seed: opts.seed, AllowImportedMutation: opts.allowImportedApply}
		if transactionShouldUseLibraryResolver(opts) {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			index, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
				CachePath: opts.libraryCache,
				Refresh:   opts.refreshLibraryCache,
			})
			stop()
			applyOptions.LibraryIndex = &index
			applyOptions.LibraryIssues = issues
		}
		applyResult := transactions.Apply(tx, applyOptions)
		result := reports.ResultWithIssues("transaction", applyResult, applyResult.Issues, applyResult.Artifacts)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if !result.OK {
			return errors.New("transaction apply failed")
		}
		return nil
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

func runTransactionFromSchematic(opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) < 2 {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "transaction.from-schematic",
			Message:  "transaction from-schematic requires project directory",
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	projectDir := opts.commandArgs[1]
	design, err := kicaddesign.ReadProjectDirectory(projectDir)
	if err != nil {
		issue := reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     filepath.ToSlash(projectDir),
			Message:  err.Error(),
		}
		if err := writeReportJSON(stdout, reports.ErrorResult("transaction", issue)); err != nil {
			return err
		}
		return errors.New(issue.Message)
	}
	transferOptions := schematicpcb.Options{}
	var libraryIssues []reports.Issue
	if transactionShouldUseLibraryResolver(opts) {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		index, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{
			CachePath: opts.libraryCache,
			Refresh:   opts.refreshLibraryCache,
		})
		transferOptions.LibraryIndex = &index
		libraryIssues = issues
	}
	data := schematicpcb.FromDesign(design, transferOptions)
	data.Issues = append(libraryIssues, data.Issues...)
	result := reports.ResultWithIssues("transaction", data, data.Issues, nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("transaction from-schematic failed")
	}
	return nil
}

type transactionValidationFeedbackPayload struct {
	Validation transactions.ValidationResult `json:"validation"`
	Feedback   transactions.FeedbackReport   `json:"feedback"`
}

type transactionPlanFeedbackPayload struct {
	Plan     transactions.Plan           `json:"plan"`
	Feedback transactions.FeedbackReport `json:"feedback"`
}

func transactionShouldUseLibraryResolver(opts cliOptions) bool {
	return pinmapShouldUseLibraryResolver(opts)
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

func runRepairCommand(opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) == 0 {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "repair", Message: "repair requires subcommand: plan, export-bundle, or apply"}
		return writeReportFailure(stdout, "repair", issue)
	}
	if strings.TrimSpace(opts.target) != "" {
		return runRepairTargetCommand(opts, stdout)
	}
	if opts.requestPath == "" {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "repair.request", Message: "repair requires --request stage issues JSON"}
		return writeReportFailure(stdout, "repair", issue)
	}
	groups, err := loadRepairStageIssues(opts.requestPath)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: filepath.ToSlash(opts.requestPath), Message: err.Error()}
		return writeReportFailure(stdout, "repair", issue)
	}
	repairOptions := repair.Options{
		Enabled:                  true,
		Apply:                    opts.commandArgs[0] == "apply",
		MaxAttempts:              opts.maxRepairAttempts,
		MaxAttemptsPerIssue:      1,
		AllowFootprintAssignment: true,
		AllowPadNetRegeneration:  true,
		AllowOutlineGeneration:   true,
		AllowPlacementRetry:      true,
		AllowRoutingRetry:        true,
		AllowZoneNetRepair:       true,
		AllowKiCadCLI:            opts.kicadCLI != "",
	}
	switch opts.commandArgs[0] {
	case "plan":
	case "apply":
		if !opts.execute {
			issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "repair.apply", Message: "repair apply requires --execute"}
			return writeReportFailure(stdout, "repair", issue)
		}
	default:
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "repair", Message: "unsupported repair subcommand " + opts.commandArgs[0]}
		return writeReportFailure(stdout, "repair", issue)
	}
	plan := repair.BuildPlan(groups, repairOptions)
	result := reports.ResultWithIssues("repair", plan, repairPlanIssues(plan), nil)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if plan.Status == repair.StatusBlocked {
		return fmt.Errorf("repair %s blocked", opts.commandArgs[0])
	}
	return nil
}

func runRepairTargetCommand(opts cliOptions, stdout io.Writer) error {
	repairOptions := repairOptionsFromCLI(opts, opts.commandArgs[0] == "apply")
	if opts.commandArgs[0] == "export-bundle" {
		if strings.TrimSpace(opts.requestPath) == "" {
			issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "repair.request", Message: "repair export-bundle requires --request stage issues JSON"}
			return writeReportFailure(stdout, "repair", issue)
		}
		groups, err := loadRepairStageIssues(opts.requestPath)
		if err != nil {
			issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: filepath.ToSlash(opts.requestPath), Message: err.Error()}
			return writeReportFailure(stdout, "repair", issue)
		}
		export := repair.ExportBundle(repair.ExportOptions{
			TargetPath:  opts.target,
			OutputPath:  opts.output,
			StageIssues: groups,
			Execute:     opts.execute,
			Overwrite:   opts.overwrite,
			Repair:      repairOptions,
		})
		result := reports.ResultWithIssues("repair", export, export.Issues, export.Artifacts)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if reports.HasBlockingIssue(export.Issues) {
			return fmt.Errorf("repair export-bundle blocked")
		}
		return nil
	}
	var bundle *repair.Bundle
	if strings.TrimSpace(opts.requestPath) != "" {
		loaded, err := repair.LoadBundle(opts.requestPath)
		if err != nil {
			issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: filepath.ToSlash(opts.requestPath), Message: err.Error()}
			return writeReportFailure(stdout, "repair", issue)
		}
		bundle = &loaded
	}
	switch opts.commandArgs[0] {
	case "plan":
		target := repair.HydrateTarget(opts.target, repair.HydrateOptions{Bundle: bundle})
		if bundle != nil {
			plan := repair.BuildPlan(bundle.StageIssues, repairOptions)
			data := struct {
				Target repair.Target `json:"target"`
				Plan   repair.Plan   `json:"plan"`
			}{Target: target, Plan: plan}
			issues := append([]reports.Issue{}, target.Issues...)
			issues = append(issues, repairPlanIssues(plan)...)
			result := reports.ResultWithIssues("repair", data, issues, nil)
			if err := writeReportJSON(stdout, result); err != nil {
				return err
			}
			if reports.HasBlockingIssue(issues) || plan.Status == repair.StatusBlocked {
				return fmt.Errorf("repair plan blocked")
			}
			return nil
		}
		result := reports.ResultWithIssues("repair", target, target.Issues, nil)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if reports.HasBlockingIssue(target.Issues) {
			return fmt.Errorf("repair plan blocked")
		}
		return nil
	case "apply":
		if bundle == nil {
			issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "repair.request", Message: "repair apply --target requires --request repair bundle JSON"}
			return writeReportFailure(stdout, "repair", issue)
		}
		apply := repair.ApplyPersistedBundle(opts.target, *bundle, repair.PersistedApplyOptions{
			Execute:        opts.execute,
			OutputDir:      repairOutputDir(opts.output, opts.target),
			Overwrite:      opts.overwrite,
			Seed:           opts.seed,
			Repair:         repairOptions,
			Board:          &transactions.BoardSize{WidthMM: opts.placementBoardWidth, HeightMM: opts.placementBoardHeight},
			PostValidation: repairPostValidationOptions(opts),
		})
		result := reports.ResultWithIssues("repair", apply, apply.Issues, apply.Artifacts)
		if err := writeReportJSON(stdout, result); err != nil {
			return err
		}
		if apply.Status == repair.StatusBlocked || reports.HasBlockingIssue(apply.Issues) {
			return fmt.Errorf("repair apply blocked")
		}
		return nil
	default:
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "repair", Message: "unsupported repair subcommand " + opts.commandArgs[0]}
		return writeReportFailure(stdout, "repair", issue)
	}
}

func repairPostValidationOptions(opts cliOptions) repair.PostValidationOptions {
	coreValidation := opts.strictZones || opts.strictUnrouted || opts.requireDRC || opts.allowMissingDRC || opts.requireERC || opts.requireKiCadRoundTrip
	return repair.PostValidationOptions{
		WriterCorrectness:       coreValidation,
		BoardValidation:         coreValidation,
		KiCadERC:                opts.requireERC,
		KiCadDRC:                opts.requireDRC || opts.allowMissingDRC,
		RoundTrip:               opts.requireKiCadRoundTrip,
		RequireKiCadERC:         opts.requireERC,
		RequireKiCadDRC:         opts.requireDRC,
		RequireRoundTrip:        opts.requireKiCadRoundTrip,
		StrictZones:             opts.strictZones,
		StrictUnrouted:          opts.strictUnrouted,
		AllowMissingKiCadChecks: opts.allowMissingDRC,
		KeepArtifacts:           opts.keepArtifacts,
		ArtifactDir:             opts.artifactDir,
		KiCadCLI:                opts.kicadCLI,
		ZoneRefill:              opts.zoneRefill,
	}
}

func repairOptionsFromCLI(opts cliOptions, apply bool) repair.Options {
	return repair.Options{
		Enabled:                  true,
		Apply:                    apply,
		MaxAttempts:              opts.maxRepairAttempts,
		MaxAttemptsPerIssue:      max(1, opts.maxRepairAttempts),
		AllowFootprintAssignment: true,
		AllowPadNetRegeneration:  true,
		AllowOutlineGeneration:   true,
		AllowPlacementRetry:      true,
		AllowRoutingRetry:        true,
		AllowZoneNetRepair:       true,
		AllowKiCadCLI:            opts.kicadCLI != "",
	}
}

func repairOutputDir(output string, target string) string {
	if output != "" {
		return output
	}
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		return filepath.Dir(target)
	}
	switch strings.ToLower(filepath.Ext(target)) {
	case ".kicad_pro", ".kicad_sch", ".kicad_pcb":
		return filepath.Dir(target)
	default:
		return target
	}
}

func loadRepairStageIssues(path string) ([]repair.StageIssues, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var groups []repair.StageIssues
	if err := json.Unmarshal(data, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func repairPlanIssues(plan repair.Plan) []reports.Issue {
	issues := []reports.Issue{}
	for _, attempt := range plan.Attempts {
		if attempt.Status != repair.StatusBlocked {
			continue
		}
		issue := attempt.Issue
		issue.Severity = reports.SeverityBlocked
		if issue.Code == "" {
			issue.Code = reports.CodeValidationFailed
		}
		if issue.Message == "" {
			issue.Message = attempt.Message
		}
		issues = append(issues, issue)
	}
	return issues
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
	case "check", "evaluate", "export", "generate", "inspect", "pinmap", "roundtrip", "transaction":
		return true
	default:
		return false
	}
}

func requiredStructuredParams(command, subcommand string) (int, bool) {
	switch command {
	case "check":
		switch subcommand {
		case "erc", "drc", "project":
			return 1, true
		}
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
	case "pinmap":
		switch subcommand {
		case "list":
			return 0, true
		case "validate":
			return 1, true
		}
	case "transaction":
		switch subcommand {
		case "from-schematic":
			return 1, true
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
	fmt.Fprintf(stdout, "token: %s\n", resolved.Redacted().Token)
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
	if result.Version == nil {
		return errors.New("KiCad version response did not include version details")
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
		Token:       opts.apiCredential,
		Environment: os.Environ(),
	}

	return config.Resolve(explicit)
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

func writePrettyReportJSON(stdout io.Writer, result reports.Result) error {
	return writeJSON(stdout, result)
}
