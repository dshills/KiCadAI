package repair

import (
	"context"
	"testing"

	"kicadai/internal/reports"
)

func TestBuiltInPostApplyValidatorsAddsWriterCorrectnessOnlyWhenEnabled(t *testing.T) {
	if validators := BuiltInPostApplyValidators(PostValidationOptions{}); len(validators) != 0 {
		t.Fatalf("validators = %d, want none", len(validators))
	}
	validators := BuiltInPostApplyValidators(PostValidationOptions{WriterCorrectness: true})
	if len(validators) != 1 {
		t.Fatalf("validators = %d, want 1", len(validators))
	}
}

func TestWriterCorrectnessPostValidatorRequiresTarget(t *testing.T) {
	validation := WriterCorrectnessValidator{}.ValidatePostApply(context.Background(), PostApplyValidationContext{})
	if len(validation.Issues) != 1 || validation.Issues[0].Code != reports.CodeInvalidArgument {
		t.Fatalf("validation issues = %+v", validation.Issues)
	}
	if validation.Name != "writer_correctness" {
		t.Fatalf("validation name = %q", validation.Name)
	}
}

func TestWriterCorrectnessPostValidatorHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	validation := WriterCorrectnessValidator{}.ValidatePostApply(ctx, PostApplyValidationContext{OutputDir: t.TempDir()})
	if len(validation.Issues) != 1 || validation.Issues[0].Code != reports.CodeOperationCanceled {
		t.Fatalf("validation issues = %+v", validation.Issues)
	}
}
