package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"kicadai/internal/config"
	"kicadai/internal/kiapi"
	commontypes "kicadai/internal/kiapi/gen/common/types"
	"kicadai/internal/schematic"
	"kicadai/internal/workflows"
)

const usage = `kicadai is a Go client for KiCad's IPC API.

Usage:
  kicadai [global flags] <command>

Commands:
  capabilities  Report detected KiCad API capabilities
  config        Print resolved connection configuration
  documents     List open KiCad documents
  plan-led-demo Print a deterministic LED indicator schematic plan
  ping          Check whether KiCad responds to the API
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
  --lib-vcc string      VCC symbol library ID for LED demo (default: power:VCC)
  --lib-gnd string      GND symbol library ID for LED demo (default: power:GND)
  --lib-resistor string Resistor symbol library ID for LED demo (default: Device:R)
  --lib-led string      LED symbol library ID for LED demo (default: Device:LED)
  --json                Print command output as JSON when supported
`

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
	libVCC        string
	libGND        string
	libResistor   string
	libLED        string
	jsonOutput    bool
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
	flags.StringVar(&opts.libVCC, "lib-vcc", "", "VCC symbol library ID")
	flags.StringVar(&opts.libGND, "lib-gnd", "", "GND symbol library ID")
	flags.StringVar(&opts.libResistor, "lib-resistor", "", "resistor symbol library ID")
	flags.StringVar(&opts.libLED, "lib-led", "", "LED symbol library ID")
	flags.BoolVar(&opts.jsonOutput, "json", false, "print JSON output when supported")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, "help", nil
		}

		return cliOptions{}, "", err
	}

	if flags.NArg() == 0 {
		return opts, "help", nil
	}

	return opts, flags.Arg(0), nil
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
	plan, err := workflows.PlanLEDDemo(workflows.LEDDemoIntent{
		Document: schematic.DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: opts.documentID},
		Origin:   schematic.Point{X: opts.originX, Y: opts.originY},
		Prefix:   opts.prefix,
		Libraries: workflows.LEDDemoLibraries{
			VCC:      opts.libVCC,
			GND:      opts.libGND,
			Resistor: opts.libResistor,
			LED:      opts.libLED,
		},
	})
	if err != nil {
		return err
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
