// Package reproduction captures only explicit bounded inputs, not project
// directories, environment variables, credentials or arbitrary replay commands.
package reproduction

import (
	"bytes"
	"context"
	"errors"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/schematicassistant"
	"kicadai/internal/simmodel"
	"os"
	"path/filepath"
)

const Schema = "kicadai.reproduction.v1"

type Record struct {
	Schema            string                  `json:"schema"`
	Workflow          string                  `json:"workflow"`
	InputSHA256       string                  `json:"input_sha256"`
	RegistrySHA256    string                  `json:"registry_sha256"`
	Identity          evidencebundle.Identity `json:"identity"`
	Status            string                  `json:"status"`
	FirstFailingStage string                  `json:"first_failing_stage,omitempty"`
	Error             string                  `json:"error,omitempty"`
	NativeRequested   bool                    `json:"native_requested"`
	Minimization      string                  `json:"minimization"`
	ReplayArgv        []string                `json:"replay_argv"`
}

// CaptureSchematic runs once in an isolated temporary directory and retains the
// exact session bytes plus diagnostics even when decoding/confirmation fails.
func CaptureSchematic(ctx context.Context, raw []byte, output string, options schematicassistant.Options) (Record, error) {
	if err := evidencebundle.CheckDestination(output); err != nil {
		return Record{}, err
	}
	record := Record{Schema: Schema, Workflow: "schematic", InputSHA256: evidencebundle.Digest(raw), RegistrySHA256: simmodel.RegistryHash(), Identity: evidencebundle.CurrentIdentity(), Status: "passed", NativeRequested: options.RequireERC || options.KiCadCLI != "", Minimization: "not_attempted: deleting passive devices would change the confirmed specification; one run, no retries", ReplayArgv: []string{"kicadai", "--target", "BUNDLE", "--output", "NEW_OUTPUT", "repro", "replay"}}
	if len(raw) > schematicassistant.MaxInputBytes {
		return record, errors.New("reproduction input exceeds session limit")
	}
	if err := ctx.Err(); err != nil {
		return record, err
	}
	files := map[string][]byte{"input.json": raw}
	session, err := schematicassistant.Decode(bytes.NewReader(raw))
	if err != nil {
		record.FirstFailingStage = "session_decode"
		record.Error = err.Error()
	} else {
		work, workErr := os.MkdirTemp("", "kicadai-reproduction-")
		if workErr != nil {
			return record, workErr
		}
		defer func() { _ = os.RemoveAll(work) }()
		result, runErr := schematicassistant.Generate(ctx, session, filepath.Join(work, "project"), options)
		if runErr != nil {
			record.FirstFailingStage = "generation"
			var stage *schematicassistant.StageError
			if errors.As(runErr, &stage) {
				record.FirstFailingStage = stage.Stage
			}
			record.Error = runErr.Error()
		} else {
			encoded, encodeErr := evidencebundle.JSON(result)
			if encodeErr != nil {
				return record, encodeErr
			}
			files["result.json"] = encoded
		}
	}
	if record.FirstFailingStage != "" {
		record.Status = "failed"
	}
	encoded, err := evidencebundle.JSON(record)
	if err != nil {
		return record, err
	}
	files["reproduction.json"] = encoded
	files["REPLAY.txt"] = []byte("kicadai --target BUNDLE --output NEW_OUTPUT repro replay\nNative KiCad checking is NEVER auto-enabled from this package. To repeat a recorded native check, explicitly add --require-erc and optionally --kicad-cli PATH before repro.\nThe recorded argv is documentation only and is never executed. Input bytes may contain private design data: inspect before sharing. No environment snapshot or directory copy is included.\n")
	return record, evidencebundle.Publish(output, files)
}

func Inspect(root string) ([]byte, Record, error) {
	var record Record
	if err := evidencebundle.Verify(root); err != nil {
		return nil, record, err
	}
	encoded, err := evidencebundle.Read(root, "reproduction.json")
	if err != nil {
		return nil, record, err
	}
	if err = evidencebundle.Decode(encoded, &record); err != nil {
		return nil, record, err
	}
	if record.Schema != Schema || record.Workflow != "schematic" {
		return nil, record, errors.New("unsupported reproduction schema or workflow")
	}
	raw, err := evidencebundle.Read(root, "input.json")
	if err != nil {
		return nil, record, err
	}
	if len(raw) > schematicassistant.MaxInputBytes || record.InputSHA256 != evidencebundle.Digest(raw) {
		return nil, record, errors.New("reproduction input identity differs")
	}
	return raw, record, nil
}

func Load(root string) ([]byte, Record, error) {
	raw, record, err := Inspect(root)
	if err == nil && record.RegistrySHA256 != simmodel.RegistryHash() {
		return nil, record, errors.New("replay model identity differs from this executable")
	}
	return raw, record, err
}
