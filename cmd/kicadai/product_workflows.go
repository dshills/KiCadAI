package main

import (
	"context"
	"errors"
	"io"
	"kicadai/internal/boardfamily"
	"kicadai/internal/componentonboarding"
	"kicadai/internal/components"
	"kicadai/internal/environmentdoctor"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/netformbridge"
	"kicadai/internal/schematicreview"
	"math"
	"os"
	"path/filepath"
	"time"
)

func runExistingReview(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if err := offlineDiagnosticOptions(opts); err != nil {
		return err
	}
	if len(opts.commandArgs) != 1 || opts.target == "" {
		return errors.New("review requires schematic or verify, and --target")
	}
	if opts.commandArgs[0] == "verify" {
		if err := schematicreview.Verify(opts.target); err != nil {
			return err
		}
		return writeJSON(stdout, map[string]any{"integrity_verified": true, "independent_attestation": false})
	}
	if opts.commandArgs[0] != "schematic" || opts.output == "" {
		return errors.New("review schematic requires --output outside the source project")
	}
	report, err := schematicreview.Write(ctx, opts.target, opts.output, schematicreview.Options{KiCadCLI: opts.kicadCLI, RequireERC: opts.requireERC})
	if err != nil {
		return err
	}
	if err = writeJSON(stdout, report); err != nil {
		return err
	}
	if report.Status == "findings" {
		return errors.New("schematic review found issues or unsupported content; report saved")
	}
	return nil
}
func runDoctor(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if err := offlineDiagnosticOptions(opts); err != nil {
		return err
	}
	if len(opts.commandArgs) != 0 {
		return errors.New("doctor accepts no subcommand")
	}
	report := environmentdoctor.Run(ctx, environmentdoctor.Options{Roots: libraryRootsFromOptions(opts), KiCadCLI: opts.kicadCLI, RequireERC: opts.requireERC})
	if err := writeJSON(stdout, report); err != nil {
		return err
	}
	if report.Status == "blocked" {
		return errors.New("required environment probe failed; inspect doctor report")
	}
	return nil
}
func runAdapter(ctx context.Context, opts cliOptions, input io.Reader, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := offlineDiagnosticOptions(opts); err != nil {
		return err
	}
	if len(opts.commandArgs) != 0 || opts.requireERC || opts.kicadCLI != "" || opts.output != "" || opts.ngspice != "" {
		return errors.New("adapter is a read-only, no-tool-execution single-request interface")
	}
	if opts.requestPath != "" {
		file, err := os.Open(opts.requestPath)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		input = file
	}
	type readResult struct {
		raw []byte
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		raw, err := io.ReadAll(io.LimitReader(input, netformbridge.MaxRequest+1))
		done <- readResult{raw, err}
	}()
	var raw []byte
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-done:
		if result.err != nil {
			return result.err
		}
		raw = result.raw
	}
	response := netformbridge.Handle(ctx, raw)
	if err := writeJSON(stdout, response); err != nil {
		return err
	}
	if !response.OK {
		return errors.New("adapter request failed: " + response.Error.Code)
	}
	return nil
}

type selfTestRequest struct {
	Schema    string                      `json:"schema"`
	Documents []onboardingDocumentFile    `json:"documents"`
	Kit       componentonboarding.TestKit `json:"kit"`
}

func runComponentSelfTest(ctx context.Context, opts cliOptions, stdout io.Writer, catalog *components.Catalog) error {
	if err := offlineDiagnosticOptions(opts); err != nil {
		return err
	}
	if opts.requireERC || opts.kicadCLI != "" {
		return errors.New("component self-test does not run native KiCad checks")
	}
	if len(opts.commandArgs) == 2 && opts.commandArgs[1] == "verify" {
		if opts.target == "" {
			return errors.New("component self-test verify requires --target")
		}
		if err := componentonboarding.VerifySelfTest(opts.target); err != nil {
			return err
		}
		return writeJSON(stdout, map[string]any{"integrity_verified": true, "independent_attestation": false, "checks_rerun": false})
	}
	if len(opts.commandArgs) != 1 || opts.requestPath == "" || opts.intentFile == "" || opts.output == "" {
		return errors.New("component self-test requires --request candidate.json --file test-request.json --output NEW_DIR")
	}
	if err := evidencebundle.CheckDestination(opts.output); err != nil {
		return err
	}
	candidate, err := decodeOnboardingArtifactFile(opts.requestPath, componentonboarding.DecodeCandidate)
	if err != nil {
		return err
	}
	request, err := decodeCapabilityFile[selfTestRequest](opts.intentFile)
	if err != nil {
		return err
	}
	if request.Schema != "kicadai.component-self-test-request.v1" {
		return errors.New("unsupported self-test request schema")
	}
	documents, err := loadOnboardingDocuments(filepath.Dir(opts.intentFile), request.Documents)
	if err != nil {
		return err
	}
	index, err := componentOnboardingLibraryIndex(ctx, opts)
	if err != nil {
		return err
	}
	models, issues := modelprovenance.LoadDefault()
	if len(issues) > 0 {
		return errors.New("model provenance unavailable")
	}
	report, err := componentonboarding.SelfTest(ctx, candidate, documents, catalog, models, index, request.Kit, opts.output)
	if writeErr := writeJSON(stdout, report); writeErr != nil {
		return writeErr
	}
	return err
}
func runOfflineFamily(opts cliOptions, stdout io.Writer) error {
	if err := offlineDiagnosticOptions(opts); err != nil {
		return err
	}
	if len(opts.commandArgs) != 1 || opts.requireERC || opts.kicadCLI != "" {
		return errors.New("family supports offline profiles, check, generate or verify; native validation is separate")
	}
	command := opts.commandArgs[0]
	if command == "profiles" {
		return writeJSON(stdout, map[string]any{"family": boardfamily.Family, "profiles": boardfamily.Profiles(), "hardware_validated": false})
	}
	if command == "verify" {
		if opts.target == "" {
			return errors.New("family verify requires --target")
		}
		if err := evidencebundle.Verify(opts.target); err != nil {
			return err
		}
		raw, err := evidencebundle.Read(opts.target, "family-report.json")
		if err != nil {
			return err
		}
		var report struct {
			Schema            string                 `json:"schema"`
			Configuration     boardfamily.Config     `json:"configuration"`
			Electrical        boardfamily.Electrical `json:"electrical"`
			NativeChecksRun   bool                   `json:"native_checks_run"`
			HardwareValidated bool                   `json:"hardware_validated"`
		}
		if err = evidencebundle.Decode(raw, &report); err != nil {
			return err
		}
		if report.Schema != "kicadai.offline-board-family.v1" {
			return errors.New("unsupported family report")
		}
		if report.NativeChecksRun || report.HardwareValidated {
			return errors.New("offline family report cannot claim native or hardware validation")
		}
		expected, checkErr := boardfamily.Check(report.Configuration)
		if checkErr != nil || !familyElectricalConsistent(expected, report.Electrical) {
			return errors.New("offline family report differs from its configuration or bounded equations")
		}
		for _, name := range []string{"board.kicad_pro", "board.kicad_sch", "board.kicad_pcb"} {
			if _, err = evidencebundle.Read(opts.target, name); err != nil {
				return err
			}
		}
		return writeJSON(stdout, map[string]any{"integrity_verified": true, "independent_attestation": false, "offline_report_consistent": true})
	}
	if command != "check" && command != "generate" {
		return errors.New("unknown family subcommand")
	}
	if opts.requestPath == "" {
		return errors.New("family requires --request configuration.json")
	}
	file, err := os.Open(opts.requestPath)
	if err != nil {
		return err
	}
	configuration, err := boardfamily.Decode(file)
	_ = file.Close()
	if err != nil {
		return err
	}
	electrical, err := boardfamily.Check(configuration)
	if err != nil {
		return err
	}
	report := map[string]any{"schema": "kicadai.offline-board-family.v1", "configuration": configuration, "electrical": electrical, "native_checks_run": false, "hardware_validated": false}
	if command == "check" {
		return writeJSON(stdout, report)
	}
	if err = evidencebundle.CheckDestination(opts.output); err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "kicadai-offline-family-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	project := filepath.Join(work, "project")
	if _, err = boardfamily.Generate(configuration, project); err != nil {
		return err
	}
	files := map[string][]byte{}
	err = filepath.WalkDir(project, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(project, path)
		if err != nil {
			return err
		}
		raw, err := evidencebundle.Read(project, filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = raw
		return nil
	})
	if err != nil {
		return err
	}
	raw, err := evidencebundle.JSON(report)
	if err != nil {
		return err
	}
	files["family-report.json"] = raw
	if err = evidencebundle.Publish(opts.output, files); err != nil {
		return err
	}
	return writeJSON(stdout, report)
}

func familyElectricalConsistent(expected, actual boardfamily.Electrical) bool {
	if expected.Profile != actual.Profile {
		return false
	}
	for _, pair := range [][2]float64{{expected.RiseTimeNS, actual.RiseTimeNS}, {expected.TimeTo80PercentNS, actual.TimeTo80PercentNS}, {expected.PullupSinkMA, actual.PullupSinkMA}, {expected.PullupPowerMW, actual.PullupPowerMW}, {expected.AllocatedCurrentMA, actual.AllocatedCurrentMA}, {expected.SourceMarginMA, actual.SourceMarginMA}} {
		a, b := pair[0], pair[1]
		if math.IsNaN(b) || math.IsInf(b, 0) || math.Abs(a-b) > 1e-9*math.Max(1, math.Max(math.Abs(a), math.Abs(b))) {
			return false
		}
	}
	return true
}
