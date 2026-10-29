package main

import (
	"context"
	"errors"
	"io"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/numericalconformance"
	"kicadai/internal/reproduction"
	"kicadai/internal/schematicassistant"
	"path/filepath"
)

func offlineDiagnosticOptions(opts cliOptions) error {
	if opts.overwrite || opts.withPCB || opts.requireDRC || opts.aiProvider != "" || opts.aiPrompt != "" || opts.aiPromptFile != "" {
		return errors.New("diagnostic commands are offline, never overwrite, and never create/check PCB artifacts")
	}
	return nil
}
func runConformance(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if err := offlineDiagnosticOptions(opts); err != nil {
		return err
	}
	if len(opts.commandArgs) != 1 {
		return errors.New("conformance requires run, replay, or verify")
	}
	if opts.commandArgs[0] == "verify" {
		if opts.target == "" {
			return errors.New("conformance verify requires --target")
		}
		if _, err := numericalconformance.Inspect(opts.target); err != nil {
			return err
		}
		return writeJSON(stdout, map[string]any{"integrity_verified": true, "independent_attestation": false, "numerical_checks_rerun": false})
	}
	if opts.output == "" {
		return errors.New("conformance requires --output new-directory")
	}
	options := numericalconformance.Options{Ngspice: opts.ngspice}
	switch opts.commandArgs[0] {
	case "run":
	case "replay":
		if opts.target == "" {
			return errors.New("conformance replay requires --target")
		}
		cases, err := numericalconformance.ReplayCases(opts.target)
		if err != nil {
			return err
		}
		options.CaseIDs = cases
	default:
		return errors.New("unknown conformance subcommand")
	}
	report, err := numericalconformance.Run(ctx, opts.output, options)
	if report.Schema == "" {
		return err
	}
	if writeErr := writeJSON(stdout, report); writeErr != nil {
		return writeErr
	}
	return err
}
func runReproduction(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if err := offlineDiagnosticOptions(opts); err != nil {
		return err
	}
	if len(opts.commandArgs) != 1 {
		return errors.New("repro requires schematic, replay, or verify")
	}
	if opts.commandArgs[0] == "verify" {
		if opts.target == "" {
			return errors.New("repro verify requires --target")
		}
		if _, _, err := reproduction.Inspect(opts.target); err != nil {
			return err
		}
		return writeJSON(stdout, map[string]any{"integrity_verified": true, "independent_attestation": false, "workflow_rerun": false})
	}
	if opts.output == "" {
		return errors.New("repro requires --output new-directory")
	}
	var raw []byte
	var err error
	switch opts.commandArgs[0] {
	case "schematic":
		if opts.requestPath == "" {
			return errors.New("repro schematic requires --request session.json")
		}
		raw, err = evidencebundle.Read(filepath.Dir(opts.requestPath), filepath.Base(opts.requestPath))
	case "replay":
		if opts.target == "" {
			return errors.New("repro replay requires --target")
		}
		var previous reproduction.Record
		raw, previous, err = reproduction.Load(opts.target)
		if err == nil && previous.NativeRequested && !opts.requireERC {
			return errors.New("original failure required native KiCad; explicitly pass --require-erc to replay that stage")
		}
	default:
		return errors.New("unknown repro subcommand")
	}
	if err != nil {
		return err
	}
	record, err := reproduction.CaptureSchematic(ctx, raw, opts.output, schematicassistant.Options{RequireERC: opts.requireERC, KiCadCLI: opts.kicadCLI})
	if err != nil {
		return err
	}
	if err = writeJSON(stdout, record); err != nil {
		return err
	}
	if record.Status == "failed" {
		return errors.New("recorded schematic failure at " + record.FirstFailingStage + "; reproduction bundle saved")
	}
	return nil
}
