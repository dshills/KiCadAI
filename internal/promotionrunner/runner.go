package promotionrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kicadai/internal/creationevidence"
	"kicadai/internal/designworkflow"
	"kicadai/internal/promotiontoolchain"
)

type Options struct {
	RepositoryRoot  string
	KiCadAI         string
	OutputRoot      string
	ScenarioTimeout time.Duration
}

type CommandRecord struct {
	Scenario    string            `json:"scenario"`
	Run         int               `json:"run"`
	Args        []string          `json:"args"`
	Environment map[string]string `json:"environment"`
	Status      string            `json:"status"`
}

type RunResult struct {
	Scenario  string          `json:"scenario"`
	Run       int             `json:"run"`
	Project   string          `json:"project"`
	Command   CommandRecord   `json:"command"`
	Promotion json.RawMessage `json:"promotion"`
}

func Run(ctx context.Context, matrix MatrixDocument, toolchain promotiontoolchain.Evidence, options Options) ([]RunResult, error) {
	if options.ScenarioTimeout <= 0 {
		options.ScenarioTimeout = 20 * time.Minute
	}
	binary, err := regularExecutable(options.KiCadAI)
	if err != nil {
		return nil, err
	}
	var results []RunResult
	if err := ensureEmptyOutputRoot(options.OutputRoot); err != nil {
		return nil, err
	}
	for _, scenario := range matrix.Matrix.Scenarios {
		for run := 1; run <= 2; run++ {
			result, err := runScenario(ctx, scenario, run, toolchain, options, binary)
			if err != nil {
				return nil, fmt.Errorf("%s run %d: %w", scenario.ID, run, err)
			}
			results = append(results, result)
		}
	}
	return results, nil
}

func runScenario(parent context.Context, scenario Scenario, run int, toolchain promotiontoolchain.Evidence, options Options, binary string) (RunResult, error) {
	lane, _ := LaneFor(scenario.Lane)
	runRoot := filepath.Join(options.OutputRoot, "scenarios", scenario.ID, fmt.Sprintf("run-%d", run))
	project := filepath.Join(runRoot, "project")
	if err := os.MkdirAll(filepath.Join(runRoot, "home"), 0o755); err != nil {
		return RunResult{}, err
	}
	configRoot, err := prepareKiCadConfig(runRoot, toolchain)
	if err != nil {
		return RunResult{}, err
	}
	request := filepath.Join(options.RepositoryRoot, filepath.FromSlash(scenario.Fixture))
	args := []string{
		"--symbols-root", toolchain.SymbolsRoot, "--footprints-root", toolchain.FootprintsRoot,
		"--kicad-cli", toolchain.KiCadCLI, "--require-erc", "--require-drc",
		"--require-kicad-roundtrip", "--strict-diffs", "--request", request,
		"--output", project, "--overwrite",
	}
	args = append(args, lane.Command...)
	ctx, cancel := context.WithTimeout(parent, options.ScenarioTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	major := strings.Split(toolchain.KiCadVersion, ".")[0]
	environment := map[string]string{
		"HOME":                             filepath.Join(runRoot, "home"),
		"KICAD_CONFIG_HOME":                configRoot,
		"KICAD" + major + "_SYMBOL_DIR":    toolchain.SymbolsRoot,
		"KICAD" + major + "_FOOTPRINT_DIR": toolchain.FootprintsRoot,
		"KICADAI_KICAD_CLI":                toolchain.KiCadCLI,
		"KICADAI_SYMBOLS_ROOT":             toolchain.SymbolsRoot,
		"KICADAI_FOOTPRINTS_ROOT":          toolchain.FootprintsRoot,
	}
	for _, name := range []string{"HOME", "KICAD_CONFIG_HOME", "KICAD" + major + "_FOOTPRINT_DIR", "KICAD" + major + "_SYMBOL_DIR", "KICADAI_FOOTPRINTS_ROOT", "KICADAI_KICAD_CLI", "KICADAI_SYMBOLS_ROOT"} {
		command.Env = append(command.Env, name+"="+environment[name])
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	if writeErr := os.WriteFile(filepath.Join(runRoot, "stdout.json"), stdout.Bytes(), 0o600); writeErr != nil {
		return RunResult{}, writeErr
	}
	if writeErr := os.WriteFile(filepath.Join(runRoot, "stderr.txt"), stderr.Bytes(), 0o600); writeErr != nil {
		return RunResult{}, writeErr
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return RunResult{}, ctx.Err()
		}
		return RunResult{}, fmt.Errorf("creation command failed: %w: %s", err, stderr.String())
	}
	var response map[string]json.RawMessage
	if decodeErr := decodeStrictJSON(stdout.Bytes(), &response); decodeErr != nil {
		return RunResult{}, fmt.Errorf("creation command did not return successful structured JSON")
	}
	var responseOK bool
	if err := json.Unmarshal(response["ok"], &responseOK); err != nil || !responseOK {
		return RunResult{}, errors.New("creation command did not report ok")
	}
	for _, artifact := range scenario.RequiredArtifacts {
		info, statErr := os.Stat(filepath.Join(project, ".kicadai", artifact))
		if statErr != nil || !info.Mode().IsRegular() {
			return RunResult{}, fmt.Errorf("required artifact %q is missing", artifact)
		}
	}
	promotionPath := filepath.Join(project, creationevidence.DesignPromotionPath)
	promotionBytes, err := os.ReadFile(promotionPath)
	if err != nil {
		return RunResult{}, err
	}
	var promotion creationevidence.DesignPromotionDocument
	if err := decodeStrictJSON(promotionBytes, &promotion); err != nil {
		return RunResult{}, fmt.Errorf("decode promotion report: %w", err)
	}
	if promotion.Status != designworkflow.PromotionStatusPass {
		return RunResult{}, fmt.Errorf("promotion status is %q", promotion.Status)
	}
	if promotion.KiCadVersion != toolchain.KiCadVersion {
		return RunResult{}, fmt.Errorf("promotion KiCad version %q does not match toolchain %q", promotion.KiCadVersion, toolchain.KiCadVersion)
	}
	if validateErr := promotion.PromotionReport.Validate(); validateErr != nil {
		return RunResult{}, fmt.Errorf("invalid promotion report: %w", validateErr)
	}
	requiredGates := make(map[string]bool, len(strictGateContract.Promotion))
	for _, gate := range strictGateContract.Promotion {
		requiredGates[gate] = false
	}
	for _, gate := range promotion.Gates {
		if len(gate.RequiredFor) > 0 && gate.Status != designworkflow.PromotionGateStatusPass {
			return RunResult{}, fmt.Errorf("required promotion gate %q is %q", gate.ID, gate.Status)
		}
		if _, required := requiredGates[gate.ID]; required && gate.Status == designworkflow.PromotionGateStatusPass {
			requiredGates[gate.ID] = true
		}
	}
	for gate, passed := range requiredGates {
		if !passed {
			return RunResult{}, fmt.Errorf("required promotion gate %q did not pass", gate)
		}
	}
	return RunResult{
		Scenario: scenario.ID, Run: run, Project: project,
		Command:   CommandRecord{Scenario: scenario.ID, Run: run, Args: args, Environment: environment, Status: "pass"},
		Promotion: append(json.RawMessage(nil), promotionBytes...),
	}, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func prepareKiCadConfig(runRoot string, toolchain promotiontoolchain.Evidence) (string, error) {
	parts := strings.Split(toolchain.KiCadVersion, ".")
	if len(parts) < 2 || toolchain.SymbolTable == "" || toolchain.FootprintTable == "" {
		return "", errors.New("resolved toolchain omitted versioned stock library tables")
	}
	root := filepath.Join(runRoot, "kicad-config")
	versionRoot := filepath.Join(root, parts[0]+"."+parts[1])
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		return "", err
	}
	tables := []struct{ name, expression, table string }{
		{"sym-lib-table", "sym_lib_table", toolchain.SymbolTable},
		{"fp-lib-table", "fp_lib_table", toolchain.FootprintTable},
	}
	for _, table := range tables {
		content := "(" + table.expression + "\n  (version 7)\n  (lib (name \"KiCad\") (type \"Table\") (uri " +
			strconv.Quote(filepath.ToSlash(table.table)) + ") (options \"\") (descr \"KiCad locked stock libraries\"))\n)\n"
		if err := os.WriteFile(filepath.Join(versionRoot, table.name), []byte(content), 0o600); err != nil {
			return "", err
		}
	}
	return root, nil
}

func ensureEmptyOutputRoot(path string) error {
	if path == "" {
		return errors.New("promotion output root is required")
	}
	if entries, err := os.ReadDir(path); err == nil && len(entries) != 0 {
		return fmt.Errorf("promotion output root %q is not empty", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func regularExecutable(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%q is not an executable regular file", absolute)
	}
	return absolute, nil
}
